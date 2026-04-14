package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestSQLiteStorage_Query tests filtering, searching, and pagination
// Verifies status filters, text search, pagination, and log queries.
func TestSQLiteStorage_Query(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "test_query.db")
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

// TestStorage_QueryORLogic tests OR logic in query filters
// Verifies multiple status filters work with OR logic.
func TestStorage_QueryORLogic(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "test_or_logic.db")
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

// TestStorage_TagFiltering tests that --tag filtering uses exact tag matching,
// not substring matching (e.g. tag "fix" must not match task tagged "prefix").
func TestStorage_TagFiltering(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "test_tag_filter.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	s.CreateTask(ctx, &core.Task{ID: "T-1", Title: "Has fix tag", Status: core.StatusTodo, Tags: []string{"fix", "alpha"}, Reference: "ref"})
	s.CreateTask(ctx, &core.Task{ID: "T-2", Title: "Has prefix tag", Status: core.StatusTodo, Tags: []string{"prefix"}, Reference: "ref"})
	s.CreateTask(ctx, &core.Task{ID: "T-3", Title: "No tags", Status: core.StatusTodo, Reference: "ref"})
	s.CreateTask(ctx, &core.Task{ID: "T-4", Title: "Has fixing tag", Status: core.StatusTodo, Tags: []string{"fixing"}, Reference: "ref"})

	// Filtering by "fix" should return exactly T-1, not T-2 (prefix) or T-4 (fixing)
	q := core.Query{
		Filters: []core.FieldFilter{
			{Field: "tags", Operator: core.OpContains, Value: "fix"},
		},
	}
	got, err := s.ListTasks(ctx, q)
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 task with tag 'fix', got %d", len(got))
	}
	if len(got) > 0 && got[0].ID != "T-1" {
		t.Errorf("expected T-1, got %s", got[0].ID)
	}
}

// TestSQLiteStorage_LimitRespectsStatusPriority verifies that
// StatusPriority causes IN_PROGRESS tasks to sort first at the SQL
// level, so LIMIT doesn't truncate them before they're surfaced.
func TestSQLiteStorage_LimitRespectsStatusPriority(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "test_status_priority.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// Create 10 tasks: 8 TODO (created first), 2 IN_PROGRESS (created last).
	// Without StatusPriority the 2 IN_PROGRESS tasks appear at the top of
	// created_at DESC, but if they were created first they'd be pushed out
	// by LIMIT. We create the IN_PROGRESS tasks with earlier timestamps so
	// a naive ORDER BY created_at DESC + LIMIT 5 would miss them.
	for i := 0; i < 8; i++ {
		task := &core.Task{
			ID:        fmt.Sprintf("T-%04d", i+1),
			Title:     fmt.Sprintf("TODO task %d", i+1),
			Status:    core.StatusTodo,
			Reference: "ref",
			CreatedAt: now.Add(time.Duration(i+3) * time.Second), // later timestamps
			UpdatedAt: now,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("create TODO task: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		task := &core.Task{
			ID:        fmt.Sprintf("T-%04d", i+9),
			Title:     fmt.Sprintf("WIP task %d", i+1),
			Status:    core.StatusInProgress,
			Reference: "ref",
			CreatedAt: now.Add(time.Duration(i) * time.Second), // earlier timestamps
			UpdatedAt: now,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("create IN_PROGRESS task: %v", err)
		}
	}

	q := core.Query{
		Limit: 5,
		Filters: []core.FieldFilter{
			{Field: "status", Value: core.StatusTodo},
			{Field: "status", Value: core.StatusInProgress},
		},
		StatusPriority: string(core.StatusInProgress),
	}

	got, err := s.ListTasks(ctx, q)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 tasks, got %d", len(got))
	}

	// Both IN_PROGRESS tasks must appear in the result.
	var ipCount int
	for _, tsk := range got {
		if tsk.Status == core.StatusInProgress {
			ipCount++
		}
	}
	if ipCount != 2 {
		t.Errorf("expected 2 IN_PROGRESS tasks in top 5, got %d", ipCount)
	}

	// IN_PROGRESS tasks must come first.
	for i, tsk := range got {
		if i < 2 && tsk.Status != core.StatusInProgress {
			t.Errorf("position %d: expected IN_PROGRESS, got %s", i, tsk.Status)
		}
		if i >= 2 && tsk.Status != core.StatusTodo {
			t.Errorf("position %d: expected TODO, got %s", i, tsk.Status)
		}
	}
}

// TestSQLiteStorage_CountTasks tests counting tasks with filters.
func TestSQLiteStorage_CountTasks(t *testing.T) {
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	dbPath := filepath.Join(tmpDir, "test_count.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	now := time.Now().UTC()
	tasks := []core.Task{
		{ID: "T-1", Title: "Alpha", Status: core.StatusTodo, Reference: "ref", CreatedAt: now, UpdatedAt: now},
		{ID: "T-2", Title: "Beta", Status: core.StatusTodo, Reference: "ref", CreatedAt: now, UpdatedAt: now},
		{ID: "T-3", Title: "Gamma", Status: core.StatusInProgress, Reference: "ref", CreatedAt: now, UpdatedAt: now},
		{ID: "T-4", Title: "Delta", Status: core.StatusDone, Reference: "ref", CreatedAt: now, UpdatedAt: now},
	}
	for i := range tasks {
		if err := s.CreateTask(ctx, &tasks[i]); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	// Count all non-archived tasks
	count, err := s.CountTasks(ctx, core.Query{})
	if err != nil {
		t.Fatalf("CountTasks failed: %v", err)
	}
	if count != 4 {
		t.Errorf("expected 4, got %d", count)
	}

	// Count with status filter
	count, err = s.CountTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "status", Operator: core.OpEq, Value: core.StatusTodo},
		},
	})
	if err != nil {
		t.Fatalf("CountTasks with filter failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 TODO tasks, got %d", count)
	}

	// Count with search
	count, err = s.CountTasks(ctx, core.Query{Search: "Alpha"})
	if err != nil {
		t.Fatalf("CountTasks with search failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 matching task, got %d", count)
	}
}
