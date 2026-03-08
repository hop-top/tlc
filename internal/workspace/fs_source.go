package workspace

import (
	"fmt"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// FilesystemSource provides task access for a local SQLite project database.
type FilesystemSource struct {
	dbPath string
	store  *storage.SQLiteStorage
}

// NewFilesystemSource creates a source for the given database path.
func NewFilesystemSource(dbPath string) *FilesystemSource {
	return &FilesystemSource{dbPath: dbPath}
}

// OpenReadOnly opens the SQLite database in read-only mode and returns
// a TaskReader handle.
func (s *FilesystemSource) OpenReadOnly() (core.TaskReader, error) {
	dsn := s.dbPath + "?mode=ro"
	store, err := storage.NewSQLiteStorage(dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s read-only: %w", s.dbPath, err)
	}
	s.store = store
	return store, nil
}

// Close releases the database connection.
func (s *FilesystemSource) Close() error {
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}
