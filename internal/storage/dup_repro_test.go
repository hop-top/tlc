package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestCreateTask_OneRowOnly_FreshDB verifies that a single CreateTask call
// against a fresh DB produces exactly one row.
//
// Refs: tlc/T-1148.
func TestCreateTask_OneRowOnly_FreshDB(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fresh.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	task := &core.Task{
		ID:          core.NewTaskID(),
		Title:       "fresh-row-task",
		Description: "non-empty description",
		Status:      core.StatusTodo,
		Tags:        []string{"foo"},
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta:        map[string]any{},
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	count := countTasksByTitle(t, s, "fresh-row-task")
	if count != 1 {
		t.Fatalf("expected exactly 1 row for title %q, got %d", task.Title, count)
	}
}

// TestCreateTask_OneRowOnly_PopulatedDB verifies that creating one task in a
// DB that already has N tasks results in N+1 rows total (no duplicates).
//
// Refs: tlc/T-1148.
func TestCreateTask_OneRowOnly_PopulatedDB(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "populated.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	const seed = 5
	for i := 0; i < seed; i++ {
		task := &core.Task{
			ID:        core.NewTaskID(),
			Title:     "seed-task",
			Status:    core.StatusTodo,
			CreatedAt: now,
			UpdatedAt: now,
			Meta:      map[string]any{},
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed CreateTask %d: %v", i, err)
		}
	}

	before := countAllTasks(t, s)
	if before != seed {
		t.Fatalf("seed: expected %d rows, got %d", seed, before)
	}

	if err := s.CreateTask(ctx, &core.Task{
		ID:          core.NewTaskID(),
		Title:       "newcomer",
		Description: "added on top of seeded DB",
		Status:      core.StatusTodo,
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta:        map[string]any{},
	}); err != nil {
		t.Fatalf("CreateTask newcomer: %v", err)
	}

	after := countAllTasks(t, s)
	if after != seed+1 {
		t.Fatalf("expected %d rows after create, got %d", seed+1, after)
	}
}

// TestCreateTask_NoEmptyMirrorRow verifies CreateTask never produces a
// secondary row with empty description when the input had a non-empty
// description.
//
// Refs: tlc/T-1148.
func TestCreateTask_NoEmptyMirrorRow(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "mirror.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	const desc = "description-must-survive"
	task := &core.Task{
		ID:          core.NewTaskID(),
		Title:       "mirror-check",
		Description: desc,
		Status:      core.StatusTodo,
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta:        map[string]any{},
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := s.ListTasks(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	matched := 0
	for _, row := range rows {
		if row.Title != "mirror-check" {
			continue
		}
		matched++
		if row.Description != desc {
			t.Errorf("row %s: description = %q, want %q", row.ID, row.Description, desc)
		}
		if strings.HasPrefix(row.ID, "T-") {
			t.Errorf("row mirrors typeid: id=%s should not be a T-NNNN alias", row.ID)
		}
	}
	if matched != 1 {
		t.Fatalf("expected exactly 1 row with title %q, got %d", task.Title, matched)
	}
}

// TestCreateTask_MultipleSequential creates 10 tasks and verifies exactly
// 10 rows result.
//
// Refs: tlc/T-1148.
func TestCreateTask_MultipleSequential(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "sequential.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	const N = 10
	for i := 0; i < N; i++ {
		if err := s.CreateTask(ctx, &core.Task{
			ID:          core.NewTaskID(),
			Title:       "loop-task",
			Description: "iter",
			Status:      core.StatusTodo,
			CreatedAt:   now,
			UpdatedAt:   now,
			Meta:        map[string]any{},
		}); err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
	}

	got := countAllTasks(t, s)
	if got != N {
		t.Fatalf("expected %d rows, got %d", N, got)
	}
}

// TestCreateTask_WithProjectID verifies a task created with explicit
// ProjectID does not result in a second row under a different project.
//
// Refs: tlc/T-1148.
func TestCreateTask_WithProjectID(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "with_project.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	pid := "org/proj"
	task := &core.Task{
		ID:          core.NewTaskID(),
		Title:       "project-scoped",
		Description: "scoped to org/proj",
		Status:      core.StatusTodo,
		ProjectID:   &pid,
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta:        map[string]any{},
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := s.ListTasks(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	projects := map[string]int{}
	for _, row := range rows {
		if row.Title != "project-scoped" {
			continue
		}
		key := ""
		if row.ProjectID != nil {
			key = *row.ProjectID
		}
		projects[key]++
	}
	if len(projects) != 1 {
		t.Fatalf("expected task in exactly 1 project, got %v", projects)
	}
	if projects[pid] != 1 {
		t.Fatalf("expected 1 row under %q, got %d (%v)", pid, projects[pid], projects)
	}
}

// TestCreateTask_DescriptionPreserved ensures every persisted row's
// description matches the input — no empty-description mirror exists.
//
// Refs: tlc/T-1148.
func TestCreateTask_DescriptionPreserved(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "desc.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	const desc = "preserve me through the entire create path"
	if err := s.CreateTask(ctx, &core.Task{
		ID:          core.NewTaskID(),
		Title:       "desc-check",
		Description: desc,
		Status:      core.StatusTodo,
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta:        map[string]any{},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := s.ListTasks(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	for _, row := range rows {
		if row.Title != "desc-check" {
			continue
		}
		if row.Description != desc {
			t.Fatalf("row %s: description = %q, want %q", row.ID, row.Description, desc)
		}
	}
}

// TestCreateTask_TagsPreserved ensures non-empty tags survive into every
// persisted row — no zero-tag mirror exists.
//
// Refs: tlc/T-1148.
func TestCreateTask_TagsPreserved(t *testing.T) {
	resetProjectDetection()
	defer resetProjectDetection()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tags.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	tags := []string{"foo", "bar"}
	if err := s.CreateTask(ctx, &core.Task{
		ID:          core.NewTaskID(),
		Title:       "tags-check",
		Description: "tagged task",
		Tags:        tags,
		Status:      core.StatusTodo,
		CreatedAt:   now,
		UpdatedAt:   now,
		Meta:        map[string]any{},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	rows, err := s.ListTasks(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	for _, row := range rows {
		if row.Title != "tags-check" {
			continue
		}
		if len(row.Tags) == 0 {
			t.Fatalf("row %s: empty tags, want %v", row.ID, tags)
		}
	}
}

// countTasksByTitle returns the number of rows whose title matches.
func countTasksByTitle(t *testing.T, s *SQLiteStorage, title string) int {
	t.Helper()
	rows, err := s.ListTasks(context.Background(), core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	n := 0
	for _, r := range rows {
		if r.Title == title {
			n++
		}
	}
	return n
}

// countAllTasks returns the total number of task rows.
func countAllTasks(t *testing.T, s *SQLiteStorage) int {
	t.Helper()
	rows, err := s.ListTasks(context.Background(), core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	return len(rows)
}
