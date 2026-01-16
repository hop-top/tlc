package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/oss-tlc-cli/internal/core"
)

func TestSQLiteStorage_CRUD(t *testing.T) {
	dbPath := "test_crud.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	taskID := "T-0001"
	task := &core.Task{
		ID:         taskID,
		Title:      "Test Task",
		Status:     core.StatusTodo,
		Reference:  "docs/test.md",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
		Tags:       []string{"test", "dev"},
		Meta:       map[string]interface{}{"priority": "high"},
	}

	// Test Create
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Test Get
	got, err := s.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got == nil {
		t.Fatal("task not found")
	}
	if got.Title != task.Title {
		t.Errorf("expected title %s, got %s", task.Title, got.Title)
	}

	// Test Update
	got.Status = core.StatusInProgress
	got.UpdatedAt = time.Now().UTC()
	if err := s.UpdateTask(ctx, got); err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	got2, _ := s.GetTask(ctx, taskID)
	if got2.Status != core.StatusInProgress {
		t.Errorf("expected status %s, got %s", core.StatusInProgress, got2.Status)
	}

	// Test Logs
	logEntry := &core.LogEntry{
		TaskID:    taskID,
		Timestamp: time.Now().UTC(),
		By:        "test-agent",
		Action:    "CLAIMED",
		Note:      "Starting test",
	}
	if err := s.AddLog(ctx, logEntry); err != nil {
		t.Fatalf("failed to add log: %v", err)
	}

	logs, err := s.GetLogs(ctx, taskID)
	if err != nil {
		t.Fatalf("failed to get logs: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	}
}

func TestSQLiteStorage_Query(t *testing.T) {
	dbPath := "test_query.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	tasks := []core.Task{
		{ID: "T-1", Title: "Task 1", Status: core.StatusTodo, Reference: "ref"},
		{ID: "T-2", Title: "Task 2", Status: core.StatusInProgress, Reference: "ref"},
		{ID: "T-3", Title: "Special Task", Status: core.StatusDone, Reference: "ref"},
	}

	for i := range tasks {
		s.CreateTask(ctx, &tasks[i])
	}

	// Test filter by status
	q := core.Query{
		Filters: []core.FieldFilter{
			{Field: "status", Operator: core.OpEq, Value: core.StatusTodo},
		},
	}
	got, _ := s.ListTasks(ctx, q)
	if len(got) != 1 {
		t.Errorf("expected 1 task, got %d", len(got))
	}

	// Test search
	q = core.Query{Search: "Special"}
		got, _ = s.ListTasks(ctx, q)
		if len(got) != 1 {
			t.Errorf("expected 1 task, got %d", len(got))
		}
	
		// Test pagination
		q = core.Query{Limit: 1, Offset: 1}
		got, _ = s.ListTasks(ctx, q)
		if len(got) != 1 {
			t.Errorf("expected 1 task for pagination, got %d", len(got))
		}
	
		// Test Log Querying
		s.AddLog(ctx, &core.LogEntry{TaskID: "T-1", Action: "CREATED", By: "user", Timestamp: time.Now()})
		s.AddLog(ctx, &core.LogEntry{TaskID: "T-2", Action: "UPDATED", By: "user", Timestamp: time.Now()})
	
		logs, _ := s.ListLogs(ctx, core.LogQuery{Action: "CREATED"})
		if len(logs) != 1 {
			t.Errorf("expected 1 CREATED log, got %d", len(logs))
		}
}

func TestStorage_QueryORLogic(t *testing.T) {
	dbPath := "test_or_logic.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-1", Title: "Task 1", Status: core.StatusTodo, Reference: "ref"})
	s.CreateTask(ctx, &core.Task{ID: "T-2", Title: "Task 2", Status: core.StatusInProgress, Reference: "ref"})
	s.CreateTask(ctx, &core.Task{ID: "T-3", Title: "Task 3", Status: core.StatusDone, Reference: "ref"})

	// Query for status=TODO OR status=IN_PROGRESS
	q := core.Query{
		Filters: []core.FieldFilter{
			{Field: "status", Operator: core.OpEq, Value: core.StatusTodo},
			{Field: "status", Operator: core.OpEq, Value: core.StatusInProgress},
		},
	}
	
	got, err := s.ListTasks(ctx, q)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	
	if len(got) != 2 {
		t.Errorf("expected 2 tasks (TODO or IN_PROGRESS), got %d", len(got))
	}
	
	// Verify we didn't get T-3 (DONE)
	for _, tsk := range got {
		if tsk.ID == "T-3" {
			t.Errorf("unexpected task T-3 in results")
		}
	}
}

func TestSQLiteStorage_FlowRuns(t *testing.T) {
	dbPath := "test_flow_runs.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	run := &core.FlowRun{
		ID:        "run:1",
		FlowID:    "flow:test",
		Status:    core.FlowStatusRunning,
		StartedAt: time.Now().UTC(),
		Results:   map[string]any{"key": "value"},
	}

	// Test Create
	if err := s.CreateFlowRun(ctx, run); err != nil {
		t.Fatalf("CreateFlowRun failed: %v", err)
	}

	// Test Get
	got, err := s.GetFlowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetFlowRun failed: %v", err)
	}
	if got == nil {
		t.Fatal("flow run not found")
	}
	if got.FlowID != run.FlowID {
		t.Errorf("expected FlowID %s, got %s", run.FlowID, got.FlowID)
	}

	// Test Update
	got.Status = core.FlowStatusSucceeded
	now := time.Now().UTC()
	got.EndedAt = &now
	if err := s.UpdateFlowRun(ctx, got); err != nil {
		t.Fatalf("UpdateFlowRun failed: %v", err)
	}

	got2, _ := s.GetFlowRun(ctx, run.ID)
	if got2.Status != core.FlowStatusSucceeded {
		t.Errorf("expected status %s, got %s", core.FlowStatusSucceeded, got2.Status)
	}
}

	