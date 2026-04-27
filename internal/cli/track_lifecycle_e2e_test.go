package cli

// End-to-end tests for track creation and lifecycle management.
// Exercises the full CLI pipeline: flag parsing → service → storage → render.

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTrackLifecycle_E2E_CreateDefaultSlug verifies that creating a track
// without --id derives the slug from the title and sets status=pending.
func TestTrackLifecycle_E2E_CreateDefaultSlug(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Browser rendering", "--type", "feature",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create: %v", err)
		}

		out := buf.String()
		if !contains(out, "browser-rendering") {
			t.Errorf("expected slug 'browser-rendering' in output; got:\n%s", out)
		}
		if !contains(out, "pending") {
			t.Errorf("expected status 'pending' in output; got:\n%s", out)
		}
		if !contains(out, "feature") {
			t.Errorf("expected type 'feature' in output; got:\n%s", out)
		}

		// Verify in storage.
		s, _ := getStorageRaw()
		defer s.Close()
		tr, err := s.GetTrack(t.Context(), "browser-rendering")
		if err != nil {
			t.Fatalf("GetTrack: %v", err)
		}
		if tr == nil {
			t.Fatal("track not found in storage")
		}
		if tr.Status != core.TrackStatusPending {
			t.Errorf("expected pending, got %s", tr.Status)
		}
		if tr.Type != core.TrackTypeFeature {
			t.Errorf("expected feature, got %s", tr.Type)
		}
	})
}

// TestTrackLifecycle_E2E_CreateCustomID verifies creating a track with
// explicit --id and --assigned-to flags.
func TestTrackLifecycle_E2E_CreateCustomID(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Auth",
			"--type", "refactor",
			"--id", "auth-rewrite",
			"--assigned-to", "@me",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create: %v", err)
		}

		out := buf.String()
		if !contains(out, "auth-rewrite") {
			t.Errorf("expected ID 'auth-rewrite'; got:\n%s", out)
		}
		if !contains(out, "me") {
			t.Errorf("expected assignee 'me'; got:\n%s", out)
		}

		s, _ := getStorageRaw()
		defer s.Close()
		tr, err := s.GetTrack(t.Context(), "auth-rewrite")
		if err != nil {
			t.Fatalf("GetTrack: %v", err)
		}
		if tr == nil {
			t.Fatal("track not found")
		}
		if tr.Type != core.TrackTypeRefactor {
			t.Errorf("expected refactor, got %s", tr.Type)
		}
		if tr.AssignedTo == nil || *tr.AssignedTo != "me" {
			t.Errorf("expected assignee=me, got %v", tr.AssignedTo)
		}
	})
}

// TestTrackLifecycle_E2E_CreateInvalidType verifies that an invalid
// --type flag produces the expected error message.
func TestTrackLifecycle_E2E_CreateInvalidType(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "X", "--type", "invalid",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for invalid type")
		}
		if !contains(err.Error(), `track type "invalid" invalid`) {
			t.Errorf("unexpected error: %v", err)
		}
		if !contains(err.Error(), "valid types:") {
			t.Errorf("error should list valid types: %v", err)
		}
	})
}

// TestTrackLifecycle_E2E_UpdateTitle verifies updating a track's title.
func TestTrackLifecycle_E2E_UpdateTitle(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()

		// Seed an active track with a linked task so we can activate it.
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "browser-rendering", Title: "Browser rendering",
			Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("seed track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "browser-rendering",
			"--title", "Browser Engine",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update: %v", err)
		}

		tr, _ := s.GetTrack(ctx, "browser-rendering")
		if tr.Title != "Browser Engine" {
			t.Errorf("expected title='Browser Engine', got %q", tr.Title)
		}
	})
}

// TestTrackLifecycle_E2E_CompleteAllDone verifies completing a track
// when all linked tasks are in terminal states (DONE/SKIPPED).
func TestTrackLifecycle_E2E_CompleteAllDone(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()

		svc := core.NewTrackService(s, s)
		trackID := "complete-track"
		track := &core.Track{
			ID: trackID, Title: "Complete Me",
			Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("seed track: %v", err)
		}

		// Link terminal tasks.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Done task", Status: core.StatusDone,
			TrackID: &trackID,
		})
		s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "Skipped task", Status: core.StatusSkipped,
			TrackID: &trackID,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", trackID, "--status", "completed",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update --status completed: %v", err)
		}

		tr, _ := s.GetTrack(ctx, trackID)
		if tr.Status != core.TrackStatusCompleted {
			t.Errorf("expected completed, got %s", tr.Status)
		}
	})
}

