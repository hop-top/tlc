package storage

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"hop.top/tlc/internal/core"
)

// TestMigrationV20_FreshDBHasTrackSeqSchema verifies a database opened at
// HEAD carries the v20 shape: a track_sequences counter table mirroring
// task_sequences, and a tracks.seq column.
func TestMigrationV20_FreshDBHasTrackSeqSchema(t *testing.T) {
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
	if version < 20 {
		t.Fatalf("schema version = %d; want >= 20", version)
	}

	// track_sequences exists with the same (project_id, next_id) shape
	// as task_sequences.
	var nextID int
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO track_sequences (project_id, next_id) VALUES ('proj-a', 7)
	`); err != nil {
		t.Fatalf("insert into track_sequences: %v", err)
	}
	if err := s.db.QueryRowContext(
		ctx, `SELECT next_id FROM track_sequences WHERE project_id = 'proj-a'`,
	).Scan(&nextID); err != nil {
		t.Fatalf("read track_sequences: %v", err)
	}
	if nextID != 7 {
		t.Errorf("next_id = %d; want 7", nextID)
	}

	// project_id is the primary key: a duplicate row must be rejected.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO track_sequences (project_id, next_id) VALUES ('proj-a', 9)
	`); err == nil {
		t.Error("duplicate project_id accepted; want PRIMARY KEY violation")
	}

	// tracks.seq column exists.
	var seq sql.NullInt64
	rows, err := s.db.QueryContext(ctx, `SELECT seq FROM tracks LIMIT 0`)
	if err != nil {
		t.Fatalf("select tracks.seq: %v", err)
	}
	defer rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("scan tracks.seq: %v", err)
	}
	_ = seq
}

