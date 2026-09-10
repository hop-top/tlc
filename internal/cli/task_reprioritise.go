package cli

// The command surface for config-driven priority derivation.
//
// One explicit verb, `tlc task reprioritise`, plus a create-time hook the
// user opts into. Nothing here runs on a read, on a timer, or as a side
// effect of an unrelated command. See the package comment on
// internal/core/priority_derivation.go for why.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var (
	taskReprioritiseDryRun bool
	taskReprioritiseAll    bool
)

// newPriorityDeriver builds the deriver from the merged `task` config.
//
// Reads the config through unmarshalConfigKey — the same door every other
// consumer uses, and NOT through core.DefaultWorkflow*. That singleton
// memoises on first call; touching it from a pre-argv path freezes config
// before `-c key=value` overrides merge and breaks them process-wide.
// This is invoked from RunE, well after argv, but going through the
// non-caching accessor keeps that hazard off this path permanently rather
// than by luck of call ordering.
func newPriorityDeriver() (*core.PriorityDeriver, error) {
	var cfg config.TaskConfig
	if err := unmarshalConfigKey("task", &cfg); err != nil {
		return nil, fmt.Errorf("failed to read task config: %w", err)
	}
	// Unwrapped on purpose: both call sites already prefix this with
	// `task.priority_derivation:`, and wrapping here too would print the
	// config path twice in one message.
	return core.NewPriorityDeriver(&cfg) //nolint:wrapcheck // callers add the config-path prefix
}

// deriveTaskPriorityOnCreate supplies a priority for a freshly built task
// that carries none, when the user opted into create-time derivation.
//
// A broken rule set is an ERROR here rather than a shrug. Create is a
// write; silently storing a task with no priority because the rules could
// not be parsed would leave the user believing derivation had considered
// it and declined.
func deriveTaskPriorityOnCreate(w io.Writer, task *core.Task) error {
	deriver, err := newPriorityDeriver()
	if err != nil {
		return fmt.Errorf("task.priority_derivation: %w", err)
	}
	if !deriver.DerivesOnCreate() {
		return nil
	}
	res := deriver.DeriveForCreate(task)
	if res.Changed() && w != nil {
		_, _ = fmt.Fprintf(w, "Priority %s derived by rule %q\n", res.To, res.RuleName)
	}
	return nil
}

// TaskReprioritiseCmd re-derives priorities across the task set.
var TaskReprioritiseCmd = &cobra.Command{
	Use:     "reprioritise [task-id...]",
	Aliases: []string{"reprioritize"},
	Short:   "Re-derive task priorities from config rules",
	Long: `Re-derive task priorities from the rules in task.priority_derivation.

Rules are evaluated in declaration order and the first match wins. A
priority a human set is never overwritten: clear it (tlc task update <id>
-p -) to hand that task back to derivation.

Tasks in an in-progress status are left alone unless
task.priority_derivation.include_active is true; terminal tasks are always
left alone.

With no task IDs, every non-archived task in the current project is
considered. Use --dry-run to see the verdicts without writing.`,
	Annotations: map[string]string{
		"kit/side-effect": "write",
		"kit/idempotent":  "yes",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		deriver, err := newPriorityDeriver()
		if err != nil {
			// Reported, not resolved. A rule set that cannot be
			// validated is never partially applied.
			return fmt.Errorf("task.priority_derivation: %w", err)
		}
		if deriver == nil {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(),
				"No priority derivation rules configured "+
					"(task.priority_derivation.rules); nothing to do.")
			return nil
		}

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		tasks, err := reprioritiseTargets(ctx, s, args)
		if err != nil {
			return err
		}

		// Dependent counts come from the WHOLE candidate set, computed
		// once. Recomputing per task would be quadratic, and computing
		// it from a per-task query would let the answer depend on
		// evaluation order.
		deps := core.CountDependents(tasks)

		var report core.DerivationReport
		for _, t := range tasks {
			res := deriver.Apply(t, deps)
			// Alias formatting lives in the CLI, so the display form is
			// attached here rather than inside the engine. Sorting still
			// keys off the durable ID.
			res.Display = formatTaskAlias(t)
			report.Add(res)
			if !res.Changed() || taskReprioritiseDryRun {
				continue
			}
			if err := s.UpdateTask(ctx, t); err != nil {
				return fmt.Errorf("failed to update %s: %w", formatTaskAlias(t), err)
			}
			if logErr := s.AddLog(ctx, &core.LogEntry{
				TaskID: t.ID,
				By:     core.GetCurrentUser(),
				Action: core.ActionUpdated,
				Note: fmt.Sprintf("priority %s -> %s (rule %q)",
					priorityForLog(res.From), res.To, res.RuleName),
				Meta: map[string]any{"fields": []string{"priority"}},
			}); logErr != nil {
				// The row is already committed; a missing audit note
				// must not make a successful edit look failed.
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"Warning: failed to log reprioritise for %s: %v\n", formatTaskAlias(t), logErr)
			}
		}

		// Sorted before rendering so repeated runs over the same data
		// produce byte-identical output regardless of the order storage
		// returned rows in.
		report.Sort()
		return renderDerivationReport(cmd.OutOrStdout(), &report, taskReprioritiseDryRun)
	},
}

