package core

import (
	"context"
	"testing"
	"time"
)

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		current TaskStatus
		next    TaskStatus
		wantErr bool
	}{
		{"TODO to IN_PROGRESS", StatusTodo, StatusInProgress, false},
		{"TODO to SKIPPED", StatusTodo, StatusSkipped, false},
		{"TODO to DONE", StatusTodo, StatusDone, true}, // Must go through IN_PROGRESS
		{"IN_PROGRESS to DONE", StatusInProgress, StatusDone, false},
		{"IN_PROGRESS to TODO", StatusInProgress, StatusTodo, false},
		{"DONE to TODO", StatusDone, StatusTodo, true},
		{"SKIPPED to IN_PROGRESS", StatusSkipped, StatusInProgress, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.current, tt.next)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTask_Transition(t *testing.T) {
	task := &Task{ID: "T-1", Status: StatusTodo}

	log, err := task.Transition(StatusInProgress, "user", "starting")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	if task.Status != StatusInProgress {
		t.Errorf("expected IN_PROGRESS, got %s", task.Status)
	}
	if log.Action != string(StatusInProgress) {
		t.Errorf("expected log action IN_PROGRESS, got %s", log.Action)
	}
}

func TestTaskService_ClaimTask(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	task := &Task{
		ID:        "T-1",
		Title:     "Test Task",
		Status:    StatusTodo,
		CreatedAt: time.Now(),
	}
	repo.CreateTask(ctx, task)

	assignee := "agent-1"
	err := service.ClaimTask(ctx, "T-1", assignee, "starting work")
	if err != nil {
		t.Fatalf("ClaimTask failed: %v", err)
	}

	claimedTask, _ := repo.GetTask(ctx, "T-1")
	if claimedTask == nil {
		t.Fatal("task not found after claim")
	}

	if claimedTask.AssignedTo == nil {
		t.Error("assignee is nil after claim, expected 'agent-1'")
	} else if *claimedTask.AssignedTo != assignee {
		t.Errorf("assignee is %s, expected %s", *claimedTask.AssignedTo, assignee)
	}

	if claimedTask.Status != StatusInProgress {
		t.Errorf("status is %s, expected IN_PROGRESS", claimedTask.Status)
	}

	if len(repo.logs) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(repo.logs))
	}
}

func TestTaskService_UnclaimTask(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	assignee := "agent-1"
	task := &Task{
		ID:         "T-1",
		Title:      "Test Task",
		Status:     StatusInProgress,
		AssignedTo: &assignee,
		CreatedAt:  time.Now(),
	}
	repo.CreateTask(ctx, task)

	err := service.UnclaimTask(ctx, "T-1", assignee, "releasing task")
	if err != nil {
		t.Fatalf("UnclaimTask failed: %v", err)
	}

	unclaimedTask, _ := repo.GetTask(ctx, "T-1")
	if unclaimedTask == nil {
		t.Fatal("task not found after unclaim")
	}

	if unclaimedTask.AssignedTo != nil {
		t.Errorf("assignee is %s, expected nil (cleared)", *unclaimedTask.AssignedTo)
	}

	if unclaimedTask.Status != StatusTodo {
		t.Errorf("status is %s, expected TODO", unclaimedTask.Status)
	}

	if len(repo.logs) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(repo.logs))
	}
}

func TestTaskService_ClaimExclusivity(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	task := &Task{
		ID:        "T-1",
		Title:     "Test Task",
		Status:    StatusInProgress,
		CreatedAt: time.Now(),
	}
	repo.CreateTask(ctx, task)

	assignee1 := "agent-1"
	err := service.ClaimTask(ctx, "T-1", assignee1, "first claim")
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}

	claimedTask, _ := repo.GetTask(ctx, "T-1")
	if *claimedTask.AssignedTo != assignee1 {
		t.Errorf("first claim assignee is %s, expected %s", *claimedTask.AssignedTo, assignee1)
	}

	assignee2 := "agent-2"
	err = service.ClaimTask(ctx, "T-1", assignee2, "second claim (should fail)")
	if err == nil {
		t.Error("expected error when claiming already claimed task, got nil")
	}

	reclaimedTask, _ := repo.GetTask(ctx, "T-1")
	if *reclaimedTask.AssignedTo != assignee1 {
		t.Errorf("assignee changed after failed re-claim, expected %s to still own it", assignee1)
	}
}

func TestTaskService_UpdateAssignee(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	assignee1 := "agent-1"
	task := &Task{
		ID:         "T-1",
		Title:      "Test Task",
		Status:     StatusInProgress,
		AssignedTo: &assignee1,
		CreatedAt:  time.Now(),
	}
	repo.CreateTask(ctx, task)

	assignee2 := "agent-2"
	task.AssignedTo = &assignee2
	err := service.UpdateTask(ctx, task, assignee1, "reassigning work")
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	updatedTask, _ := repo.GetTask(ctx, "T-1")
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	if updatedTask.AssignedTo == nil {
		t.Error("assignee is nil after update")
	} else if *updatedTask.AssignedTo != assignee2 {
		t.Errorf("assignee is %s, expected %s", *updatedTask.AssignedTo, assignee2)
	}
}
