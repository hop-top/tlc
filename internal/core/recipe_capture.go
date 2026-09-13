package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

// CaptureOpts tunes CaptureTrack.
type CaptureOpts struct {
	// Name and Version head the recipe; they default to the track slug
	// and captureDefaultVersion.
	Name    string
	Version string
	// Vars maps a var name to the literal it stands for. Every whole-word
	// occurrence of the literal in a templatable field becomes {{name}}
	// and the var is declared as required. The vars of the track's latest
	// run seed the map; explicit entries win.
	Vars map[string]string
	// Plan is the track's plan body, emitted as track.plan.
	Plan string
	// IncludeExternalDeps pulls tasks outside the track that track tasks
	// depend on in as steps instead of dropping the edge.
	IncludeExternalDeps bool
}

const (
	captureDefaultVersion = "0.1.0"
	captureStepIDMaxLen   = 40
	captureDateLayout     = "2006-01-02"
	captureFallbackStepID = "step"
)

// CaptureTrack describes a track's tasks as a recipe: one step per task
// in dependency order, ids from recipe provenance where the task has it,
// depends_on from intra-track blocked_by, and the literals behind the
// given vars lifted back into placeholders. The result is validated like
// a parsed recipe. Warnings report what could not be captured exactly.
func CaptureTrack(
	ctx context.Context, repo Repository, runs RecipeRunStore, track *Track, opts CaptureOpts,
) (*Recipe, []string, error) {
	if track == nil {
		return nil, nil, errors.New("capture: no track given")
	}
	c := &capture{repo: repo, track: track, opts: opts, stepID: map[string]string{}, external: map[string]bool{}}
	tasks, err := c.collectTasks(ctx)
	if err != nil {
		return nil, nil, err
	}
	ordered, err := orderTasksByBatch(tasks)
	if err != nil {
		return nil, nil, fmt.Errorf("capture track %s: %w", c.label(), err)
	}
	renamed := c.assignStepIDs(ordered)
	steps := c.buildSteps(ordered, renamed)
	vars, err := c.resolveVars(ctx, runs)
	if err != nil {
		return nil, nil, err
	}
	c.substituteVars(steps, vars)
	c.warnLiterals(steps)
	rec := c.assemble(steps, vars)
	if err := ValidateRecipe(rec); err != nil {
		return nil, nil, fmt.Errorf("capture track %s: %w", c.label(), err)
	}
	return rec, c.warnings, nil
}

// capture is the state of one CaptureTrack call.
type capture struct {
	repo     Repository
	track    *Track
	opts     CaptureOpts
	warnings []string
	stepID   map[string]string // task id → step id
	external map[string]bool   // task ids pulled in from outside the track
}

func (c *capture) warnf(format string, args ...any) {
	c.warnings = append(c.warnings, fmt.Sprintf(format, args...))
}

func (c *capture) label() string {
	if c.track.Slug != "" {
		return c.track.Slug
	}
	return c.track.ID
}

func projectIDOf(track *Track) string {
	if track.ProjectID == nil {
		return ""
	}
	return *track.ProjectID
}

// collectTasks lists the track's tasks the way linked-task accounting
// does: scoped by track and project, archived rows included.
func (c *capture) collectTasks(ctx context.Context) ([]*Task, error) {
	tasks, err := c.repo.ListTasks(ctx, Query{
		Filters: []FieldFilter{
			{Field: filterTrackID, Operator: OpEq, Value: c.track.ID},
			{Field: filterProjectID, Operator: OpEq, Value: projectIDOf(c.track)},
		},
		AllProjects:     true,
		IncludeArchived: true,
	})
	if err != nil {
		return nil, fmt.Errorf("capture track %s: list tasks: %w", c.label(), err)
	}
	if len(tasks) == 0 {
		return nil, fmt.Errorf(
			"capture track %s: no tasks to capture; add tasks with 'tlc task create \"<title>\" --track %s' first",
			c.label(), c.label(),
		)
	}
	if !c.opts.IncludeExternalDeps {
		return tasks, nil
	}
	return c.pullExternalDeps(ctx, tasks)
}

