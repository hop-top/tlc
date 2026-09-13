package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// recipeReviewFixture is the recipe the create verbs are exercised
// against: a required var, a defaulted var, a track block and three steps
// of different kinds chained by depends_on.
const recipeReviewFixture = `recipe: review
version: 1.0.0
description: Review a change
vars:
  pr: {required: true, description: PR number}
  depth: {default: standard}
track:
  title: "Review PR {{pr}}"
  type: feature
  plan: |
    # Review {{pr}}
    Depth: {{depth}}
steps:
  - id: lint
    kind: exec
    exec: {argv: [sh, -c, "echo lint {{pr}}"]}
  - id: review
    title: "Review {{pr}} ({{depth}})"
    depends_on: [lint]
  - id: sign-off
    kind: human
    assignee: "@lead"
    human: {timeout: 48h}
    depends_on: [review]
`

// plantRecipe writes a recipe under <cwd>/recipes and points recipe.dir
// at that directory; setupTestDir has already moved cwd to a temp dir.
func plantRecipe(t *testing.T, name, body string) {
	t.Helper()
	writeRecipeFixture(t, "recipes", name, body)
	viper.Set("recipe.dir", "recipes")
}

// runRecipeCLI runs argv in-process against a fresh root that carries the
// task and track groups and mirrors kit's global --dry-run flag. Flag state
// is reset first so two invocations in one test do not accumulate repeated
// flags. Returns the combined output.
func runRecipeCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetTaskFlags()
	// Other suites call SetOut on these global leaves directly; a writer
	// left there shadows the root's and swallows the output.
	for _, leaf := range []*cobra.Command{TaskExecCmd, trackExecuteCmd, TaskCreateCmd, trackCreateCmd} {
		leaf.SetOut(nil)
		leaf.SetErr(nil)
	}
	cmd := newTestCmd()
	cmd.PersistentFlags().Bool("dry-run", false, "preview without writing")
	cmd.AddCommand(TaskCmd, TrackCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func openTestStorage(t *testing.T) *storage.SQLiteStorage {
	t.Helper()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func tasksOfTrack(t *testing.T, ctx context.Context, s *storage.SQLiteStorage, trackID string) []*core.Task {
	t.Helper()
	tasks, err := s.ListTasks(ctx, core.Query{
		Filters:         []core.FieldFilter{{Field: "track_id", Operator: core.OpEq, Value: trackID}},
		AllProjects:     true,
		IncludeArchived: true,
	})
	if err != nil {
		t.Fatalf("ListTasks(track %s): %v", trackID, err)
	}
	return tasks
}

func allTasks(t *testing.T, ctx context.Context, s *storage.SQLiteStorage) []*core.Task {
	t.Helper()
	tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true, IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	return tasks
}

func tasksByStep(tasks []*core.Task) map[string]*core.Task {
	out := make(map[string]*core.Task, len(tasks))
	for _, task := range tasks {
		out[task.StepID] = task
	}
	return out
}

func recipeRunsOf(t *testing.T, ctx context.Context, s *storage.SQLiteStorage, recipe, trackID string) []*core.RecipeRun {
	t.Helper()
	runs, err := s.ListRecipeRuns(ctx, core.RecipeRunQuery{RecipeID: recipe, TrackID: trackID, AllProjects: true})
	if err != nil {
		t.Fatalf("ListRecipeRuns: %v", err)
	}
	return runs
}

func mustTrack(t *testing.T, ctx context.Context, s *storage.SQLiteStorage, ref string) *core.Track {
	t.Helper()
	track, err := s.GetTrack(ctx, ref)
	if err != nil {
		t.Fatalf("GetTrack(%q): %v", ref, err)
	}
	if track == nil {
		t.Fatalf("track %q not found", ref)
	}
	return track
}

func assertContains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestTrackCreateRecipe_E2E_HappyPath(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)

		out, err := runRecipeCLI(t, "track", "create", "Review twelve", "--recipe", "review", "--var", "pr=12")
		if err != nil {
			t.Fatalf("track create --recipe: %v\n%s", err, out)
		}
		assertContains(t, out, "Created track review-twelve", "Materialized review@1.0.0 into L-", "created 3 task(s)",
			"lint  (exec)", "review  (agent)", "sign-off  (human)")

		s := openTestStorage(t)
		track := mustTrack(t, ctx, s, "review-twelve")
		if track.Type != core.TrackTypeFeature {
			t.Errorf("type = %q; want the recipe's feature", track.Type)
		}
		tasks := tasksOfTrack(t, ctx, s, track.ID)
		if len(tasks) != 3 {
			t.Fatalf("track has %d tasks; want 3", len(tasks))
		}
		byStep := tasksByStep(tasks)
		review, lint, sign := byStep["review"], byStep["lint"], byStep["sign-off"]
		if review == nil || lint == nil || sign == nil {
			t.Fatalf("steps missing: %v", byStep)
		}
		if got := review.BlockedBy(); len(got) != 1 || got[0] != lint.ID {
			t.Errorf("review blocked_by = %v; want [%s]", got, lint.ID)
		}
		if review.RunID == "" || review.RunID != lint.RunID {
			t.Errorf("run ids = %q / %q; want one shared run", review.RunID, lint.RunID)
		}
		if sign.Kind != core.TaskKindHuman || sign.AssignedTo == nil || *sign.AssignedTo != "lead" {
			t.Errorf("sign-off = kind %q assignee %v; want human/@lead", sign.Kind, sign.AssignedTo)
		}
		if prov := review.Provenance(); prov.Recipe != "review" || prov.RecipeVersion != "1.0.0" {
			t.Errorf("provenance = %+v", prov)
		}

		runs := recipeRunsOf(t, ctx, s, "review", track.ID)
		if len(runs) != 1 {
			t.Fatalf("runs = %d; want 1", len(runs))
		}
		if runs[0].ID != review.RunID || runs[0].Vars["pr"] != "12" || runs[0].Vars["depth"] != "standard" {
			t.Errorf("run = %+v; want id %s, pr=12, depth=standard", runs[0], review.RunID)
		}
		rows, err := s.ListRecipeRunTasks(ctx, runs[0].ID)
		if err != nil || len(rows) != 3 {
			t.Errorf("run tasks = %v (%v); want 3 rows", rows, err)
		}
	})
}

