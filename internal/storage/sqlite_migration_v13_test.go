package storage

import (
	"context"
	"testing"
)

// TestMigrationV13_TasksHaveSeqColumn verifies migration v13 rebuilds the
// tasks table with the seq column, the (project_id, seq) UNIQUE constraint,
// and the typeid-friendly id PRIMARY KEY.
func TestMigrationV13_TasksHaveSeqColumn(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Insert a typeid-shaped row.
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, project_id, title, status, created_at, updated_at)
		VALUES ('task_01h455vb4pex5vsknk084sn02q', 1, 'proj-a',
		        'Test', 'pending', '2026-01-01', '2026-01-01')
	`)
	if err != nil {
		t.Fatalf("insert with seq: %v", err)
	}

	// Same project + same seq must violate UNIQUE.
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, project_id, title, status, created_at, updated_at)
		VALUES ('task_02h455vb4pex5vsknk084sn02q', 1, 'proj-a',
		        'Dup seq', 'pending', '2026-01-01', '2026-01-01')
	`)
	if err == nil {
		t.Fatalf("duplicate (project_id, seq) accepted; want UNIQUE violation")
	}

	// Different project may reuse the same seq.
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, project_id, title, status, created_at, updated_at)
		VALUES ('task_03h455vb4pex5vsknk084sn02q', 1, 'proj-b',
		        'Other project', 'pending', '2026-01-01', '2026-01-01')
	`)
	if err != nil {
		t.Fatalf("seq=1 in different project rejected: %v", err)
	}
}

// TestMigrationV13_TracksHaveSlugColumn verifies tracks gain the slug column
// with (project_id, slug) UNIQUE.
func TestMigrationV13_TracksHaveSlugColumn(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tracks (id, slug, project_id, title, type, status, created_at, updated_at)
		VALUES ('track_01h455vbqkfsn02nk084ksn02q', 'auth-rewrite', 'proj-a',
		        'Auth rewrite', 'feature', 'pending', '2026-01-01', '2026-01-01')
	`)
	if err != nil {
		t.Fatalf("insert track with slug: %v", err)
	}

	// Duplicate slug in same project must fail.
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tracks (id, slug, project_id, title, type, status, created_at, updated_at)
		VALUES ('track_02h455vbqkfsn02nk084ksn02q', 'auth-rewrite', 'proj-a',
		        'Dup', 'feature', 'pending', '2026-01-01', '2026-01-01')
	`)
	if err == nil {
		t.Fatalf("duplicate (project_id, slug) accepted; want UNIQUE violation")
	}

	// Same slug in another project: allowed.
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tracks (id, slug, project_id, title, type, status, created_at, updated_at)
		VALUES ('track_03h455vbqkfsn02nk084ksn02q', 'auth-rewrite', 'proj-b',
		        'Other project', 'feature', 'pending', '2026-01-01', '2026-01-01')
	`)
	if err != nil {
		t.Fatalf("same slug in different project rejected: %v", err)
	}
}

// TestMigrationV15_TaskLogsNoCascade verifies migration v15 drops the
// ON DELETE CASCADE that v13 placed on task_logs.task_id. Deleting a
// task must leave its log entries intact so the --note recorded at
// delete time (T-1178) survives for audit / policy enforcement
// (T-1192).
func TestMigrationV15_TaskLogsNoCascade(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable FK: %v", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, project_id, title, status, created_at, updated_at)
		VALUES ('task_01h455vb4pex5vsknk084sn02q', 1, 'proj-a',
		        'Test', 'pending', '2026-01-01', '2026-01-01')
	`); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO task_logs (task_id, timestamp, by, action, note)
		VALUES ('task_01h455vb4pex5vsknk084sn02q', '2026-01-01', 'noor', 'deleted', 'why-i-deleted-it')
	`); err != nil {
		t.Fatalf("insert task_log: %v", err)
	}

	// Delete task — log row must survive (no cascade post-v15).
	if _, err := s.db.ExecContext(
		ctx,
		`DELETE FROM tasks WHERE id = 'task_01h455vb4pex5vsknk084sn02q'`,
	); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	var n int
	var note string
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT count(*) FROM task_logs WHERE task_id = 'task_01h455vb4pex5vsknk084sn02q'`,
	).Scan(&n); err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if n != 1 {
		t.Fatalf("after task delete, log rows = %d; want 1 (cascade dropped)", n)
	}

	if err := s.db.QueryRowContext(
		ctx,
		`SELECT note FROM task_logs WHERE task_id = 'task_01h455vb4pex5vsknk084sn02q'`,
	).Scan(&note); err != nil {
		t.Fatalf("read note: %v", err)
	}
	if note != "why-i-deleted-it" {
		t.Errorf("note = %q; want %q (note must survive task delete)", note, "why-i-deleted-it")
	}
}