// pullExternalDeps appends every task outside the track that a listed
// task depends on, transitively; tasks grows while it is scanned.
func (c *capture) pullExternalDeps(ctx context.Context, tasks []*Task) ([]*Task, error) {
	known := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		known[t.ID] = true
	}
	for i := 0; i < len(tasks); i++ {
		for _, dep := range tasks[i].BlockedBy() {
			if known[dep] {
				continue
			}
			known[dep] = true
			ext, err := c.repo.GetTask(ctx, dep)
			if err != nil {
				return nil, fmt.Errorf("capture track %s: get dependency %s: %w", c.label(), dep, err)
			}
			if ext == nil {
				continue
			}
			c.external[ext.ID] = true
			tasks = append(tasks, ext)
		}
	}
	return tasks, nil
}

// orderTasksByBatch returns tasks in execution order: dependency batches
// first, sequence number within a batch.
func orderTasksByBatch(tasks []*Task) ([]*Task, error) {
	g, err := NewDepGraph(tasks)
	if err != nil {
		return nil, err
	}
	strategy, err := g.ComputeStrategy()
	if err != nil {
		return nil, err
	}
	out := make([]*Task, 0, len(tasks))
	for _, batch := range strategy.Batches {
		members := slices.Clone(batch.Tasks)
		sort.SliceStable(members, func(i, j int) bool {
			if members[i].Seq != members[j].Seq {
				return members[i].Seq < members[j].Seq
			}
			return members[i].ID < members[j].ID
		})
		out = append(out, members...)
	}
	return out, nil
}

// assignStepIDs picks a unique step id per task and returns the renames
// of recipe-born ids so their results.<id> references can follow.
func (c *capture) assignStepIDs(tasks []*Task) map[string]string {
	used := map[string]bool{}
	renamed := map[string]string{}
	for _, t := range tasks {
		want, born := c.wantedStepID(t)
		id := uniqueStepID(want, used)
		if id != want {
			c.warnf("step %s: id %q is already taken by an earlier task; renamed to %q", id, want, id)
		}
		used[id] = true
		c.stepID[t.ID] = id
		if born && t.StepID != id {
			renamed[t.StepID] = id
		}
	}
	return renamed
}

// wantedStepID is the provenance step id of a recipe-born task, flattened
// when it came out of an include or repeat block, else a slug of the
// title. The bool reports whether the id is recipe-born.
func (c *capture) wantedStepID(t *Task) (string, bool) {
	if t.StepID == "" || t.Provenance().Recipe == "" {
		return sanitizeStepID(SlugFromTitle(t.Title, captureStepIDMaxLen)), false
	}
	if stepIDRe.MatchString(t.StepID) {
		return t.StepID, true
	}
	flat := sanitizeStepID(strings.ReplaceAll(t.StepID, "/", "-"))
	c.warnf(
		"step %s: id %q came from an expanded include or repeat block and cannot be re-rolled; flattened",
		flat, t.StepID,
	)
	return flat, true
}

func uniqueStepID(want string, used map[string]bool) string {
	id := want
	for n := 2; used[id]; n++ {
		id = fmt.Sprintf("%s-%d", want, n)
	}
	return id
}

// sanitizeStepID keeps what stepIDRe accepts; the fallback id when
// nothing survives.
func sanitizeStepID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		return captureFallbackStepID
	}
	return id
}

func (c *capture) buildSteps(tasks []*Task, renamed map[string]string) []RecipeStep {
	steps := make([]RecipeStep, 0, len(tasks))
	for i, t := range tasks {
		step := c.stepFromTask(t)
		step.Ordinal = i + 1
		rewriteStepRefs(&step, renamed)
		steps = append(steps, step)
	}
	return steps
}

