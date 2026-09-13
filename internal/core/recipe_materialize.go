package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hop.top/kit/go/core/util"
)

// metaKeyRecipeOwned is the task.Meta key recording which
// meta["blocked_by"] entries a recipe run authored. Like metaKeyPlanOwned
// for plans: a run owns exactly the edges it declared, so a later pass
// may replace those and must leave edges added out-of-band untouched.
const metaKeyRecipeOwned = "blocked_by_recipe"

// Materializer turns expanded, selected and rendered recipe steps into
// tasks and records each pass in the run ledger. It never executes
// anything — `track execute` is the single engine — and never updates or
// deletes a task it did not create in the same call.
type Materializer struct {
	Repo  Repository
	IDGen IDGenerator
	Runs  RecipeRunStore
	// Now supplies task timestamps and the reference time relative dues
	// ("+2d", "friday") resolve against; nil means time.Now().UTC().
	Now func() time.Time
}

// MaterializeInput is one materialization request.
type MaterializeInput struct {
	Recipe *Recipe
	// Steps are already expanded, selected and rendered: flat, with
	// Ordinal set and Due.Raw holding a string the due parser accepts.
	Steps []RecipeStep

	TrackID   string // "" materializes trackless tasks
	ProjectID string
	By        string

	SubjectType string // "" when the recipe has no subject
	SubjectID   string

	Vars        map[string]any
	Selection   string              // raw --task selector, recorded on the run
	DroppedDeps map[string][]string // step id → deps dropped by selection

	// Existing maps step ids already materialized to their task ids.
	// Steps present here are skipped; dependencies on them resolve
	// through it.
	Existing map[string]string

	// Recreate lets Reconcile create a step again when the ledger says
	// it was materialized once but its task has since been deleted.
	Recreate bool
	// DryRun reports what would be created without allocating ids or
	// writing tasks or ledger rows.
	DryRun bool
	// ParentRun links the run to the one it continues; Reconcile sets it
	// to the latest run of the same recipe on the track.
	ParentRun string
	// RunID, when set, is the id the run is recorded under, so a caller
	// that rendered {{run.id}} into the steps records the same id; empty
	// mints one.
	RunID string

	// PreCreate, when set, gates every task before it is persisted; an
	// error aborts the batch.
	PreCreate func(*Task) error
}

// StepTask pairs a step with the task that materializes it.
type StepTask struct {
	StepID  string `json:"step_id" yaml:"step_id"`
	TaskID  string `json:"task_id,omitempty" yaml:"task_id,omitempty"`
	Ordinal int    `json:"ordinal,omitempty" yaml:"ordinal,omitempty"`
}

