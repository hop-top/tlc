package core

import (
	"context"
	"testing"
)

// T-0589: In track mode, StateUpdater.Update must be called with
// targetType="task" (not "track") for each individual task.
// StateUpdater.Update no-ops on non-"task" targetType, so passing
// "track" would silently skip the transition. This test verifies
// the task-level path is exercised.
func TestStateUpdater_TrackMode_UsesTaskTargetType(t *testing.T) {
	su, repo, _, _ := setupStateUpdater(t)
	ctx := context.Background()

	// Create two tasks as if linked to a track.
	tasks := []struct {
		id    string
		title string
	}{
		{"T-0100", "First task"},
		{"T-0101", "Second task"},
	}
	for _, tt := range tasks {
		_ = repo.CreateTask(ctx, &Task{
			ID:     tt.id,
			Title:  tt.title,
			Status: StatusInProgress,
		})
	}

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusSucceeded,
		Summary: "Done",
	}

	// Simulate what runAgentRun does in track mode: for each task
	// context, it calls Update with updateType="task" when the
	// original targetType is "track" and ac.TaskID is non-empty.
	for _, tt := range tasks {
		// The key assertion: targetType MUST be "task", not "track".
		// If "track" were passed, Update would no-op (line 90-92).
		err := su.Update(ctx, result, "task", tt.id, UpdateOpts{})
		if err != nil {
			t.Fatalf("Update(%s) failed: %v", tt.id, err)
		}
	}

	// Verify both tasks transitioned to DONE.
	for _, tt := range tasks {
		task, _ := repo.GetTask(ctx, tt.id)
		if task.Status != StatusDone {
			t.Errorf("task %s status = %q, want DONE", tt.id, task.Status)
		}
	}
}

// T-0589: Passing targetType="track" to Update is a no-op — verifies
// the guard clause that makes the task path essential.
func TestStateUpdater_TrackTargetType_NoOp(t *testing.T) {
	su, repo, _, _ := setupStateUpdater(t)
	ctx := context.Background()

	_ = repo.CreateTask(ctx, &Task{
		ID:     "T-0200",
		Title:  "Should not transition",
		Status: StatusInProgress,
	})

	result := &AgentResult{
		Version: 1,
		Status:  AgentStatusSucceeded,
		Summary: "Done",
	}

	// Calling with targetType="track" should be a no-op.
	err := su.Update(ctx, result, "track", "T-0200", UpdateOpts{})
	if err != nil {
		t.Fatalf("Update with track type should not error: %v", err)
	}

	// Task must remain IN_PROGRESS — not transitioned.
	task, _ := repo.GetTask(ctx, "T-0200")
	if task.Status != StatusInProgress {
		t.Errorf("task status = %q, want IN_PROGRESS (track type should no-op)",
			task.Status)
	}
}