// TestTrackLifecycle_E2E_CompleteOpenTasks verifies that completing a
// track with non-terminal tasks produces an error.
func TestTrackLifecycle_E2E_CompleteOpenTasks(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()

		svc := core.NewTrackService(s, s)
		trackID := "open-track"
		track := &core.Track{
			ID: trackID, Title: "Has open tasks",
			Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("seed track: %v", err)
		}

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Open task", Status: core.StatusTodo,
			TrackID: &trackID,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", trackID, "--status", "completed",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error completing track with open tasks")
		}
		if !contains(err.Error(), "non-terminal tasks") {
			t.Errorf("expected non-terminal error, got: %v", err)
		}
	})
}

// TestTrackLifecycle_E2E_Abandon verifies abandoning an active track.
func TestTrackLifecycle_E2E_Abandon(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()

		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "abandon-me", Title: "Abandon Me",
			Type: core.TrackTypeBug, Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("seed track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "abandon", "abandon-me"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track abandon: %v", err)
		}

		tr, _ := s.GetTrack(ctx, "abandon-me")
		if tr.Status != core.TrackStatusAbandoned {
			t.Errorf("expected abandoned, got %s", tr.Status)
		}
	})
}

// TestTrackLifecycle_E2E_Archive verifies archiving a completed track.
func TestTrackLifecycle_E2E_Archive(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()

		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "archive-me", Title: "Archive Me",
			Type: core.TrackTypeFeature, Status: core.TrackStatusCompleted,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("seed track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "archive", "archive-me"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track archive: %v", err)
		}

		tr, _ := s.GetTrack(ctx, "archive-me")
		if tr.Status != core.TrackStatusArchived {
			t.Errorf("expected archived, got %s", tr.Status)
		}
	})
}

// TestTrackLifecycle_E2E_DeleteLinked verifies that deleting a track
// with linked tasks produces an error.
func TestTrackLifecycle_E2E_DeleteLinked(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()

		svc := core.NewTrackService(s, s)
		trackID := "linked-track"
		track := &core.Track{
			ID: trackID, Title: "Has tasks",
			Type: core.TrackTypeFeature, Status: core.TrackStatusPending,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("seed track: %v", err)
		}

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Linked task", Status: core.StatusTodo,
			TrackID: &trackID,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "delete", trackID})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error deleting track with linked tasks")
		}
		// The storage layer returns an error when tasks reference the track.
		if !contains(err.Error(), "delete") && !contains(err.Error(), "task") {
			t.Errorf("expected delete/task error, got: %v", err)
		}
	})
}

// TestTrackLifecycle_E2E_DeleteUnlinked verifies successful deletion
// of a track with no linked tasks.
func TestTrackLifecycle_E2E_DeleteUnlinked(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, _ := getStorageRaw()
		defer s.Close()

		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "orphan-track", Title: "No tasks",
			Type: core.TrackTypeFeature, Status: core.TrackStatusPending,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("seed track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "delete", "orphan-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track delete: %v", err)
		}

		tr, _ := s.GetTrack(ctx, "orphan-track")
		if tr != nil {
			t.Error("track should be deleted but still exists")
		}
	})
}

// TestTrackLifecycle_E2E_InvalidID verifies that a too-short custom ID
// produces the expected validation error.
func TestTrackLifecycle_E2E_InvalidID(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "X", "--type", "feature", "--id", "a",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for short ID")
		}
		if !contains(err.Error(), "too short") {
			t.Errorf("expected 'too short' in error, got: %v", err)
		}
		if !contains(err.Error(), "min 3") {
			t.Errorf("expected 'min 3' in error, got: %v", err)
		}
	})
}

