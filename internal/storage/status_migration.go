package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// dataMigrationStatusV1 is the marker name recorded in the data_migrations
// table once the SCREAMING_CASE -> lowercase status rewrite has run.
const dataMigrationStatusV1 = "status-migration-v1"

// statusBackupFn is the function used to back up the DB before the rewrite.
// Indirected via a variable so tests can force backup failures without
// resorting to filesystem permission tricks (which interact badly with
// SQLite's WAL/journal behavior and macOS sandbox quirks).
var statusBackupFn = backupForStatusMigration

// Status labels used by the migration are kept as string literals (not
// references to internal/core constants) so this file compiles before the
// lowercase-status rework (T-0738/T-0739) lands. Old: TODO, IN_PROGRESS,
// DONE, SKIPPED. New: pending, doing, done, skipped.

// runStatusDataMigration rewrites SCREAMING_CASE task statuses to lowercase
// in a single transaction, with a mandatory pre-write backup of the DB
// file. Idempotent: skips on subsequent invocations via a marker row in
// the data_migrations table.
//
// Hard rules (non-negotiable):
//   - Backup MUST succeed before any UPDATE runs.
//   - Backup integrity is verified by PRAGMA integrity_check on the .bak.
//   - Any failure before the UPDATE leaves the DB byte-identical.
//   - Marker is checked first → zero overhead on subsequent runs.
//
// Uses raw SQL on s.db so it is not affected by future write-path guards
// that reject SCREAMING_CASE inserts/updates (T-0739).
func (s *SQLiteStorage) runStatusDataMigration() error {
	ctx := context.Background()

	if err := s.ensureDataMigrationsTable(ctx); err != nil {
		return fmt.Errorf("status-migration: ensure data_migrations: %w", err)
	}

	applied, err := s.dataMigrationApplied(ctx, dataMigrationStatusV1)
	if err != nil {
		return fmt.Errorf("status-migration: check marker: %w", err)
	}
	if applied {
		return nil
	}

	// Detect: zero old-label rows -> mark applied & exit silently. This
	// keeps the marker honest (means: this DB is known free of old labels)
	// and skips all backup/migration work on every future invocation.
	count, err := s.countOldStatusRows(ctx)
	if err != nil {
		return fmt.Errorf("status-migration: detect: %w", err)
	}
	if count == 0 {
		return s.recordDataMigration(ctx, dataMigrationStatusV1)
	}

	if s.dbPath == "" || s.dbPath == ":memory:" {
		// Cannot back up an in-memory or path-less DB. Run the rewrite
		// without backup, but only because there is no on-disk state to
		// protect (test path).
		return s.applyStatusRewrite(ctx)
	}

	backupPath, err := statusBackupFn(s.dbPath)
	if err != nil {
		return fmt.Errorf("status-migration: backup failed (DB unchanged): %w", err)
	}
	if err := verifyBackupIntegrity(backupPath); err != nil {
		// Best-effort cleanup: a corrupt backup is worse than no backup.
		_ = os.Remove(backupPath)
		return fmt.Errorf("status-migration: backup integrity check failed: %w", err)
	}

	if err := s.applyStatusRewrite(ctx); err != nil {
		return fmt.Errorf("status-migration: rewrite failed (backup at %s): %w", backupPath, err)
	}

	if err := s.recordDataMigration(ctx, dataMigrationStatusV1); err != nil {
		return fmt.Errorf("status-migration: record marker (rewrite committed; backup at %s): %w", backupPath, err)
	}

	fmt.Fprintf(os.Stderr,
		"tlc: migrated %d task statuses (TODO->pending, IN_PROGRESS->doing, DONE->done, SKIPPED->skipped); backup at %s\n",
		count, backupPath,
	)
	return nil
}

func (s *SQLiteStorage) ensureDataMigrationsTable(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS data_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create data_migrations: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) dataMigrationApplied(ctx context.Context, name string) (bool, error) {
	var got string
	err := s.db.QueryRowContext(ctx,
		`SELECT name FROM data_migrations WHERE name = ?`, name,
	).Scan(&got)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("scan: %w", err)
}

