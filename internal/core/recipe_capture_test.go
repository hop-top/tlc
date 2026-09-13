package core

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

func capTrack() *Track {
	pid := matProject
	return &Track{ID: matTrack, Slug: "release", Title: "Release", Type: "feature", ProjectID: &pid}
}

// capSteps is a recipe-born track: every dispatch field a step carries,
// a results reference in a condition and a literal PR number that the
// ledger var "pr" stands for.
func capSteps() []RecipeStep {
	return []RecipeStep{
		{ID: "plan", Title: "Plan release 1234", Description: "Write the plan for 1234.", Ordinal: 1, Agent: "architect"},
		{
			ID: "build", Title: "Build", Description: "Run the build.", Ordinal: 2, DependsOn: []string{"plan"},
			Kind: TaskKindExec, Exec: &ExecSpec{Argv: []string{"make", "build"}, Env: map[string]string{"PR": "1234"}},
			Retry: &RetrySpec{MaxAttempts: 3}, Due: DueSpec{Raw: "2026-10-06"},
			When: "results.plan.exit_code == 0",
		},
		{
			ID: "sign-off", Title: "Sign off", Description: "Approve.", Ordinal: 3, DependsOn: []string{"plan", "build"},
			Kind: TaskKindHuman, Assignee: "@lead", Human: &HumanSpec{Timeout: "48h", OnTimeout: "reject"},
			Gate: &StepGate{Contract: "release-ok"}, Due: DueSpec{Raw: "+2d"},
		},
	}
}

func capInput(steps []RecipeStep) MaterializeInput {
	return MaterializeInput{
		Recipe: matRecipe(), Steps: steps, TrackID: matTrack, ProjectID: matProject, By: "jad",
		Vars: map[string]any{"pr": "1234"},
	}
}

// addTask stores a hand-made task on the capture track with the next
// sequence number; ID, track and project default to the fixture's.
func (f *matFixture) addTask(t *testing.T, task *Task) *Task {
	t.Helper()
	ctx := context.Background()
	seq, err := f.repo.GetNextSequenceID(ctx, matProject)
	if err != nil {
		t.Fatalf("seq: %v", err)
	}
	task.Seq = int64(seq)
	if task.ID == "" {
		task.ID = NewTaskID()
	}
	if task.TrackID == nil {
		task.TrackID = optString(matTrack)
	}
	if task.ProjectID == nil {
		task.ProjectID = optString(matProject)
	}
	if task.Status == "" {
		task.Status = StatusTodo
	}
	if err := f.repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create %s: %v", task.Title, err)
	}
	return task
}

func (f *matFixture) capture(t *testing.T, opts CaptureOpts) (*Recipe, []string) {
	t.Helper()
	rec, warnings, err := CaptureTrack(context.Background(), f.repo, f.runs, capTrack(), opts)
	if err != nil {
		t.Fatalf("CaptureTrack: %v", err)
	}
	return rec, warnings
}

func stepByID(t *testing.T, rec *Recipe, id string) *RecipeStep {
	t.Helper()
	for i := range rec.Steps {
		if rec.Steps[i].ID == id {
			return &rec.Steps[i]
		}
	}
	t.Fatalf("step %q not in %v", id, stepIDs(rec.Steps))
	return nil
}

func hasWarning(warnings []string, substrs ...string) bool {
	for _, w := range warnings {
		ok := true
		for _, s := range substrs {
			ok = ok && strings.Contains(w, s)
		}
		if ok {
			return true
		}
	}
	return false
}

func TestCaptureTrack_OrderFollowsBatches(t *testing.T) {
	f := newMatFixture()
	aID := NewTaskID()
	charlie := &Task{Title: "Charlie", Description: "c"}
	charlie.SetBlockedBy([]string{aID})
	f.addTask(t, charlie)                                          // seq 1, but blocked by alpha
	f.addTask(t, &Task{ID: aID, Title: "Alpha", Description: "a"}) // seq 2
	bravo := &Task{Title: "Bravo", Description: "b"}
	bravo.SetBlockedBy([]string{aID})
	f.addTask(t, bravo)                                   // seq 3
	f.addTask(t, &Task{Title: "Delta", Description: "d"}) // seq 4

	rec, _ := f.capture(t, CaptureOpts{})

	want := []string{"alpha", "delta", "charlie", "bravo"}
	if got := stepIDs(rec.Steps); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v; want batches then seq %v", got, want)
	}
	for i, s := range rec.Steps {
		if s.Ordinal != i+1 {
			t.Errorf("step %s ordinal = %d; want %d", s.ID, s.Ordinal, i+1)
		}
	}
	if deps := stepByID(t, rec, "charlie").DependsOn; !reflect.DeepEqual(deps, []string{"alpha"}) {
		t.Errorf("charlie depends_on = %v; want [alpha]", deps)
	}
	if deps := stepByID(t, rec, "bravo").DependsOn; !reflect.DeepEqual(deps, []string{"alpha"}) {
		t.Errorf("bravo depends_on = %v; want [alpha]", deps)
	}
	if rec.Name != "release" || rec.Version != "0.1.0" || rec.Track == nil || rec.Track.Title != "Release" || rec.Track.Type != "feature" {
		t.Errorf("header = %s@%s track=%+v; want the track slug, 0.1.0 and the track block", rec.Name, rec.Version, rec.Track)
	}
}

