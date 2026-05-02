package storage

import (
	"context"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestLogEntry_TaskIDIsTypeID verifies LogEntry round-trips with a typeid
// as TaskID. Confirms storage layer is format-agnostic and durable identity
// is preserved across reads.
func TestLogEntry_TaskIDIsTypeID(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// AddLog re-detects the project from cwd, so use the detected ID for
	// the task or the WHERE clauses won't match.
	proj := core.DetectProject()
	var projPtr *string
	if proj != nil && proj.InProject && proj.ProjectID != "" {
		pid := proj.ProjectID
		projPtr = &pid
	}

	task := &core.Task{
		ID:        core.NewTaskID(),
		Title:     "audit me",
		Status:    core.StatusTodo,
		ProjectID: projPtr,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	entry := &core.LogEntry{
		TaskID:    task.ID,
		Timestamp: time.Now().UTC(),
		By:        "noor",
		Action:    "CREATED",
		Note:      "round-trip check",
	}
	if err := s.AddLog(ctx, entry); err != nil {
		t.Fatalf("AddLog: %v", err)
	}

	logs, err := s.GetLogs(ctx, task.ID, "asc")
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d logs; want 1", len(logs))
	}
	if logs[0].TaskID != task.ID {
		t.Errorf("logs[0].TaskID = %q; want %q (typeid)", logs[0].TaskID, task.ID)
	}
	if !core.IsTaskID(logs[0].TaskID) {
		t.Errorf("returned TaskID %q is not a valid TypeID", logs[0].TaskID)
	}
}
