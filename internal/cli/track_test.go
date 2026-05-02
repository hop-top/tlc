package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestTrackCreate_DefaultSlug(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "create", "Browser rendering", "--type", "feature"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "browser-rendering") {
			t.Errorf("expected slug 'browser-rendering' in output, got: %s", output)
		}
		if !contains(output, "feature") {
			t.Errorf("expected type 'feature' in output, got: %s", output)
		}

		// Verify persisted.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		track, err := s.GetTrack(ctx, "browser-rendering")
		if err != nil {
			t.Fatalf("GetTrack: %v", err)
		}
		if track == nil {
			t.Fatal("track not found after create")
		}
		if track.Title != "Browser rendering" {
			t.Errorf("expected title 'Browser rendering', got %q", track.Title)
		}
		if track.Status != core.TrackStatusPending {
			t.Errorf("expected status pending, got %s", track.Status)
		}
	})
}

func TestTrackCreate_ExplicitID(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "My Feature",
			"--type", "bug",
			"--id", "custom-slug",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "custom-slug") {
			t.Errorf("expected 'custom-slug' in output, got: %s", output)
		}

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		track, err := s.GetTrack(ctx, "custom-slug")
		if err != nil {
			t.Fatalf("GetTrack: %v", err)
		}
		if track == nil {
			t.Fatal("track not found after create with explicit ID")
		}
		if track.Type != "bug" {
			t.Errorf("expected type 'bug', got %q", track.Type)
		}
	})
}

func TestTrackCreate_WithAssignee(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Refactor storage",
			"--type", "refactor",
			"--assigned-to", "@me",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create failed: %v", err)
		}

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		track, err := s.GetTrack(ctx, "refactor-storage")
		if err != nil {
			t.Fatalf("GetTrack: %v", err)
		}
		if track == nil {
			t.Fatal("track not found")
		}
		if track.AssignedTo == nil || *track.AssignedTo != "me" {
			t.Errorf("expected assignee 'me', got %v", track.AssignedTo)
		}
	})
}

func TestTrackCreate_InvalidType(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "create", "Bad type",
			"--type", "invalid",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for invalid type, got nil")
		}
		if !contains(err.Error(), "invalid") {
			t.Errorf("expected 'invalid' in error, got: %s", err.Error())
		}
	})
}

func TestTrackUpdate_Title(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		svc := core.NewTrackService(s, s)
		track := &core.Track{ID: "test-track", Title: "Old Title", Type: "feature"}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}
		s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "update", "test-track", "--title", "New Title"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update failed: %v", err)
		}

		s2, _ := getStorageRaw()
		defer s2.Close()
		updated, _ := s2.GetTrack(ctx, "test-track")
		if updated == nil {
			t.Fatal("track not found after update")
		}
		if updated.Title != "New Title" {
			t.Errorf("expected title 'New Title', got %q", updated.Title)
		}
	})
}

func TestTrackUpdate_Status(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}

		// Create an active track with a linked terminal task so we can
		// complete it.
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "active-track", Title: "Active", Type: "feature",
			Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}
		// Link a terminal task.
		tid := "active-track"
		task := &core.Task{
			ID: "T-0001", Title: "Done task", Status: core.StatusDone,
			TrackID: &tid,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "active-track", "--status", "completed",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update status failed: %v", err)
		}

		if !contains(buf.String(), "Updated track active-track") {
			t.Errorf("unexpected output: %s", buf.String())
		}
	})
}

func TestTrackUpdate_InvalidTransition(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "pending-track", Title: "Pending", Type: "feature",
			Status: core.TrackStatusPending,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}
		s.Close()

		// pending -> completed is not allowed
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "pending-track", "--status", "completed",
		})

		err = cmd.Execute()
		if err == nil {
			t.Fatal("expected error for invalid transition, got nil")
		}
		if !contains(err.Error(), "cannot transition") {
			t.Errorf("expected transition error, got: %s", err.Error())
		}
	})
}

func TestTrackArchive(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		// Create a completed track so archive is valid.
		track := &core.Track{
			ID: "done-track", Title: "Done", Type: "feature",
			Status: core.TrackStatusCompleted,
		}
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}
		s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "archive", "done-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track archive failed: %v", err)
		}

		if !contains(buf.String(), "Archived track done-track") {
			t.Errorf("unexpected output: %s", buf.String())
		}
	})
}

