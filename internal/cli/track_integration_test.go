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

		// Create a track first.
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID:    "my-track",
			Title: "My Track",
			Type:  core.TrackTypeFeature,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("failed to create track: %v", err)
		}

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

		// Verify task has track_id set.
		task, err := s.GetTask(ctx, "T-0001")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if task == nil {
			t.Fatal("task not found after create")
		}
		if task.TrackID == nil || *task.TrackID != "my-track" {
			t.Errorf("expected track_id=my-track, got %v", task.TrackID)
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
		if !contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", err)
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
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID:    "filter-track",
			Title: "Filter Track",
			Type:  core.TrackTypeFeature,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("failed to create track: %v", err)
		}

		trackID := "filter-track"
		// Create tasks: one with track, one without.
		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "With track", Status: core.StatusTodo,
			TrackID: &trackID,
		})
		s.CreateTask(ctx, &core.Task{
			ID: "T-0002", Title: "No track", Status: core.StatusTodo,
		})

		// List with --track filter.
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
		svc := core.NewTrackService(s, s)
		for _, id := range []string{"track-a", "track-b"} {
			tr := &core.Track{
				ID: id, Title: id, Type: core.TrackTypeFeature,
			}
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("failed to create track %s: %v", id, err)
			}
		}

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
		if task.TrackID == nil || *task.TrackID != "track-a" {
			t.Errorf("expected track_id=track-a, got %v", task.TrackID)
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
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID:    "auto-track",
			Title: "Auto Track",
			Type:  core.TrackTypeFeature,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("failed to create track: %v", err)
		}

		// Verify track is pending.
		tr, _ := s.GetTrack(ctx, "auto-track")
		if tr.Status != core.TrackStatusPending {
			t.Fatalf("expected pending, got %s", tr.Status)
		}

		// Create a task linked to the track.
		trackID := "auto-track"
		s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Linked task", Status: core.StatusTodo,
			TrackID: &trackID,
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
		tr, _ = s.GetTrack(ctx, "auto-track")
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
		trackID := "active-track"
		track := &core.Track{
			ID: trackID, Title: "Active", Type: core.TrackTypeFeature,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		// Create a linked task, then activate the track.
		s.CreateTask(context.Background(), &core.Task{
			ID: "T-0001", Title: "t1", Status: core.StatusTodo,
			TrackID: &trackID,
		})
		if err := svc.UpdateTrack(ctx, trackID, func(t *core.Track) error {
			t.Status = core.TrackStatusActive
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
		tr, _ := s.GetTrack(ctx, trackID)
		if tr.Status != core.TrackStatusActive {
			t.Errorf("expected active, got %s", tr.Status)
		}
	})
}