// stepFromTask copies the fields a task carries back into a step. Spec
// blocks are deep-copied so the recipe never aliases the task row.
func (c *capture) stepFromTask(t *Task) RecipeStep {
	id := c.stepID[t.ID]
	step := RecipeStep{ID: id, Title: t.Title, Description: t.Description, DependsOn: c.stepDeps(id, t)}
	if t.Kind != "" && t.Kind != TaskKindAgent {
		step.Kind = t.Kind
	}
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		step.Assignee = "@" + *t.AssignedTo
	}
	if t.DueAt != nil {
		step.Due.Raw = t.DueAt.UTC().Format(captureDateLayout)
	}
	if t.Spec != nil {
		step.Agent, step.When = t.Spec.Agent, t.Spec.When
		step.Exec, step.Human, step.Retry, step.Gate = t.Spec.Exec, t.Spec.Human, t.Spec.Retry, t.Spec.Gate
	}
	if c.external[t.ID] {
		c.warnf("step %s: captured from a task outside the track", id)
	}
	if strings.TrimSpace(t.Description) == "" {
		c.warnf("step %s: description is empty", id)
	}
	return copyStep(step)
}

// stepDeps maps blocked_by edges to step ids; an edge to a task that is
// not being captured is dropped with a warning.
func (c *capture) stepDeps(id string, t *Task) []string {
	var deps []string
	for _, dep := range t.BlockedBy() {
		if to, ok := c.stepID[dep]; ok {
			deps = append(deps, to)
			continue
		}
		c.warnf("step %s: dependency on task %s outside the track dropped; pass --external-deps to capture it", id, dep)
	}
	return deps
}

// resolveVars seeds the var map from the track's latest run, then lets
// the explicit entries win.
func (c *capture) resolveVars(ctx context.Context, runs RecipeRunStore) (map[string]string, error) {
	vars := map[string]string{}
	if runs != nil {
		latest, err := runs.ListRecipeRuns(ctx, RecipeRunQuery{
			TrackID: c.track.ID, ProjectID: projectIDOf(c.track), AllProjects: true, Limit: 1,
		})
		if err != nil {
			return nil, fmt.Errorf("capture track %s: list runs: %w", c.label(), err)
		}
		if len(latest) > 0 {
			for name, value := range latest[0].Vars {
				vars[name] = templateValue(value)
			}
		}
	}
	maps.Copy(vars, c.opts.Vars)
	return vars, nil
}

// assemble builds the recipe document around the steps. An agent shared
// by every step is hoisted to the header, undoing the materializer's
// per-task fallback.
func (c *capture) assemble(steps []RecipeStep, vars map[string]string) *Recipe {
	rec := &Recipe{
		Name:    c.recipeName(),
		Version: c.opts.Version,
		Agent:   hoistAgent(steps),
		Track:   &RecipeTrack{Title: c.track.Title, Type: c.track.Type, Plan: c.opts.Plan},
		Steps:   steps,
	}
	if rec.Version == "" {
		rec.Version = captureDefaultVersion
	}
	if len(vars) > 0 {
		rec.Vars = make(map[string]VarDef, len(vars))
		for name := range vars {
			rec.Vars[name] = VarDef{Required: true}
		}
	}
	return rec
}

func (c *capture) recipeName() string {
	switch {
	case c.opts.Name != "":
		return c.opts.Name
	case c.track.Slug != "":
		return c.track.Slug
	}
	return SlugFromTitle(c.track.Title, 0)
}

func hoistAgent(steps []RecipeStep) string {
	if len(steps) == 0 || steps[0].Agent == "" {
		return ""
	}
	agent := steps[0].Agent
	for i := range steps[1:] {
		if steps[i+1].Agent != agent {
			return ""
		}
	}
	for i := range steps {
		steps[i].Agent = ""
	}
	return agent
}
