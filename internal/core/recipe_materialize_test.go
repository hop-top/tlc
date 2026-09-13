package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"hop.top/kit/go/core/util"
)

var matNow = time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)

const (
	matTrack   = "track_01k4zq9x8b2c3d4e5f6g7h8j9k"
	matProject = "proj-a"
)

type matFixture struct {
	repo *MockRepository
	runs *fakeRunStore
	m    *Materializer
}

func newMatFixture() *matFixture {
	repo := NewMockRepository()
	runs := newFakeRunStore()
	return &matFixture{
		repo: repo,
		runs: runs,
		m:    &Materializer{Repo: repo, IDGen: repo, Runs: runs, Now: func() time.Time { return matNow }},
	}
}

func matRecipe() *Recipe {
	return &Recipe{Name: "release", Version: "1.2.0", Hash: "sha256:abc", Agent: "planner"}
}

// matSteps is a three-step chain exercising every field a step carries
// into a task: agent precedence, exec/human kinds, retry, gate, due in
// absolute and relative form, and an @-prefixed assignee.
func matSteps() []RecipeStep {
	return []RecipeStep{
		{
			ID: "plan", Title: "Plan the release", Description: "  Write the plan.\n",
			Ordinal: 1, Agent: "architect", When: "vars.ship",
		},
		{
			ID: "build", Title: "Build", Ordinal: 2, DependsOn: []string{"plan"}, Kind: TaskKindExec,
			Exec:  &ExecSpec{Argv: []string{"make", "build"}},
			Retry: &RetrySpec{MaxAttempts: 3},
			Due:   DueSpec{Raw: "2026-10-06"},
		},
		{
			ID: "sign-off", Ordinal: 3, DependsOn: []string{"plan", "build"}, Kind: TaskKindHuman,
			Assignee: "@lead",
			Human:    &HumanSpec{Timeout: "48h", OnTimeout: "reject"},
			Gate:     &StepGate{Contract: "release-ok"},
			Due:      DueSpec{Raw: "+2d"},
		},
	}
}

func matInput(steps []RecipeStep) MaterializeInput {
	return MaterializeInput{
		Recipe: matRecipe(), Steps: steps,
		TrackID: matTrack, ProjectID: matProject, By: "jad",
		SubjectType: SubjectTask, SubjectID: "task_subject",
		Vars:        map[string]any{"ship": true},
		Selection:   "1-3",
		DroppedDeps: map[string][]string{"sign-off": {"qa"}},
	}
}

func (f *matFixture) materialize(t *testing.T, in MaterializeInput) *MaterializeResult {
	t.Helper()
	res, err := f.m.Materialize(context.Background(), in)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	return res
}

func (f *matFixture) reconcile(t *testing.T, in MaterializeInput) *MaterializeResult {
	t.Helper()
	res, err := f.m.Reconcile(context.Background(), in)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	return res
}

func (f *matFixture) task(t *testing.T, id string) *Task {
	t.Helper()
	task, err := f.repo.GetTask(context.Background(), id)
	if err != nil || task == nil {
		t.Fatalf("task %q not in repo (err=%v)", id, err)
	}
	return task
}

func (f *matFixture) run(t *testing.T, id string) *RecipeRun {
	t.Helper()
	run := f.runs.runs[id]
	if run == nil {
		t.Fatalf("run %q not in ledger", id)
	}
	return run
}

func stepTaskIDs(res *MaterializeResult) map[string]string {
	out := map[string]string{}
	for _, c := range res.Created {
		out[c.StepID] = c.TaskID
	}
	return out
}

func stepTaskSteps(entries []StepTask) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.StepID
	}
	return out
}

func assertSteps(t *testing.T, label string, got []StepTask, want ...string) {
	t.Helper()
	gotIDs := stepTaskSteps(got)
	if len(gotIDs) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Errorf("%s = %v; want %v", label, gotIDs, want)
	}
}

