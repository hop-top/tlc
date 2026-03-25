package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestSQLiteStorage_FlowRuns tests flow run CRUD operations
// Verifies flow run lifecycle: create, retrieve, update with status changes.
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

// TestSQLiteStorage_ChangeTracking tests sync status tracking
// Identifies tasks needing push: dirty (updated after sync) and never synced.
func TestSQLiteStorage_ChangeTracking(t *testing.T) {
	dbPath := "test_change_tracking.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	origin := "github"
	now := time.Now().UTC().Truncate(time.Second) // SQLite precision

	tasks := []*core.Task{
		{
			ID: "T-1", Title: "Local Only", Status: core.StatusTodo,
			UpdatedAt: now, OriginSystem: nil, LastSyncAt: nil, Reference: "ref",
		},
		{
			ID: "T-2", Title: "Synced & Clean", Status: core.StatusTodo,
			UpdatedAt: now, OriginSystem: &origin, LastSyncAt: &now, Reference: "ref",
		},
		{
			ID: "T-3", Title: "Synced & Dirty", Status: core.StatusTodo,
			UpdatedAt: now.Add(time.Minute), OriginSystem: &origin, LastSyncAt: &now, Reference: "ref",
		},
		{
			ID: "T-4", Title: "Never Synced", Status: core.StatusTodo,
			UpdatedAt: now, OriginSystem: &origin, LastSyncAt: nil, Reference: "ref",
		},
	}

	for _, tsk := range tasks {
		if err := s.CreateTask(ctx, tsk); err != nil {
			t.Fatalf("failed to create task %s: %v", tsk.ID, err)
		}
	}

	needingPush, err := s.GetTasksNeedingPush(ctx)
	if err != nil {
		t.Fatalf("GetTasksNeedingPush failed: %v", err)
	}

	if len(needingPush) != 2 {
		t.Errorf("expected 2 tasks needing push, got %d", len(needingPush))
	}

	found := make(map[string]bool)
	for _, tsk := range needingPush {
		found[tsk.ID] = true
	}

	if !found["T-3"] || !found["T-4"] {
		t.Errorf("expected T-3 and T-4 to need push, got %v", found)
	}
}
