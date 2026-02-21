package core

import (
	"context"
	"testing"
	"time"
)

const testTaskID1 = "T-1"

func TestLogEntry_Structure(t *testing.T) {
	entry := &LogEntry{
		TaskID: testTaskID1,
		Action: "CREATED",
	}

	if entry.TaskID != testTaskID1 {
		t.Errorf("expected TaskID %s, got %s", testTaskID1, entry.TaskID)
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
		ID:        testTaskID1,
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
