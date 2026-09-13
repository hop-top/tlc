package storage

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// applyV22 runs the v22 migration against an already-open database.
func applyV22(t *testing.T, db *sql.DB) {
	t.Helper()

	for _, m := range migrations {
		if m.version != 22 {
			continue
		}
		if _, err := db.ExecContext(context.Background(), m.query); err != nil {
			t.Fatalf("apply migration v22: %v", err)
		}
		return
	}
	t.Fatal("migration v22 not found")
}

// assertNoFlowRuns fails when the flow_runs table is still present.
func assertNoFlowRuns(t *testing.T, q rowQuerier) {
	t.Helper()
	var n int
	if err := q.QueryRowContext(
		context.Background(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'flow_runs'`,
	).Scan(&n); err != nil {
		t.Fatalf("lookup flow_runs: %v", err)
	}
	if n != 0 {
		t.Error("flow_runs still present; want dropped")
	}
}

// TestMigrationV22_UpgradeDropsFlowRuns seeds a v21 database whose
// flow_runs table holds a row, upgrades to v22, and asserts the table is
// gone while the recipe ledger survives.
func TestMigrationV22_UpgradeDropsFlowRuns(t *testing.T) {
	db := openAtV20(t)
	ctx := context.Background()
	applyV21(t, db)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO flow_runs (id, flow_id, status, started_at)
		VALUES ('run_legacy', 'deploy', 'succeeded', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed flow_runs: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, title, status, created_at, updated_at)
		VALUES ('task_v22', 1, 't', 'TODO', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	applyV22(t, db)

	assertNoFlowRuns(t, db)
	assertRowCount(t, db, "tasks", 1)
	assertRowCount(t, db, "recipe_runs", 0)
	assertRowCount(t, db, "recipe_run_tasks", 0)
}

// TestMigrationV22_FreshDBHasNoFlowRuns verifies a database opened at
// HEAD never exposes flow_runs.
func TestMigrationV22_FreshDBHasNoFlowRuns(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	version, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version < 22 || LatestMigrationVersion < 22 {
		t.Fatalf("schema/latest version = %d/%d; want >= 22", version, LatestMigrationVersion)
	}
	assertNoFlowRuns(t, s.db)
}
