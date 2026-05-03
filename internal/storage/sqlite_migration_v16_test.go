package storage

import (
	"context"
	"testing"
)

// TestMigrationV16_DropsTNNNNMirrorRows seeds a DB with the dup pattern
// observed in T-1148 — a populated typeid row alongside an empty T-NNNN
// mirror that shares title and project_id — then verifies migration v16
// removes the mirror but preserves the typeid sibling and any unrelated
// T-NNNN rows that have populated descriptions.
func TestMigrationV16_DropsTNNNNMirrorRows(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Populated typeid row.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, project_id, title, description, status,
			reference, created_at, updated_at)
		VALUES ('task_01h455vb4pex5vsknk084sn02q', 100, 'proj-a',
			'duplicated title', 'real description that survives', 'pending',
			'', '2026-01-01', '2026-01-01')
	`); err != nil {
		t.Fatalf("insert typeid row: %v", err)
	}

	// Empty mirror keyed by T-NNNN — same title, same project, empty desc.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, project_id, title, description, status,
			reference, created_at, updated_at)
		VALUES ('T-0100', 101, 'proj-a',
			'duplicated title', '', 'pending',
			'', '2026-01-01', '2026-01-01')
	`); err != nil {
		t.Fatalf("insert mirror row: %v", err)
	}

	// Unrelated populated T-NNNN row (legacy pre-v13 data — must NOT be dropped).
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, project_id, title, description, status,
			reference, created_at, updated_at)
		VALUES ('T-0200', 102, 'proj-a',
			'standalone legacy', 'legacy description', 'pending',
			'', '2026-01-01', '2026-01-01')
	`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	// Run the migration query directly (in :memory:, the schema was
	// already applied via NewSQLiteStorage; we re-run the v16 cleanup
	// to assert idempotence and the delete predicate).
	for _, m := range migrations {
		if m.version == 16 {
			if _, err := s.db.ExecContext(ctx, m.query); err != nil {
				t.Fatalf("re-run v16 cleanup: %v", err)
			}
			break
		}
	}

	// Mirror row gone.
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE id = 'T-0100'`)
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count mirror: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 rows for T-0100 after migration, got %d", n)
	}

	// Typeid sibling preserved.
	row = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE id = 'task_01h455vb4pex5vsknk084sn02q'`)
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count typeid: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row for typeid sibling, got %d", n)
	}

	// Unrelated populated T-NNNN preserved.
	row = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE id = 'T-0200'`)
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count legacy: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row for legacy T-0200, got %d", n)
	}

	// Idempotent: re-run is a no-op.
	for _, m := range migrations {
		if m.version == 16 {
			if _, err := s.db.ExecContext(ctx, m.query); err != nil {
				t.Fatalf("re-run v16 cleanup (2nd time): %v", err)
			}
			break
		}
	}
	row = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`)
	if err := row.Scan(&n); err != nil {
		t.Fatalf("count after idempotent rerun: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows after idempotent rerun, got %d", n)
	}
}