func TestCaptureTrack_IdsFromProvenance(t *testing.T) {
	f := newMatFixture()
	f.materialize(t, capInput(capSteps()))

	rec, _ := f.capture(t, CaptureOpts{})

	if got := stepIDs(rec.Steps); !reflect.DeepEqual(got, []string{"plan", "build", "sign-off"}) {
		t.Fatalf("ids = %v; want the provenance step ids in order", got)
	}
	plan := stepByID(t, rec, "plan")
	if plan.Agent != "architect" || plan.Kind != "" {
		t.Errorf("plan = %+v; want agent architect and the default kind", plan)
	}
	assertCapturedBuild(t, stepByID(t, rec, "build"))
	assertCapturedSignOff(t, stepByID(t, rec, "sign-off"))
	if rec.Agent != "" {
		t.Errorf("recipe agent = %q; want none when steps disagree", rec.Agent)
	}
}

func assertCapturedBuild(t *testing.T, build *RecipeStep) {
	t.Helper()
	if build.Kind != TaskKindExec || build.Exec == nil || !reflect.DeepEqual(build.Exec.Argv, []string{"make", "build"}) {
		t.Errorf("build exec = %+v; want kind exec with argv", build)
	}
	if build.Retry == nil || build.Retry.MaxAttempts != 3 || build.Due.Raw != "2026-10-06" || build.Agent != "planner" {
		t.Errorf("build = %+v; want retry 3, due 2026-10-06, recipe agent", build)
	}
	if !reflect.DeepEqual(build.DependsOn, []string{"plan"}) || build.When != "results.plan.exit_code == 0" {
		t.Errorf("build deps/when = %v %q", build.DependsOn, build.When)
	}
}

func assertCapturedSignOff(t *testing.T, signOff *RecipeStep) {
	t.Helper()
	if signOff.Kind != TaskKindHuman || signOff.Assignee != "@lead" || signOff.Human == nil || signOff.Human.OnTimeout != "reject" {
		t.Errorf("sign-off = %+v; want kind human, @lead and the human block", signOff)
	}
	if signOff.Gate == nil || signOff.Gate.Contract != "release-ok" || signOff.Due.Raw != "2026-09-16" {
		t.Errorf("sign-off gate/due = %+v %q; want release-ok and the resolved due date", signOff.Gate, signOff.Due.Raw)
	}
	if !reflect.DeepEqual(signOff.DependsOn, []string{"plan", "build"}) {
		t.Errorf("sign-off depends_on = %v", signOff.DependsOn)
	}
}

func TestCaptureTrack_SlugCollision(t *testing.T) {
	f := newMatFixture()
	f.addTask(t, &Task{Title: "Review", Description: "x"})
	f.addTask(t, &Task{Title: "Review!", Description: "y"})
	f.addTask(t, &Task{Title: "???"})

	rec, warnings := f.capture(t, CaptureOpts{})

	if got := stepIDs(rec.Steps); !reflect.DeepEqual(got, []string{"review", "review-2", "step"}) {
		t.Fatalf("ids = %v; want a uniquified slug and a fallback id", got)
	}
	if !hasWarning(warnings, "review-2", "review") {
		t.Errorf("warnings = %v; want the collision reported", warnings)
	}
	if !hasWarning(warnings, "step", "description is empty") {
		t.Errorf("warnings = %v; want the empty description reported", warnings)
	}
}

