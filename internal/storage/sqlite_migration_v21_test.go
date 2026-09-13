package storage

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// rowQuerier is the slice of *sql.DB the recipe-column helpers need.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// recipeCols is the raw recipe-era column set of one tasks row.
type recipeCols struct {
	kind     string
	attempts int
	nullable map[string]sql.NullString // claimed_at, run_id, step_id, spec, result
	ordinal  sql.NullInt64
}

// readRecipeCols reads the recipe-era columns of task id.
func readRecipeCols(t *testing.T, q rowQuerier, id string) recipeCols {
	t.Helper()
	var c recipeCols
	var claimedAt, runID, stepID, spec, result sql.NullString
	if err := q.QueryRowContext(context.Background(), `
		SELECT kind, attempts, claimed_at, run_id, step_id, step_ordinal, spec, result
		FROM tasks WHERE id = ?
	`, id).Scan(&c.kind, &c.attempts, &claimedAt, &runID, &stepID, &c.ordinal, &spec, &result); err != nil {
		t.Fatalf("read recipe columns of %s: %v", id, err)
	}
	c.nullable = map[string]sql.NullString{
		"claimed_at": claimedAt, "run_id": runID, "step_id": stepID, "spec": spec, "result": result,
	}
	return c
}

// assertRecipeDefaults checks a row that no recipe or executor ever
// touched: kind is the explicit default, attempts zero, every nullable
// recipe column NULL.
func assertRecipeDefaults(t *testing.T, label string, c recipeCols) {
	t.Helper()
	if c.kind != "agent" || c.attempts != 0 {
		t.Errorf("%s: kind/attempts = %q/%d; want agent/0", label, c.kind, c.attempts)
	}
	for name, v := range c.nullable {
		if v.Valid {
			t.Errorf("%s: %s stored as %q; want NULL", label, name, v.String)
		}
	}
	if c.ordinal.Valid {
		t.Errorf("%s: step_ordinal stored as %d; want NULL", label, c.ordinal.Int64)
	}
}

