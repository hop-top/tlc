package core

import (
	"context"
	"fmt"
	"time"
)

// TrackService provides business logic for track lifecycle management.
type TrackService struct {
	repo     TrackRepository
	taskRepo Repository
}

// NewTrackService creates a new TrackService.
func NewTrackService(repo TrackRepository, taskRepo Repository) *TrackService {
	return &TrackService{
		repo:     repo,
		taskRepo: taskRepo,
	}
}

// CreateTrack validates and persists a new track.
func (s *TrackService) CreateTrack(ctx context.Context, track *Track) error {
	if err := ValidateTrackID(track.ID); err != nil {
		return err
	}
	if !ValidTrackType(track.Type) {
		return fmt.Errorf(
			"track type %q invalid; valid types: %s",
			track.Type, TrackTypeList(),
		)
	}

	// Auto-assign project_id from context if not already set.
	if track.ProjectID == nil {
		if proj := DetectProject(); proj != nil && proj.InProject && proj.ProjectID != "" {
			track.ProjectID = &proj.ProjectID
		}
	}

	now := time.Now().UTC()
	if track.Status == "" {
		track.Status = TrackStatusPending
	}
	if track.CreatedAt.IsZero() {
		track.CreatedAt = now
	}
	if track.UpdatedAt.IsZero() {
		track.UpdatedAt = now
	}

	if err := s.repo.CreateTrack(ctx, track); err != nil {
		return fmt.Errorf("failed to create track: %w", err)
	}
	return nil
}

// GetTrack retrieves a track by ID.
func (s *TrackService) GetTrack(ctx context.Context, id string) (*Track, error) {
	track, err := s.repo.GetTrack(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get track: %w", err)
	}
	if track == nil {
		return nil, fmt.Errorf(
			"track %q not found; run 'tlc track list' to see available tracks",
			id,
		)
	}
	return track, nil
}

// ListTracks returns tracks matching the query.
func (s *TrackService) ListTracks(
	ctx context.Context,
	query TrackQuery,
) ([]*Track, error) {
	tracks, err := s.repo.ListTracks(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list tracks: %w", err)
	}
	return tracks, nil
}

// UpdateTrack validates and persists track changes. If the status changed,
// the transition is validated against business rules.
func (s *TrackService) UpdateTrack(
	ctx context.Context,
	id string,
	updates func(*Track) error,
) error {
	track, err := s.repo.GetTrack(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get track: %w", err)
	}
	if track == nil {
		return fmt.Errorf(
			"track %q not found; run 'tlc track list' to see available tracks",
			id,
		)
	}

	oldStatus := track.Status
	if err := updates(track); err != nil {
		return err
	}

	// Validate status transition if status changed.
	if track.Status != oldStatus {
		linkedCount, allTerminal, err := s.linkedTaskStats(ctx, track)
		if err != nil {
			return err
		}
		if err := ValidateTrackTransition(
			oldStatus, track.Status, linkedCount, allTerminal,
		); err != nil {
			return err
		}
	}

	track.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateTrack(ctx, track); err != nil {
		return fmt.Errorf("failed to update track: %w", err)
	}
	return nil
}

// DeleteTrack removes a track. The repository enforces that no tasks
// reference the track.
func (s *TrackService) DeleteTrack(ctx context.Context, id string) error {
	if err := s.repo.DeleteTrack(ctx, id); err != nil {
		return fmt.Errorf("failed to delete track: %w", err)
	}
	return nil
}

// AutoTransitionOnTaskClaim transitions a pending track to active when a
// linked task is claimed, provided the track has at least one linked task.
// If the track is not pending or the transition is invalid, it is a no-op.
func (s *TrackService) AutoTransitionOnTaskClaim(ctx context.Context, trackID string) error {
	track, err := s.repo.GetTrack(ctx, trackID)
	if err != nil {
		return fmt.Errorf("failed to get track for auto-transition: %w", err)
	}
	if track == nil || track.Status != TrackStatusPending {
		return nil
	}

	linkedCount, _, err := s.linkedTaskStats(ctx, track)
	if err != nil {
		return fmt.Errorf("failed to get linked task stats: %w", err)
	}
	if linkedCount == 0 {
		return nil
	}

	return s.UpdateTrack(ctx, trackID, func(t *Track) error {
		t.Status = TrackStatusActive
		return nil
	})
}

// AbandonTrack transitions a track to abandoned without touching linked tasks.
func (s *TrackService) AbandonTrack(ctx context.Context, id string) error {
	return s.UpdateTrack(ctx, id, func(t *Track) error {
		t.Status = TrackStatusAbandoned
		return nil
	})
}

