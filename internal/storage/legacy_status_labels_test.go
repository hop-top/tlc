package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestOpen_LegacyStatusLabels_Untouched verifies opening a file DB that
// holds SCREAMING_CASE status rows leaves them as-is: the default config
// vocabulary is still TODO/IN_PROGRESS/DONE/SKIPPED, so rows must stay
// readable and updatable.
func TestOpen_LegacyStatusLabels_Untouched(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tlc.db")
	seed := seedLegacyDB(t, dbPath)

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer func() { _ = s.Close() }()
	assertLegacyUntouched(t, ctx, s, dbPath, seed)
}

func seedLegacyDB(t *testing.T, dbPath string) map[string]string {
	t.Helper()
	ctx := context.Background()

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	seed := map[string]string{
		"T-1": "TODO",
		"T-2": "IN_PROGRESS",
		"T-3": "DONE",
		"T-4": "SKIPPED",
	}
	now := "2026-04-27T00:00:00Z"
	seq := int64(1)
	for id, status := range seed {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO tasks (project_id, id, seq, title, description, status, reference, created_at, updated_at)
			VALUES ('', ?, ?, ?, '', ?, ?, ?, ?)
		`, id, seq, "title-"+id, status, "ref:"+id, now, now); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		seq++
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Ensure no leftover marker state from the first open.
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `DROP TABLE IF EXISTS data_migrations`); err != nil {
		t.Fatalf("drop marker table: %v", err)
	}
	_ = raw.Close()

	return seed
}

func assertLegacyUntouched(t *testing.T, ctx context.Context, s *SQLiteStorage, dbPath string, seed map[string]string) { //nolint:revive // ctx order
	t.Helper()
	for id, want := range seed {
		got, err := s.GetTaskInProject(ctx, id, "")
		if err != nil || got == nil {
			t.Fatalf("get %s: %v (task %v)", id, err, got)
		}
		if string(got.Status) != want {
			t.Errorf("task %s: status %q, want %q", id, got.Status, want)
		}
	}

	tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != len(seed) {
		t.Fatalf("list: got %d tasks, want %d", len(tasks), len(seed))
	}

	// Transition: TODO -> IN_PROGRESS persists.
	task, err := s.GetTaskInProject(ctx, "T-1", "")
	if err != nil || task == nil {
		t.Fatalf("get T-1: %v", err)
	}
	task.Status = core.TaskStatus("IN_PROGRESS")
	if err := s.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := s.GetTaskInProject(ctx, "T-1", "")
	if err != nil || got == nil {
		t.Fatalf("re-get: %v", err)
	}
	if string(got.Status) != "IN_PROGRESS" {
		t.Errorf("after transition: %q", got.Status)
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(dbPath), backupSubdir, "*pre-status-migration*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("unexpected status-migration backups: %v", matches)
	}
}
