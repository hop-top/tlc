package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var (
	taskRemindCheck bool
)

var TaskRemindCmd = &cobra.Command{
	Use:   "remind",
	Short: "Show upcoming and overdue task reminders",
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

		now := time.Now()
		var overdue, dueToday, upcoming []*core.Task

		for _, t := range tasks {
			if t.Status == core.StatusDone ||
				t.Status == core.StatusSkipped {
				continue
			}
			if t.DueAt == nil && t.RemindAt == nil &&
				t.RemindEvery == nil {
				continue
			}

			if t.DueAt != nil {
				if t.DueAt.Before(now) {
					overdue = append(overdue, t)
					continue
				}
				y1, m1, d1 := now.Date()
				y2, m2, d2 := t.DueAt.Date()
				if y1 == y2 && m1 == m2 && d1 == d2 {
					dueToday = append(dueToday, t)
					continue
				}
			}
			upcoming = append(upcoming, t)
		}

		if taskRemindCheck {
			if len(overdue) > 0 {
				for _, t := range overdue {
					fmt.Fprintf(os.Stderr,
						"OVERDUE: %s %s (due %s)\n",
						t.ID, t.Title,
						t.DueAt.Format("2006-01-02"),
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
				fmt.Fprintf(w, "  %s  %-40s  due %s\n",
					t.ID, t.Title,
					t.DueAt.Format("2006-01-02 15:04"),
				)
			}
			fmt.Fprintln(w)
		}

		if len(dueToday) > 0 {
			fmt.Fprintln(w, "DUE TODAY:")
			for _, t := range dueToday {
				fmt.Fprintf(w, "  %s  %-40s  due %s\n",
					t.ID, t.Title,
					t.DueAt.Format("15:04"),
				)
			}
			fmt.Fprintln(w)
		}

		if len(upcoming) > 0 {
			fmt.Fprintln(w, "UPCOMING:")
			for _, t := range upcoming {
				label := ""
				if t.DueAt != nil {
					label = "due " + t.DueAt.Format("2006-01-02")
				} else if nr := t.NextReminder(); nr != nil {
					label = "next " + nr.Format("2006-01-02 15:04")
				}
				fmt.Fprintf(w, "  %s  %-40s  %s\n",
					t.ID, t.Title, label,
				)
			}
		}

		// Age-based nudges (tasks without explicit due)
		nudges := CollectAgeNudges(tasks)
		if len(nudges) > 0 {
			fmt.Fprintln(w, "AGE NUDGES:")
			for _, n := range nudges {
				fmt.Fprintf(w, "  %s  %-30s  %s (%s idle)\n",
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
