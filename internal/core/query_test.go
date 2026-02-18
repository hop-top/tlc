package core

import (
	"context"
	"testing"
)

func TestQuery_FilterByTag(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	task1 := &Task{
		ID:     "T-1",
		Title:  "Auth task",
		Status: StatusTodo,
		Tags:   []string{"auth", "security"},
	}
	task2 := &Task{
		ID:     "T-2",
		Title:  "Docs task",
		Status: StatusTodo,
		Tags:   []string{"docs"},
	}
	repo.CreateTask(ctx, task1)
	repo.CreateTask(ctx, task2)

	query := Query{
		Filters: []FieldFilter{
			{Field: "tags", Operator: OpContains, Value: "auth"},
		},
	}

	tasks, err := service.ListTasks(ctx, query)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("expected 1 task with 'auth' tag, got %d", len(tasks))
	}

	if len(tasks) > 0 && tasks[0].ID != "T-1" {
		t.Errorf("expected task T-1, got %s", tasks[0].ID)
	}
}

func TestQuery_FilterByAssignee(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	assignee1 := "engineer-1"
	assignee2 := "engineer-2"
	task1 := &Task{
		ID:         "T-1",
		Title:      "Task for engineer-1",
		Status:     StatusTodo,
		AssignedTo: &assignee1,
	}
	task2 := &Task{
		ID:         "T-2",
		Title:      "Task for engineer-2",
		Status:     StatusTodo,
		AssignedTo: &assignee2,
	}
	repo.CreateTask(ctx, task1)
	repo.CreateTask(ctx, task2)

	query := Query{
		Filters: []FieldFilter{
			{Field: "assigned_to", Operator: OpEq, Value: "engineer-1"},
		},
	}

	tasks, err := service.ListTasks(ctx, query)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("expected 1 task assigned to engineer-1, got %d", len(tasks))
	}

	if len(tasks) > 0 && tasks[0].ID != "T-1" {
		t.Errorf("expected task T-1, got %s", tasks[0].ID)
	}
}

func TestQuery_FilterByStatus(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	task1 := &Task{
		ID:     "T-1",
		Title:  "TODO task",
		Status: StatusTodo,
	}
	task2 := &Task{
		ID:     "T-2",
		Title:  "IN_PROGRESS task",
		Status: StatusInProgress,
	}
	repo.CreateTask(ctx, task1)
	repo.CreateTask(ctx, task2)

	query := Query{
		Filters: []FieldFilter{
			{Field: "status", Operator: OpEq, Value: StatusTodo},
		},
	}

	tasks, err := service.ListTasks(ctx, query)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("expected 1 task with TODO status, got %d", len(tasks))
	}

	if len(tasks) > 0 && tasks[0].ID != "T-1" {
		t.Errorf("expected task T-1, got %s", tasks[0].ID)
	}
}

func TestQuery_FilterUnassignedTasks(t *testing.T) {
	repo := &mockRepo{tasks: make(map[string]*Task)}
	service := NewTaskService(repo, repo)

	ctx := context.Background()
	assignee1 := "engineer-1"
	task1 := &Task{
		ID:         "T-1",
		Title:      "Unassigned task",
		Status:     StatusTodo,
		AssignedTo: nil,
	}
	task2 := &Task{
		ID:         "T-2",
		Title:      "Assigned task",
		Status:     StatusTodo,
		AssignedTo: &assignee1,
	}
	repo.CreateTask(ctx, task1)
	repo.CreateTask(ctx, task2)

	query := Query{
		Filters: []FieldFilter{
			{Field: "assigned_to", Operator: OpNotEq, Value: nil},
		},
	}

	tasks, err := service.ListTasks(ctx, query)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	if len(tasks) != 1 {
		t.Errorf("expected 1 assigned task, got %d", len(tasks))
	}

	if len(tasks) > 0 && tasks[0].ID != "T-2" {
		t.Errorf("expected task T-2, got %s", tasks[0].ID)
	}
}