func TestTrackCreateRecipe_E2E_MissingRequiredVar(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)

		out, err := runRecipeCLI(t, "track", "create", "No vars", "--recipe", "review")
		if err == nil {
			t.Fatalf("expected an error; output:\n%s", out)
		}
		if !strings.Contains(err.Error(), "pr") || !strings.Contains(err.Error(), "--var pr=") {
			t.Errorf("err = %v; want the missing var and the --var fix", err)
		}
		s := openTestStorage(t)
		tracks, _ := s.ListTracks(ctx, core.TrackQuery{})
		if len(tracks) != 0 || len(allTasks(t, ctx, s)) != 0 {
			t.Errorf("tracks=%d tasks=%d; want nothing written", len(tracks), len(allTasks(t, ctx, s)))
		}
	})
}

func TestTrackCreateRecipe_E2E_VarSubstitution(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)

		out, err := runRecipeCLI(t, "track", "create", "Deep", "--recipe", "review", "--var", "pr=12,depth=deep")
		if err != nil {
			t.Fatalf("track create --recipe: %v\n%s", err, out)
		}
		s := openTestStorage(t)
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, mustTrack(t, ctx, s, "deep").ID))
		if got := byStep["review"].Title; got != "Review 12 (deep)" {
			t.Errorf("review title = %q; want vars rendered", got)
		}
		lint := byStep["lint"]
		if lint.Spec == nil || lint.Spec.Exec == nil || lint.Spec.Exec.Argv[2] != "echo lint 12" {
			t.Errorf("lint spec = %+v; want the var rendered into argv", lint.Spec)
		}
	})
}

func TestTrackCreateRecipe_E2E_TaskSelection(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)

		// Ordinals; the dependency on the unselected review is dropped.
		out, err := runRecipeCLI(t, "track", "create", "Partial", "--recipe", "review", "--var", "pr=1", "--task", "1,3")
		if err != nil {
			t.Fatalf("track create --task: %v\n%s", err, out)
		}
		assertContains(t, out, "created 2 task(s)", "Warnings:", "sign-off", "review", "dropped")
		s := openTestStorage(t)
		track := mustTrack(t, ctx, s, "partial")
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, track.ID))
		if len(byStep) != 2 || byStep["review"] != nil {
			t.Fatalf("steps = %v; want lint and sign-off only", byStep)
		}
		if got := byStep["sign-off"].BlockedBy(); len(got) != 0 {
			t.Errorf("sign-off blocked_by = %v; want the dropped edge gone", got)
		}
		runs := recipeRunsOf(t, ctx, s, "review", track.ID)
		if len(runs) != 1 || runs[0].Selection != "1,3" || len(runs[0].DroppedDeps) != 1 || runs[0].DroppedDeps[0] != "sign-off:review" {
			t.Errorf("run = %+v; want selection 1,3 and dropped sign-off:review", runs[0])
		}

		// Ids with --with-deps pull the closure in and drop nothing.
		out, err = runRecipeCLI(t, "track", "create", "Closure", "--recipe", "review", "--var", "pr=1", "--task", "review", "--with-deps")
		if err != nil {
			t.Fatalf("track create --with-deps: %v\n%s", err, out)
		}
		if strings.Contains(out, "Warnings:") {
			t.Errorf("unexpected warnings:\n%s", out)
		}
		byStep = tasksByStep(tasksOfTrack(t, ctx, s, mustTrack(t, ctx, s, "closure").ID))
		if len(byStep) != 2 || byStep["lint"] == nil || byStep["review"] == nil {
			t.Fatalf("steps = %v; want lint and review", byStep)
		}
		if got := byStep["review"].BlockedBy(); len(got) != 1 || got[0] != byStep["lint"].ID {
			t.Errorf("review blocked_by = %v; want lint kept through the closure", got)
		}
	})
}