func assertBlockedBy(t *testing.T, task *Task, want ...string) {
	t.Helper()
	got := task.BlockedBy()
	if len(want) == 0 {
		if got != nil {
			t.Errorf("%s blocked_by = %v; want none", task.StepID, got)
		}
		if _, ok := task.Meta[metaKeyRecipeOwned]; ok {
			t.Errorf("%s carries %s without edges", task.StepID, metaKeyRecipeOwned)
		}
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s blocked_by = %v; want %v", task.StepID, got, want)
	}
	if owned := NormalizeStringSliceMeta(task.Meta[metaKeyRecipeOwned]); !reflect.DeepEqual(owned, want) {
		t.Errorf("%s %s = %v; want %v", task.StepID, metaKeyRecipeOwned, owned, want)
	}
}

func TestMaterialize_BlockedByFromDeps(t *testing.T) {
	f := newMatFixture()
	in := matInput(matSteps())
	res := f.materialize(t, in)

	if !IsRecipeRunID(res.RunID) {
		t.Fatalf("RunID = %q; want run_ typeid", res.RunID)
	}
	assertSteps(t, "Created", res.Created, "plan", "build", "sign-off")
	assertSteps(t, "Skipped", res.Skipped)
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v; want none", res.Warnings)
	}
	for i, c := range res.Created {
		if c.Ordinal != i+1 || !IsTaskID(c.TaskID) {
			t.Errorf("Created[%d] = %+v; want ordinal %d and a task typeid", i, c, i+1)
		}
	}

	ids := stepTaskIDs(res)
	assertBlockedBy(t, f.task(t, ids["plan"]))
	assertBlockedBy(t, f.task(t, ids["build"]), ids["plan"])
	assertBlockedBy(t, f.task(t, ids["sign-off"]), ids["plan"], ids["build"])

	rows, _ := f.runs.ListRecipeRunTasks(context.Background(), res.RunID)
	wantRows := []RecipeRunTask{
		{RunID: res.RunID, StepID: "plan", TaskID: ids["plan"]},
		{RunID: res.RunID, StepID: "build", TaskID: ids["build"]},
		{RunID: res.RunID, StepID: "sign-off", TaskID: ids["sign-off"]},
	}
	if !reflect.DeepEqual(rows, wantRows) {
		t.Errorf("ledger rows = %+v; want %+v", rows, wantRows)
	}

	run := f.run(t, res.RunID)
	want := &RecipeRun{
		ID: res.RunID, ProjectID: matProject, RecipeID: "release", Version: "1.2.0", Hash: "sha256:abc",
		Vars: in.Vars, SubjectType: SubjectTask, SubjectID: "task_subject", TrackID: matTrack,
		Selection: "1-3", DroppedDeps: []string{"sign-off:qa"}, CreatedBy: "jad", CreatedAt: matNow,
	}
	if !reflect.DeepEqual(run, want) {
		t.Errorf("run = %+v; want %+v", run, want)
	}
}

func TestMaterialize_ProvenanceAndSpec(t *testing.T) {
	f := newMatFixture()
	res := f.materialize(t, matInput(matSteps()))
	ids := stepTaskIDs(res)
	wantProv := TaskProvenance{Recipe: "release", RecipeVersion: "1.2.0", RecipeHash: "sha256:abc", Subject: "task_subject"}
	for _, c := range res.Created {
		if got := f.task(t, c.TaskID).Provenance(); got != wantProv {
			t.Errorf("%s provenance = %+v; want %+v", c.StepID, got, wantProv)
		}
	}
	assertAgentStepTask(t, f.task(t, ids["plan"]), res.RunID)
	assertExecStepTask(t, f.task(t, ids["build"]))
	assertHumanStepTask(t, f.task(t, ids["sign-off"]))
}

