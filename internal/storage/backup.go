package storage

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// backupSubdir is the hidden directory beside the SQLite DB where rotated
// pre-migration backups land. Keeps the parent .tlc/ scannable.
const backupSubdir = ".dbs"

// stashBackup moves a backup file dropped by the kit primitive into the
// hidden <dbDir>/.dbs/ subdir. Returns the new path. If srcPath is empty
// (no backup taken — first-run case) it is a no-op returning ("", nil).
func stashBackup(dbDir, srcPath string) (string, error) {
	if srcPath == "" {
		return "", nil
	}
	dst := filepath.Join(dbDir, backupSubdir)
	if err := os.MkdirAll(dst, 0o750); err != nil {
		return "", fmt.Errorf("backup: mkdir %s: %w", dst, err)
	}
	final := filepath.Join(dst, filepath.Base(srcPath))
	if err := os.Rename(srcPath, final); err != nil {
		return "", fmt.Errorf("backup: rename %s -> %s: %w", srcPath, final, err)
	}
	return final, nil
}

// migrateOldBackups is a one-time bootstrap that relocates pre-existing
// db.*.bak files (dropped beside the live DB by older tlc versions) into
// <dbDir>/.dbs/. Idempotent: zero-cost when no old backups present.
//
// Match rule: any regular file in dbDir whose name ends with ".bak". The
// kit primitive's naming is "<base>.pre-v<N>.<ts>.bak" so the .bak suffix
// is sufficient and avoids missing legacy variants.
func migrateOldBackups(dbDir string) {
	entries, err := os.ReadDir(dbDir)
	if err != nil {
		// dbDir may not exist yet (first run) — nothing to migrate.
		return
	}
	dst := filepath.Join(dbDir, backupSubdir)
	moved := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".bak") {
			continue
		}
		if moved == 0 {
			if err := os.MkdirAll(dst, 0o750); err != nil {
				log.Printf("migrateOldBackups: mkdir %s: %v", dst, err)
				return
			}
		}
		src := filepath.Join(dbDir, name)
		final := filepath.Join(dst, name)
		if err := os.Rename(src, final); err != nil {
			log.Printf("migrateOldBackups: rename %s -> %s: %v", src, final, err)
			continue
		}
		moved++
	}
	if moved > 0 {
		log.Printf("migrated %d backups to %s/", moved, backupSubdir)
	}
}
