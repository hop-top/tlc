package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// seedOldStatusRows inserts SCREAMING_CASE status rows directly via raw SQL,
// bypassing core.Task validation. This simulates a DB written by a pre-T-0738
// tlc binary.
func seedOldStatusRows(t *testing.T, s *SQLiteStorage, rows map[string]string) {
	t.Helper()
	ctx := context.Background()
	now := "2026-04-27T00:00:00Z"
	seq := int64(1)
	for id, status := range rows {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO tasks (project_id, id, seq, title, status, reference, created_at, updated_at)
			VALUES ('', ?, ?, ?, ?, ?, ?, ?)
		`, id, seq, "title-"+id, status, "ref:"+id, now, now)
		if err != nil {
			t.Fatalf("seed row %s/%s: %v", id, status, err)
		}
		seq++
	}
}

// clearStatusMigrationMarker removes the marker so the next open re-runs the
// data migration. Used by tests that seed old-label rows after the first
// open (when the migration already saw zero old rows and recorded the
// "applied" marker).
func clearStatusMigrationMarker(t *testing.T, s *SQLiteStorage) {
	t.Helper()
	_, err := s.db.ExecContext(
		context.Background(),
		`DELETE FROM data_migrations WHERE name = ?`, dataMigrationStatusV1,
	)
	if err != nil {
		t.Fatalf("clear marker: %v", err)
	}
}

// fileSHA256 returns the sha256 of a file's bytes.
func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("hash %s: %v", path, err)
	}
	return string(h.Sum(nil))
}

// listBackupFiles returns all *.bak files in the .dbs/ subdir beside dbPath
// that match the status-migration backup naming convention.
func listBackupFiles(t *testing.T, dbPath string) []string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(dbPath), backupSubdir)
	base := filepath.Base(dbPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	prefix := name + ".pre-status-migration."
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("readdir %s: %v", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".bak") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// readStatuses returns id->status map for all tasks in the DB.
func readStatuses(t *testing.T, s *SQLiteStorage) map[string]string {
	t.Helper()
	rows, err := s.db.QueryContext(context.Background(), `SELECT id, status FROM tasks`)
	if err != nil {
		t.Fatalf("query statuses: %v", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[id] = st
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}

// TestAutoMigrate_GoldenDB_OldLabels verifies the auto-migration rewrites
// old SCREAMING_CASE statuses to lowercase on first run, leaves new-label
// rows alone, and produces a backup file.
func TestAutoMigrate_GoldenDB_OldLabels(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tlc.db")

	// First open: standard schema migrations run.
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	// Seed mixed old + new label rows directly.
	seedOldStatusRows(t, s, map[string]string{
		"T-1": "TODO",
		"T-2": "IN_PROGRESS",
		"T-3": "DONE",
		"T-4": "SKIPPED",
		"T-5": "pending", // already-new row, must remain untouched
	})
	// First open already recorded the "applied" marker (zero old rows
	// were present at migration time). Clear it so the next open
	// detects the seeded rows and runs the rewrite.
	clearStatusMigrationMarker(t, s)
	s.Close()

	// Re-open: migration should fire on this invocation.
	s, err = NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("re-open storage: %v", err)
	}
	defer s.Close()

	got := readStatuses(t, s)
	want := map[string]string{
		"T-1": "pending",
		"T-2": "doing",
		"T-3": "done",
		"T-4": "skipped",
		"T-5": "pending",
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("task %s: got %q, want %q", id, got[id], w)
		}
	}

	backups := listBackupFiles(t, dbPath)
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup file, got %d: %v", len(backups), backups)
	}

	// Verify backup file integrity by size match against pre-migration source.
	// Backup is taken before write, so it must be readable as a SQLite DB.
	bdb, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = bdb.Close() }()
	var n int
	if err := bdb.QueryRowContext(
		context.Background(),
		`SELECT count(*) FROM tasks WHERE status IN ('TODO','IN_PROGRESS','DONE','SKIPPED')`,
	).Scan(&n); err != nil {
		t.Fatalf("query backup: %v", err)
	}
	if n != 4 {
		t.Errorf("backup should preserve 4 old-label rows, got %d", n)
	}
}

// TestAutoMigrate_AlreadyMigrated_NoOp verifies a second invocation does
// not write a new backup file or touch any rows.
func TestAutoMigrate_AlreadyMigrated_NoOp(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tlc.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	seedOldStatusRows(t, s, map[string]string{"T-1": "TODO"})
	clearStatusMigrationMarker(t, s)
	s.Close()

	// Trigger first migration.
	s, err = NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("first re-open: %v", err)
	}
	s.Close()

	firstBackups := listBackupFiles(t, dbPath)
	if len(firstBackups) != 1 {
		t.Fatalf("expected 1 backup after first migration, got %d", len(firstBackups))
	}
	hashBefore := fileSHA256(t, dbPath)

	// Second invocation must be a no-op.
	s, err = NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("second re-open: %v", err)
	}
	defer s.Close()

	secondBackups := listBackupFiles(t, dbPath)
	if len(secondBackups) != len(firstBackups) {
		t.Errorf("second invocation created new backup: %v", secondBackups)
	}
	hashAfter := fileSHA256(t, dbPath)
	if hashBefore != hashAfter {
		t.Error("second invocation modified DB file (should be no-op)")
	}
}

// TestAutoMigrate_NoOldRows_NoOp verifies a DB with only new-label rows
// (or no rows at all) produces no backup and no migration writes.
func TestAutoMigrate_NoOldRows_NoOp(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tlc.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	// Seed only new-label rows.
	seedOldStatusRows(t, s, map[string]string{"T-1": "pending", "T-2": "done"})
	s.Close()

	s, err = NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer s.Close()

	if backups := listBackupFiles(t, dbPath); len(backups) != 0 {
		t.Errorf("no-old-rows path created %d backup(s): %v", len(backups), backups)
	}
}

// TestAutoMigrate_BackupFailure_AbortsCleanly forces the backup function
// to fail and verifies the migration aborts with the DB byte-identical
// and rows unchanged.
func TestAutoMigrate_BackupFailure_AbortsCleanly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tlc.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	seedOldStatusRows(t, s, map[string]string{"T-1": "TODO", "T-2": "IN_PROGRESS"})
	clearStatusMigrationMarker(t, s)
	s.Close()

	// Force backup to fail.
	prev := statusBackupFn
	t.Cleanup(func() { statusBackupFn = prev })
	statusBackupFn = func(string) (string, error) {
		return "", errors.New("simulated disk full")
	}

	hashBefore := fileSHA256(t, dbPath)

	_, openErr := NewSQLiteStorage(dbPath)
	if openErr == nil {
		t.Fatal("expected NewSQLiteStorage to return error when backup fails")
	}
	if !strings.Contains(openErr.Error(), "backup") {
		t.Errorf("error should mention backup, got: %v", openErr)
	}

	hashAfter := fileSHA256(t, dbPath)
	if hashBefore != hashAfter {
		t.Error("DB file changed after backup failure — must remain byte-identical")
	}

	// Verify rows untouched: old labels still present.
	s, err = openStorageWithoutMigration(dbPath)
	if err != nil {
		t.Fatalf("open without migration: %v", err)
	}
	defer s.Close()
	got := readStatuses(t, s)
	if got["T-1"] != "TODO" || got["T-2"] != "IN_PROGRESS" {
		t.Errorf("rows changed despite backup failure: %v", got)
	}

	// No backup file should be left behind.
	if backups := listBackupFiles(t, dbPath); len(backups) != 0 {
		t.Errorf("backup file leaked despite failure: %v", backups)
	}
}

// TestAutoMigrate_BackupIntegrity verifies the produced backup is a valid
// SQLite DB that opens and answers PRAGMA integrity_check with "ok".
func TestAutoMigrate_BackupIntegrity(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tlc.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	seedOldStatusRows(t, s, map[string]string{"T-1": "TODO"})
	clearStatusMigrationMarker(t, s)
	s.Close()

	// Trigger migration.
	s, err = NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	s.Close()

	backups := listBackupFiles(t, dbPath)
	if len(backups) != 1 {
		t.Fatalf("want 1 backup, got %d", len(backups))
	}

	bdb, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer func() { _ = bdb.Close() }()
	var result string
	if err := bdb.QueryRowContext(context.Background(), `PRAGMA integrity_check`).Scan(&result); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if result != "ok" {
		t.Errorf("backup integrity_check = %q, want %q", result, "ok")
	}
}

// TestAutoMigrate_MarkerPersisted verifies the data-migration marker is
// written to the data_migrations table after successful migration.
func TestAutoMigrate_MarkerPersisted(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tlc.db")

	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	seedOldStatusRows(t, s, map[string]string{"T-1": "TODO"})
	clearStatusMigrationMarker(t, s)
	s.Close()

	s, err = NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	defer s.Close()

	var name string
	err = s.db.QueryRowContext(
		context.Background(),
		`SELECT name FROM data_migrations WHERE name = ?`, dataMigrationStatusV1,
	).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			t.Fatal("marker row missing from data_migrations")
		}
		t.Fatalf("query marker: %v", err)
	}
}
