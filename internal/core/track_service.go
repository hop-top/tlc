package core

import (
	"context"
	"fmt"
	"time"

	"hop.top/kit/go/runtime/domain"
)

// TrackService provides business logic for track lifecycle management.
// When a domain.Service is configured (via WithDomainTrackRepo), write
// operations (Create, Update, Delete) delegate to it. Reads use the
// legacy TrackRepository for rich filtering. Custom orchestration
// (auto-transition, plan ingestion, abandon-with-tasks) stays as
// wrappers.
type TrackService struct {
	repo       TrackRepository
	taskRepo   Repository
	domainSvc  *domain.Service[Track]
	slugMaxLen int
}

// TrackServiceOption configures a TrackService.
type TrackServiceOption func(*TrackService)

// WithDomainTrackRepo wires a domain.Repository[Track] to enable
// kit/domain CRUD delegation.
func WithDomainTrackRepo(
	dr domain.Repository[Track],
	opts ...domain.Option[Track],
) TrackServiceOption {
	return func(s *TrackService) {
		s.domainSvc = domain.NewService[Track](dr, opts...)
	}
}

// WithSlugMaxLen sets the write-path length limit applied to slugs of
// newly created tracks. Zero keeps DefaultNewTrackSlugMaxLen. Existing
// tracks are unaffected: lookups keep the MaxTrackSlugLen read ceiling.
func WithSlugMaxLen(n int) TrackServiceOption {
	return func(s *TrackService) {
		s.slugMaxLen = n
	}
}

// NewTrackService creates a new TrackService.
func NewTrackService(repo TrackRepository, taskRepo Repository, opts ...TrackServiceOption) *TrackService {
	s := &TrackService{
		repo:     repo,
		taskRepo: taskRepo,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// CreateTrack validates and persists a new track. Track.ID is the durable
// TypeID (auto-generated upstream); Track.Slug is the user-facing alias and
// is what we validate.
//
// Compatibility: callers that pass a slug-shaped string in Track.ID (and
// leave Slug empty) get the slug promoted to Track.Slug and a fresh TypeID
// minted for Track.ID. This eases test/code paths that pre-date the slug
// split without forcing a tombstoned interface.
func (s *TrackService) CreateTrack(ctx context.Context, track *Track) error {
	switch {
	case track.ID == "" && track.Slug == "":
		return fmt.Errorf("track requires either ID (typeid) or slug")
	case track.ID == "":
		track.ID = NewTrackID()
	case IsTrackID(track.ID):
		// good — already a typeid
	default:
		// Treat the supplied "ID" as the user-facing slug.
		if track.Slug == "" {
			track.Slug = track.ID
		}
		track.ID = NewTrackID()
	}
	// Write path: new slugs obey the configured limit. Reads keep the
	// wider ceiling so pre-existing long slugs stay resolvable.
	if err := ValidateNewTrackSlug(track.Slug, s.slugMaxLen); err != nil {
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

	if s.domainSvc != nil {
		if err := s.domainSvc.Create(ctx, track); err != nil {
			return fmt.Errorf("failed to create track: %w", err)
		}
	} else {
		if err := s.repo.CreateTrack(ctx, track); err != nil {
			return fmt.Errorf("failed to create track: %w", err)
		}
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
	if s.domainSvc != nil {
		if err := s.domainSvc.Update(ctx, track); err != nil {
			return fmt.Errorf("failed to update track: %w", err)
		}
	} else {
		if err := s.repo.UpdateTrack(ctx, track); err != nil {
			return fmt.Errorf("failed to update track: %w", err)
		}
	}
	return nil
}

// DeleteTrack removes a track. The repository enforces that no tasks
// reference the track.
func (s *TrackService) DeleteTrack(ctx context.Context, id string) error {
	if s.domainSvc != nil {
		if err := s.domainSvc.Delete(ctx, id); err != nil {
			return fmt.Errorf("failed to delete track: %w", err)
		}
	} else {
		if err := s.repo.DeleteTrack(ctx, id); err != nil {
			return fmt.Errorf("failed to delete track: %w", err)
		}
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
	flags, progress, err := s.ComputeStateForTrack(ctx, track, staleThreshold)
	if err != nil {
		return nil, nil, nil, err
	}
	return track, flags, progress, nil
}

// ComputeStateForTrack returns state flags and progress for an already-
// fetched track. Unlike GetTrackWithState, this skips the re-fetch via
// GetTrack — required for callers that iterate over cross-project track
// lists (e.g. `tlc track list --all-projects` from inside a project),
// where GetTrack's auto-scoping to the current project would error on
// rows that belong to other projects.
func (s *TrackService) ComputeStateForTrack(
	ctx context.Context,
	track *Track,
	staleThreshold time.Duration,
) ([]TrackStateFlag, *TrackProgress, error) {
	tasks, err := s.linkedTasks(ctx, track)
	if err != nil {
		return nil, nil, err
	}
	flags := ComputeTrackState(track, tasks, staleThreshold)
	progress := ComputeTrackProgress(tasks)
	return flags, &progress, nil
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
		// Linked-task accounting must see the full set a track owns. Archival
		// is a list-hygiene concern (aged-out DONE tasks hidden from default
		// views), not an unlink. Omitting this made ListTasks append
		// `archived = 0`, so a track whose tasks had all completed and aged
		// past the archive window read as "0 linked" — breaking progress,
		// terminal-state checks, and the pending→active guard.
		IncludeArchived: true,
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
