package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/displaytime"
)

var taskRemindCheck bool

var TaskRemindCmd = &cobra.Command{
	Use:   "remind",
	Short: "Show upcoming and overdue task reminders",
	Long: `Report tasks that are overdue, due today, or scheduled for the near
future based on each task's RemindAt / DueAt fields.

Read-only by default. Wall-clock buckets ("due today") are projected
into the configured display timezone (ui.timezone) before bucketing.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		tasks, err := s.ListTasks(ctx, core.Query{})
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Overdue uses absolute-instant comparison (Before), so it is
		// timezone-agnostic and does not need normalisation. "Due today"
		// is a wall-clock bucket, so both sides must be projected into
		// the configured display timezone before extracting Y/M/D —
		// otherwise a task due at, e.g., 23:30 UTC on day N renders as
		// "today" in UTC but should be "tomorrow" in Asia/Tokyo (or vice
		// versa around midnight). See docs/temporal-spec-0.1.md §6.
		loc := displaytime.Resolve()
		now := time.Now()
		nowInDisplay := now.In(loc)
		var overdue, dueToday, upcoming []*core.Task

		for _, t := range tasks {
			if t.Status == core.StatusDone ||
				t.Status == core.StatusSkipped {
				continue
			}
			if t.DueAt == nil && t.RemindAt == nil &&
				t.RRule == "" {
				continue
			}

			if t.DueAt != nil {
				if t.DueAt.Before(now) {
					overdue = append(overdue, t)
					continue
				}
				y1, m1, d1 := nowInDisplay.Date()
				y2, m2, d2 := t.DueAt.In(loc).Date()
				if y1 == y2 && m1 == m2 && d1 == d2 {
					dueToday = append(dueToday, t)
					continue
				}
			}
			upcoming = append(upcoming, t)
		}

		// `task remind` is a table-style view per spec §6, so DueAt
		// and reminder timestamps humanise across all sections
		// ("overdue 2h", "in 3h", "in 2d"). T-1384.
		if taskRemindCheck {
			if len(overdue) > 0 {
				for _, t := range overdue {
					fmt.Fprintf(
						os.Stderr,
						"OVERDUE: %s %s (due %s)\n",
						t.ID, t.Title,
						DisplayTimePtrRelative(t.DueAt),
					)
				}
				return fmt.Errorf(
					"%d overdue task(s)", len(overdue),
				)
			}
			return nil
		}

		w := cmd.OutOrStdout()

		if len(overdue) > 0 {
			fmt.Fprintln(w, "OVERDUE:")
			for _, t := range overdue {
				fmt.Fprintf(
					w, "  %s  %-40s  due %s\n",
					t.ID, t.Title,
					DisplayTimePtrRelative(t.DueAt),
				)
			}
			fmt.Fprintln(w)
		}

		if len(dueToday) > 0 {
			fmt.Fprintln(w, "DUE TODAY:")
			for _, t := range dueToday {
				fmt.Fprintf(
					w, "  %s  %-40s  due %s\n",
					t.ID, t.Title,
					DisplayTimePtrRelative(t.DueAt),
				)
			}
			fmt.Fprintln(w)
		}

		if len(upcoming) > 0 {
			fmt.Fprintln(w, "UPCOMING:")
			for _, t := range upcoming {
				label := ""
				if t.DueAt != nil {
					label = "due " + DisplayTimePtrRelative(t.DueAt)
				} else if nr := t.NextReminder(); nr != nil {
					label = "next " + DisplayTimeRelative(*nr)
				}
				fmt.Fprintf(
					w, "  %s  %-40s  %s\n",
					t.ID, t.Title, label,
				)
			}
		}

		// Age-based nudges (tasks without explicit due)
		nudges := CollectAgeNudges(tasks)
		if len(nudges) > 0 {
			fmt.Fprintln(w, "AGE NUDGES:")
			for _, n := range nudges {
				fmt.Fprintf(
					w, "  %s  %-30s  %s (%s idle)\n",
					n.TaskID, n.Status, n.Action,
					n.Age.Truncate(time.Hour),
				)
			}
		}

		total := len(overdue) + len(dueToday) +
			len(upcoming) + len(nudges)
		if total == 0 {
			fmt.Fprintln(w, "No scheduled tasks.")
		}

		return nil
	},
}

func init() {
	TaskRemindCmd.Flags().BoolVar(
		&taskRemindCheck, "check", false,
		"Exit 1 if any tasks are overdue (scriptable)",
	)
	TaskCmd.AddCommand(TaskRemindCmd)
}
