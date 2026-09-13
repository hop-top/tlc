package cli

import (
	"context"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

const recipePipeFixture = `recipe: pipe
version: 1.0.0
vars:
  name: {default: x}
steps:
  - id: a
    kind: exec
    exec: {argv: [sh, -c, "echo a {{name}}"]}
  - id: b
    kind: exec
    exec: {argv: [sh, -c, "echo b"]}
    depends_on: [a]
`

const recipePipeThreeFixture = recipePipeFixture + `  - id: c
    kind: exec
    exec: {argv: [sh, -c, "echo c {{name}}"]}
    depends_on: [b]
`

// seedPipeTrack materializes pipe@1.0.0 into a new track through the CLI
// and returns the track.
func seedPipeTrack(t *testing.T, ctx context.Context, s *storage.SQLiteStorage) *core.Track {
	t.Helper()
	plantRecipe(t, "pipe.yaml", recipePipeFixture)
	out, err := runRecipeCLI(t, "track", "create", "Pipe", "--recipe", "pipe", "--var", "name=y")
	if err != nil {
		t.Fatalf("seed track: %v\n%s", err, out)
	}
	return mustTrack(t, ctx, s, "pipe")
}

func TestTrackExecuteRecipe_E2E_CreatesMissingOnly(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		s := openTestStorage(t)
		track := seedPipeTrack(t, ctx, s)
		first := recipeRunsOf(t, ctx, s, "pipe", track.ID)
		plantRecipe(t, "pipe.yaml", recipePipeThreeFixture)

		out, err := runRecipeCLI(t, "track", "execute", "pipe", "--recipe", "pipe")
		if err != nil {
			t.Fatalf("track execute --recipe: %v\n%s", err, out)
		}
		assertContains(t, out, "Reconciled pipe@1.0.0 on L-", "created 1 task(s)", "c  (exec)", "Skipped (2)")

		byStep := tasksByStep(tasksOfTrack(t, ctx, s, track.ID))
		if len(byStep) != 3 || byStep["c"] == nil {
			t.Fatalf("steps = %v; want a, b and the new c", byStep)
		}
		if got := byStep["c"].BlockedBy(); len(got) != 1 || got[0] != byStep["b"].ID {
			t.Errorf("c blocked_by = %v; want the existing b", got)
		}
		if byStep["c"].Spec.Exec.Argv[2] != "echo c y" {
			t.Errorf("c argv = %v; want the first run's vars carried", byStep["c"].Spec.Exec.Argv)
		}
		runs := recipeRunsOf(t, ctx, s, "pipe", track.ID)
		if len(runs) != 2 || runs[0].ParentRun != first[0].ID || runs[0].Vars["name"] != "y" {
			t.Errorf("runs = %+v; want a second run parented to %s with name=y", runs, first[0].ID)
		}
		// Execution continued after the reconcile.
		for _, id := range []string{"a", "b", "c"} {
			if got := byStep[id].Status; got != core.StatusDone {
				t.Errorf("%s = %q; want DONE\n%s", id, got, out)
			}
		}
	})
}

func TestTrackExecuteRecipe_E2E_DeletedNotRecreated(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		s := openTestStorage(t)
		track := seedPipeTrack(t, ctx, s)
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, track.ID))
		if err := s.DeleteTask(ctx, byStep["b"].ID); err != nil {
			t.Fatalf("DeleteTask: %v", err)
		}

		out, err := runRecipeCLI(t, "track", "execute", "pipe", "--recipe", "pipe")
		if err != nil {
			t.Fatalf("track execute --recipe: %v\n%s", err, out)
		}
		assertContains(t, out, "created 0 task(s)", "has been deleted; not recreated", "--recreate")
		if got := len(tasksOfTrack(t, ctx, s, track.ID)); got != 1 {
			t.Errorf("track has %d tasks; want only a", got)
		}
		if runs := recipeRunsOf(t, ctx, s, "pipe", track.ID); len(runs) != 1 {
			t.Errorf("runs = %d; want no new run when nothing was created", len(runs))
		}
	})
}

func TestTrackExecuteRecipe_E2E_Recreate(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		s := openTestStorage(t)
		track := seedPipeTrack(t, ctx, s)
		byStep := tasksByStep(tasksOfTrack(t, ctx, s, track.ID))
		if err := s.DeleteTask(ctx, byStep["b"].ID); err != nil {
			t.Fatalf("DeleteTask: %v", err)
		}

		out, err := runRecipeCLI(t, "track", "execute", "pipe", "--recipe", "pipe", "--recreate")
		if err != nil {
			t.Fatalf("track execute --recreate: %v\n%s", err, out)
		}
		assertContains(t, out, "created 1 task(s)", "b  (exec)")
		again := tasksByStep(tasksOfTrack(t, ctx, s, track.ID))
		if len(again) != 2 || again["b"] == nil || again["b"].ID == byStep["b"].ID {
			t.Fatalf("steps = %v; want a fresh b", again)
		}
		if got := again["b"].BlockedBy(); len(got) != 1 || got[0] != again["a"].ID {
			t.Errorf("recreated b blocked_by = %v; want the existing a", got)
		}
		if strings.Contains(out, "not recreated") {
			t.Errorf("--recreate still warned:\n%s", out)
		}
	})
}

func TestTrackExecuteRecipe_E2E_DriftWarning(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		_ = withHome(t)
		s := openTestStorage(t)
		track := seedPipeTrack(t, ctx, s)
		plantRecipe(t, "pipe.yaml", strings.Replace(recipePipeFixture, "version: 1.0.0", "version: 1.1.0", 1))

		out, err := runRecipeCLI(t, "track", "execute", "pipe", "--recipe", "pipe", "--dry-run")
		if err != nil {
			t.Fatalf("track execute --dry-run: %v\n%s", err, out)
		}
		assertContains(t, out, "Dry run", "used version 1.0.0, now 1.1.0", "batch")
		if got := len(tasksOfTrack(t, ctx, s, track.ID)); got != 2 {
			t.Errorf("track has %d tasks; want the dry run to write nothing", got)
		}
		if runs := recipeRunsOf(t, ctx, s, "pipe", track.ID); len(runs) != 1 {
			t.Errorf("runs = %d; want the dry run to record nothing", len(runs))
		}
	})
}
