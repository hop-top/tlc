package cli

import (
	"bytes"
	"context"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestTaskCreateWithTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		// Create a track first. Service mints a TypeID and promotes the
		// supplied "ID" string into Track.Slug.
		track := &core.Track{
			ID:    "my-track",
			Title: "My Track",
			Type:  core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

		// Create a task linked to the track.
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "create", "Track-linked task", "--track", "my-track",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with --track failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Created task") {
			t.Errorf("expected 'Created task' in output, got: %s", output)
		}

		// Verify task has track_id set to the track's TypeID.
		task, err := s.GetTask(ctx, "T-0001")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if task == nil {
			t.Fatal("task not found after create")
		}
		if task.TrackID == nil || *task.TrackID != trackTypeID {
			t.Errorf("expected track_id=%s, got %v", trackTypeID, task.TrackID)
		}
	})
}

func TestTaskCreateWithInvalidTrack(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "create", "Bad track", "--track", "nonexistent",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for nonexistent track")
		}
		if !contains(err.Error(), "does not exist") {
			t.Errorf("expected 'does not exist' in error, got: %v", err)
		}
	})
}

func TestTaskListFilterByTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		// Create a track.
		track := &core.Track{
			ID:    "filter-track",
			Title: "Filter Track",
			Type:  core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

		// Create tasks: one with track (linked by TypeID), one without.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "With track", Status: core.StatusTodo,
			TrackID: &trackTypeID,
		})
		s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "No track", Status: core.StatusTodo,
		})

		// List with --track filter (slug accepted; resolved to TypeID).
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "list", "--track", "filter-track",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --track failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "T-0001") {
			t.Errorf("expected T-0001 in filtered output, got: %s", output)
		}
		if contains(output, "T-0002") {
			t.Errorf("T-0002 should not appear in track-filtered output, got: %s", output)
		}
	})
}

func TestTaskUpdateTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		// Create tracks.
		trackA := &core.Track{
			ID: "track-a", Title: "track-a", Type: core.TrackTypeFeature,
		}
		trackATypeID := seedTrack(t, ctx, s, s, trackA)
		seedTrack(t, ctx, s, s, &core.Track{
			ID: "track-b", Title: "track-b", Type: core.TrackTypeFeature,
		})

		// Create a task.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Update track test", Status: core.StatusTodo,
		})

		// Update: link to track-a.
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--track", "track-a"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --track track-a failed: %v", err)
		}

		task, _ := s.GetTask(ctx, "T-0001")
		if task.TrackID == nil || *task.TrackID != trackATypeID {
			t.Errorf("expected track_id=%s, got %v", trackATypeID, task.TrackID)
		}

		// Update: unlink with --track -.
		resetTaskFlags()
		cmd2 := newTestCmd()
		cmd2.AddCommand(TaskCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"task", "update", "T-0001", "--track", "-"})

		if err := cmd2.Execute(); err != nil {
			t.Fatalf("task update --track - failed: %v", err)
		}

		task, _ = s.GetTask(ctx, "T-0001")
		if task.TrackID != nil {
			t.Errorf("expected track_id=nil after unlink, got %v", *task.TrackID)
		}
	})
}

func TestAutoTransitionPendingToActive(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		// Create a pending track.
		track := &core.Track{
			ID:    "auto-track",
			Title: "Auto Track",
			Type:  core.TrackTypeFeature,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

		// Verify track is pending.
		tr, _ := s.GetTrack(ctx, trackTypeID)
		if tr.Status != core.TrackStatusPending {
			t.Fatalf("expected pending, got %s", tr.Status)
		}

		// Create a task linked to the track.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Linked task", Status: core.StatusTodo,
			TrackID: &trackTypeID,
		})

		// Claim the task — should auto-transition track to active.
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "claim", "T-0001"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task claim failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Claimed task T-0001") {
			t.Errorf("expected claim message, got: %s", output)
		}

		// Verify track is now active.
		tr, _ = s.GetTrack(ctx, trackTypeID)
		if tr.Status != core.TrackStatusActive {
			t.Errorf(
				"expected track status=active after claim, got %s",
				tr.Status,
			)
		}
	})
}

func TestAutoTransitionNoOpWhenTrackAlreadyActive(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		svc := core.NewTrackService(s, s)

		// Create a track and manually set to active.
		track := &core.Track{
			ID: "active-track", Title: "Active", Type: core.TrackTypeFeature,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}
		trackTypeID := track.ID

		// Create a linked task, then activate the track.
		s.CreateTask(context.Background(), &core.Task{
			ID: "T-0001", Title: "t1", Status: core.StatusTodo,
			TrackID: &trackTypeID,
		})
		if err := svc.UpdateTrack(ctx, trackTypeID, func(tt *core.Track) error {
			tt.Status = core.TrackStatusActive
			return nil
		}); err != nil {
			t.Fatalf("activate track: %v", err)
		}

		// Claiming should not error even though track is already active.
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "claim", "T-0001"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task claim failed: %v", err)
		}

		// Track should still be active (no error, no change).
		tr, _ := s.GetTrack(ctx, trackTypeID)
		if tr.Status != core.TrackStatusActive {
			t.Errorf("expected active, got %s", tr.Status)
		}
	})
}