// assertAgentStepTask checks the plan step's content: title, trimmed
// description, TODO status, and the step's own agent winning over the
// recipe's.
func assertAgentStepTask(t *testing.T, plan *Task, runID string) {
	t.Helper()
	if plan.Title != "Plan the release" || plan.Description != "Write the plan." || plan.Status != StatusTodo {
		t.Errorf("plan = %+v; want title/description/TODO", plan)
	}
	if plan.Kind != TaskKindAgent || plan.Spec == nil || plan.Spec.Agent != "architect" || plan.Spec.When != "vars.ship" {
		t.Errorf("plan kind/spec = %v %+v; want agent kind, step agent, when", plan.Kind, plan.Spec)
	}
	if plan.DueAt != nil || plan.AssignedTo != nil {
		t.Errorf("plan due/assignee = %v %v; want unset", plan.DueAt, plan.AssignedTo)
	}
	assertStepTaskPlacement(t, plan, runID)
}

// assertStepTaskPlacement checks where the plan step landed: run and
// step identity, track and project, timestamps and sequence.
func assertStepTaskPlacement(t *testing.T, plan *Task, runID string) {
	t.Helper()
	if plan.RunID != runID || plan.StepID != "plan" || plan.StepOrdinal != 1 {
		t.Errorf("plan run/step = %q %q %d; want %q plan 1", plan.RunID, plan.StepID, plan.StepOrdinal, runID)
	}
	if plan.TrackID == nil || *plan.TrackID != matTrack || plan.ProjectID == nil || *plan.ProjectID != matProject {
		t.Errorf("plan track/project = %v %v", plan.TrackID, plan.ProjectID)
	}
	if !plan.CreatedAt.Equal(matNow) || !plan.UpdatedAt.Equal(matNow) || plan.Seq == 0 {
		t.Errorf("plan timestamps/seq = %v %v %d; want %v and an allocated seq", plan.CreatedAt, plan.UpdatedAt, plan.Seq, matNow)
	}
}

// assertExecStepTask checks the build step: exec kind, the recipe agent
// as fallback, retry, and an absolute due.
func assertExecStepTask(t *testing.T, build *Task) {
	t.Helper()
	if build.Kind != TaskKindExec || build.Spec == nil || build.Spec.Agent != "planner" {
		t.Errorf("build kind/spec = %v %+v; want exec kind and the recipe agent", build.Kind, build.Spec)
	}
	if !reflect.DeepEqual(build.Spec.Exec, &ExecSpec{Argv: []string{"make", "build"}}) ||
		!reflect.DeepEqual(build.Spec.Retry, &RetrySpec{MaxAttempts: 3}) {
		t.Errorf("build exec/retry = %+v %+v", build.Spec.Exec, build.Spec.Retry)
	}
	wantDue, _ := util.ParseUntilAt("2026-10-06", matNow)
	if build.DueAt == nil || !build.DueAt.Equal(wantDue) {
		t.Errorf("build due = %v; want %v", build.DueAt, wantDue)
	}
}

// assertHumanStepTask checks the sign-off step: title fallback to the
// step id, stripped assignee, human spec, gate, and a relative due.
func assertHumanStepTask(t *testing.T, signoff *Task) {
	t.Helper()
	if signoff.Title != "sign-off" {
		t.Errorf("sign-off title = %q; want the step id as fallback", signoff.Title)
	}
	if signoff.AssignedTo == nil || *signoff.AssignedTo != "lead" {
		t.Errorf("sign-off assignee = %v; want lead", signoff.AssignedTo)
	}
	if signoff.Kind != TaskKindHuman || signoff.Spec == nil ||
		!reflect.DeepEqual(signoff.Spec.Human, &HumanSpec{Timeout: "48h", OnTimeout: "reject"}) ||
		!reflect.DeepEqual(signoff.Spec.Gate, &StepGate{Contract: "release-ok"}) {
		t.Errorf("sign-off kind/spec = %v %+v", signoff.Kind, signoff.Spec)
	}
	if signoff.DueAt == nil || !signoff.DueAt.Equal(matNow.Add(48*time.Hour)) {
		t.Errorf("sign-off due = %v; want now+2d", signoff.DueAt)
	}
	if signoff.StepOrdinal != 3 {
		t.Errorf("sign-off ordinal = %d; want 3", signoff.StepOrdinal)
	}
}

