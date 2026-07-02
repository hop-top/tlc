package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func newTrackTestStorage(t *testing.T) *SQLiteStorage {
	t.Helper()
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	t.Cleanup(func() {
		os.Chdir(oldCwd)
		resetProjectDetection()
	})

	dbPath := filepath.Join(tmpDir, "test_track.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSQLiteStorage_TrackCRUD(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	track := &core.Track{
		ID:        "feat-login",
		Slug:      "feat-login",
		Title:     "Login Feature",
		Type:      core.TrackTypeFeature,
		Status:    core.TrackStatusPending,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
		Meta:      map[string]any{"team": "auth"},
	}

	// Create
	if err := s.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack failed: %v", err)
	}

	// Get
	got, err := s.GetTrack(ctx, "feat-login")
	if err != nil {
		t.Fatalf("GetTrack failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected track, got nil")
	}
	if got.Title != "Login Feature" {
		t.Errorf("expected title %q, got %q", "Login Feature", got.Title)
	}
	if got.Type != core.TrackTypeFeature {
		t.Errorf("expected type %q, got %q", core.TrackTypeFeature, got.Type)
	}
	if got.Status != core.TrackStatusPending {
		t.Errorf("expected status %q, got %q", core.TrackStatusPending, got.Status)
	}
	if got.Meta["team"] != "auth" {
		t.Errorf("expected meta team=auth, got %v", got.Meta["team"])
	}

	// Update
	got.Status = core.TrackStatusActive
	got.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.UpdateTrack(ctx, got); err != nil {
		t.Fatalf("UpdateTrack failed: %v", err)
	}
	got2, _ := s.GetTrack(ctx, "feat-login")
	if got2.Status != core.TrackStatusActive {
		t.Errorf("expected status %q, got %q", core.TrackStatusActive, got2.Status)
	}

	// Delete
	if err := s.DeleteTrack(ctx, "feat-login"); err != nil {
		t.Fatalf("DeleteTrack failed: %v", err)
	}
	got3, err := s.GetTrack(ctx, "feat-login")
	if err != nil {
		t.Fatalf("GetTrack after delete failed: %v", err)
	}
	if got3 != nil {
		t.Errorf("expected nil after delete, got %+v", got3)
	}
}

func TestSQLiteStorage_TrackGetNotFound(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	got, err := s.GetTrack(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetTrack unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent track, got %+v", got)
	}
}

func TestSQLiteStorage_TrackUpdateNotFound(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	track := &core.Track{
		ID:        "ghost",
		Slug:      "ghost",
		Title:     "Ghost",
		Type:      "feature",
		Status:    core.TrackStatusPending,
		UpdatedAt: time.Now().UTC(),
	}
	err := s.UpdateTrack(ctx, track)
	if err == nil {
		t.Fatal("expected error updating nonexistent track")
	}
}

func TestSQLiteStorage_TrackDeleteNotFound(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	err := s.DeleteTrack(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent track")
	}
}