func TestCaptureTrack_DepsIntraTrackOnly(t *testing.T) {
	f := newMatFixture()
	outside := f.addTask(t, &Task{Title: "Outside", Description: "o", TrackID: optString("track_other")})
	inside := &Task{Title: "Inside", Description: "i"}
	inside.SetBlockedBy([]string{outside.ID})
	f.addTask(t, inside)

	rec, warnings := f.capture(t, CaptureOpts{})
	if got := stepIDs(rec.Steps); !reflect.DeepEqual(got, []string{"inside"}) {
		t.Fatalf("ids = %v; want only the track's task", got)
	}
	if deps := rec.Steps[0].DependsOn; len(deps) != 0 {
		t.Errorf("depends_on = %v; want the external edge dropped", deps)
	}
	if !hasWarning(warnings, outside.ID, "dropped") {
		t.Errorf("warnings = %v; want the dropped edge reported", warnings)
	}

	rec, warnings = f.capture(t, CaptureOpts{IncludeExternalDeps: true})
	if got := stepIDs(rec.Steps); !reflect.DeepEqual(got, []string{"outside", "inside"}) {
		t.Fatalf("ids = %v; want the external task pulled in first", got)
	}
	if deps := stepByID(t, rec, "inside").DependsOn; !reflect.DeepEqual(deps, []string{"outside"}) {
		t.Errorf("inside depends_on = %v; want [outside]", deps)
	}
	if !hasWarning(warnings, "outside", "outside the track") {
		t.Errorf("warnings = %v; want the pulled-in task reported", warnings)
	}
}

func TestCaptureTrack_ReverseSubstitution(t *testing.T) {
	f := newMatFixture()
	f.addTask(t, &Task{
		Title:       "Review PR 1234 for v1.2.3",
		Description: "Ship v1.2.3 to 1234-staging; see https://x.io/1234 and 12345",
	})

	rec, warnings := f.capture(t, CaptureOpts{Vars: map[string]string{
		"pr": "1234", "version": "v1.2.3", "short": "1.2.3", "p": "pr",
	}})

	s := rec.Steps[0]
	if s.Title != "Review PR {{pr}} for {{version}}" {
		t.Errorf("title = %q; want longest value first, whole words only", s.Title)
	}
	if want := "Ship {{version}} to {{pr}}-staging; see https://x.io/{{pr}} and 12345"; s.Description != want {
		t.Errorf("description = %q; want %q", s.Description, want)
	}
	if strings.Contains(s.Title+s.Description, "{{{{") {
		t.Errorf("a substituted placeholder was substituted again: %q", s.Title)
	}
	for _, name := range []string{"pr", "version", "short", "p"} {
		if def, ok := rec.Vars[name]; !ok || !def.Required || def.Default != nil {
			t.Errorf("vars[%s] = %+v, %v; want required with no default", name, def, ok)
		}
	}
	if !hasWarning(warnings, "var short", "does not appear") || !hasWarning(warnings, "var p", "does not appear") {
		t.Errorf("warnings = %v; want unused vars reported", warnings)
	}
	if !hasWarning(warnings, "12345", "looks like a value") {
		t.Errorf("warnings = %v; want the leftover literal reported", warnings)
	}
}

func TestCaptureTrack_LedgerVarsSeed(t *testing.T) {
	f := newMatFixture()
	f.materialize(t, capInput(capSteps()))

	rec, _ := f.capture(t, CaptureOpts{})
	plan := stepByID(t, rec, "plan")
	if plan.Title != "Plan release {{pr}}" || plan.Description != "Write the plan for {{pr}}." {
		t.Errorf("plan = %q / %q; want the ledger var substituted", plan.Title, plan.Description)
	}
	if env := stepByID(t, rec, "build").Exec.Env["PR"]; env != "{{pr}}" {
		t.Errorf("exec env PR = %q; want the ledger var substituted", env)
	}
	if def, ok := rec.Vars["pr"]; !ok || !def.Required {
		t.Errorf("vars = %+v; want pr declared required", rec.Vars)
	}

	rec, warnings := f.capture(t, CaptureOpts{Vars: map[string]string{"pr": "9999"}})
	if title := stepByID(t, rec, "plan").Title; title != "Plan release 1234" {
		t.Errorf("title = %q; want the explicit value to win over the ledger", title)
	}
	if !hasWarning(warnings, "var pr", "9999") || !hasWarning(warnings, "1234", "looks like a value") {
		t.Errorf("warnings = %v; want the unused var and the literal reported", warnings)
	}
}

func TestCaptureTrack_RoundTripRecipeBornTrack(t *testing.T) {
	f := newMatFixture()
	f.materialize(t, capInput(capSteps()))
	rec, _ := f.capture(t, CaptureOpts{Name: "release-copy", Version: "2.0.0", Plan: "# Plan\n\nShip it.\n"})

	data, err := MarshalRecipe(rec)
	if err != nil {
		t.Fatalf("MarshalRecipe: %v", err)
	}
	parsed, err := ParseRecipe(bytes.NewReader(data), "release-copy.yaml")
	if err != nil {
		t.Fatalf("ParseRecipe:\n%s\n%v", data, err)
	}
	if parsed.Name != "release-copy" || parsed.Version != "2.0.0" || parsed.Track == nil || parsed.Track.Plan != "# Plan\n\nShip it.\n" {
		t.Errorf("header = %s@%s track=%+v", parsed.Name, parsed.Version, parsed.Track)
	}
	for i, want := range capSteps() {
		got := parsed.Steps[i]
		if got.ID != want.ID || got.EffectiveKind() != want.EffectiveKind() || !reflect.DeepEqual(got.DependsOn, want.DependsOn) {
			t.Errorf("step %d = %s/%s/%v; want %s/%s/%v", i, got.ID, got.EffectiveKind(), got.DependsOn, want.ID, want.EffectiveKind(), want.DependsOn)
		}
		if got.Ordinal != i+1 {
			t.Errorf("step %s ordinal = %d", got.ID, got.Ordinal)
		}
	}
	assertRecipeYAMLShape(t, string(data))
}