func TestMaterialize_EmptySpecIsNil(t *testing.T) {
	f := newMatFixture()
	in := matInput([]RecipeStep{{ID: "only", Ordinal: 1}})
	in.Recipe = &Recipe{Name: "bare", Version: "0.1.0"}
	res := f.materialize(t, in)
	task := f.task(t, res.Created[0].TaskID)
	if task.Spec != nil {
		t.Errorf("Spec = %+v; want nil when every field is empty", task.Spec)
	}
	if task.Kind != TaskKindAgent {
		t.Errorf("Kind = %q; want agent", task.Kind)
	}
}

func TestMaterialize_InvalidDueNamesStep(t *testing.T) {
	f := newMatFixture()
	steps := matSteps()[:1]
	steps[0].Due = DueSpec{Raw: "someday"}
	_, err := f.m.Materialize(context.Background(), matInput(steps))
	if err == nil || !strings.Contains(err.Error(), "plan") || !strings.Contains(err.Error(), "someday") {
		t.Fatalf("err = %v; want the step and the raw due named", err)
	}
	if len(f.repo.Tasks) != 0 || len(f.runs.runs) != 0 {
		t.Errorf("tasks=%d runs=%d after a due error; want nothing written", len(f.repo.Tasks), len(f.runs.runs))
	}
}

func TestMaterialize_TracklessTasks(t *testing.T) {
	f := newMatFixture()
	in := matInput(matSteps())
	in.TrackID, in.ProjectID = "", ""
	res := f.materialize(t, in)
	for _, c := range res.Created {
		task := f.task(t, c.TaskID)
		if task.TrackID != nil || task.ProjectID != nil {
			t.Errorf("%s track/project = %v %v; want nil", c.StepID, task.TrackID, task.ProjectID)
		}
	}
	if run := f.run(t, res.RunID); run.TrackID != "" || run.ProjectID != "" {
		t.Errorf("run track/project = %q %q; want empty", run.TrackID, run.ProjectID)
	}
}

func TestMaterialize_PreCreateGateAborts(t *testing.T) {
	ctx := context.Background()
	gateOn := func(step string) func(*Task) error {
		return func(task *Task) error {
			if task.StepID == step {
				return errors.New("policy: exec tasks need approval")
			}
			return nil
		}
	}

	t.Run("first step rejected leaves nothing", func(t *testing.T) {
		f := newMatFixture()
		in := matInput(matSteps())
		in.PreCreate = gateOn("plan")
		_, err := f.m.Materialize(ctx, in)
		if err == nil || !strings.Contains(err.Error(), "plan") || !strings.Contains(err.Error(), "policy") {
			t.Fatalf("err = %v; want the step and the gate reason", err)
		}
		if len(f.repo.Tasks) != 0 || len(f.runs.runs) != 0 {
			t.Errorf("tasks=%d runs=%d; want nothing written", len(f.repo.Tasks), len(f.runs.runs))
		}
	})

	t.Run("later step rejected records the created prefix", func(t *testing.T) {
		f := newMatFixture()
		in := matInput(matSteps())
		in.PreCreate = gateOn("build")
		_, err := f.m.Materialize(ctx, in)
		if err == nil || !strings.Contains(err.Error(), "build") {
			t.Fatalf("err = %v; want the step named", err)
		}
		if len(f.repo.Tasks) != 1 {
			t.Fatalf("tasks = %d; want the gated prefix (plan) only", len(f.repo.Tasks))
		}
		if len(f.runs.runs) != 1 {
			t.Fatalf("runs = %d; want the run kept so reconcile can resume", len(f.runs.runs))
		}
		rows, _ := f.runs.ListRecipeRunTasksByTrack(ctx, matTrack)
		if len(rows) != 1 || rows[0].StepID != "plan" {
			t.Fatalf("ledger rows = %+v; want the plan row only", rows)
		}

		in.PreCreate = nil
		res := f.reconcile(t, in)
		assertSteps(t, "Created", res.Created, "build", "sign-off")
		assertSteps(t, "Skipped", res.Skipped, "plan")
	})
}

