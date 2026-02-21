package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestStorage_MigrationIntegration verifies migration system works correctly
// Tests fresh initialization and re-opening with migration check
func TestStorage_MigrationIntegration(t *testing.T) {
	dbPath := "test_integration.db"
	defer os.Remove(dbPath)

	// Test fresh initialization
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	s.Close()

	// Test re-opening (migration check)
	s, err = NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to re-open storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	// Verify tables exist by performing basic operations
	if err := s.CreateTask(ctx, &core.Task{ID: "T-INT", Title: "Integration", Status: core.StatusTodo, Reference: "ref"}); err != nil {
		t.Errorf("CreateTask failed: %v", err)
	}

	if err := s.CreateFlowRun(ctx, &core.FlowRun{ID: "run:INT", FlowID: "flow:INT", Status: core.FlowStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Errorf("CreateFlowRun failed: %v", err)
	}
}

// TestStorage_TransactionalIntegrity validates foreign key constraints
// Ensures adding logs for non-existent tasks fails appropriately
func TestStorage_TransactionalIntegrity(t *testing.T) {
	// This tests if foreign keys and other constraints are respected
	dbPath := "test_constraints.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Test logging for non-existent task
	err = s.AddLog(ctx, &core.LogEntry{
		TaskID:    "NON-EXISTENT",
		Timestamp: time.Now(),
		By:        "user",
		Action:    "COMMENT",
		Note:      "should fail if FKs enabled",
	})

	if err == nil {
		t.Error("expected error when adding log for non-existent task, but got nil")
	}
}
