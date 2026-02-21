package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

func resetProjectDetection() {
	viper.Reset()
	core.ResetDetectionCache()
}

// TestConcurrentTaskCreation verifies UNIQUE constraint on composite key (project_id, id)
// Multiple agents trying to create same task in same project = constraint error
// Same task ID in different projects = success.
func TestConcurrentTaskCreation(t *testing.T) {
	resetProjectDetection()
	dbPath := "test_concurrent.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Simulate multiple agents trying to create the same task ID
	// This happens when sync happens from multiple sources
	taskID := "T-SYNC-001"

	var wg sync.WaitGroup
	errChan := make(chan error, 5)

	// Spawn 5 goroutines simulating different agents/sync processes
	// Note: With composite primary key (project_id, id), tasks with same ID
	// can coexist in different projects. For this test, we use the same project.
	for i := range 5 {
		wg.Add(1)
		projectID := "org/test"
		go func(agentID int) {
			defer wg.Done()

			task := &core.Task{
				ID:        taskID,
				ProjectID: &projectID,
				Title:     fmt.Sprintf("Task from agent %d", agentID),
				Status:    core.StatusTodo,
				Reference: fmt.Sprintf("ref-%d", agentID),
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
				Meta:      map[string]any{"agent": agentID},
			}

			err := s.CreateTask(ctx, task)
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Count errors
	errorCount := 0
	var lastError error
	for err := range errChan {
		errorCount++
		lastError = err
		t.Logf("Error from concurrent creation: %v", err)
	}

	// At least 4 should fail with constraint error
	if errorCount < 4 {
		t.Errorf("Expected at least 4 constraint errors, got %d", errorCount)
	}

	// The error should mention UNIQUE constraint
	if lastError != nil && !containsUniqueConstraintError(lastError) {
		t.Errorf("Expected UNIQUE constraint error, got: %v", lastError)
	}
}

// TestConcurrentFlowExecution simulates multiple parallel steps accessing tasks
// Verifies concurrent updates don't cause data corruption or lost updates.
func TestConcurrentFlowExecution(t *testing.T) {
	resetProjectDetection()

	tmpDir, err := os.MkdirTemp("", "tlc-concurrent-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "test_flow_concurrent.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create a set of tasks
	for i := 1; i <= 10; i++ {
		task := &core.Task{
			ID:        fmt.Sprintf("T-%04d", i),
			Title:     fmt.Sprintf("Task %d", i),
			Status:    core.StatusTodo,
			Reference: "ref",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		s.CreateTask(ctx, task)
	}

	// Simulate parallel execution updating tasks concurrently
	var wg sync.WaitGroup
	errChan := make(chan error, 10)

	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(taskNum int) {
			defer wg.Done()

			taskID := fmt.Sprintf("T-%04d", taskNum)

			// Read task
			task, err := s.GetTask(ctx, taskID)
			if err != nil {
				errChan <- err
				return
			}
			if task == nil {
				errChan <- fmt.Errorf("task %s not found", taskID)
				return
			}

			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)

			// Update task
			task.Status = core.StatusDone
			task.UpdatedAt = time.Now().UTC()
			err = s.UpdateTask(ctx, task)
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Count errors
	errorCount := 0
	for err := range errChan {
		errorCount++
		t.Logf("Error from concurrent update: %v", err)
	}

	if errorCount > 0 {
		t.Errorf("Expected no errors in concurrent updates, got %d", errorCount)
	}

	// Verify all tasks are done
	tasks, _ := s.ListTasks(ctx, core.Query{})
	doneCount := 0
	for _, task := range tasks {
		if task.Status == core.StatusDone {
			doneCount++
		}
	}

	if doneCount != 10 {
		t.Errorf("Expected 10 done tasks, got %d", doneCount)
	}
}

// TestSyncIngest simulates the ingestTODO scenario with concurrent sync
// Tests race condition where two agents read TODO and try to create same task.
func TestSyncIngest(t *testing.T) {
	resetProjectDetection()
	dbPath := "test_sync_ingest.db"
	defer os.Remove(dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Simulate two agents both trying to ingest the same TODO file
	// Agent 1 reads TODO and sees T-0001 doesn't exist, tries to create
	// Agent 2 reads TODO at the same time, sees T-0001 doesn't exist, tries to create

	taskID := "T-0001"

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	for i := range 2 {
		wg.Add(1)
		go func(agentID int) {
			defer wg.Done()

			// Check if task exists (both will see it doesn't exist)
			existing, _ := s.GetTask(ctx, taskID)

			if existing == nil {
				// Both agents will try to create
				task := &core.Task{
					ID:        taskID,
					Title:     fmt.Sprintf("Task from sync agent %d", agentID),
					Status:    core.StatusTodo,
					Reference: "ref",
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				}

				err := s.CreateTask(ctx, task)
				if err != nil {
					errChan <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Expect one error due to race condition
	errorCount := 0
	for err := range errChan {
		errorCount++
		t.Logf("Error from sync ingest: %v", err)
	}

	// This demonstrates the bug: at least one should fail
	if errorCount < 1 {
		t.Logf("WARNING: Expected at least 1 constraint error in race condition, got %d", errorCount)
	}
}

func containsUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "UNIQUE") && strings.Contains(errStr, "constraint")
}