func TestMaterialize_SkipsExistingSteps(t *testing.T) {
	f := newMatFixture()
	in := matInput(matSteps())
	in.Existing = map[string]string{"plan": "task_existing"}
	res := f.materialize(t, in)
	assertSteps(t, "Created", res.Created, "build", "sign-off")
	assertSteps(t, "Skipped", res.Skipped, "plan")
	if res.Skipped[0].TaskID != "task_existing" || res.Skipped[0].Ordinal != 1 {
		t.Errorf("Skipped[0] = %+v; want the existing task id and ordinal 1", res.Skipped[0])
	}
	ids := stepTaskIDs(res)
	assertBlockedBy(t, f.task(t, ids["build"]), "task_existing")
	assertBlockedBy(t, f.task(t, ids["sign-off"]), "task_existing", ids["build"])
}

func TestReconcile_SkipsLedgerRows(t *testing.T) {
	f := newMatFixture()
	first := f.materialize(t, matInput(matSteps()[:2]))
	firstIDs := stepTaskIDs(first)
	before := *f.task(t, firstIDs["build"])

	second := f.reconcile(t, matInput(matSteps()))
	assertSteps(t, "Created", second.Created, "sign-off")
	assertSteps(t, "Skipped", second.Skipped, "plan", "build")
	if len(second.Warnings) != 0 {
		t.Errorf("Warnings = %v; want none", second.Warnings)
	}
	for _, s := range second.Skipped {
		if s.TaskID != firstIDs[s.StepID] {
			t.Errorf("Skipped %s = %q; want %q", s.StepID, s.TaskID, firstIDs[s.StepID])
		}
	}
	if second.RunID == first.RunID {
		t.Fatalf("reconcile reused run %s", first.RunID)
	}
	signoff := f.task(t, stepTaskIDs(second)["sign-off"])
	assertBlockedBy(t, signoff, firstIDs["plan"], firstIDs["build"])
	if signoff.RunID != second.RunID {
		t.Errorf("sign-off run = %q; want %q", signoff.RunID, second.RunID)
	}
	if run := f.run(t, second.RunID); run.ParentRun != first.RunID {
		t.Errorf("ParentRun = %q; want %q", run.ParentRun, first.RunID)
	}
	rows, _ := f.runs.ListRecipeRunTasks(context.Background(), second.RunID)
	if len(rows) != 1 || rows[0].StepID != "sign-off" {
		t.Errorf("second run rows = %+v; want sign-off only", rows)
	}
	if after := *f.task(t, firstIDs["build"]); !reflect.DeepEqual(before, after) {
		t.Errorf("existing task changed by reconcile:\n before %+v\n after  %+v", before, after)
	}
	if len(f.repo.Tasks) != 3 {
		t.Errorf("tasks = %d; want 3", len(f.repo.Tasks))
	}
}

func TestReconcile_DoesNotRecreateDeleted(t *testing.T) {
	f := newMatFixture()
	first := f.materialize(t, matInput(matSteps()[:2]))
	firstIDs := stepTaskIDs(first)
	if err := f.repo.DeleteTask(context.Background(), firstIDs["build"]); err != nil {
		t.Fatal(err)
	}

	res := f.reconcile(t, matInput(matSteps()))
	assertSteps(t, "Created", res.Created, "sign-off")
	assertSteps(t, "Skipped", res.Skipped, "plan", "build")
	if res.Skipped[1].TaskID != firstIDs["build"] {
		t.Errorf("Skipped build = %q; want the deleted task id %q", res.Skipped[1].TaskID, firstIDs["build"])
	}
	if len(f.repo.Tasks) != 2 {
		t.Errorf("tasks = %d; want plan and sign-off only (build not recreated)", len(f.repo.Tasks))
	}
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "build") || !strings.Contains(joined, firstIDs["build"]) {
		t.Errorf("Warnings = %v; want the deleted step and task named", res.Warnings)
	}
	signoff := f.task(t, stepTaskIDs(res)["sign-off"])
	assertBlockedBy(t, signoff, firstIDs["plan"])
	run := f.run(t, res.RunID)
	if !reflect.DeepEqual(run.DroppedDeps, []string{"sign-off:build", "sign-off:qa"}) {
		t.Errorf("DroppedDeps = %v; want the dep on the deleted step recorded", run.DroppedDeps)
	}
}