func assertIndexExists(t *testing.T, q rowQuerier, name string) {
	t.Helper()
	var n int
	if err := q.QueryRowContext(
		context.Background(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name,
	).Scan(&n); err != nil {
		t.Fatalf("lookup index %s: %v", name, err)
	}
	if n != 1 {
		t.Errorf("index %s missing", name)
	}
}

// TestMigrationV21_FreshDBHasRecipeSchema verifies a database opened at
// HEAD carries the v21 shape: the recipe-era task columns, and the two
// ledger tables with their keys and indexes.
func TestMigrationV21_FreshDBHasRecipeSchema(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	version, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version < 21 || LatestMigrationVersion < 21 {
		t.Fatalf("schema/latest version = %d/%d; want >= 21", version, LatestMigrationVersion)
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, seq, title, status, created_at, updated_at)
		VALUES ('task_v21', 1, 't', 'TODO', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	assertRecipeDefaults(t, "fresh insert", readRecipeCols(t, s.db, "task_v21"))

	assertLedgerTables(t, s.db)
	for _, idx := range []string{
		"idx_tasks_run_id", "idx_recipe_runs_recipe_id", "idx_recipe_runs_track_id",
		"idx_recipe_runs_project_id", "idx_recipe_run_tasks_task_id",
	} {
		assertIndexExists(t, s.db, idx)
	}
}

// assertLedgerTables exercises recipe_runs / recipe_run_tasks: a run with
// one step→task row, the (run_id, step_id) primary key, and the declared
// FK. Enforcement depends on the connection's foreign_keys pragma, which
// the store does not enable, so the declaration is asserted rather than
// the cascade.
func assertLedgerTables(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO recipe_runs (id, recipe_id, version, hash, vars, subject_type, subject_id, track_id, created_by, created_at)
		VALUES ('run_1', 'code-review', '1.2.0', 'h', '{}', 'task', 'task_v21', 'track_1', 'jad', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("insert recipe_runs: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO recipe_run_tasks (run_id, step_id, task_id) VALUES ('run_1', 'lint', 'task_v21')
	`); err != nil {
		t.Fatalf("insert recipe_run_tasks: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO recipe_run_tasks (run_id, step_id, task_id) VALUES ('run_1', 'lint', 'task_other')
	`); err == nil {
		t.Error("duplicate (run_id, step_id) accepted; want PRIMARY KEY violation")
	}

	var fkTable, onDelete string
	if err := db.QueryRowContext(ctx, `
		SELECT "table", on_delete FROM pragma_foreign_key_list('recipe_run_tasks')
	`).Scan(&fkTable, &onDelete); err != nil {
		t.Fatalf("read recipe_run_tasks foreign keys: %v", err)
	}
	if fkTable != "recipe_runs" || onDelete != "CASCADE" {
		t.Errorf("recipe_run_tasks FK = %s ON DELETE %s; want recipe_runs ON DELETE CASCADE", fkTable, onDelete)
	}
}

// openAtV20 builds an in-memory database migrated to exactly v20 — the
// schema head immediately before the recipe columns landed.
func openAtV20(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);
	`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, m := range migrations {
		if m.version > 20 {
			break
		}
		if _, err := db.ExecContext(ctx, m.query); err != nil {
			t.Fatalf("apply migration v%d: %v", m.version, err)
		}
		if _, err := db.ExecContext(
			ctx, "INSERT INTO schema_migrations (version) VALUES (?)", m.version,
		); err != nil {
			t.Fatalf("record migration v%d: %v", m.version, err)
		}
	}
	return db
}

// applyV21 runs the v21 migration against an already-open v20 database.
func applyV21(t *testing.T, db *sql.DB) {
	t.Helper()

	for _, m := range migrations {
		if m.version != 21 {
			continue
		}
		if _, err := db.ExecContext(context.Background(), m.query); err != nil {
			t.Fatalf("apply migration v21: %v", err)
		}
		return
	}
	t.Fatal("migration v21 not found")
}

// TestMigrationV21_UpgradePreservesTasksWithDefaults seeds a v20 database
// with tasks across two projects, upgrades, and asserts every row survives
// with the recipe defaults. v13 once dropped every row in this schema's
// history; this pins preservation for the ALTER-only path too.
func TestMigrationV21_UpgradePreservesTasksWithDefaults(t *testing.T) {
	db := openAtV20(t)
	ctx := context.Background()

	seed := []struct{ id, projectID string }{
		{"task_a1", "proj-a"},
		{"task_a2", "proj-a"},
		{"task_b1", "proj-b"},
		{"task_g1", ""},
	}
	for i, s := range seed {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO tasks (project_id, id, seq, title, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'TODO', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
		`, s.projectID, s.id, i+1, s.id); err != nil {
			t.Fatalf("seed task %s: %v", s.id, err)
		}
	}

	applyV21(t, db)

	assertRowCount(t, db, "tasks", len(seed))
	for _, s := range seed {
		assertRecipeDefaults(t, s.id, readRecipeCols(t, db, s.id))
	}
	// Ledger tables exist and are empty on upgrade; flow_runs is untouched
	// by v21 (its drop is a later migration).
	assertRowCount(t, db, "recipe_runs", 0)
	assertRowCount(t, db, "recipe_run_tasks", 0)
	if _, err := db.ExecContext(ctx, `SELECT id FROM flow_runs LIMIT 0`); err != nil {
		t.Errorf("flow_runs missing after v21: %v", err)
	}
}

func assertRowCount(t *testing.T, q rowQuerier, table string, want int) {
	t.Helper()
	var n int
	if err := q.QueryRowContext(context.Background(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if n != want {
		t.Errorf("%s has %d rows; want %d", table, n, want)
	}
}
