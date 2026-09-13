package core

import (
	"context"
	"errors"
	"fmt"
)

// Reconcile brings a track up to date with a recipe: steps the ledger
// already covers are skipped, the rest are materialized in a new run
// whose ParentRun is the latest run of the same recipe on the track.
//
// The ledger, not the task table, decides what exists: a step whose task
// was deleted after materialization is reported and left alone unless
// in.Recreate is set, and a dependency on such a step is dropped rather
// than left dangling. Existing tasks are never updated or deleted.
func (m *Materializer) Reconcile(ctx context.Context, in MaterializeInput) (*MaterializeResult, error) {
	if in.Recipe == nil {
		return nil, errors.New("reconcile: no recipe given")
	}
	if in.TrackID == "" {
		return nil, fmt.Errorf(
			"reconcile: recipe %s: no track to reconcile against; materialize into a track first",
			in.Recipe.Name,
		)
	}
	ledger, err := m.readLedger(ctx, in)
	if err != nil {
		return nil, err
	}
	plan := materializePlan{existing: map[string]string{}, dropped: cloneDroppedDeps(in.DroppedDeps)}
	for id, taskID := range in.Existing {
		plan.existing[id] = taskID
	}
	if ledger.latest != nil {
		in.ParentRun = ledger.latest.ID
		plan.warnings = append(plan.warnings, driftWarnings(in.Recipe, ledger.latest)...)
	}
	deleted, err := m.partitionSteps(ctx, in, ledger, &plan)
	if err != nil {
		return nil, err
	}
	dropDeletedDeps(&plan, deleted)
	return m.run(ctx, in, plan)
}

// ledgerView is the recipe's history on the track: the latest run and,
// per step, the task the most recent run mapped it to.
type ledgerView struct {
	latest *RecipeRun
	byStep map[string]string // step id → task id, later runs win
	runOf  map[string]string // step id → run that created it
}

// readLedger scopes the track's ledger to in.Recipe: another recipe's
// steps on the same track are neither drift nor existing steps, even
// when their ids collide.
func (m *Materializer) readLedger(ctx context.Context, in MaterializeInput) (*ledgerView, error) {
	view := &ledgerView{byStep: map[string]string{}, runOf: map[string]string{}}
	runs, err := m.Runs.ListRecipeRuns(ctx, RecipeRunQuery{
		RecipeID: in.Recipe.Name, TrackID: in.TrackID, ProjectID: in.ProjectID, AllProjects: true,
	})
	if err != nil {
		return nil, fmt.Errorf("reconcile: list runs of recipe %s on track %s: %w", in.Recipe.Name, in.TrackID, err)
	}
	if len(runs) == 0 {
		return view, nil
	}
	view.latest = runs[0]
	mine := make(map[string]bool, len(runs))
	for _, run := range runs {
		mine[run.ID] = true
	}
	rows, err := m.Runs.ListRecipeRunTasksByTrack(ctx, in.TrackID)
	if err != nil {
		return nil, fmt.Errorf("reconcile: list run tasks on track %s: %w", in.TrackID, err)
	}
	for _, row := range rows { // oldest first, so a later run overwrites
		if mine[row.RunID] {
			view.byStep[row.StepID] = row.TaskID
			view.runOf[row.StepID] = row.RunID
		}
	}
	return view, nil
}

// partitionSteps sorts in.Steps into plan.create and plan.skipped and
// returns the steps whose task is gone and is not being recreated.
func (m *Materializer) partitionSteps(
	ctx context.Context, in MaterializeInput, ledger *ledgerView, plan *materializePlan,
) (map[string]bool, error) {
	deleted := map[string]bool{}
	skip := func(step RecipeStep, taskID string) {
		plan.skipped = append(plan.skipped, StepTask{StepID: step.ID, TaskID: taskID, Ordinal: step.Ordinal})
	}
	for _, step := range in.Steps {
		if taskID, ok := in.Existing[step.ID]; ok {
			skip(step, taskID)
			continue
		}
		taskID, recorded := ledger.byStep[step.ID]
		if !recorded {
			plan.create = append(plan.create, step)
			continue
		}
		task, err := m.Repo.GetTask(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("reconcile: recipe %s step %s: get task %s: %w", in.Recipe.Name, step.ID, taskID, err)
		}
		switch {
		case task != nil:
			plan.existing[step.ID] = taskID
			skip(step, taskID)
		case in.Recreate:
			plan.create = append(plan.create, step)
		default:
			deleted[step.ID] = true
			skip(step, taskID)
			plan.warnings = append(plan.warnings, fmt.Sprintf(
				"step %s: task %s created by run %s has been deleted; not recreated (pass --recreate to create it again)",
				step.ID, taskID, ledger.runOf[step.ID],
			))
		}
	}
	return deleted, nil
}

// dropDeletedDeps removes dependencies on deleted, not-recreated steps
// from the steps about to be created, recording each drop on the run.
func dropDeletedDeps(plan *materializePlan, deleted map[string]bool) {
	if len(deleted) == 0 {
		return
	}
	for i := range plan.create {
		step := &plan.create[i]
		var kept []string
		for _, dep := range step.DependsOn {
			if !deleted[dep] {
				kept = append(kept, dep)
				continue
			}
			plan.dropped[step.ID] = append(plan.dropped[step.ID], dep)
			plan.warnings = append(plan.warnings, fmt.Sprintf(
				"step %s: dependency on deleted step %s dropped", step.ID, dep,
			))
		}
		step.DependsOn = kept
	}
}

// driftWarnings reports when the recipe differs from what the latest run
// recorded; existing tasks keep the definition they were created from.
func driftWarnings(recipe *Recipe, latest *RecipeRun) []string {
	var out []string
	if latest.Version != recipe.Version {
		out = append(out, fmt.Sprintf(
			"recipe %s: latest run %s used version %s, now %s; existing tasks keep the old definition",
			recipe.Name, latest.ID, latest.Version, recipe.Version,
		))
	}
	if latest.Hash != recipe.Hash {
		out = append(out, fmt.Sprintf(
			"recipe %s: latest run %s used content hash %s, now %s; existing tasks keep the old definition",
			recipe.Name, latest.ID, latest.Hash, recipe.Hash,
		))
	}
	return out
}

// cloneDroppedDeps copies the caller's map so reconcile can add to it
// without mutating the input.
func cloneDroppedDeps(src map[string][]string) map[string][]string {
	out := make(map[string][]string, len(src))
	for step, deps := range src {
		out[step] = append([]string(nil), deps...)
	}
	return out
}