// LinkedNonTerminalTasks returns tasks linked to a track that are not in a
// terminal state (DONE or SKIPPED).
func (s *TrackService) LinkedNonTerminalTasks(
	ctx context.Context,
	trackID string,
) ([]*Task, error) {
	track, err := s.GetTrack(ctx, trackID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.linkedTasks(ctx, track)
	if err != nil {
		return nil, err
	}
	wm := DefaultWorkflow()
	var result []*Task
	for _, t := range tasks {
		if !wm.IsTerminal(t.Status) {
			result = append(result, t)
		}
	}
	return result, nil
}

// AbandonTrackWithTasks abandons the track and transitions all linked
// non-terminal tasks to SKIPPED. Returns the list of tasks that were
// skipped. The caller is responsible for confirming with the user
// before calling this method.
func (s *TrackService) AbandonTrackWithTasks(
	ctx context.Context,
	trackID string,
) ([]*Task, error) {
	// Validate track transition before touching tasks to avoid partial
	// state if the track itself can't be abandoned (e.g. archived).
	track, err := s.GetTrack(ctx, trackID)
	if err != nil {
		return nil, err
	}
	linkedCount, allTerminal, err := s.linkedTaskStats(ctx, track)
	if err != nil {
		return nil, err
	}
	if err := ValidateTrackTransition(
		track.Status, TrackStatusAbandoned, linkedCount, allTerminal,
	); err != nil {
		return nil, err
	}

	nonTerminal, err := s.LinkedNonTerminalTasks(ctx, trackID)
	if err != nil {
		return nil, err
	}

	// Skip all non-terminal tasks, then abandon the track.
	wm := DefaultWorkflow()
	for _, task := range nonTerminal {
		logEntry, err := task.TransitionWithWorkflow(
			StatusSkipped, "track-abandon", "track abandoned",
			wm, true,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to skip task %s: %w", task.ID, err,
			)
		}
		if err := s.taskRepo.UpdateTaskWithLog(ctx, task, logEntry); err != nil {
			return nil, fmt.Errorf(
				"failed to persist task %s: %w", task.ID, err,
			)
		}
	}

	if err := s.AbandonTrack(ctx, trackID); err != nil {
		return nil, err
	}

	return nonTerminal, nil
}

// ArchiveTrack transitions a completed or abandoned track to archived.
func (s *TrackService) ArchiveTrack(ctx context.Context, id string) error {
	return s.UpdateTrack(ctx, id, func(t *Track) error {
		t.Status = TrackStatusArchived
		return nil
	})
}

// GetTrackWithState fetches a track plus its computed state flags and
// phase progress from linked tasks.
func (s *TrackService) GetTrackWithState(
	ctx context.Context,
	id string,
	staleThreshold time.Duration,
) (*Track, []TrackStateFlag, *TrackProgress, error) {
	track, err := s.GetTrack(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}

	tasks, err := s.linkedTasks(ctx, track)
	if err != nil {
		return nil, nil, nil, err
	}

	flags := ComputeTrackState(track, tasks, staleThreshold)
	progress := ComputeTrackProgress(tasks)

	return track, flags, &progress, nil
}

// linkedTasks returns all tasks linked to a track, scoped by both track_id
// AND the track's owning project_id. Track IDs are unique only within a
// (project_id, id) composite, so a bare track_id filter would merge tasks
// from other projects that happen to share the same track_id string.
func (s *TrackService) linkedTasks(ctx context.Context, track *Track) ([]*Task, error) {
	var projectID string
	if track.ProjectID != nil {
		projectID = *track.ProjectID
	}
	tasks, err := s.taskRepo.ListTasks(ctx, Query{
		Filters: []FieldFilter{
			{Field: "track_id", Operator: OpEq, Value: track.ID},
			{Field: "project_id", Operator: OpEq, Value: projectID},
		},
		AllProjects: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list linked tasks: %w", err)
	}
	return tasks, nil
}

// linkedTaskStats returns the count of linked tasks and whether all are terminal.
func (s *TrackService) linkedTaskStats(
	ctx context.Context,
	track *Track,
) (int, bool, error) {
	tasks, err := s.linkedTasks(ctx, track)
	if err != nil {
		return 0, false, err
	}
	if len(tasks) == 0 {
		return 0, true, nil
	}

	wm := DefaultWorkflow()
	allTerminal := true
	for _, t := range tasks {
		if !wm.IsTerminal(t.Status) {
			allTerminal = false
			break
		}
	}
	return len(tasks), allTerminal, nil
}