// openAtV19 builds an in-memory database migrated to exactly v19 — the
// schema head immediately before track sequences landed.
func openAtV19(t *testing.T) *sql.DB {
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
		if m.version > 19 {
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

// applyV20 runs the v20 migration against an already-open v19 database.
func applyV20(t *testing.T, db *sql.DB) {
	t.Helper()

	for _, m := range migrations {
		if m.version != 20 {
			continue
		}
		if _, err := db.ExecContext(context.Background(), m.query); err != nil {
			t.Fatalf("apply migration v20: %v", err)
		}
		return
	}
	t.Fatal("migration v20 not found")
}

// TestMigrationV20_BackfillsExistingTracksPerProject seeds a v19 database
// with tracks across two projects (plus the empty-project bucket), then
// upgrades and asserts seq is backfilled in created_at order, scoped per
// project, starting at 1.
//
// Migration v13 once dropped every existing row in this schema's history;
// this test pins row preservation explicitly so a backfill can never
// repeat that.
func TestMigrationV20_BackfillsExistingTracksPerProject(t *testing.T) {
	db := openAtV19(t)
	ctx := context.Background()

	seed := []struct {
		id        string
		slug      string
		projectID string
		createdAt string
	}{
		// proj-a, deliberately inserted out of created_at order so the
		// backfill cannot pass by accident on insertion order.
		{"track_a2", "a-second", "proj-a", "2026-01-02T00:00:00Z"},
		{"track_a1", "a-first", "proj-a", "2026-01-01T00:00:00Z"},
		{"track_a3", "a-third", "proj-a", "2026-01-03T00:00:00Z"},
		// proj-b: independent counter.
		{"track_b1", "b-first", "proj-b", "2026-02-01T00:00:00Z"},
		{"track_b2", "b-second", "proj-b", "2026-02-02T00:00:00Z"},
		// empty-project bucket.
		{"track_g1", "g-first", "", "2026-03-01T00:00:00Z"},
	}
	for _, s := range seed {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO tracks (id, slug, project_id, title, type, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'feature', 'pending', ?, ?)
		`, s.id, s.slug, s.projectID, s.slug, s.createdAt, s.createdAt); err != nil {
			t.Fatalf("seed track %s: %v", s.id, err)
		}
	}

	before := trackSlugSet(t, db)
	if len(before) != len(seed) {
		t.Fatalf("seeded %d tracks, read back %d", len(seed), len(before))
	}

	applyV20(t, db)

	// Data-preservation invariant: every row and slug survives.
	after := trackSlugSet(t, db)
	if len(after) != len(before) {
		t.Fatalf("track row count changed across migration: %d -> %d", len(before), len(after))
	}
	for slug := range before {
		if _, ok := after[slug]; !ok {
			t.Errorf("track %q lost across migration", slug)
		}
	}

	// Per-project backfill in created_at order, starting at 1.
	wantSeq := map[string]int64{
		"a-first": 1, "a-second": 2, "a-third": 3,
		"b-first": 1, "b-second": 2,
		"g-first": 1,
	}
	for slug, want := range wantSeq {
		var got sql.NullInt64
		if err := db.QueryRowContext(
			ctx, `SELECT seq FROM tracks WHERE slug = ?`, slug,
		).Scan(&got); err != nil {
			t.Fatalf("read seq for %s: %v", slug, err)
		}
		if !got.Valid {
			t.Errorf("%s: seq is NULL; want %d", slug, want)
			continue
		}
		if got.Int64 != want {
			t.Errorf("%s: seq = %d; want %d", slug, got.Int64, want)
		}
	}

	// Counters seeded past the highest backfilled value per project.
	// The empty-project bucket normalizes to "default", mirroring
	// task_sequences.
	wantNext := map[string]int{"proj-a": 4, "proj-b": 3, "default": 2}
	for projectID, want := range wantNext {
		var got int
		if err := db.QueryRowContext(
			ctx, `SELECT next_id FROM track_sequences WHERE project_id = ?`, projectID,
		).Scan(&got); err != nil {
			t.Fatalf("read track_sequences for %q: %v", projectID, err)
		}
		if got != want {
			t.Errorf("track_sequences[%q].next_id = %d; want %d", projectID, got, want)
		}
	}
}

// trackSlugSet reads the slug of every track row.
func trackSlugSet(t *testing.T, db *sql.DB) map[string]struct{} {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), `SELECT slug FROM tracks`)
	if err != nil {
		t.Fatalf("query tracks: %v", err)
	}
	defer rows.Close()

	out := map[string]struct{}{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			t.Fatalf("scan slug: %v", err)
		}
		out[slug] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tracks: %v", err)
	}
	return out
}

// TestMigrationV20_AllocAfterBackfillDoesNotCollide pins the seam between
// the backfill and the runtime allocator: the first seq handed out after
// an upgrade must sit past every backfilled value.
func TestMigrationV20_AllocAfterBackfillDoesNotCollide(t *testing.T) {
	db := openAtV19(t)
	ctx := context.Background()

	for i, slug := range []string{"legacy-one", "legacy-two", "legacy-three"} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO tracks (id, slug, project_id, title, type, status, created_at, updated_at)
			VALUES (?, ?, 'proj-a', ?, 'feature', 'pending', ?, ?)
		`, "track_legacy_"+slug, slug, slug,
			[]string{"2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", "2026-01-03T00:00:00Z"}[i],
			"2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}

	applyV20(t, db)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	got, err := allocTrackSeqInTx(ctx, tx, "proj-a")
	if err != nil {
		t.Fatalf("allocTrackSeqInTx: %v", err)
	}
	if got != 4 {
		t.Errorf("first alloc after backfill = %d; want 4 (past the 3 backfilled rows)", got)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// No collision with an existing row.
	var n int
	if err := db.QueryRowContext(
		ctx, `SELECT count(*) FROM tracks WHERE project_id = 'proj-a' AND seq = ?`, got,
	).Scan(&n); err != nil {
		t.Fatalf("collision check: %v", err)
	}
	if n != 0 {
		t.Errorf("allocated seq %d collides with %d existing track row(s)", got, n)
	}
}

// TestAllocTrackSeqInTx_IndependentPerProject verifies each project gets
// its own monotonic counter starting at 1, and that the empty project
// normalizes to the "default" bucket (mirroring allocSeqInTx).
func TestAllocTrackSeqInTx_IndependentPerProject(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	alloc := func(projectID string) int {
		t.Helper()
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("BeginTx: %v", err)
		}
		n, err := allocTrackSeqInTx(ctx, tx, projectID)
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("allocTrackSeqInTx(%q): %v", projectID, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		return n
	}

	// Two projects allocate independently, each starting at 1.
	for i, want := range []int{1, 2, 3} {
		if got := alloc("proj-a"); got != want {
			t.Errorf("proj-a alloc #%d = %d; want %d", i+1, got, want)
		}
	}
	for i, want := range []int{1, 2} {
		if got := alloc("proj-b"); got != want {
			t.Errorf("proj-b alloc #%d = %d; want %d", i+1, got, want)
		}
	}
	// proj-a keeps counting from where it left off.
	if got := alloc("proj-a"); got != 4 {
		t.Errorf("proj-a alloc after proj-b activity = %d; want 4", got)
	}

	// Empty project stores under "default" but counts independently.
	if got := alloc(""); got != 1 {
		t.Errorf("empty-project alloc = %d; want 1", got)
	}
	var next int
	if err := s.db.QueryRowContext(
		ctx, `SELECT next_id FROM track_sequences WHERE project_id = 'default'`,
	).Scan(&next); err != nil {
		t.Fatalf("read default bucket: %v", err)
	}
	if next != 2 {
		t.Errorf("default bucket next_id = %d; want 2", next)
	}

	// Track sequences must not share state with task sequences.
	var taskRows int
	if err := s.db.QueryRowContext(
		ctx, `SELECT count(*) FROM task_sequences`,
	).Scan(&taskRows); err != nil {
		t.Fatalf("count task_sequences: %v", err)
	}
	if taskRows != 0 {
		t.Errorf("task_sequences has %d row(s); track allocation must not touch it", taskRows)
	}
}

// TestCreateTrack_AllocatesSeq verifies the repository write path assigns
// a per-project sequence number to every new track.
func TestCreateTrack_AllocatesSeq(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	for i, slug := range []string{"seq-one", "seq-two", "seq-three"} {
		if err := s.CreateTrack(ctx, newSeqTestTrack(slug)); err != nil {
			t.Fatalf("CreateTrack(%s): %v", slug, err)
		}

		var seq sql.NullInt64
		if err := s.db.QueryRowContext(
			ctx, `SELECT seq FROM tracks WHERE slug = ?`, slug,
		).Scan(&seq); err != nil {
			t.Fatalf("read seq for %s: %v", slug, err)
		}
		if !seq.Valid {
			t.Fatalf("%s: seq is NULL; want %d", slug, i+1)
		}
		if seq.Int64 != int64(i+1) {
			t.Errorf("%s: seq = %d; want %d", slug, seq.Int64, i+1)
		}
	}
}

// newSeqTestTrack builds a minimal valid track for sequence assertions.
func newSeqTestTrack(slug string) *core.Track {
	now := time.Now().UTC().Truncate(time.Second)
	return &core.Track{
		ID:        slug,
		Slug:      slug,
		Title:     slug,
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
