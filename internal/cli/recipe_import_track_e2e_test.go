package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

const e2eRecipeTrackSlug = "release-flow"

// e2eRecipeSteps is a three-step chain with one literal the ledger var
// "pr" stands for.
func e2eRecipeSteps() []core.RecipeStep {
	return []core.RecipeStep{
		{ID: "plan", Title: "Plan release 1234", Description: "Write the plan.", Ordinal: 1},
		{
			ID: "build", Title: "Build", Description: "Run the build.", Ordinal: 2, DependsOn: []string{"plan"},
			Kind: core.TaskKindExec, Exec: &core.ExecSpec{Argv: []string{"make", "build"}},
		},
		{ID: "sign-off", Title: "Sign off", Description: "Approve.", Ordinal: 3, DependsOn: []string{"build"}, Kind: core.TaskKindHuman},
	}
}

// seedMaterializedTrack creates the e2e track and materializes steps of
// rec into it through the ledger.
func seedMaterializedTrack(
	t *testing.T, ctx context.Context, s *storage.SQLiteStorage, rec *core.Recipe, steps []core.RecipeStep, vars map[string]any,
) (*core.Track, *core.MaterializeResult) {
	t.Helper()
	track := &core.Track{Slug: e2eRecipeTrackSlug, Title: "Release flow", Type: "feature"}
	seedTrack(t, ctx, s, s, track)
	return track, materializeInto(t, ctx, s, track, rec, steps, vars)
}

func materializeInto(
	t *testing.T, ctx context.Context, s *storage.SQLiteStorage, track *core.Track,
	rec *core.Recipe, steps []core.RecipeStep, vars map[string]any,
) *core.MaterializeResult {
	t.Helper()
	var projectID string
	if track.ProjectID != nil {
		projectID = *track.ProjectID
	}
	m := &core.Materializer{Repo: s, IDGen: s, Runs: s}
	res, err := m.Materialize(ctx, core.MaterializeInput{
		Recipe: rec, Steps: steps, TrackID: track.ID, ProjectID: projectID, By: "test", Vars: vars,
	})
	if err != nil {
		t.Fatalf("Materialize %s: %v", rec.Name, err)
	}
	return res
}

func parseRecipeFile(t *testing.T, path string) *core.Recipe {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	rec, err := core.ParseRecipe(bytes.NewReader(data), path)
	if err != nil {
		t.Fatalf("parse %s:\n%s\n%v", path, data, err)
	}
	return rec
}

func TestRecipeImport_E2E_CapturesTrack(t *testing.T) {
	withTestLock(func() {
		tmp := filepath.Dir(resetTestDB(t))
		ctx := context.Background()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer func() { _ = s.Close() }()
		rec := &core.Recipe{Name: "release", Version: "1.0.0", Hash: "sha256:seed"}
		seedMaterializedTrack(t, ctx, s, rec, e2eRecipeSteps(), map[string]any{"pr": "1234"})
		planDir := filepath.Join(tmp, ".tlc", "tracks", e2eRecipeTrackSlug)
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		plan := "---\ntitle: Release flow\n---\n\n# Release flow\n\nShip it.\n"
		if err := os.WriteFile(filepath.Join(planDir, "plan.md"), []byte(plan), 0o644); err != nil {
			t.Fatalf("write plan: %v", err)
		}

		// Default destination: <name>.yaml in the working directory.
		out, err := runRecipeCmd(t, "import", e2eRecipeTrackSlug)
		if err != nil {
			t.Fatalf("recipe import: %v\n%s", err, out)
		}
		dest := filepath.Join(tmp, e2eRecipeTrackSlug+".yaml")
		if !contains(out, "Wrote recipe release-flow@0.1.0 (3 steps) to "+e2eRecipeTrackSlug+".yaml") {
			t.Errorf("output = %q; want the destination named", out)
		}
		got := parseRecipeFile(t, dest)
		if got.Name != e2eRecipeTrackSlug || got.Version != "0.1.0" {
			t.Errorf("header = %s@%s", got.Name, got.Version)
		}
		if ids := stepIDList(got.Steps); strings.Join(ids, ",") != "plan,build,sign-off" {
			t.Errorf("steps = %v", ids)
		}
		if got.Steps[0].Title != "Plan release {{pr}}" || !got.Vars["pr"].Required {
			t.Errorf("ledger var not lifted: title=%q vars=%+v", got.Steps[0].Title, got.Vars)
		}
		if got.Track == nil || got.Track.Title != "Release flow" || got.Track.Plan != "# Release flow\n\nShip it." {
			t.Errorf("track = %+v; want title and the plan body without frontmatter", got.Track)
		}
		if contains(out, "warning:") {
			t.Errorf("unexpected warnings:\n%s", out)
		}

		// A second run refuses to clobber the file unless forced.
		out, err = runRecipeCmd(t, "import", e2eRecipeTrackSlug)
		if err == nil || !contains(err.Error(), "already exists") || !contains(err.Error(), "--force") {
			t.Errorf("overwrite: err = %v\n%s", err, out)
		}
		if _, err := runRecipeCmd(t, "import", e2eRecipeTrackSlug, "--force"); err != nil {
			t.Errorf("--force: %v", err)
		}

		// Explicit -o, header overrides, reverse --var and stdout.
		out, err = runRecipeCmd(t, "import", e2eRecipeTrackSlug, "-o", "-", "--name", "copy", "--version", "2.0.0", "--var", "pr=1234")
		if err != nil {
			t.Fatalf("-o -: %v\n%s", err, out)
		}
		if !strings.HasPrefix(out, "recipe: copy\nversion: 2.0.0\n") || !contains(out, "{{pr}}") {
			t.Errorf("stdout = %q; want the document with the overrides", out)
		}

		// --install lands in the project's recipes dir.
		out, err = runRecipeCmd(t, "import", e2eRecipeTrackSlug, "--install")
		if err != nil {
			t.Fatalf("--install: %v\n%s", err, out)
		}
		installed := filepath.Join(tmp, ".tlc", "recipes", e2eRecipeTrackSlug+".yaml")
		if _, err := os.Stat(installed); err != nil {
			t.Errorf("installed file: %v\n%s", err, out)
		}
		if _, err := runRecipeCmd(t, "import", e2eRecipeTrackSlug, "--install", "-o", "x.yaml"); err == nil {
			t.Error("--install with -o: expected an error")
		}

		// Unknown track and unusable --var.
		if _, err := runRecipeCmd(t, "import", "nope-track", "-o", "-"); err == nil || !contains(err.Error(), "nope-track") {
			t.Errorf("unknown track: err = %v", err)
		}
		if _, err := runRecipeCmd(t, "import", e2eRecipeTrackSlug, "-o", "-", "--var", "broken"); err == nil {
			t.Error("bad --var: expected an error")
		}
	})
}

