package core

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// stubTrackRepo is an in-memory TrackRepository for unit tests.
type stubTrackRepo struct {
	tracks map[string]*Track
}

func newStubTrackRepo() *stubTrackRepo {
	return &stubTrackRepo{tracks: make(map[string]*Track)}
}

func (r *stubTrackRepo) CreateTrack(_ context.Context, track *Track) error {
	if _, exists := r.tracks[track.ID]; exists {
		return fmt.Errorf("track %q already exists", track.ID)
	}
	cp := *track
	r.tracks[track.ID] = &cp
	return nil
}

func (r *stubTrackRepo) GetTrack(_ context.Context, id string) (*Track, error) {
	t, ok := r.tracks[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (r *stubTrackRepo) UpdateTrack(_ context.Context, track *Track) error {
	if _, ok := r.tracks[track.ID]; !ok {
		return fmt.Errorf("track %q not found", track.ID)
	}
	cp := *track
	r.tracks[track.ID] = &cp
	return nil
}

func (r *stubTrackRepo) DeleteTrack(_ context.Context, id string) error {
	if _, ok := r.tracks[id]; !ok {
		return fmt.Errorf("track %q not found", id)
	}
	delete(r.tracks, id)
	return nil
}

func (r *stubTrackRepo) ListTracks(
	_ context.Context,
	query TrackQuery,
) ([]*Track, error) {
	var result []*Track
	for _, t := range r.tracks {
		if len(query.Status) > 0 {
			match := false
			for _, s := range query.Status {
				if t.Status == s {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		if query.Type != "" && t.Type != query.Type {
			continue
		}
		cp := *t
		result = append(result, &cp)
	}
	return result, nil
}

// stubTaskRepo is a minimal Repository stub for track service tests.
type stubTaskRepo struct {
	tasks []*Task
}

func (r *stubTaskRepo) CreateTask(_ context.Context, _ *Task) error { return nil }
func (r *stubTaskRepo) GetTask(_ context.Context, _ string) (*Task, error) {
	return nil, nil
}
func (r *stubTaskRepo) UpdateTask(_ context.Context, _ *Task) error { return nil }
func (r *stubTaskRepo) UpdateTaskWithLog(_ context.Context, _ *Task, _ *LogEntry) error {
	return nil
}
func (r *stubTaskRepo) DeleteTask(_ context.Context, _ string) error { return nil }
func (r *stubTaskRepo) GetTasksNeedingPush(_ context.Context) ([]*Task, error) {
	return nil, nil
}
func (r *stubTaskRepo) FindTaskByOrigin(_ context.Context, _, _ string) (*Task, error) {
	return nil, nil
}
func (r *stubTaskRepo) ArchiveTasks(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}
func (r *stubTaskRepo) CreateFlowRun(_ context.Context, _ *FlowRun) error { return nil }
func (r *stubTaskRepo) GetFlowRun(_ context.Context, _ string) (*FlowRun, error) {
	return nil, nil
}
func (r *stubTaskRepo) UpdateFlowRun(_ context.Context, _ *FlowRun) error { return nil }
func (r *stubTaskRepo) ListFlowRuns(_ context.Context, _ Query) ([]*FlowRun, error) {
	return nil, nil
}

func (r *stubTaskRepo) GetNextSequenceID(_ context.Context, _ string) (int, error) {
	return 1, nil
}

func (r *stubTaskRepo) ListTasks(_ context.Context, query Query) ([]*Task, error) {
	// Filter by track_id if requested.
	for _, f := range query.Filters {
		if f.Field == "track_id" {
			trackID, _ := f.Value.(string)
			var result []*Task
			for _, t := range r.tasks {
				if t.TrackID != nil && *t.TrackID == trackID {
					result = append(result, t)
				}
			}
			return result, nil
		}
	}
	return r.tasks, nil
}

func TestTrackService_CreateTrack(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	track := &Track{
		ID:    "feat-login",
		Title: "Login Feature",
		Type:  TrackTypeFeature,
	}

	if err := svc.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack failed: %v", err)
	}

	if track.Status != TrackStatusPending {
		t.Errorf("expected default status pending, got %q", track.Status)
	}
	if track.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestTrackService_CreateTrack_InvalidID(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	track := &Track{ID: "AB", Title: "Bad", Type: TrackTypeFeature}
	if err := svc.CreateTrack(ctx, track); err == nil {
		t.Fatal("expected error for short ID")
	}
}

func TestTrackService_CreateTrack_InvalidType(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	track := &Track{ID: "valid-id", Title: "T", Type: "epic"}
	if err := svc.CreateTrack(ctx, track); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestTrackService_GetTrack_NotFound(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	_, err := svc.GetTrack(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent track")
	}
}

func TestTrackService_ListTracks(t *testing.T) {
	repo := newStubTrackRepo()
	svc := NewTrackService(repo, &stubTaskRepo{})
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["a"] = &Track{
		ID: "aaa", Status: TrackStatusPending, Type: "feature",
		CreatedAt: now, UpdatedAt: now,
	}
	repo.tracks["b"] = &Track{
		ID: "bbb", Status: TrackStatusActive, Type: "bug",
		CreatedAt: now, UpdatedAt: now,
	}

	result, err := svc.ListTracks(ctx, TrackQuery{Type: "bug"})
	if err != nil {
		t.Fatalf("ListTracks failed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

func TestTrackService_UpdateTrack(t *testing.T) {
	repo := newStubTrackRepo()
	svc := NewTrackService(repo, &stubTaskRepo{})
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["feat-x"] = &Track{
		ID: "feat-x", Title: "X", Type: "feature",
		Status: TrackStatusPending, CreatedAt: now, UpdatedAt: now,
	}

	err := svc.UpdateTrack(ctx, "feat-x", func(t *Track) error {
		t.Title = "Updated X"
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateTrack failed: %v", err)
	}

	got := repo.tracks["feat-x"]
	if got.Title != "Updated X" {
		t.Errorf("expected title 'Updated X', got %q", got.Title)
	}
}

func TestTrackService_UpdateTrack_NotFound(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	err := svc.UpdateTrack(ctx, "ghost", func(t *Track) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error for nonexistent track")
	}
}

func TestTrackService_UpdateTrack_StatusTransitionBlocked(t *testing.T) {
	repo := newStubTrackRepo()
	taskRepo := &stubTaskRepo{} // no linked tasks
	svc := NewTrackService(repo, taskRepo)
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["feat-y"] = &Track{
		ID: "feat-y", Title: "Y", Type: "feature",
		Status: TrackStatusPending, CreatedAt: now, UpdatedAt: now,
	}

	// pending -> active requires linked tasks > 0
	err := svc.UpdateTrack(ctx, "feat-y", func(t *Track) error {
		t.Status = TrackStatusActive
		return nil
	})
	if err == nil {
		t.Fatal("expected error for pending->active with 0 linked tasks")
	}
}

func TestTrackService_AbandonTrack(t *testing.T) {
	repo := newStubTrackRepo()
	trackID := "track-id-for-abandon"
	taskRepo := &stubTaskRepo{
		tasks: []*Task{
			{ID: "T-1", Status: StatusTodo, TrackID: &trackID},
		},
	}
	svc := NewTrackService(repo, taskRepo)
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks[trackID] = &Track{
		ID: trackID, Title: "Z", Type: "feature",
		Status: TrackStatusActive, CreatedAt: now, UpdatedAt: now,
	}

	if err := svc.AbandonTrack(ctx, trackID); err != nil {
		t.Fatalf("AbandonTrack failed: %v", err)
	}

	got := repo.tracks[trackID]
	if got.Status != TrackStatusAbandoned {
		t.Errorf("expected abandoned, got %q", got.Status)
	}
}

func TestTrackService_AbandonTrack_FromPending(t *testing.T) {
	repo := newStubTrackRepo()
	svc := NewTrackService(repo, &stubTaskRepo{})
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["aaa"] = &Track{
		ID: "aaa", Title: "A", Type: "feature",
		Status: TrackStatusPending, CreatedAt: now, UpdatedAt: now,
	}

	if err := svc.AbandonTrack(ctx, "aaa"); err != nil {
		t.Fatalf("pending → abandoned should be allowed, got: %v", err)
	}
	if repo.tracks["aaa"].Status != TrackStatusAbandoned {
		t.Errorf("expected abandoned, got %s", repo.tracks["aaa"].Status)
	}
}

func TestTrackService_AbandonTrack_FromArchived(t *testing.T) {
	repo := newStubTrackRepo()
	svc := NewTrackService(repo, &stubTaskRepo{})
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["aaa"] = &Track{
		ID: "aaa", Title: "A", Type: "feature",
		Status: TrackStatusArchived, CreatedAt: now, UpdatedAt: now,
	}

	if err := svc.AbandonTrack(ctx, "aaa"); err == nil {
		t.Fatal("expected error for archived → abandoned transition")
	}
}

func TestTrackService_ArchiveTrack(t *testing.T) {
	repo := newStubTrackRepo()
	svc := NewTrackService(repo, &stubTaskRepo{})
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["bbb"] = &Track{
		ID: "bbb", Title: "B", Type: "feature",
		Status: TrackStatusCompleted, CreatedAt: now, UpdatedAt: now,
	}

	if err := svc.ArchiveTrack(ctx, "bbb"); err != nil {
		t.Fatalf("ArchiveTrack failed: %v", err)
	}

	got := repo.tracks["bbb"]
	if got.Status != TrackStatusArchived {
		t.Errorf("expected archived, got %q", got.Status)
	}
}

func TestTrackService_ArchiveTrack_InvalidTransition(t *testing.T) {
	repo := newStubTrackRepo()
	svc := NewTrackService(repo, &stubTaskRepo{})
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["ccc"] = &Track{
		ID: "ccc", Title: "C", Type: "feature",
		Status: TrackStatusPending, CreatedAt: now, UpdatedAt: now,
	}

	err := svc.ArchiveTrack(ctx, "ccc")
	if err == nil {
		t.Fatal("expected error for pending->archived transition")
	}
}

func TestTrackService_DeleteTrack(t *testing.T) {
	repo := newStubTrackRepo()
	svc := NewTrackService(repo, &stubTaskRepo{})
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["ddd"] = &Track{
		ID: "ddd", Title: "D", Type: "feature",
		Status: TrackStatusPending, CreatedAt: now, UpdatedAt: now,
	}

	if err := svc.DeleteTrack(ctx, "ddd"); err != nil {
		t.Fatalf("DeleteTrack failed: %v", err)
	}
	if _, ok := repo.tracks["ddd"]; ok {
		t.Error("expected track to be deleted")
	}
}

func TestTrackService_GetTrackWithState(t *testing.T) {
	repo := newStubTrackRepo()
	trackID := "with-state"
	taskRepo := &stubTaskRepo{
		tasks: []*Task{
			{
				ID: "T-1", Status: StatusDone, TrackID: &trackID,
				Tags:      []string{"phase:1"},
				UpdatedAt: time.Now().UTC(),
			},
			{
				ID: "T-2", Status: StatusTodo, TrackID: &trackID,
				Tags:      []string{"phase:2"},
				UpdatedAt: time.Now().UTC(),
			},
		},
	}
	svc := NewTrackService(repo, taskRepo)
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks[trackID] = &Track{
		ID: trackID, Title: "S", Type: "feature",
		Status: TrackStatusActive, CreatedAt: now, UpdatedAt: now,
	}

	track, flags, progress, err := svc.GetTrackWithState(ctx, trackID, 48*time.Hour)
	if err != nil {
		t.Fatalf("GetTrackWithState failed: %v", err)
	}
	if track.ID != trackID {
		t.Errorf("expected track ID %q, got %q", trackID, track.ID)
	}
	if len(flags) == 0 {
		t.Error("expected at least one state flag")
	}
	if progress == nil {
		t.Fatal("expected progress, got nil")
	}
	if progress.TotalTasks != 2 {
		t.Errorf("expected 2 total tasks, got %d", progress.TotalTasks)
	}
	if progress.CompletedTasks != 1 {
		t.Errorf("expected 1 completed task, got %d", progress.CompletedTasks)
	}
	if progress.CurrentPhase != 2 {
		t.Errorf("expected current phase 2, got %d", progress.CurrentPhase)
	}
}

func TestTrackService_GetTrackWithState_Unlinked(t *testing.T) {
	repo := newStubTrackRepo()
	svc := NewTrackService(repo, &stubTaskRepo{})
	ctx := context.Background()

	now := time.Now().UTC()
	repo.tracks["empty"] = &Track{
		ID: "empty", Title: "E", Type: "feature",
		Status: TrackStatusPending, CreatedAt: now, UpdatedAt: now,
	}

	_, flags, _, err := svc.GetTrackWithState(ctx, "empty", 48*time.Hour)
	if err != nil {
		t.Fatalf("GetTrackWithState failed: %v", err)
	}
	if len(flags) != 1 || flags[0] != TrackStateUnlinked {
		t.Errorf("expected [unlinked], got %v", flags)
	}
}
