package storage

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestTaskTemporalFields_UTCRoundTrip is the T-1383 regression. A Task
// whose DueAt/RemindAt are set with a non-UTC location must:
//   - serialise to the database as RFC3339 ending in "Z" (the
//     TemporalSpec §4 contract; lexicographic ordering on TEXT
//     indexes depends on every value carrying the same offset)
//   - round-trip back into Go as a UTC time.Time
//   - preserve the absolute instant (Equal())
//
// Same invariant for CreatedAt / UpdatedAt / LastSyncAt / StaleFiredAt.
func TestTaskTemporalFields_UTCRoundTrip(t *testing.T) {
	dbPath := "test_task_utc_round_trip.db"
	t.Cleanup(func() { _ = os.Remove(dbPath) })

	storage, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer storage.Close()

	ctx := context.Background()

	// Pick a non-UTC zone with a fixed offset (DST-free) so the
	// assertion holds independent of when the test runs.
	edt := time.FixedZone("EDT", -4*3600)
	dueAt := time.Date(2026, 6, 15, 10, 0, 0, 0, edt)
	remindAt := time.Date(2026, 6, 15, 9, 0, 0, 0, edt)
	lastSyncAt := time.Date(2026, 6, 14, 23, 30, 0, 0, edt)
	staleFiredAt := time.Date(2026, 6, 14, 22, 0, 0, 0, edt)

	task := &core.Task{
		ID:           "task_test_utc_roundtrip",
		Seq:          1,
		Title:        "UTC round trip",
		Status:       core.StatusTodo,
		CreatedAt:    time.Date(2026, 6, 14, 18, 0, 0, 0, edt),
		UpdatedAt:    time.Date(2026, 6, 14, 18, 0, 0, 0, edt),
		DueAt:        &dueAt,
		RemindAt:     &remindAt,
		LastSyncAt:   &lastSyncAt,
		StaleFiredAt: &staleFiredAt,
	}

	if err := storage.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// (1) Stored SQL value: every temporal column must end in "Z".
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer rawDB.Close()

	var dueStr, remindStr, lastSyncStr, staleFiredStr, createdStr, updatedStr string
	err = rawDB.QueryRowContext(
		ctx,
		`SELECT due_at, remind_at, last_sync_at, stale_fired_at, created_at, updated_at
		 FROM tasks WHERE id = ?`,
		task.ID,
	).Scan(&dueStr, &remindStr, &lastSyncStr, &staleFiredStr, &createdStr, &updatedStr)
	if err != nil {
		t.Fatalf("raw scan: %v", err)
	}

	for name, got := range map[string]string{
		"due_at":         dueStr,
		"remind_at":      remindStr,
		"last_sync_at":   lastSyncStr,
		"stale_fired_at": staleFiredStr,
		"created_at":     createdStr,
		"updated_at":     updatedStr,
	} {
		if !strings.HasSuffix(got, "Z") {
			t.Errorf("%s stored as %q; want suffix Z (UTC RFC3339)", name, got)
		}
		if strings.Contains(got, "-04:00") || strings.Contains(got, "+") {
			t.Errorf("%s stored with non-Z offset: %q", name, got)
		}
	}

	// (2) Round-tripped time.Time.Location() must be time.UTC.
	// Use GetTaskInProject with empty project_id to bypass auto-detect
	// — the test seeds a task without a project context.
	got, err := storage.GetTaskInProject(ctx, task.ID, "")
	if err != nil {
		t.Fatalf("GetTaskInProject: %v", err)
	}
	if got == nil {
		t.Fatal("GetTaskInProject returned nil")
	}

	fields := map[string]*time.Time{
		"DueAt":        got.DueAt,
		"RemindAt":     got.RemindAt,
		"LastSyncAt":   got.LastSyncAt,
		"StaleFiredAt": got.StaleFiredAt,
	}
	for name, p := range fields {
		if p == nil {
			t.Errorf("%s round-tripped as nil", name)
			continue
		}
		if p.Location() != time.UTC {
			t.Errorf("%s.Location() = %v; want time.UTC", name, p.Location())
		}
	}
	if got.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt.Location() = %v; want time.UTC", got.CreatedAt.Location())
	}
	if got.UpdatedAt.Location() != time.UTC {
		t.Errorf("UpdatedAt.Location() = %v; want time.UTC", got.UpdatedAt.Location())
	}

	// (3) Absolute instants must be preserved.
	pairs := []struct {
		name string
		want time.Time
		got  *time.Time
	}{
		{"DueAt", dueAt, got.DueAt},
		{"RemindAt", remindAt, got.RemindAt},
		{"LastSyncAt", lastSyncAt, got.LastSyncAt},
		{"StaleFiredAt", staleFiredAt, got.StaleFiredAt},
	}
	for _, p := range pairs {
		if p.got == nil {
			continue
		}
		if !p.got.Equal(p.want) {
			t.Errorf("%s instant lost: got %v, want %v", p.name, *p.got, p.want)
		}
	}
}
