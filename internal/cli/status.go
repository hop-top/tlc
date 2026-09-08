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
	"io"
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

		// Project + identity are cheap and cannot fail.
		proj := core.DetectProject()
		user := core.GetCurrentUser()

		// Storage may not be available outside a project; degrade
		// gracefully rather than erroring.
		s, err := getStorage()
		if err != nil {
			writeStatusHeader(w, proj, user)
			_, _ = fmt.Fprintln(w, "Storage:  unavailable (run 'tlc init' or chdir into a project)")
			return nil
		}
		defer func() { _ = s.Close() }()

		return writeStatusSnapshot(ctx, w, s, proj, user)
	},
}

// writeStatusSnapshot gathers the whole snapshot, then writes it.
//
// Gather-then-write is the point, not an implementation detail. Writing the
// Project/User/Tasks lines and only then returning a count error left a
// half-written snapshot on stdout next to a non-zero exit, and a caller
// reading those lines cannot tell the report was cut short. So on any
// failure nothing at all is written and the error is returned; either the
// whole snapshot prints, or none of it does.
func writeStatusSnapshot(
	ctx context.Context, w io.Writer, s statusReader,
	proj *core.ProjectDetection, user string,
) error {
	snapshot, err := collectStatusSnapshot(ctx, s, user)
	if err != nil {
		return err
	}

	writeStatusHeader(w, proj, user)
	_, _ = fmt.Fprintf(w, "Tasks:    %d in progress, %d todo (yours)\n",
		snapshot.inProgress, snapshot.todo)
	if snapshot.overdue > 0 {
		_, _ = fmt.Fprintf(w, "Overdue:  %d task(s)\n", snapshot.overdue)
	}

	// Surface the most relevant in-progress items inline.
	if len(snapshot.inProgressTasks) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, "In progress:")
		for _, t := range snapshot.inProgressTasks {
			_, _ = fmt.Fprintf(w, "  %s  %s\n", formatTaskAlias(t), t.Title)
		}
	}

	return nil
}

// statusSnapshot holds every number and row `tlc status` prints, gathered
// before any of it is written.
type statusSnapshot struct {
	inProgress      int
	todo            int
	overdue         int
	inProgressTasks []*core.Task
}

// statusReader is the read surface `tlc status` needs from the store.
type statusReader interface {
	CountTasksByStatus(ctx context.Context, query core.Query) (map[string]int, error)
	CountTasks(ctx context.Context, query core.Query) (int, error)
	ListTasks(ctx context.Context, query core.Query) ([]*core.Task, error)
}

// collectStatusSnapshot gathers the whole snapshot, returning the first
// error rather than a partial result.
//
// The counts are computed in SQL over the full match set rather than by
// measuring a capped slice: the numbers are printed as fact, and a count
// that silently stops at the page size is indistinguishable from a real one.
func collectStatusSnapshot(ctx context.Context, s statusReader, user string) (statusSnapshot, error) {
	var snap statusSnapshot

	mineCounts, err := s.CountTasksByStatus(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "assigned_to", Value: user},
		},
	})
	if err != nil {
		return snap, fmt.Errorf("count assigned tasks: %w; retry, or check the store with 'tlc task list'", err)
	}
	snap.inProgress = mineCounts[string(core.StatusInProgress)]
	snap.todo = mineCounts[string(core.StatusTodo)]

	// Overdue across the project. core.Query.Overdue carries the whole
	// definition — due_at in the past AND the task still open — so this
	// and `task list --overdue` cannot mean different rows.
	now := time.Now()
	snap.overdue, err = s.CountTasks(ctx, core.Query{Overdue: &now})
	if err != nil {
		return snap, fmt.Errorf("count overdue tasks: %w; retry, or check the store with 'tlc task list --overdue'", err)
	}

	// The inline list is a preview, so it stays paginated — unlike the
	// counts above, a capped list is self-evident.
	snap.inProgressTasks, err = s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "assigned_to", Value: user},
			{Field: "status", Value: string(core.StatusInProgress)},
		},
		Limit: 50,
	})
	if err != nil {
		return snap, fmt.Errorf("list in-progress tasks: %w; retry, or check the store with 'tlc task list'", err)
	}

	return snap, nil
}

// writeStatusHeader writes the project and identity lines.
func writeStatusHeader(w io.Writer, proj *core.ProjectDetection, user string) {
	if proj != nil && proj.InProject {
		_, _ = fmt.Fprintf(w, "Project:  %s\n", proj.ProjectID)
	} else {
		_, _ = fmt.Fprintln(w, "Project:  (none detected)")
	}
	_, _ = fmt.Fprintf(w, "User:     %s\n", user)
}

func init() {
	RootCmd.AddCommand(StatusCmd)
}
