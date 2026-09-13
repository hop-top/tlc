package storage

import (
	"context"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

const (
	claimProject = "proj-a"
	claimActor   = "executor-1"
)

// claimTestTask builds a task in project claimProject.
func claimTestTask(id string, status core.TaskStatus, assignee string) *core.Task {
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	project := claimProject
	task := &core.Task{ID: id, Title: id, Status: status, CreatedAt: now, UpdatedAt: now, ProjectID: &project}
	if assignee != "" {
		task.AssignedTo = &assignee
	}
	return task
}

// TestClaimTask_CompareAndSet pins the cross-process claim the executor
// relies on: exactly one claimant wins a TODO task, the loser sees false
// rather than an error, and the winning row carries actor, claimed_at
// and the active status atomically.
func TestClaimTask_CompareAndSet(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 11, 0, 0, 0, time.UTC)

	if err := s.CreateTask(ctx, claimTestTask("task_free", core.StatusTodo, "")); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	won, err := s.ClaimTask(ctx, "task_free", claimProject, core.StatusTodo, core.StatusInProgress, claimActor, now)
	if err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}
	if !won {
		t.Fatal("first claim lost; want won")
	}
	got, err := s.GetTaskInProject(ctx, "task_free", claimProject)
	if err != nil || got == nil {
		t.Fatalf("GetTaskInProject: %v, %v", got, err)
	}
	assertClaimedBy(t, got, claimActor, now)

	// A second claimant loses: the status no longer matches and the row
	// is owned by someone else.
	won, err = s.ClaimTask(ctx, "task_free", claimProject, core.StatusTodo, core.StatusInProgress, "executor-2", now.Add(time.Second))
	if err != nil {
		t.Fatalf("second ClaimTask: %v", err)
	}
	if won {
		t.Error("second claimant won; want lost")
	}
	again, _ := s.GetTaskInProject(ctx, "task_free", claimProject)
	if again.AssignedTo == nil || *again.AssignedTo != claimActor {
		t.Errorf("owner changed by losing claim: %v", again.AssignedTo)
	}
}

// assertClaimedBy checks the row a winning claim leaves behind: active
// status, the actor as assignee, claimed_at and updated_at at the claim
// instant.
func assertClaimedBy(t *testing.T, got *core.Task, actor string, at time.Time) {
	t.Helper()
	if got.Status != core.StatusInProgress {
		t.Errorf("status = %q; want %q", got.Status, core.StatusInProgress)
	}
	if got.AssignedTo == nil || *got.AssignedTo != actor {
		t.Errorf("assigned_to = %v; want %s", got.AssignedTo, actor)
	}
	if got.ClaimedAt == nil || !got.ClaimedAt.Equal(at) {
		t.Errorf("claimed_at = %v; want %v", got.ClaimedAt, at)
	}
	if !got.UpdatedAt.Equal(at) {
		t.Errorf("updated_at = %v; want %v", got.UpdatedAt, at)
	}
}

// TestClaimTask_Guards covers the predicates that must refuse a claim:
// wrong starting status, a row owned by another actor (even in the
// starting status), the wrong project bucket, and a missing task.
func TestClaimTask_Guards(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 11, 0, 0, 0, time.UTC)

	for _, task := range []*core.Task{
		claimTestTask("task_done", core.StatusDone, ""),
		claimTestTask("task_owned", core.StatusTodo, "someone-else"),
		claimTestTask("task_mine", core.StatusTodo, claimActor),
	} {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	cases := []struct {
		name, id, project, actor string
		want                     bool
	}{
		{"wrong status", "task_done", claimProject, claimActor, false},
		{"owned by another actor", "task_owned", claimProject, claimActor, false},
		{"already mine", "task_mine", claimProject, claimActor, true},
		{"wrong project", "task_mine", "proj-b", claimActor, false},
		{"missing", "task_nope", claimProject, claimActor, false},
	}
	for _, tc := range cases {
		won, err := s.ClaimTask(ctx, tc.id, tc.project, core.StatusTodo, core.StatusInProgress, tc.actor, now)
		if err != nil {
			t.Fatalf("%s: ClaimTask: %v", tc.name, err)
		}
		if won != tc.want {
			t.Errorf("%s: won = %v; want %v", tc.name, won, tc.want)
		}
	}
}

// TestClaimTask_ImplementsInterface pins the narrow contract.
func TestClaimTask_ImplementsInterface(t *testing.T) {
	var _ core.TaskClaimer = (*SQLiteStorage)(nil)
}
