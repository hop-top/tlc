package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateOldBackups verifies the one-time bootstrap moves stray
// db.*.bak files into <dbDir>/.dbs/ and is idempotent.
func TestMigrateOldBackups(t *testing.T) {
	dbDir := t.TempDir()

	// Plant two legacy backups + one unrelated file.
	legacy := []string{
		"db.test.bak",
		"db.pre-v13.20260502-151820.bak",
	}
	for _, name := range legacy {
		p := filepath.Join(dbDir, name)
		if err := os.WriteFile(p, []byte("payload-"+name), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	keep := filepath.Join(dbDir, "db.sqlite")
	if err := os.WriteFile(keep, []byte("live"), 0o600); err != nil {
		t.Fatalf("seed live db: %v", err)
	}

	migrateOldBackups(dbDir)

	dst := filepath.Join(dbDir, backupSubdir)
	for _, name := range legacy {
		final := filepath.Join(dst, name)
		if _, err := os.Stat(final); err != nil {
			t.Errorf("expected backup at %s: %v", final, err)
		}
		old := filepath.Join(dbDir, name)
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Errorf("expected old %s gone, stat err=%v", old, err)
		}
	}
	// Live DB untouched.
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("live db should remain at %s: %v", keep, err)
	}

	// Idempotent: second run is a no-op (no files to move, no error).
	migrateOldBackups(dbDir)
	for _, name := range legacy {
		final := filepath.Join(dst, name)
		if _, err := os.Stat(final); err != nil {
			t.Errorf("after second run, expected backup at %s: %v", final, err)
		}
	}
}

// TestMigrateOldBackups_NoOp ensures the bootstrap is cheap (and silent)
// when the dir is empty or non-existent.
func TestMigrateOldBackups_NoOp(t *testing.T) {
	// Empty dir: no .dbs/ should be created.
	dbDir := t.TempDir()
	migrateOldBackups(dbDir)
	if _, err := os.Stat(filepath.Join(dbDir, backupSubdir)); !os.IsNotExist(err) {
		t.Errorf("expected no .dbs/ for empty dir, got err=%v", err)
	}

	// Missing dir: should not panic or error.
	migrateOldBackups(filepath.Join(dbDir, "does-not-exist"))
}

// TestStashBackup verifies stashBackup relocates a kit-emitted backup
// into <dbDir>/.dbs/ and tolerates an empty src (first-run case).
func TestStashBackup(t *testing.T) {
	dbDir := t.TempDir()

	// Empty src: no-op.
	if final, err := stashBackup(dbDir, ""); err != nil || final != "" {
		t.Fatalf("empty src: got (%q, %v), want (\"\", nil)", final, err)
	}

	// Plant a fake kit-emitted backup beside the DB.
	src := filepath.Join(dbDir, "db.pre-v1.20260101-000000.bak")
	if err := os.WriteFile(src, []byte("backup"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	final, err := stashBackup(dbDir, src)
	if err != nil {
		t.Fatalf("stashBackup: %v", err)
	}
	want := filepath.Join(dbDir, backupSubdir, "db.pre-v1.20260101-000000.bak")
	if final != want {
		t.Errorf("final = %q, want %q", final, want)
	}
	if _, err := os.Stat(final); err != nil {
		t.Errorf("expected file at %s: %v", final, err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected src %s gone, stat err=%v", src, err)
	}
}
