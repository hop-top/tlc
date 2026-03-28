package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestSQLiteStorage_CRUD tests basic Create, Read, Update operations for tasks
// Verifies task lifecycle: create, retrieve, update, and log entries.
func TestSQLiteStorage_CRUD(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "test_crud.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	taskID := "T-0001"
	task := &core.Task{
		ID:        taskID,
		Title:     "Test Task",
		Status:    core.StatusTodo,
		Reference: "docs/test.md",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Tags:      []string{"test", "dev"},
		Meta:      map[string]interface{}{"priority": "high"},
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

	logs, err := s.GetLogs(ctx, taskID, "desc")
	if err != nil {
		t.Fatalf("failed to get logs: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	}
}

// TestSQLiteStorage_GetTaskLogs tests the GetTaskLogs wrapper.
func TestSQLiteStorage_GetTaskLogs(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "test_gettasklogs.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	task := &core.Task{
		ID: "T-1", Title: "Test", Status: core.StatusTodo,
		Reference: "ref", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	s.AddLog(ctx, &core.LogEntry{TaskID: "T-1", Action: "CREATED", By: "user", Timestamp: now})
	s.AddLog(ctx, &core.LogEntry{TaskID: "T-1", Action: "CLAIMED", By: "user", Timestamp: now.Add(time.Second)})

	logs, err := s.GetTaskLogs(ctx, "T-1")
	if err != nil {
		t.Fatalf("GetTaskLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}
}

func TestUpdateTaskWithLog_GlobalTask(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldCwd) }()
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "global_task.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create task without project context (ProjectID = nil → stored as project_id='')
	task := &core.Task{
		ID:        "T-0001",
		Title:     "Global task",
		Status:    core.StatusTodo,
		Reference: "ref",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Read back to get the stored form (ProjectID = &"")
	stored, err := s.GetTask(ctx, "T-0001")
	if err != nil || stored == nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if stored.ProjectID == nil || *stored.ProjectID != "" {
		t.Fatalf("expected ProjectID='', got %v", stored.ProjectID)
	}

	// Simulate claim: transition to IN_PROGRESS via UpdateTaskWithLog
	stored.Status = core.StatusInProgress
	stored.UpdatedAt = time.Now().UTC()
	logEntry := &core.LogEntry{
		TaskID:    "T-0001",
		Timestamp: stored.UpdatedAt,
		By:        "testuser",
		Action:    "CLAIMED",
		Note:      "",
	}
	if err := s.UpdateTaskWithLog(ctx, stored, logEntry); err != nil {
		t.Fatalf("UpdateTaskWithLog failed: %v", err)
	}

	// Verify status persisted (not silently dropped)
	after, err := s.GetTask(ctx, "T-0001")
	if err != nil || after == nil {
		t.Fatalf("GetTask after update failed: %v", err)
	}
	if after.Status != core.StatusInProgress {
		t.Errorf("expected status IN_PROGRESS after claim, got %s — update was a no-op", after.Status)
	}
}

// TestUpdateTask_GlobalTask verifies UpdateTask (used by saveTaskWithLog) persists
// changes for tasks with project_id='' under an active project context.
//
// Regression: when claim runs in project context X, GetTask returns the project-X
// row. That row's UpdateTask call should succeed even if a project_id='' row also
// exists for the same task ID.
func TestUpdateTask_GlobalTask(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldCwd) }()
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "multi_project.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Simulate: task created globally (project_id='') then also exists in a project
	projectID := "hop-top/aps"
	s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "Task", Status: core.StatusTodo,
		Reference: "ref", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "Task", Status: core.StatusTodo, ProjectID: &projectID,
		Reference: "ref", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})

	// Read the project-scoped row (simulates GetTask in project context)
	projTask, err := s.GetTaskInProject(ctx, "T-0001", projectID)
	if err != nil || projTask == nil {
		t.Fatalf("GetTaskInProject failed: %v", err)
	}

	// Claim: update the project-scoped row
	projTask.Status = core.StatusInProgress
	projTask.UpdatedAt = time.Now().UTC()
	if err := s.UpdateTask(ctx, projTask); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	// Project row should be IN_PROGRESS
	after, err := s.GetTaskInProject(ctx, "T-0001", projectID)
	if err != nil || after == nil {
		t.Fatalf("GetTaskInProject after update failed: %v", err)
	}
	if after.Status != core.StatusInProgress {
		t.Errorf("expected status IN_PROGRESS, got %s — UpdateTask was a no-op", after.Status)
	}

	// Global '' row should be unaffected
	global, err := s.GetTaskInProject(ctx, "T-0001", "")
	if err != nil || global == nil {
		t.Fatalf("GetTaskInProject(global) failed: %v", err)
	}
	if global.Status != core.StatusTodo {
		t.Errorf("expected global row status TODO (unaffected), got %s", global.Status)
	}
}

// TestStaleFields_RoundTrip verifies that StaleTimeout, BlockedReason, and StaleFiredAt
// round-trip correctly through CreateTask / GetTask.
func TestStaleFields_RoundTrip(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldCwd) }()
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "stale_fields.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	timeout := 2 * time.Hour
	reason := "waiting on T-0001"
	firedAt := time.Now().UTC().Truncate(time.Second)

	task := &core.Task{
		ID:            "T-0099",
		Title:         "stale test",
		Status:        core.StatusInProgress,
		Reference:     "task://T-0099",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC().Add(-3 * time.Hour),
		StaleTimeout:  &timeout,
		BlockedReason: &reason,
		StaleFiredAt:  &firedAt,
	}

	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	got, err := s.GetTask(ctx, "T-0099")
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if got == nil {
		t.Fatal("task not found")
	}

	if got.StaleTimeout == nil {
		t.Fatal("StaleTimeout is nil, expected non-nil")
	}
	if *got.StaleTimeout != timeout {
		t.Fatalf("StaleTimeout mismatch: got %v, want %v", *got.StaleTimeout, timeout)
	}

	if got.BlockedReason == nil {
		t.Fatal("BlockedReason is nil, expected non-nil")
	}
	if *got.BlockedReason != reason {
		t.Fatalf("BlockedReason mismatch: got %q, want %q", *got.BlockedReason, reason)
	}

	if got.StaleFiredAt == nil {
		t.Fatal("StaleFiredAt is nil, expected non-nil")
	}
	if !got.StaleFiredAt.Equal(firedAt) {
		t.Fatalf("StaleFiredAt mismatch: got %v, want %v", *got.StaleFiredAt, firedAt)
	}
}
