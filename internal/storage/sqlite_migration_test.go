package storage

import (
	"context"
	"database/sql"
	"errors"
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

// TestSQLiteStorage_TracksTable verifies the tracks table created by
// migration v13 accepts inserts and exposes the expected columns and
// indexes. (Originally written for v6; v13 supersedes that schema with
// the typeid PK and the slug NOT NULL alias column.)
func TestSQLiteStorage_MigrationV6_TracksTable(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Verify tracks table exists by inserting a row.
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tracks (id, slug, title, type, status, created_at, updated_at)
		VALUES ('track_01h455vbqkfsn02nk084ksn02q', 'test-track',
		        'Test Track', 'feature', 'pending', '2026-01-01', '2026-01-01')
	`)
	if err != nil {
		t.Fatalf("tracks table insert failed: %v", err)
	}

	// Verify track_id column exists on tasks by querying it.
	var trackID *string
	err = s.db.QueryRowContext(ctx, `
		SELECT track_id FROM tasks LIMIT 0
	`).Scan(&trackID)
	// sql.ErrNoRows is expected — column exists but no rows.
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("track_id column query failed: %v", err)
	}

	// Verify indexes exist.
	indexes := []string{
		"idx_tracks_status",
		"idx_tracks_project_id",
		"idx_tasks_track_id",
	}
	for _, idx := range indexes {
		var name string
		err = s.db.QueryRowContext(
			ctx,
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected index %q to exist: %v", idx, err)
		}
	}
}
