package storage

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigrationV15_PreservesExistingLogsAcrossUpgrade simulates a
// real-world upgrade: open a DB at v14 (the cascade-on schema), seed it
// with a task and a log entry, then bring the DB up to v15 and confirm
// that deleting the parent task no longer removes the log row.
func TestMigrationV15_PreservesExistingLogsAcrossUpgrade(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable FK: %v", err)
	}

	// Apply migrations up to (and including) v14 only — the legacy
	// schema where task_logs.task_id had ON DELETE CASCADE.
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);
	`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, m := range migrations {
		if m.version > 14 {
			break
		}
		if _, err := db.ExecContext(ctx, m.query); err != nil {
			t.Fatalf("apply migration v%d: %v", m.version, err)
		}
		if _, err := db.ExecContext(
			ctx,
			"INSERT INTO schema_migrations (version) VALUES (?)", m.version,
		); err != nil {
			t.Fatalf("record migration v%d: %v", m.version, err)
		}
	}

	// Seed a task and a log entry under the v14 (cascade-on) schema.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, project_id, title, status, created_at, updated_at)
		VALUES ('task_01legacycascade000000000', 1, 'proj-a',
		        'Legacy task', 'pending', '2026-01-01', '2026-01-01')
	`); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO task_logs (task_id, project_id, timestamp, by, action, note)
		VALUES ('task_01legacycascade000000000', 'proj-a',
		        '2026-01-02', 'noor', 'deleted', 'pre-existing reason')
	`); err != nil {
		t.Fatalf("seed task_log: %v", err)
	}

	// Now apply v15 to drop the cascade.
	var v15 *migration
	for i := range migrations {
		if migrations[i].version == 15 {
			v15 = &migrations[i]
			break
		}
	}
	if v15 == nil {
		t.Fatal("migration v15 missing from migrations slice")
	}
	if _, err := db.ExecContext(ctx, v15.query); err != nil {
		t.Fatalf("apply v15: %v", err)
	}

	// Re-enable FK enforcement (the migration toggles it OFF/ON inside
	// the script, but the connection-level state is what matters here).
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("re-enable FK: %v", err)
	}

	// Pre-existing log row must still be there after migration.
	var n int
	if err := db.QueryRowContext(
		ctx,
		`SELECT count(*) FROM task_logs WHERE task_id = 'task_01legacycascade000000000'`,
	).Scan(&n); err != nil {
		t.Fatalf("count logs post-migrate: %v", err)
	}
	if n != 1 {
		t.Fatalf("post-migrate logs = %d; want 1", n)
	}

	// Delete the parent task — log must survive.
	if _, err := db.ExecContext(
		ctx,
		`DELETE FROM tasks WHERE id = 'task_01legacycascade000000000'`,
	); err != nil {
		t.Fatalf("delete task: %v", err)
	}

	if err := db.QueryRowContext(
		ctx,
		`SELECT count(*) FROM task_logs WHERE task_id = 'task_01legacycascade000000000'`,
	).Scan(&n); err != nil {
		t.Fatalf("count logs post-delete: %v", err)
	}
	if n != 1 {
		t.Errorf("after task delete, logs = %d; want 1 (cascade should be dropped)", n)
	}
}

// TestMigrationV15_LatestMigrationVersion verifies the constant is bumped.
func TestMigrationV15_LatestMigrationVersion(t *testing.T) {
	if LatestMigrationVersion < 15 {
		t.Errorf("LatestMigrationVersion = %d; want >= 15", LatestMigrationVersion)
	}
}