// assertRecipeYAMLShape checks header key order, the absence of load-time
// fields and the scalar due form in a marshaled recipe.
func assertRecipeYAMLShape(t *testing.T, text string) {
	t.Helper()
	order := []string{"recipe:", "version:", "vars:", "track:", "steps:"}
	last := -1
	for _, key := range order {
		idx := strings.Index("\n"+text, "\n"+key)
		if idx < 0 || idx < last {
			t.Errorf("key %q at %d after %d; want order %v in:\n%s", key, idx, last, order, text)
		}
		last = idx
	}
	for _, absent := range []string{"ordinal", "path:", "source:", "hash:", "due:\n"} {
		if strings.Contains(text, absent) {
			t.Errorf("yaml carries %q:\n%s", absent, text)
		}
	}
	if !strings.Contains(text, "due: \"2026-10-06\"") && !strings.Contains(text, "due: 2026-10-06") {
		t.Errorf("yaml lacks the scalar due form:\n%s", text)
	}
}

func TestCaptureTrack_FlatExpansionsWarn(t *testing.T) {
	f := newMatFixture()
	f.materialize(t, capInput([]RecipeStep{
		{ID: "loop/1/review", Title: "Review 1", Description: "r1", Ordinal: 1},
		{
			ID: "loop/2/review", Title: "Review 2", Description: "r2", Ordinal: 2,
			DependsOn: []string{"loop/1/review"}, When: "results.loop/1/review.exit_code != 0",
		},
	}))

	rec, warnings := f.capture(t, CaptureOpts{})

	if got := stepIDs(rec.Steps); !reflect.DeepEqual(got, []string{"loop-1-review", "loop-2-review"}) {
		t.Fatalf("ids = %v; want expanded ids flattened", got)
	}
	second := stepByID(t, rec, "loop-2-review")
	if !reflect.DeepEqual(second.DependsOn, []string{"loop-1-review"}) || second.When != "results.loop-1-review.exit_code != 0" {
		t.Errorf("deps/when = %v %q; want references remapped", second.DependsOn, second.When)
	}
	if !hasWarning(warnings, "loop/1/review", "flattened") || !hasWarning(warnings, "loop/2/review", "flattened") {
		t.Errorf("warnings = %v; want each flattened id reported", warnings)
	}
}

func TestCaptureTrack_ValidatesOutput(t *testing.T) {
	ctx := context.Background()
	f := newMatFixture()
	f.addTask(t, &Task{Title: "Only", Description: "x"})

	_, _, err := CaptureTrack(ctx, f.repo, f.runs, capTrack(), CaptureOpts{Name: "Bad_Name"})
	if err == nil || !strings.Contains(err.Error(), "must match") {
		t.Errorf("bad name: err = %v; want the name rule", err)
	}

	f.addTask(t, &Task{Title: "Guarded", Description: "g", Spec: &TaskSpec{When: "results.ghost.exit_code == 0"}})
	_, _, err = CaptureTrack(ctx, f.repo, f.runs, capTrack(), CaptureOpts{})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Errorf("dangling when: err = %v; want the unknown step named", err)
	}

	_, _, err = CaptureTrack(ctx, newMatFixture().repo, nil, capTrack(), CaptureOpts{})
	if err == nil || !strings.Contains(err.Error(), "no tasks") {
		t.Errorf("empty track: err = %v; want a no-tasks error", err)
	}
}

func TestCaptureTrack_NoSourceIdentifiers(t *testing.T) {
	f := newMatFixture()
	res := f.materialize(t, capInput(capSteps()))
	rec, _ := f.capture(t, CaptureOpts{})

	data, err := MarshalRecipe(rec)
	if err != nil {
		t.Fatalf("MarshalRecipe: %v", err)
	}
	forbidden := make([]string, 0, 6+len(res.Created))
	forbidden = append(forbidden, matTrack, matProject, res.RunID, "T-000", matRecipe().Hash, "task_")
	for _, c := range res.Created {
		forbidden = append(forbidden, c.TaskID)
	}
	for _, s := range forbidden {
		if strings.Contains(string(data), s) {
			t.Errorf("yaml carries source identifier %q:\n%s", s, data)
		}
	}
}