// MaterializeResult reports one pass. RunID is empty when nothing had to
// be created; in a dry run Created entries carry no task id.
type MaterializeResult struct {
	RunID    string     `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	Created  []StepTask `json:"created,omitempty" yaml:"created,omitempty"`
	Skipped  []StepTask `json:"skipped,omitempty" yaml:"skipped,omitempty"`
	Warnings []string   `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// materializePlan is what Materialize and Reconcile hand to run: the
// steps still to create in recipe order, the ones already covered, the
// task id every out-of-batch dependency resolves to, and the dropped
// dependencies to record on the run.
type materializePlan struct {
	create   []RecipeStep
	skipped  []StepTask
	warnings []string
	existing map[string]string
	dropped  map[string][]string
}

// Materialize creates one task per step not present in in.Existing,
// wires blocked_by from depends_on, and records the pass as a run. The
// run row is written first, then the tasks, then the step→task rows.
func (m *Materializer) Materialize(ctx context.Context, in MaterializeInput) (*MaterializeResult, error) {
	if in.Recipe == nil {
		return nil, errors.New("materialize: no recipe given")
	}
	plan := materializePlan{existing: in.Existing, dropped: in.DroppedDeps}
	for _, step := range in.Steps {
		if taskID, ok := in.Existing[step.ID]; ok {
			plan.skipped = append(plan.skipped, StepTask{StepID: step.ID, TaskID: taskID, Ordinal: step.Ordinal})
			continue
		}
		plan.create = append(plan.create, step)
	}
	return m.run(ctx, in, plan)
}

func (m *Materializer) run(ctx context.Context, in MaterializeInput, plan materializePlan) (*MaterializeResult, error) {
	res := &MaterializeResult{Skipped: plan.skipped, Warnings: plan.warnings}
	if len(plan.create) == 0 {
		return res, nil
	}
	now := m.now()
	prepared, err := prepareSteps(in.Recipe.Name, plan, now)
	if err != nil {
		return nil, err
	}
	res.RunID = in.RunID
	if res.RunID == "" {
		res.RunID = NewRecipeRunID()
	}
	if in.DryRun {
		for _, step := range plan.create {
			res.Created = append(res.Created, StepTask{StepID: step.ID, Ordinal: step.Ordinal})
		}
		return res, nil
	}

	run := runRecord(in, res.RunID, plan.dropped, now)
	if err := m.Runs.CreateRecipeRun(ctx, run); err != nil {
		return nil, fmt.Errorf("materialize: record run %s: %w", run.ID, err)
	}
	frame := &materializeRun{in: in, runID: res.RunID, now: now, steps: plan.create, prepared: prepared}
	ids, createErr := createSpecTasks(ctx, m.Repo, specTasks{
		IDGen: m.IDGen, ProjectID: in.ProjectID, Count: len(plan.create),
		Label: frame.label, Build: frame.build, PreCreate: in.PreCreate,
	})
	rows := make([]RecipeRunTask, len(ids))
	for i, id := range ids {
		rows[i] = RecipeRunTask{RunID: res.RunID, StepID: plan.create[i].ID, TaskID: id}
		res.Created = append(res.Created, StepTask{StepID: plan.create[i].ID, TaskID: id, Ordinal: plan.create[i].Ordinal})
	}
	if createErr != nil {
		return nil, m.abandonRun(ctx, res.RunID, rows, createErr)
	}
	if err := m.Runs.AddRecipeRunTasks(ctx, res.RunID, rows); err != nil {
		return nil, fmt.Errorf("materialize: record run %s tasks: %w", res.RunID, err)
	}
	return res, nil
}

// abandonRun settles the ledger after task creation failed part-way:
// the tasks that were created are recorded so a later Reconcile resumes
// from them, and a run that created nothing is removed. A ledger failure
// is appended to the original error rather than replacing it.
func (m *Materializer) abandonRun(ctx context.Context, runID string, rows []RecipeRunTask, cause error) error {
	var settle error
	if len(rows) == 0 {
		settle = m.Runs.DeleteRecipeRun(ctx, runID)
	} else {
		settle = m.Runs.AddRecipeRunTasks(ctx, runID, rows)
	}
	if settle != nil {
		return fmt.Errorf("%w (materialize: settle run %s: %w)", cause, runID, settle)
	}
	return cause
}

func (m *Materializer) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now().UTC()
}

// stepDep is a resolved dependency: an index into the batch being created
// or the id of a task materialized earlier.
type stepDep struct {
	index  int
	taskID string
}

// preparedStep holds what buildTask needs beyond the step itself.
type preparedStep struct {
	deps []stepDep
	due  *time.Time
}

// prepareSteps validates every dependency and due before anything is
// allocated or written, so a bad step leaves no partial run behind.
func prepareSteps(recipe string, plan materializePlan, now time.Time) ([]preparedStep, error) {
	index := make(map[string]int, len(plan.create))
	for i, step := range plan.create {
		index[step.ID] = i
	}
	out := make([]preparedStep, len(plan.create))
	for i, step := range plan.create {
		for _, dep := range step.DependsOn {
			if j, ok := index[dep]; ok {
				out[i].deps = append(out[i].deps, stepDep{index: j})
				continue
			}
			if taskID, ok := plan.existing[dep]; ok && taskID != "" {
				out[i].deps = append(out[i].deps, stepDep{taskID: taskID})
				continue
			}
			return nil, fmt.Errorf(
				"recipe %s step %s: depends_on %q is neither in this run nor already materialized; "+
					"include it in the selection or materialize it first",
				recipe, step.ID, dep,
			)
		}
		raw := strings.TrimSpace(step.Due.Raw)
		if raw == "" {
			continue
		}
		due, err := util.ParseUntilAt(raw, now)
		if err != nil {
			return nil, fmt.Errorf("recipe %s step %s: invalid due %q: %w", recipe, step.ID, raw, err)
		}
		out[i].due = &due
	}
	return out, nil
}