func TestReconcile_RecreateFlag(t *testing.T) {
	f := newMatFixture()
	first := f.materialize(t, matInput(matSteps()[:2]))
	firstIDs := stepTaskIDs(first)
	if err := f.repo.DeleteTask(context.Background(), firstIDs["build"]); err != nil {
		t.Fatal(err)
	}

	in := matInput(matSteps())
	in.Recreate = true
	res := f.reconcile(t, in)
	assertSteps(t, "Created", res.Created, "build", "sign-off")
	assertSteps(t, "Skipped", res.Skipped, "plan")
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v; want none", res.Warnings)
	}
	ids := stepTaskIDs(res)
	if ids["build"] == firstIDs["build"] {
		t.Errorf("build reused the deleted id %q", ids["build"])
	}
	assertBlockedBy(t, f.task(t, ids["build"]), firstIDs["plan"])
	assertBlockedBy(t, f.task(t, ids["sign-off"]), firstIDs["plan"], ids["build"])
	if len(f.repo.Tasks) != 3 {
		t.Errorf("tasks = %d; want 3", len(f.repo.Tasks))
	}

	// The ledger now holds build twice; the later run wins, so a further
	// reconcile sees the recreated task, not the deleted one.
	again := f.reconcile(t, matInput(matSteps()))
	assertSteps(t, "Created", again.Created)
	assertSteps(t, "Skipped", again.Skipped, "plan", "build", "sign-off")
	if again.Skipped[1].TaskID != ids["build"] || len(again.Warnings) != 0 {
		t.Errorf("second reconcile build = %q warnings = %v; want the recreated id %q and no warnings",
			again.Skipped[1].TaskID, again.Warnings, ids["build"])
	}
}

func TestMaterialize_DryRunNoWrites(t *testing.T) {
	ctx := context.Background()
	f := newMatFixture()
	in := matInput(matSteps())
	in.Existing = map[string]string{"plan": "task_existing"}
	in.DryRun = true
	res := f.materialize(t, in)

	assertSteps(t, "Created", res.Created, "build", "sign-off")
	assertSteps(t, "Skipped", res.Skipped, "plan")
	for _, c := range res.Created {
		if c.TaskID != "" {
			t.Errorf("dry-run Created %s has task id %q", c.StepID, c.TaskID)
		}
	}
	if len(f.repo.Tasks) != 0 || len(f.runs.runs) != 0 {
		t.Errorf("tasks=%d runs=%d; want nothing written", len(f.repo.Tasks), len(f.runs.runs))
	}
	if seq, _ := f.repo.GetNextSequenceID(ctx, matProject); seq != 1 {
		t.Errorf("next seq = %d; want 1 (dry run must not allocate)", seq)
	}

	bad := matInput(matSteps()[1:2])
	bad.DryRun = true
	if _, err := f.m.Materialize(ctx, bad); err == nil {
		t.Error("dry run swallowed an unmapped dependency")
	}
}

func TestReconcile_ParentRun(t *testing.T) {
	f := newMatFixture()
	r1 := f.materialize(t, matInput(matSteps()[:1]))
	r2 := f.reconcile(t, matInput(matSteps()[:2]))
	r3 := f.reconcile(t, matInput(matSteps()))
	if got := f.run(t, r1.RunID).ParentRun; got != "" {
		t.Errorf("first run ParentRun = %q; want empty", got)
	}
	if got := f.run(t, r2.RunID).ParentRun; got != r1.RunID {
		t.Errorf("second run ParentRun = %q; want %q", got, r1.RunID)
	}
	if got := f.run(t, r3.RunID).ParentRun; got != r2.RunID {
		t.Errorf("third run ParentRun = %q; want the latest run %q", got, r2.RunID)
	}

	explicit := matInput([]RecipeStep{{ID: "extra", Ordinal: 9}})
	explicit.ParentRun = "run_custom"
	if got := f.run(t, f.materialize(t, explicit).RunID).ParentRun; got != "run_custom" {
		t.Errorf("explicit ParentRun = %q; want run_custom", got)
	}

	trackless := matInput(matSteps())
	trackless.TrackID = ""
	if _, err := f.m.Reconcile(context.Background(), trackless); err == nil {
		t.Error("Reconcile without a track succeeded; want an error")
	}
}

