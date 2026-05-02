package cli

// End-to-end tests for task-track integration (story 072).
// Exercises: task create/update/list --track, auto-transition on claim,
// task show Track field.
//
// Note: Tasks now store the track's TypeID in TrackID (not the slug).
// Tests seed tracks via TrackService (which auto-mints a TypeID and
// promotes the slug into Track.Slug) and use the resulting Track.ID for
// downstream linkage and assertions.

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

		// Seed track (mutates ID to TypeID, sets Slug="browser-rendering").
		track := &core.Track{
			ID: "browser-rendering", Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

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

		task := getTaskByAlias(t, ctx, "T-0001")
		if task == nil {
			t.Fatal("task not found after create")
		}
		if task.TrackID == nil || *task.TrackID != trackTypeID {
			t.Errorf(
				"expected TrackID=%s, got %v",
				trackTypeID, task.TrackID,
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

		track := &core.Track{
			ID: "browser-rendering", Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

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
			*task.TrackID != trackTypeID {
			t.Errorf(
				"expected TrackID=%s, got %v",
				trackTypeID, task.TrackID,
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

		track := &core.Track{
			ID: "browser-rendering", Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Linked task",
			Status: core.StatusTodo, TrackID: &trackTypeID,
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

		track := &core.Track{
			ID: "browser-rendering", Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "With track",
			Status: core.StatusTodo, TrackID: &trackTypeID,
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

		track := &core.Track{
			ID: "pending-track", Title: "Pending Track",
			Type: core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

		// Verify track is pending.
		tr, _ := s.GetTrack(ctx, trackTypeID)
		if tr.Status != core.TrackStatusPending {
			t.Fatalf("expected pending, got %s", tr.Status)
		}

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Linked task",
			Status: core.StatusTodo, TrackID: &trackTypeID,
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
		tr, _ = s.GetTrack(ctx, trackTypeID)
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

		track := &core.Track{
			ID: "active-track", Title: "Active Track",
			Type: core.TrackTypeFeature,
		}
		svc := core.NewTrackService(s, s)
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}
		trackTypeID := track.ID

		// Seed a task and activate the track.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "First task",
			Status: core.StatusTodo, TrackID: &trackTypeID,
		})
		if err := svc.UpdateTrack(ctx, trackTypeID, func(tr *core.Track) error {
			tr.Status = core.TrackStatusActive
			return nil
		}); err != nil {
			t.Fatalf("activate track: %v", err)
		}

		// Create a second task and claim it.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "Second task",
			Status: core.StatusTodo, TrackID: &trackTypeID,
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
		tr, _ := s.GetTrack(ctx, trackTypeID)
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

		track := &core.Track{
			ID: "browser-rendering", Title: "Browser Rendering",
			Type: core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Parse HTML",
			Status: core.StatusTodo, TrackID: &trackTypeID,
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
