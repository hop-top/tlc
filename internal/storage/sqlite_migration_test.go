package storage

import (
	"testing"
)

// TestSQLiteStorage_MigrationV2 verifies schema version is at the latest version after migration.
func TestSQLiteStorage_MigrationV2(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	version, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion failed: %v", err)
	}
	if version != LatestMigrationVersion {
		t.Errorf("expected schema version %d, got %d", LatestMigrationVersion, version)
	}
}
