// Package cli: `tlc status` root subcommand.
//
// Implements the reserved `status` subcommand required by kit's shape
// validator (kit/console/cli.checkReservedStatus). Surfaces a short
// project-health summary covering the active project, the caller's
// in-progress tasks, and the count of overdue items.
package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

// StatusCmd is the root-level `tlc status` subcommand. Reserved by kit
// as a presence-by-name requirement on the root command; tlc shadows it
// with a useful project snapshot.
var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project + caller status snapshot",
	Long: `Show a short status snapshot for the current scope.

Reports the detected project, the caller's identity, counts of
in-progress + TODO tasks assigned to the caller, and overdue items.
Read-only; never mutates storage.`,
	Annotations: map[string]string{
		"kit/side-effect":    "read",
		"kit/idempotent":     "yes",
		"kit/top-level-verb": "true",
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := context.Background()
		w := cmd.OutOrStdout()

		// Project context.
		proj := core.DetectProject()
		if proj != nil && proj.InProject {
			_, _ = fmt.Fprintf(w, "Project:  %s\n", proj.ProjectID)
		} else {
			_, _ = fmt.Fprintln(w, "Project:  (none detected)")
		}

		user := core.GetCurrentUser()
		_, _ = fmt.Fprintf(w, "User:     %s\n", user)

		// Storage may not be available outside a project; degrade gracefully.
		s, err := getStorage()
		if err != nil {
			_, _ = fmt.Fprintln(w, "Storage:  unavailable (run 'tlc init' or chdir into a project)")
			return nil
		}
		defer func() { _ = s.Close() }()

		// Caller's task counts. Counted in SQL over the full match
		// set rather than by measuring a capped slice: the numbers are
		// printed as fact, and a count that silently stops at the page
		// size is indistinguishable from a real one.
		mineCounts, err := s.CountTasksByStatus(ctx, core.Query{
			Filters: []core.FieldFilter{
				{Field: "assigned_to", Value: user},
			},
		})
		if err != nil {
			return fmt.Errorf("count assigned tasks: %w", err)
		}

		_, _ = fmt.Fprintf(w, "Tasks:    %d in progress, %d todo (yours)\n",
			mineCounts[string(core.StatusInProgress)],
			mineCounts[string(core.StatusTodo)])

		// Overdue across project. Overdue-ness is a due_at comparison
		// the store can express, so it counts in SQL too.
		now := time.Now()
		overdueCounts, err := s.CountTasksByStatus(ctx, core.Query{
			Filters: []core.FieldFilter{
				{Field: "status", Value: string(core.StatusInProgress)},
				{Field: "status", Value: string(core.StatusTodo)},
			},
			DueBefore: &now,
		})
		if err != nil {
			return fmt.Errorf("count overdue tasks: %w", err)
		}
		overdue := 0
		for _, n := range overdueCounts {
			overdue += n
		}
		if overdue > 0 {
			_, _ = fmt.Fprintf(w, "Overdue:  %d task(s)\n", overdue)
		}

		// The inline list below is a preview, so it stays paginated —
		// unlike the counts above, a capped list is self-evident.
		mine, err := s.ListTasks(ctx, core.Query{
			Filters: []core.FieldFilter{
				{Field: "assigned_to", Value: user},
				{Field: "status", Value: string(core.StatusInProgress)},
			},
			Limit: 50,
		})
		if err != nil {
			return fmt.Errorf("list in-progress tasks: %w", err)
		}

		// Surface the most relevant in-progress items inline.
		if len(mine) > 0 {
			_, _ = fmt.Fprintln(w)
			_, _ = fmt.Fprintln(w, "In progress:")
			for _, t := range mine {
				_, _ = fmt.Fprintf(w, "  %s  %s\n", formatTaskAlias(t), t.Title)
			}
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(StatusCmd)
}
