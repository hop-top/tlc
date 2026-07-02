package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestMigrationV18_TracksDueAt verifies the v18 schema migration adds
// the due_at column and idx_tracks_due_at index to the tracks table,
// and that the column round-trips RFC3339 UTC strings as designed
// (mirror of tasks.due_at from v12; spec: docs/temporal-spec-0.1.md §4).
func TestMigrationV18_TracksDueAt(t *testing.T) {
	// Use the track-test helper so project detection is detached from
	// the worktree (matches sqlite_track_test.go pattern).
	s := newTrackTestStorage(t)
	ctx := context.Background()

	// Schema is at HEAD after NewSQLiteStorage; assert HEAD >= 18
	// (v18 shipped or any later migration applied on top).
	version, err := s.SchemaVersion()
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version < 18 {
		t.Fatalf("expected schema version >= 18, got %d", version)
	}

	// Column exists (selecting it does not error).
	var dueAt sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT due_at FROM tracks LIMIT 0`).Scan(&dueAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("due_at column query failed: %v", err)
	}

	// Index exists.
	var name string
	err = s.db.QueryRowContext(
		ctx,
		"SELECT name FROM sqlite_master WHERE type='index' AND name=?",
		"idx_tracks_due_at",
	).Scan(&name)
	if err != nil {
		t.Errorf("expected index idx_tracks_due_at to exist: %v", err)
	}

	// Round-trip a track with DueAt set through the repository so we
	// catch any storage layer regression on top of the schema change.
	// Slug-keyed (not a TypeID) matches the TestSQLiteStorage_TrackCRUD
	// idiom so GetTrack hits the slug fallback under empty-project scope.
	due := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	track := &core.Track{
		ID:        "test-v18-a",
		Slug:      "test-v18-a",
		Title:     "v18 due_at round-trip",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusPending,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
		DueAt:     &due,
	}
	if err := s.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	got, err := s.GetTrack(ctx, "test-v18-a")
	if err != nil {
		t.Fatalf("GetTrack: %v", err)
	}
	if got == nil {
		t.Fatal("expected track, got nil")
	}
	if got.DueAt == nil {
		t.Fatal("expected DueAt set, got nil")
	}
	if !got.DueAt.Equal(due) {
		t.Errorf("expected DueAt %v, got %v", due, *got.DueAt)
	}

	// Track without DueAt: column is nullable.
	track2 := &core.Track{
		ID:        "test-v18-b",
		Slug:      "test-v18-b",
		Title:     "no due",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusPending,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := s.CreateTrack(ctx, track2); err != nil {
		t.Fatalf("CreateTrack (no due): %v", err)
	}
	got2, err := s.GetTrack(ctx, "test-v18-b")
	if err != nil {
		t.Fatalf("GetTrack (no due): %v", err)
	}
	if got2.DueAt != nil {
		t.Errorf("expected DueAt nil, got %v", *got2.DueAt)
	}

	// Update path: setting and clearing DueAt round-trips.
	next := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	got2.DueAt = &next
	got2.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTrack(ctx, got2); err != nil {
		t.Fatalf("UpdateTrack (set due): %v", err)
	}
	got2b, _ := s.GetTrack(ctx, "test-v18-b")
	if got2b.DueAt == nil || !got2b.DueAt.Equal(next) {
		t.Errorf("expected DueAt %v after set, got %v", next, got2b.DueAt)
	}

	got2b.DueAt = nil
	got2b.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTrack(ctx, got2b); err != nil {
		t.Fatalf("UpdateTrack (clear due): %v", err)
	}
	got2c, _ := s.GetTrack(ctx, "test-v18-b")
	if got2c.DueAt != nil {
		t.Errorf("expected DueAt nil after clear, got %v", *got2c.DueAt)
	}
}
