package core

import (
	"context"
	"testing"
	"time"
)

func TestLogEntry_Structure(t *testing.T) {
	now := time.Now().UTC()
	entry := &LogEntry{
		ID:        1,
		TaskID:    "T-1",
		Timestamp: now,
		By:        "user",
		Action:    "CREATED",
		Note:      "initial",
		Meta:      map[string]interface{}{"key": "value"},
	}

	if entry.TaskID != "T-1" {
		t.Errorf("expected TaskID T-1, got %s", entry.TaskID)
	}
	if entry.Action != "CREATED" {
		t.Errorf("expected Action CREATED, got %s", entry.Action)
	}
}

func TestTaskService_LoggingIntegration(t *testing.T) {
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	service := NewTaskService(repo, logRepo)
	ctx := context.Background()

	task := &Task{
		ID:        "T-1",
		Title:     "Test",
		Status:    StatusTodo,
		CreatedAt: time.Now(),
	}

	err := service.CreateTask(ctx, task, "user", "test note")
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	logs, _ := logRepo.ListLogs(ctx, LogQuery{})
	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	} else {
		if logs[0].Action != "CREATED" {
			t.Errorf("expected CREATED action, got %s", logs[0].Action)
		}
		if logs[0].Note != "test note" {
			t.Errorf("expected note 'test note', got %s", logs[0].Note)
		}
	}
}
