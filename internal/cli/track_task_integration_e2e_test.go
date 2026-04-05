package cli

// End-to-end tests for task-track integration (story 072).
// Exercises: task create/update/list --track, auto-transition on claim,
// task show Track field.

import (
	"bytes"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestTaskTrackIntegration_E2E_CreateWithTrack verifies that
// `task create --track <id>` sets TrackID on the created task.
func TestTaskTrackIntegration_E2E_CreateWithTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		// Seed track.
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: "browser-rendering", Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "create", "Parse HTML",
			"--track", "browser-rendering",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create --track: %v", err)
		}

		out := buf.String()
		if !contains(out, "Created task") {
			t.Errorf("expected 'Created task' in output; got:\n%s", out)
		}

		task, err := s.GetTask(ctx, "T-0001")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if task.TrackID == nil || *task.TrackID != "browser-rendering" {
			t.Errorf(
				"expected TrackID=browser-rendering, got %v",
				task.TrackID,
			)
		}
	})
}

// TestTaskTrackIntegration_E2E_UpdateTrackLink verifies that
// `task update --track <id>` links an existing task to a track.
func TestTaskTrackIntegration_E2E_UpdateTrackLink(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: "browser-rendering", Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Existing task",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--track", "browser-rendering",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --track: %v", err)
		}

		task, _ := s.GetTask(ctx, "T-0001")
		if task.TrackID == nil ||
			*task.TrackID != "browser-rendering" {
			t.Errorf(
				"expected TrackID=browser-rendering, got %v",
				task.TrackID,
			)
		}
	})
}

// TestTaskTrackIntegration_E2E_UnlinkTrack verifies that
// `task update --track -` unlinks a task from its track.
func TestTaskTrackIntegration_E2E_UnlinkTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		trackID := "browser-rendering"
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: trackID, Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Linked task",
			Status: core.StatusTodo, TrackID: &trackID,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001", "--track", "-",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --track -: %v", err)
		}

		task, _ := s.GetTask(ctx, "T-0001")
		if task.TrackID != nil {
			t.Errorf(
				"expected TrackID=nil after unlink, got %v",
				*task.TrackID,
			)
		}
	})
}

// TestTaskTrackIntegration_E2E_ListByTrack verifies that
// `task list --track <id>` filters to tasks with that TrackID.
func TestTaskTrackIntegration_E2E_ListByTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		trackID := "browser-rendering"
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: trackID, Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "With track",
			Status: core.StatusTodo, TrackID: &trackID,
		})
		s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "No track",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "list", "--track", "browser-rendering",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --track: %v", err)
		}

		out := buf.String()
		if !contains(out, "T-0001") {
			t.Errorf(
				"expected T-0001 in filtered output; got:\n%s", out,
			)
		}
		if contains(out, "T-0002") {
			t.Errorf(
				"T-0002 should not appear in filtered output; got:\n%s",
				out,
			)
		}
	})
}

// TestTaskTrackIntegration_E2E_InvalidTrack verifies that
// `task create --track <nonexistent>` returns an error.
func TestTaskTrackIntegration_E2E_InvalidTrack(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "create", "Bad track task",
			"--track", "nonexistent",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for nonexistent track")
		}
		if !contains(err.Error(), "does not exist") {
			t.Errorf(
				"expected 'does not exist' in error; got: %v", err,
			)
		}
	})
}

// TestTaskTrackIntegration_E2E_AutoTransition verifies that
// claiming a task linked to a pending track auto-transitions the
// track to active.
func TestTaskTrackIntegration_E2E_AutoTransition(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		trackID := "pending-track"
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: trackID, Title: "Pending Track",
			Type: core.TrackTypeFeature,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		// Verify track is pending.
		tr, _ := s.GetTrack(ctx, trackID)
		if tr.Status != core.TrackStatusPending {
			t.Fatalf("expected pending, got %s", tr.Status)
		}

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Linked task",
			Status: core.StatusTodo, TrackID: &trackID,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "claim", "T-0001"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task claim: %v", err)
		}

		// Track should now be active.
		tr, _ = s.GetTrack(ctx, trackID)
		if tr.Status != core.TrackStatusActive {
			t.Errorf(
				"expected track active after claim, got %s",
				tr.Status,
			)
		}
	})
}

// TestTaskTrackIntegration_E2E_AlreadyActive verifies that
// claiming a task on an already-active track is a no-op (no error,
// track stays active).
func TestTaskTrackIntegration_E2E_AlreadyActive(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		trackID := "active-track"
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: trackID, Title: "Active Track",
			Type: core.TrackTypeFeature,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		// Seed a task and activate the track.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "First task",
			Status: core.StatusTodo, TrackID: &trackID,
		})
		if err := svc.UpdateTrack(ctx, trackID, func(tr *core.Track) error {
			tr.Status = core.TrackStatusActive
			return nil
		}); err != nil {
			t.Fatalf("activate track: %v", err)
		}

		// Create a second task and claim it.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "Second task",
			Status: core.StatusTodo, TrackID: &trackID,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "claim", "T-0002"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task claim: %v", err)
		}

		// Track should still be active.
		tr, _ := s.GetTrack(ctx, trackID)
		if tr.Status != core.TrackStatusActive {
			t.Errorf("expected active, got %s", tr.Status)
		}
	})
}

// TestTaskTrackIntegration_E2E_ShowTrack verifies that
// `task show` displays the Track field for a linked task.
func TestTaskTrackIntegration_E2E_ShowTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		trackID := "browser-rendering"
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: trackID, Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Parse HTML",
			Status: core.StatusTodo, TrackID: &trackID,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show: %v", err)
		}

		out := buf.String()
		if !contains(out, "Track:") {
			t.Errorf("expected 'Track:' in show output; got:\n%s", out)
		}
		if !contains(out, "browser-rendering") {
			t.Errorf(
				"expected 'browser-rendering' in output; got:\n%s",
				out,
			)
		}
	})
}