func TestSQLiteStorage_TrackListFilters(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	tracks := []*core.Track{
		{
			ID: "t1", Slug: "t1", Title: "T1", Type: "feature", Status: core.TrackStatusPending,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "t2", Slug: "t2", Title: "T2", Type: "bug", Status: core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "t3", Slug: "t3", Title: "T3", Type: "feature", Status: core.TrackStatusCompleted,
			CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, tr := range tracks {
		if err := s.CreateTrack(ctx, tr); err != nil {
			t.Fatalf("CreateTrack failed: %v", err)
		}
	}

	// Filter by status
	result, err := s.ListTracks(ctx, core.TrackQuery{
		Status: []core.TrackStatus{core.TrackStatusPending, core.TrackStatusActive},
	})
	if err != nil {
		t.Fatalf("ListTracks by status failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(result))
	}

	// Filter by type
	result, err = s.ListTracks(ctx, core.TrackQuery{Type: "bug"})
	if err != nil {
		t.Fatalf("ListTracks by type failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 track, got %d", len(result))
	}
	if result[0].ID != "t2" {
		t.Errorf("expected t2, got %s", result[0].ID)
	}

	// Limit and offset
	result, err = s.ListTracks(ctx, core.TrackQuery{Limit: 1})
	if err != nil {
		t.Fatalf("ListTracks with limit failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 track with limit, got %d", len(result))
	}

	result, err = s.ListTracks(ctx, core.TrackQuery{Limit: 10, Offset: 2})
	if err != nil {
		t.Fatalf("ListTracks with offset failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 track with offset, got %d", len(result))
	}
}

func TestSQLiteStorage_TrackListByProjectID(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	pid := "proj-a"
	tracks := []*core.Track{
		{
			ID: "t1", Slug: "t1", Title: "T1", Type: "feature", Status: core.TrackStatusPending,
			CreatedAt: now, UpdatedAt: now, ProjectID: &pid,
		},
		{
			ID: "t2", Slug: "t2", Title: "T2", Type: "feature", Status: core.TrackStatusPending,
			CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, tr := range tracks {
		if err := s.CreateTrack(ctx, tr); err != nil {
			t.Fatalf("CreateTrack failed: %v", err)
		}
	}

	result, err := s.ListTracks(ctx, core.TrackQuery{ProjectID: &pid})
	if err != nil {
		t.Fatalf("ListTracks by project_id failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 track for project, got %d", len(result))
	}
}

func TestSQLiteStorage_TrackDeleteWithLinkedTasks(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	track := &core.Track{
		ID: "feat-x", Slug: "feat-x", Title: "X", Type: "feature",
		Status: core.TrackStatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack failed: %v", err)
	}

	// Create a task linked to this track via track_id column.
	task := &core.Task{
		ID:        "T-0099",
		Title:     "Linked task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	// Link task to track via direct SQL (track_id column).
	_, err := s.db.ExecContext(ctx,
		"UPDATE tasks SET track_id = ? WHERE id = ?", "feat-x", "T-0099")
	if err != nil {
		t.Fatalf("failed to link task to track: %v", err)
	}

	// Delete should fail because of linked task.
	err = s.DeleteTrack(ctx, "feat-x")
	if err == nil {
		t.Fatal("expected error deleting track with linked tasks")
	}

	// Unlink and retry.
	_, err = s.db.ExecContext(ctx,
		"UPDATE tasks SET track_id = NULL WHERE id = ?", "T-0099")
	if err != nil {
		t.Fatalf("failed to unlink task: %v", err)
	}
	if err := s.DeleteTrack(ctx, "feat-x"); err != nil {
		t.Fatalf("DeleteTrack after unlinking failed: %v", err)
	}
}

func TestSQLiteStorage_TrackAssignedTo(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	assignee := "agent-1"
	track := &core.Track{
		ID: "t-assigned", Slug: "t-assigned", Title: "Assigned Track", Type: "feature",
		Status: core.TrackStatusPending, CreatedAt: now, UpdatedAt: now,
		AssignedTo: &assignee,
	}
	if err := s.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack failed: %v", err)
	}

	got, _ := s.GetTrack(ctx, "t-assigned")
	if got.AssignedTo == nil || *got.AssignedTo != "agent-1" {
		t.Errorf("expected assigned_to=agent-1, got %v", got.AssignedTo)
	}
}

func TestSQLiteStorage_TrackNilMeta(t *testing.T) {
	s := newTrackTestStorage(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	track := &core.Track{
		ID: "no-meta", Slug: "no-meta", Title: "No Meta", Type: "bug",
		Status: core.TrackStatusPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack failed: %v", err)
	}

	got, _ := s.GetTrack(ctx, "no-meta")
	if got == nil {
		t.Fatal("expected track, got nil")
	}
	// Meta should be nil or empty map — either is fine.
}
