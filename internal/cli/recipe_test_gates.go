package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/flowtest"
	"hop.top/tlc/internal/storage"
)

// validateRunGates checks a run's scripted gates against the expanded
// recipe: each must name a human step. The whole recipe counts, not just
// the --task selection, so a fixture stays valid whatever subset runs.
func validateRunGates(expanded []core.RecipeStep, run flowtest.Run) error {
	kinds := make(map[string]core.TaskKind, len(expanded))
	for _, step := range expanded {
		kind := step.Kind
		if kind == "" {
			kind = core.TaskKindAgent
		}
		kinds[step.ID] = kind
	}
	ids := make([]string, 0, len(run.Gates))
	for id := range run.Gates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		kind, ok := kinds[id]
		switch {
		case !ok:
			return fmt.Errorf("run %q: gate %q is not a step of the recipe", run.Name, id)
		case kind != core.TaskKindHuman:
			return fmt.Errorf("run %q: gate %q is a %s step; only human steps take a decision", run.Name, id, kind)
		}
	}
	return nil
}

// runWithGates executes the tasks and, whenever the executor stops with
// human tasks waiting, decides each from the run's scripted gates and
// resumes. It ends when nothing is waiting, or when what waits has no
// scripted decision. Decisions are applied the same way in replay and in
// --record: nothing prompts.
func runWithGates(
	ctx context.Context, cmd *cobra.Command, executor *core.Executor, store *storage.SQLiteStorage,
	fixture *flowtest.Run, taskIDs []string,
) (*core.ExecReport, error) {
	merged := &core.ExecReport{}
	applied := map[string]struct{}{}
	for {
		report, err := executor.RunTasks(ctx, taskIDs)
		if err != nil {
			return merged, err //nolint:wrapcheck // the caller adds the context
		}
		merged.Done = append(merged.Done, report.Done...)
		merged.Skipped = append(merged.Skipped, report.Skipped...)
		merged.Failed = append(merged.Failed, report.Failed...)
		merged.Blocked = append(merged.Blocked, report.Blocked...)
		merged.Reclaimed = append(merged.Reclaimed, report.Reclaimed...)
		merged.WaitingHuman = report.WaitingHuman
		if len(report.WaitingHuman) == 0 {
			return merged, nil
		}

		decided, undecided := 0, ""
		for _, id := range report.WaitingHuman {
			task, err := store.GetTask(ctx, id)
			if err != nil || task == nil {
				return merged, fmt.Errorf("read waiting task %s: %w", id, err)
			}
			if _, again := applied[id]; again {
				return merged, fmt.Errorf("gate %q: still waiting after its decision", task.StepID)
			}
			gate, ok := fixture.Gates[task.StepID]
			if !ok {
				if undecided == "" {
					undecided = task.StepID
				}
				continue
			}
			if err := decideGate(ctx, cmd, store, task, gate); err != nil {
				return merged, err
			}
			if gate.Decision == flowtest.GateApprove {
				merged.Done = append(merged.Done, id)
			}
			applied[id] = struct{}{}
			decided++
		}
		if decided == 0 {
			return merged, fmt.Errorf("gate %q: no decision in test.yaml gates", undecided)
		}
	}
}

// decideGate applies one scripted decision through the same core decision
// `tlc task approve|reject` uses.
func decideGate(ctx context.Context, out *cobra.Command, store *storage.SQLiteStorage, task *core.Task, gate flowtest.GateDecision) error {
	by := approvalBy(gate.By)
	in := core.HumanDecisionInput{Decision: core.HumanApproved, By: by, Note: gate.Note}
	if gate.Decision == flowtest.GateReject {
		in = core.HumanDecisionInput{Decision: core.HumanRejected, By: by, Reason: gate.Reason}
	}
	if err := core.DecideHumanTask(ctx, store, core.DefaultWorkflow(), task, in); err != nil {
		return fmt.Errorf("gate %q: %w", task.StepID, err)
	}
	_, _ = fmt.Fprintf(out.OutOrStdout(), "  gate %q: %s by %s\n", task.StepID, in.Decision, by)
	return nil
}

// gateVerdict is what the scripted gates add to a run's grade.
type gateVerdict struct {
	// accounted counts the tasks a rejection explains: the rejected gates
	// and their dependents that were never reached.
	accounted int
	// problems are the failures, one line each.
	problems []string
}

// gradeGates checks the run's tasks against its scripted gates. A gate must
// end where its decision says: approved is finished, rejected is blocked by
// a rejection. Behind a rejected gate nothing may have run: its transitive
// dependents must still be in the initial status, which tells a task that
// was never reached from one a false when: skipped.
func gradeGates(
	ctx context.Context, store *storage.SQLiteStorage, fixture flowtest.Run,
	taskIDs []string, finished map[string]struct{},
) (gateVerdict, error) {
	var v gateVerdict
	if len(fixture.Gates) == 0 {
		return v, nil
	}
	tasks := make([]*core.Task, 0, len(taskIDs))
	byStep := map[string]*core.Task{}
	for _, id := range taskIDs {
		task, err := store.GetTask(ctx, id)
		if err != nil || task == nil {
			return v, fmt.Errorf("read task %s: %w", id, err)
		}
		tasks = append(tasks, task)
		byStep[task.StepID] = task
	}
	wm := core.DefaultWorkflow()

	var rejected []*core.Task
	for _, step := range sortedGateSteps(fixture.Gates) {
		task, ok := byStep[step]
		if !ok { // the --task selection left the gate out
			continue
		}
		want := fixture.Gates[step].Decision
		if got := gateEndState(task, wm); got != want {
			v.problems = append(v.problems, fmt.Sprintf("gate %q: declared %s, ended %s", step, want, got))
			continue
		}
		if want == flowtest.GateReject {
			rejected = append(rejected, task)
			if _, ok := finished[task.ID]; !ok {
				v.accounted++
			}
		}
	}
	if len(rejected) == 0 {
		return v, nil
	}

	initial, err := wm.StatusForRole(config.RoleInitial)
	if err != nil {
		return v, fmt.Errorf("workflow has no initial status: %w", err)
	}
	graph, err := core.NewDepGraph(tasks)
	if err != nil {
		return v, fmt.Errorf("run graph: %w", err)
	}
	byID := make(map[string]*core.Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	seen := map[string]struct{}{}
	for _, gate := range rejected {
		queue := []string{gate.ID}
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			for _, dep := range graph.Dependents(id) {
				if _, ok := seen[dep]; ok {
					continue
				}
				seen[dep] = struct{}{}
				queue = append(queue, dep)
				task := byID[dep]
				if task == nil {
					continue
				}
				if task.Status != initial {
					v.problems = append(v.problems, fmt.Sprintf("%q ran behind rejected gate %q (status %s)", task.StepID, gate.StepID, task.Status))
					continue
				}
				if _, ok := finished[dep]; !ok {
					v.accounted++
				}
			}
		}
	}
	return v, nil
}

// gateEndState names where a gate task ended: approve when finished, reject
// when blocked by a rejection, otherwise still undecided.
func gateEndState(task *core.Task, wm *core.WorkflowManager) string {
	switch {
	case wm.IsTerminal(task.Status):
		return flowtest.GateApprove
	case task.BlockedReason != nil && strings.HasPrefix(*task.BlockedReason, "rejected:"):
		return flowtest.GateReject
	}
	return "undecided"
}

func sortedGateSteps(gates map[string]flowtest.GateDecision) []string {
	steps := make([]string, 0, len(gates))
	for step := range gates {
		steps = append(steps, step)
	}
	sort.Strings(steps)
	return steps
}