func TestRecipeImport_E2E_WarningsAndExternalDeps(t *testing.T) {
	withTestLock(func() {
		resetTestDB(t)
		ctx := context.Background()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer func() { _ = s.Close() }()
		track := &core.Track{Slug: e2eRecipeTrackSlug, Title: "Release flow", Type: "feature"}
		seedTrack(t, ctx, s, s, track)
		other := &core.Track{Slug: "other-track", Title: "Other", Type: "feature"}
		seedTrack(t, ctx, s, s, other)
		outside := createTrackTask(t, ctx, s, other, "Outside step", nil)
		createTrackTask(t, ctx, s, track, "Inside step", []string{outside.ID})

		out, err := runRecipeCmd(t, "import", e2eRecipeTrackSlug, "-o", "-")
		if err != nil {
			t.Fatalf("recipe import: %v\n%s", err, out)
		}
		if !contains(out, "warning:") || !contains(out, "dropped") || !contains(out, "description is empty") {
			t.Errorf("output lacks the dropped-edge and empty-description warnings:\n%s", out)
		}
		if contains(out, "depends_on") {
			t.Errorf("external edge kept:\n%s", out)
		}

		out, err = runRecipeCmd(t, "import", e2eRecipeTrackSlug, "-o", "-", "--external-deps")
		if err != nil {
			t.Fatalf("--external-deps: %v\n%s", err, out)
		}
		if !contains(out, "- id: outside-step") || !contains(out, "depends_on:\n      - outside-step") {
			t.Errorf("external task not captured:\n%s", out)
		}
	})
}

func TestRecipeImport_E2E_DryRunWritesNothing(t *testing.T) {
	withTestLock(func() {
		tmp := filepath.Dir(resetTestDB(t))
		ctx := context.Background()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer func() { _ = s.Close() }()
		rec := &core.Recipe{Name: "release", Version: "1.0.0"}
		seedMaterializedTrack(t, ctx, s, rec, e2eRecipeSteps(), nil)

		dest := filepath.Join(tmp, "dry.yaml")
		resetRecipeFlags()
		cmd := newTestCmd()
		cmd.PersistentFlags().Bool("dry-run", false, "preview only")
		cmd.AddCommand(RecipeCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"recipe", "import", e2eRecipeTrackSlug, "-o", dest, "--dry-run"})
		err = cmd.Execute()
		resetRecipeFlags()
		if err != nil {
			t.Fatalf("dry run: %v\n%s", err, buf.String())
		}
		if !contains(buf.String(), "recipe: "+e2eRecipeTrackSlug) || !contains(buf.String(), "dry-run") {
			t.Errorf("output = %q; want the document and a dry-run note", buf.String())
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Errorf("dry run wrote %s (stat err = %v)", dest, err)
		}
	})
}

func TestRecipeImport_E2E_URLNeedsModelKey(t *testing.T) {
	withTestLock(func() {
		resetTestDB(t)
		for _, name := range []string{"LLM_API_KEY", "OPENROUTER_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
			t.Setenv(name, "")
		}
		_, err := runRecipeCmd(t, "import", "https://example.test/procedure.md", "-o", "-")
		if err == nil || !contains(err.Error(), "LLM_API_KEY") {
			t.Errorf("err = %v; want the missing key named", err)
		}
		_, err = runRecipeCmd(t, "import", "https://example.test/procedure.md", "-o", "-", "--var", "x=1")
		if err == nil || !contains(err.Error(), "--var") {
			t.Errorf("err = %v; want --var rejected for URL imports", err)
		}
	})
}

// createTrackTask stores a hand-made task on track with the given
// blocked_by edges and an empty description.
func createTrackTask(
	t *testing.T, ctx context.Context, s *storage.SQLiteStorage, track *core.Track, title string, blockedBy []string,
) *core.Task {
	t.Helper()
	var projectID string
	if track.ProjectID != nil {
		projectID = *track.ProjectID
	}
	seq, err := s.GetNextSequenceID(ctx, projectID)
	if err != nil {
		t.Fatalf("seq: %v", err)
	}
	task := &core.Task{
		ID: core.NewTaskID(), Seq: int64(seq), Title: title, Status: core.StatusTodo,
		TrackID: &track.ID, ProjectID: &projectID,
	}
	task.SetBlockedBy(blockedBy)
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("create %s: %v", title, err)
	}
	return task
}

func stepIDList(steps []core.RecipeStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.ID
	}
	return out
}