func (s *SQLiteStorage) recordDataMigration(ctx context.Context, name string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO data_migrations (name, applied_at) VALUES (?, ?)`,
		name, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("insert marker: %w", err)
	}
	return nil
}

func (s *SQLiteStorage) countOldStatusRows(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM tasks WHERE status IN ('TODO','IN_PROGRESS','DONE','SKIPPED')`,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return n, nil
}

// applyStatusRewrite runs the rewrite UPDATE inside a single transaction.
// On any error the transaction is rolled back, leaving rows unchanged.
func (s *SQLiteStorage) applyStatusRewrite(ctx context.Context) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE tasks SET status = CASE status
			WHEN 'TODO'        THEN 'pending'
			WHEN 'IN_PROGRESS' THEN 'doing'
			WHEN 'DONE'        THEN 'done'
			WHEN 'SKIPPED'     THEN 'skipped'
			ELSE status
		END
		WHERE status IN ('TODO','IN_PROGRESS','DONE','SKIPPED')
	`)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("rewrite failed: %v; rollback failed: %w", err, rbErr)
		}
		return fmt.Errorf("rewrite update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// backupForStatusMigration copies the DB file at dbPath to a sibling backup
// named <basename>.pre-status-migration.<UTC-timestamp>.bak and returns the
// backup path. Performs a WAL checkpoint first so the .bak is self-contained.
//
// Reuses the established kit/sqlstore backup pattern (file copy + WAL
// checkpoint) but with the spec-mandated `pre-status-migration` suffix
// (kit's BackupBeforeMigrate hardcodes `pre-v<N>` for schema migrations).
func backupForStatusMigration(dbPath string) (string, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return "", fmt.Errorf("stat db: %w", err)
	}

	// Best-effort WAL checkpoint so all committed pages are in the main
	// DB file before we copy. Errors are non-fatal: the file might not
	// be in WAL mode or might be inaccessible — copy still proceeds.
	if db, err := sql.Open("sqlite", dbPath); err == nil {
		_, _ = db.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)") //nolint:errcheck // best-effort checkpoint
		_ = db.Close()
	}

	ts := time.Now().UTC().Format("20060102-150405")
	base := filepath.Base(dbPath)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	backupName := fmt.Sprintf("%s.pre-status-migration.%s.bak", stem, ts)
	dbDir := filepath.Dir(dbPath)
	backupDir := filepath.Join(dbDir, backupSubdir)
	if err := os.MkdirAll(backupDir, 0o750); err != nil {
		return "", fmt.Errorf("mkdir backup dir %s: %w", backupDir, err)
	}
	backupPath := filepath.Join(backupDir, backupName)

	if err := copyFileWithSync(dbPath, backupPath); err != nil {
		// Cleanup partial file so we never leave a truncated .bak behind.
		_ = os.Remove(backupPath)
		return "", err
	}

	srcInfo, err := os.Stat(dbPath)
	if err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("stat src after copy: %w", err)
	}
	dstInfo, err := os.Stat(backupPath)
	if err != nil {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("stat backup: %w", err)
	}
	if srcInfo.Size() != dstInfo.Size() {
		_ = os.Remove(backupPath)
		return "", fmt.Errorf("backup size mismatch: src=%d backup=%d", srcInfo.Size(), dstInfo.Size())
	}
	return backupPath, nil
}

// verifyBackupIntegrity opens the backup file as a SQLite DB and runs
// PRAGMA integrity_check. Returns nil iff the result is "ok".
func verifyBackupIntegrity(backupPath string) error {
	db, err := sql.Open("sqlite", backupPath)
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer func() { _ = db.Close() }()
	var result string
	if err := db.QueryRowContext(context.Background(), `PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("integrity_check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check returned %q", result)
	}
	return nil
}

// copyFileWithSync copies src to dst and fsyncs dst before closing.
// Returns an error if any step fails.
func copyFileWithSync(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy bytes: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("fsync backup: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close backup: %w", err)
	}
	return nil
}

// openStorageWithoutMigration is a test helper that opens the DB without
// running schema or data migrations. Used by tests that need to inspect
// post-failure state without re-triggering the migration path.
func openStorageWithoutMigration(path string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("fk on: %w", err)
	}
	return &SQLiteStorage{db: db, dbPath: path}, nil
}