func TestReconcile_HashDriftWarning(t *testing.T) {
	f := newMatFixture()
	f.materialize(t, matInput(matSteps()[:1]))

	same := f.reconcile(t, matInput(matSteps()[:2]))
	if len(same.Warnings) != 0 {
		t.Errorf("Warnings = %v; want none for the same version and hash", same.Warnings)
	}

	hashed := matInput(matSteps())
	hashed.Recipe.Hash = "sha256:def"
	res := f.reconcile(t, hashed)
	joined := strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "sha256:abc") || !strings.Contains(joined, "sha256:def") {
		t.Errorf("Warnings = %v; want the recorded and current hash", res.Warnings)
	}

	bumped := matInput(append(matSteps(), RecipeStep{ID: "announce", Ordinal: 4}))
	bumped.Recipe.Version = "1.3.0"
	res = f.reconcile(t, bumped)
	joined = strings.Join(res.Warnings, "\n")
	if !strings.Contains(joined, "1.2.0") || !strings.Contains(joined, "1.3.0") {
		t.Errorf("Warnings = %v; want the recorded and current version", res.Warnings)
	}
}

func TestReconcile_ScopedToRecipe(t *testing.T) {
	f := newMatFixture()
	other := matInput([]RecipeStep{{ID: "plan", Title: "Other plan", Ordinal: 1}})
	other.Recipe = &Recipe{Name: "other", Version: "9.0.0", Hash: "sha256:other"}
	f.materialize(t, other)

	res := f.reconcile(t, matInput(matSteps()[:1]))
	assertSteps(t, "Created", res.Created, "plan")
	assertSteps(t, "Skipped", res.Skipped)
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v; want none (another recipe's run is not drift)", res.Warnings)
	}
	if got := f.run(t, res.RunID).ParentRun; got != "" {
		t.Errorf("ParentRun = %q; want empty (another recipe's run is not a parent)", got)
	}
}

func TestMaterialize_UnmappedDependencyIsError(t *testing.T) {
	ctx := context.Background()
	f := newMatFixture()
	_, err := f.m.Materialize(ctx, matInput(matSteps()[1:2]))
	if err == nil || !strings.Contains(err.Error(), "build") || !strings.Contains(err.Error(), `"plan"`) {
		t.Fatalf("err = %v; want the step and its missing dependency named", err)
	}
	if len(f.repo.Tasks) != 0 || len(f.runs.runs) != 0 {
		t.Errorf("tasks=%d runs=%d; want nothing written", len(f.repo.Tasks), len(f.runs.runs))
	}
	if seq, _ := f.repo.GetNextSequenceID(ctx, matProject); seq != 1 {
		t.Errorf("next seq = %d; want 1 (validation runs before allocation)", seq)
	}

	in := matInput(matSteps()[1:2])
	in.Existing = map[string]string{"plan": "task_existing"}
	res := f.materialize(t, in)
	assertSteps(t, "Created", res.Created, "build")
	assertSteps(t, "Skipped", res.Skipped)
	assertBlockedBy(t, f.task(t, res.Created[0].TaskID), "task_existing")
}

func TestMaterialize_NilRecipe(t *testing.T) {
	f := newMatFixture()
	if _, err := f.m.Materialize(context.Background(), MaterializeInput{Steps: matSteps()}); err == nil {
		t.Error("nil recipe succeeded; want an error")
	}
}