func TestTrackCreateRecipe_E2E_DryRunWritesNothing(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)

		out, err := runRecipeCLI(t, "track", "create", "Preview", "--recipe", "review", "--var", "pr=7", "--dry-run")
		if err != nil {
			t.Fatalf("track create --dry-run: %v\n%s", err, out)
		}
		assertContains(t, out, "Dry run", "preview", "review@1.0.0", "3 task(s)", "lint", "sign-off")
		s := openTestStorage(t)
		tracks, _ := s.ListTracks(ctx, core.TrackQuery{})
		runs, _ := s.ListRecipeRuns(ctx, core.RecipeRunQuery{RecipeID: "review", AllProjects: true})
		if len(tracks) != 0 || len(allTasks(t, ctx, s)) != 0 || len(runs) != 0 {
			t.Errorf("tracks=%d tasks=%d runs=%d; want nothing written", len(tracks), len(allTasks(t, ctx, s)), len(runs))
		}
		if _, statErr := os.Stat(filepath.Join(".tlc", "tracks", "preview")); statErr == nil {
			t.Errorf("dry run scaffolded a track directory")
		}

		// The plain path honors the global flag too.
		out, err = runRecipeCLI(t, "track", "create", "Plain", "--type", "feature", "--dry-run")
		if err != nil {
			t.Fatalf("track create --dry-run: %v\n%s", err, out)
		}
		assertContains(t, out, "Dry run", "plain", "feature")
		if tracks, _ = s.ListTracks(ctx, core.TrackQuery{}); len(tracks) != 0 {
			t.Errorf("plain dry run wrote %d track(s)", len(tracks))
		}
	})
}

func TestTrackCreateRecipe_E2E_PlanWritten(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)

		out, err := runRecipeCLI(t, "track", "create", "Planned", "--recipe", "review", "--var", "pr=12")
		if err != nil {
			t.Fatalf("track create --recipe: %v\n%s", err, out)
		}
		plan, err := os.ReadFile(filepath.Join(".tlc", "tracks", "planned", "plan.md"))
		if err != nil {
			t.Fatalf("plan.md: %v\n%s", err, out)
		}
		assertContains(t, string(plan), "# Review 12", "Depth: standard")
		if strings.Contains(string(plan), "TODO: describe the goal") {
			t.Errorf("plan.md is the scaffold template, not the recipe's plan:\n%s", plan)
		}
	})
}

func TestTrackCreateRecipe_E2E_Assign(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)
		writeRecipeFixture(t, "assignees", "reviewer.yaml", `assignee_id: "assignee:reviewer:1.0"
name: Reviewer
version: "1.0"
description: Reviews changes
capabilities:
  task_types: [review]
`)
		viper.Set("recipe.assignees_dir", "assignees")

		out, err := runRecipeCLI(t, "track", "create", "Assigned", "--recipe", "review", "--var", "pr=3", "--assign")
		if err != nil {
			t.Fatalf("track create --assign: %v\n%s", err, out)
		}
		s := openTestStorage(t)
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, mustTrack(t, ctx, s, "assigned").ID))
		for _, id := range []string{"lint", "review"} {
			if got := byStep[id].AssignedTo; got == nil || *got != "assignee:reviewer:1.0" {
				t.Errorf("%s assignee = %v; want the engine's pick", id, got)
			}
		}
		if got := byStep["sign-off"].AssignedTo; got == nil || *got != "lead" {
			t.Errorf("sign-off assignee = %v; want the step's own @lead kept", got)
		}
	})
}

func TestTrackCreateRecipe_E2E_TrackTitleFromRecipe(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		plantRecipe(t, "review.yaml", recipeReviewFixture)
		plantRecipe(t, "bare.yaml", "recipe: bare\nversion: 1.0.0\nsteps:\n  - id: one\n")

		out, err := runRecipeCLI(t, "track", "create", "--recipe", "review", "--var", "pr=12")
		if err != nil {
			t.Fatalf("track create without title: %v\n%s", err, out)
		}
		s := openTestStorage(t)
		track := mustTrack(t, ctx, s, "review-pr-12")
		if track.Title != "Review PR 12" || track.Type != core.TrackTypeFeature {
			t.Errorf("track = %q/%q; want the rendered track.title and track.type", track.Title, track.Type)
		}

		// An explicit --type wins over the recipe's.
		if out, err = runRecipeCLI(t, "track", "create", "Bugged", "--recipe", "review", "--var", "pr=1", "--type", "bug"); err != nil {
			t.Fatalf("track create --type: %v\n%s", err, out)
		}
		if got := mustTrack(t, ctx, s, "bugged").Type; got != core.TrackTypeBug {
			t.Errorf("type = %q; want bug", got)
		}

		// No title anywhere is an error naming both ways to give one.
		_, err = runRecipeCLI(t, "track", "create", "--recipe", "bare")
		if err == nil || !strings.Contains(err.Error(), "title") || !strings.Contains(err.Error(), "track.title") {
			t.Errorf("err = %v; want a title error naming the argument and track.title", err)
		}
	})
}