// materializeRun is the build state for one run's batch.
type materializeRun struct {
	in       MaterializeInput
	runID    string
	now      time.Time
	steps    []RecipeStep
	prepared []preparedStep
}

func (r *materializeRun) label(i int) string {
	return fmt.Sprintf("recipe %s step %s", r.in.Recipe.Name, r.steps[i].ID)
}

// build turns step i into a task. Dependencies resolve through the batch
// ids or the existing map; the edges are recorded as recipe-owned.
func (r *materializeRun) build(i int, ids []string, seqs []int64) (*Task, error) {
	step, recipe := r.steps[i], r.in.Recipe
	title := step.Title
	if title == "" {
		title = step.ID
	}
	task := &Task{
		ID:          ids[i],
		Seq:         seqs[i],
		Title:       title,
		Description: strings.TrimSpace(step.Description),
		Status:      StatusTodo,
		AssignedTo:  assigneeFromSpec(step.Assignee),
		Kind:        step.EffectiveKind(),
		Spec:        stepSpec(step, recipe.Agent),
		DueAt:       r.prepared[i].due,
		RunID:       r.runID,
		StepID:      step.ID,
		StepOrdinal: step.Ordinal,
		TrackID:     optString(r.in.TrackID),
		ProjectID:   optString(r.in.ProjectID),
		CreatedAt:   r.now,
		UpdatedAt:   r.now,
	}
	task.SetProvenance(TaskProvenance{
		Recipe: recipe.Name, RecipeVersion: recipe.Version, RecipeHash: recipe.Hash, Subject: r.in.SubjectID,
	})
	if deps := r.prepared[i].deps; len(deps) > 0 {
		edges := make([]string, len(deps))
		for j, dep := range deps {
			edges[j] = dep.taskID
			if dep.taskID == "" {
				edges[j] = ids[dep.index]
			}
		}
		task.SetBlockedBy(mergeOwnedBlockedBy(task, metaKeyRecipeOwned, edges))
		setStringSliceMeta(task, metaKeyRecipeOwned, edges)
	}
	return task, nil
}

// stepSpec maps a step's dispatch fields onto a TaskSpec, falling back to
// the recipe agent; nil when every field is empty so tasks that carry no
// dispatch data serialize exactly like hand-made ones.
func stepSpec(step RecipeStep, recipeAgent string) *TaskSpec {
	agent := step.Agent
	if agent == "" {
		agent = recipeAgent
	}
	spec := TaskSpec{Agent: agent, When: step.When, Exec: step.Exec, Human: step.Human, Retry: step.Retry, Gate: step.Gate}
	if spec == (TaskSpec{}) {
		return nil
	}
	return &spec
}

// runRecord is the ledger row for a pass.
func runRecord(in MaterializeInput, runID string, dropped map[string][]string, now time.Time) *RecipeRun {
	return &RecipeRun{
		ID:          runID,
		ProjectID:   in.ProjectID,
		RecipeID:    in.Recipe.Name,
		Version:     in.Recipe.Version,
		Hash:        in.Recipe.Hash,
		Vars:        in.Vars,
		SubjectType: in.SubjectType,
		SubjectID:   in.SubjectID,
		TrackID:     in.TrackID,
		Selection:   in.Selection,
		DroppedDeps: flattenDroppedDeps(dropped),
		ParentRun:   in.ParentRun,
		CreatedBy:   in.By,
		CreatedAt:   now,
	}
}

// flattenDroppedDeps renders step→deps as sorted "<step>:<dep>" entries;
// nil when nothing was dropped.
func flattenDroppedDeps(dropped map[string][]string) []string {
	var out []string
	for step, deps := range dropped {
		for _, dep := range deps {
			out = append(out, step+":"+dep)
		}
	}
	sort.Strings(out)
	return out
}