func TestTrackAbandon(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		track := &core.Track{
			ID: "active-track", Title: "Active", Type: "bug",
			Status: core.TrackStatusActive,
		}
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}
		s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "abandon", "active-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track abandon failed: %v", err)
		}

		if !contains(buf.String(), "Abandoned track active-track") {
			t.Errorf("unexpected output: %s", buf.String())
		}
	})
}

func TestTrackDelete(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		track := &core.Track{
			ID: "del-track", Title: "To Delete", Type: "refactor",
		}
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("CreateTrack: %v", err)
		}
		s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "delete", "del-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track delete failed: %v", err)
		}

		if !contains(buf.String(), "Deleted track del-track") {
			t.Errorf("unexpected output: %s", buf.String())
		}

		// Verify gone.
		s2, _ := getStorageRaw()
		defer s2.Close()
		got, _ := s2.GetTrack(ctx, "del-track")
		if got != nil {
			t.Error("track still exists after delete")
		}
	})
}

func TestTrackDelete_WithLinkedTasks(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		track := &core.Track{
			ID: "linked-track", Title: "Linked", Type: "feature",
		}
		tid := seedTrack(t, ctx, s, s, track)
		// Link a task by track TypeID.
		task := &core.Task{
			ID: "T-0001", Title: "Linked task", Status: core.StatusTodo,
			TrackID: &tid,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "delete", "linked-track"})

		err = cmd.Execute()
		if err == nil {
			t.Fatal("expected error deleting track with linked tasks, got nil")
		}
	})
}

func TestTrackCreate_ScaffoldsDirectory(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cwd, _ := os.Getwd()
		// resolveConfigDir derives .tlc/ from the flat .tlc.yaml config
		// that setupTestDir creates. The dir is created automatically if
		// it doesn't exist.
		tlcDir := filepath.Join(cwd, ".tlc")

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "create", "My Feature", "--type", "feature", "--id", "my-feature"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create failed: %v", err)
		}

		trackDir := filepath.Join(tlcDir, "tracks", "my-feature")

		// metadata.json
		metaPath := filepath.Join(trackDir, "metadata.json")
		if _, err := os.Stat(metaPath); err != nil {
			t.Errorf("metadata.json not created: %v", err)
		}

		// plan.md with frontmatter
		planPath := filepath.Join(trackDir, "plan.md")
		planData, err := os.ReadFile(planPath)
		if err != nil {
			t.Fatalf("plan.md not created: %v", err)
		}
		if !contains(string(planData), "tracks:") {
			t.Error("plan.md missing 'tracks:' frontmatter")
		}
		if !contains(string(planData), "tasks: []") {
			t.Error("plan.md missing 'tasks: []' frontmatter")
		}
		if !contains(string(planData), "my-feature") {
			t.Error("plan.md missing track ID")
		}

		// tracks/tracks.md registry
		registryPath := filepath.Join(tlcDir, "tracks", "tracks.md")
		registryData, err := os.ReadFile(registryPath)
		if err != nil {
			t.Fatalf("tracks.md not created: %v", err)
		}
		if !contains(string(registryData), "my-feature") {
			t.Error("tracks.md missing track entry")
		}

		// Next-step instructions in output
		output := buf.String()
		if !contains(output, "Scaffolded:") {
			t.Errorf("expected 'Scaffolded:' in output, got: %s", output)
		}
		if !contains(output, "[required]") {
			t.Errorf("expected '[required]' next steps in output, got: %s", output)
		}
		if !contains(output, "--add-plan") {
			t.Errorf("expected '--add-plan' instruction in output, got: %s", output)
		}
	})
}

func TestTrackCreate_PlanMDIdempotent(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cwd, _ := os.Getwd()
		tlcDir := filepath.Join(cwd, ".tlc")
		trackDir := filepath.Join(tlcDir, "tracks", "idem-test")
		if err := os.MkdirAll(trackDir, 0o755); err != nil {
			t.Fatal(err)
		}
		existingContent := "# pre-existing"
		planPath := filepath.Join(trackDir, "plan.md")
		if err := os.WriteFile(planPath, []byte(existingContent), 0o644); err != nil {
			t.Fatal(err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "create", "Idem Test", "--type", "bug", "--id", "idem-test"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track create failed: %v", err)
		}

		data, err := os.ReadFile(planPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != existingContent {
			t.Errorf("plan.md was overwritten; expected %q, got %q", existingContent, string(data))
		}
	})
}