// priorityForLog renders an empty priority as a readable placeholder, so
// the audit note for a first-ever derivation says what happened rather
// than showing a gap.
func priorityForLog(p core.Priority) string {
	if p == "" {
		return "(none)"
	}
	return string(p)
}

// reprioritiseTargets resolves the tasks a run considers: the named ones,
// or the whole non-archived set when none were named.
func reprioritiseTargets(
	ctx context.Context, s *storage.SQLiteStorage, args []string,
) ([]*core.Task, error) {
	if len(args) == 0 {
		tasks, err := s.ListTasks(ctx, core.Query{AllProjects: taskReprioritiseAll})
		if err != nil {
			return nil, fmt.Errorf("failed to list tasks: %w", err)
		}
		return tasks, nil
	}

	resolved, _, err := resolveTaskIDs(ctx, args, s, core.Query{})
	if err != nil {
		return nil, err
	}
	out := make([]*core.Task, 0, len(resolved))
	for _, r := range resolved {
		if r == nil || r.Task == nil {
			continue
		}
		// A named task resolving into another project's store would be
		// written back through THIS store's handle by the caller. Refuse
		// rather than write to the wrong database.
		if r.Storage != nil && r.Storage != s {
			return nil, fmt.Errorf(
				"task %s lives in another project; run reprioritise from "+
					"that project", formatTaskAlias(r.Task))
		}
		out = append(out, r.Task)
	}
	return out, nil
}

// derivationRow is the JSON shape of one verdict.
type derivationRow struct {
	TaskID   string `json:"task_id"`
	Outcome  string `json:"outcome"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	RuleName string `json:"rule,omitempty"`
}

// renderDerivationReport writes the run's verdicts, honouring the
// configured output format.
func renderDerivationReport(w io.Writer, rep *core.DerivationReport, dryRun bool) error {
	if viper.GetString("output.format") == "json" {
		rows := make([]derivationRow, 0, len(rep.Results))
		for _, r := range rep.Results {
			rows = append(rows, derivationRow{
				TaskID:   r.Label(),
				Outcome:  r.Outcome.String(),
				From:     string(r.From),
				To:       string(r.To),
				RuleName: r.RuleName,
			})
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{
			"dry_run": dryRun,
			"results": rows,
			"summary": rep.Summary(),
		}); err != nil {
			return fmt.Errorf("failed to encode derivation report: %w", err)
		}
		return nil
	}

	for _, r := range rep.Results {
		if !r.Changed() {
			continue
		}
		_, _ = fmt.Fprintf(w, "%s  %s -> %s  (rule %q)\n",
			r.Label(), priorityForLog(r.From), r.To, r.RuleName)
	}
	if dryRun {
		_, _ = fmt.Fprintf(w, "Dry run: %s\n", rep.Summary())
		return nil
	}
	_, _ = fmt.Fprintf(w, "%s\n", rep.Summary())
	return nil
}

func init() {
	TaskReprioritiseCmd.Flags().BoolVar(&taskReprioritiseDryRun, "dry-run", false,
		"Report the verdicts without writing them")
	TaskReprioritiseCmd.Flags().BoolVar(&taskReprioritiseAll, "all-projects", false,
		"Consider tasks across all projects, not just the current one")
}