// TestTrackLifecycle_E2E_HopModeScaffoldPaths verifies that in hop mode
// (.hop/tlc/ config layout), scaffold output and registry references
// use the full ".hop/tlc/" prefix — not just "tlc/" — so users can
// follow the printed next-step commands and find the files.
//
// Regression for: https://github.com/hop-top/tlc/issues/T-0723
// Symptom: printed hints said "tlc/tracks/<id>/plan.md" instead of
// ".hop/tlc/tracks/<id>/plan.md", causing users to look in the wrong
// place (and sometimes manually create a top-level "tracks/" dir).
func TestTrackLifecycle_E2E_HopModeScaffoldPaths(t *testing.T) {
	withTestLock(func() {
		// Build a hop-shaped temp project: <tmp>/.hop/tlc/config.yaml.
		tmpDir, err := os.MkdirTemp("", "tlc-hop-test-*")
		if err != nil {
			t.Fatalf("mkdir temp: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		hopCfgDir := filepath.Join(tmpDir, ".hop", "tlc")
		if err := os.MkdirAll(hopCfgDir, 0o755); err != nil {
			t.Fatalf("mkdir .hop/tlc: %v", err)
		}
		dbPath := filepath.Join(hopCfgDir, "test.sqlite")
		todoPath := filepath.Join(hopCfgDir, "todo.txt")
		cfgPath := filepath.Join(hopCfgDir, "config.yaml")
		cfgYAML := "storage:\n  backend: sqlite\n  db_path: " + dbPath +
			"\ntask:\n  todo_file: " + todoPath + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}

		origDir, _ := os.Getwd()
		if err := os.Chdir(tmpDir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		t.Setenv("TLC_MODE", "hop")
		t.Cleanup(func() { _ = os.Chdir(origDir) })

		viper.Reset()
		viper.SetConfigFile(cfgPath)
		viper.Set("storage.backend", "sqlite")
		viper.Set("storage.db_path", dbPath)
		viper.Set("task.todo_file", todoPath)
		_ = viper.ReadInConfig()
		dbSyncOnce = sync.Once{}
		touchOnce = sync.Once{}
		core.ResetDetectionCache()
		resetTrackFlags()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Hop Demo",
			"--type", "feature", "--id", "hop-demo",
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create: %v", err)
		}

		out := buf.String()

		// 1) Printed scaffold path must use full .hop/tlc/ prefix.
		if !contains(out, ".hop/tlc/tracks/tracks.md") {
			t.Errorf(
				"expected printed registry path to include "+
					"'.hop/tlc/tracks/tracks.md'; got:\n%s", out,
			)
		}
		if !contains(out, ".hop/tlc/tracks/hop-demo/plan.md") {
			t.Errorf(
				"expected printed plan path to include "+
					"'.hop/tlc/tracks/hop-demo/plan.md'; got:\n%s", out,
			)
		}
		// Negative: must NOT print "tlc/tracks/..." without the .hop/
		// prefix (i.e., a 'tlc/tracks/' substring not preceded by '.hop/').
		// We check by ensuring no occurrence of "  tlc/tracks/" or
		// "Edit tlc/tracks/".
		if contains(out, "Edit tlc/tracks/") || contains(out, " tlc/tracks/tracks.md") {
			t.Errorf(
				"unexpected stripped 'tlc/tracks/' prefix in hint output; "+
					"should always include '.hop/' prefix:\n%s", out,
			)
		}

		// 2) Files must actually land at <tmp>/.hop/tlc/tracks/hop-demo/.
		wantTrackDir := filepath.Join(tmpDir, ".hop", "tlc", "tracks", "hop-demo")
		if _, err := os.Stat(filepath.Join(wantTrackDir, "metadata.json")); err != nil {
			t.Errorf("metadata.json not at %s: %v", wantTrackDir, err)
		}
		if _, err := os.Stat(filepath.Join(wantTrackDir, "plan.md")); err != nil {
			t.Errorf("plan.md not at %s: %v", wantTrackDir, err)
		}

		// 3) Files must NOT land at top-level <tmp>/tracks/.
		if _, err := os.Stat(filepath.Join(tmpDir, "tracks", "hop-demo")); err == nil {
			t.Errorf(
				"track scaffold leaked to top-level tracks/ dir at %s; "+
					"should be inside .hop/tlc/tracks/", tmpDir,
			)
		}
	})
}
