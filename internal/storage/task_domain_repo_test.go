package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/kit/domain"
	"hop.top/tlc/internal/core"
)

func newDomainRepoTestStorage(t *testing.T) *SQLiteStorage {
	t.Helper()
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() {
		os.Chdir(oldCwd)
		resetProjectDetection()
	})

	dbPath := filepath.Join(tmpDir, "test_domain_repo.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLiteStorage_DomainRepoCreate(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()

	task := &core.Task{
		ID:        "T-0001",
		Title:     "test task",
		Status:    core.StatusTodo,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := s.Create(ctx, task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, "T-0001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Title != "test task" {
		t.Errorf("Title = %q, want %q", got.Title, "test task")
	}
}

func TestSQLiteStorage_DomainRepoUpdate(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()

	task := &core.Task{
		ID:        "T-0002",
		Title:     "original",
		Status:    core.StatusTodo,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.Create(ctx, task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	task.Title = "updated"
	task.UpdatedAt = time.Now().UTC()
	if err := s.Update(ctx, task); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(ctx, "T-0002")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "updated" {
		t.Errorf("Title = %q, want %q", got.Title, "updated")
	}
}

func TestSQLiteStorage_DomainRepoDelete(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()

	task := &core.Task{
		ID:        "T-0003",
		Title:     "to delete",
		Status:    core.StatusTodo,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.Create(ctx, task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, "T-0003"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := s.Get(ctx, "T-0003")
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestSQLiteStorage_DomainRepoList(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()

	for _, id := range []string{"T-A", "T-B", "T-C"} {
		task := &core.Task{
			ID:        id,
			Title:     "task " + id,
			Status:    core.StatusTodo,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := s.Create(ctx, task); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	tasks, err := s.List(ctx, domain.Query{Limit: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("List returned %d tasks, want 2", len(tasks))
	}
}

func TestSplitCompoundKey(t *testing.T) {
	tests := []struct {
		input    string
		wantProj string
		wantID   string
	}{
		{"T-0001", "", "T-0001"},
		{"proj:T-0001", "proj", "T-0001"},
		{"hop-top/tlc:T-0001", "hop-top/tlc", "T-0001"},
	}
	for _, tt := range tests {
		proj, id := splitCompoundKey(tt.input)
		if proj != tt.wantProj || id != tt.wantID {
			t.Errorf("splitCompoundKey(%q) = (%q, %q), want (%q, %q)",
				tt.input, proj, id, tt.wantProj, tt.wantID)
		}
	}
}

func TestBindTaskColumnCount(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	pid := "test-proj"
	task := core.Task{
		ID:        "T-0099",
		Title:     "roundtrip test",
		Status:    core.StatusInProgress,
		Effort:    core.EffortM,
		Priority:  core.PriorityP1,
		CreatedAt: now,
		UpdatedAt: now,
		ProjectID: &pid,
	}

	cols, vals := BindTask(task)
	if len(cols) != 20 {
		t.Errorf("BindTask returned %d columns, want 20", len(cols))
	}
	if len(vals) != 20 {
		t.Errorf("BindTask returned %d values, want 20", len(vals))
	}
	if cols[0] != "id" {
		t.Errorf("first column = %q, want %q", cols[0], "id")
	}
}
