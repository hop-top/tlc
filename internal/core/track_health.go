package core

import (
	"context"
	"fmt"
)

// ProjectHealth holds computed project-level health metrics.
type ProjectHealth struct {
	ActiveLoad  int
	AvgProgress float64
	MaxActive   int
	MinProgress int
}

// ComputeProjectHealth derives project-level health from active tracks.
//
// active_load = count of tracks with status=active.
// avg_progress = mean(completed/total * 100) across active tracks.
// Tracks with zero tasks contribute 0% to the average.
func ComputeProjectHealth(
	tracks []*Track,
	tasks map[string][]*Task,
	maxActive int,
	minProgress int,
) ProjectHealth {
	var activeCount int
	var progressSum float64

	for _, t := range tracks {
		if t.Status != TrackStatusActive {
			continue
		}
		activeCount++

		linked := tasks[t.ID]
		if len(linked) == 0 {
			continue
		}

		p := ComputeTrackProgress(linked)
		if p.TotalTasks > 0 {
			progressSum += float64(p.CompletedTasks) / float64(p.TotalTasks) * 100
		}
	}

	var avg float64
	if activeCount > 0 {
		avg = progressSum / float64(activeCount)
	}

	return ProjectHealth{
		ActiveLoad:  activeCount,
		AvgProgress: avg,
		MaxActive:   maxActive,
		MinProgress: minProgress,
	}
}

// CheckOvercommitWarning returns a warning string if active_load >= maxActive.
// Returns empty string if within limits.
func (s *TrackService) CheckOvercommitWarning(
	ctx context.Context,
	maxActive int,
	minProgress int,
) (string, error) {
	tracks, err := s.ListTracks(ctx, TrackQuery{
		Status: []TrackStatus{TrackStatusActive},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list active tracks: %w", err)
	}

	taskMap := make(map[string][]*Task, len(tracks))
	for _, t := range tracks {
		linked, err := s.linkedTasks(ctx, t.ID)
		if err != nil {
			return "", fmt.Errorf(
				"failed to list tasks for track %q: %w", t.ID, err,
			)
		}
		taskMap[t.ID] = linked
	}

	health := ComputeProjectHealth(tracks, taskMap, maxActive, minProgress)
	if health.ActiveLoad < maxActive && (health.ActiveLoad == 0 || health.AvgProgress >= float64(minProgress)) {
		return "", nil
	}

	if health.ActiveLoad >= maxActive {
		return fmt.Sprintf(
			"%d tracks already active (avg %.0f%% progress); "+
				"consider completing existing work before starting new tracks",
			health.ActiveLoad,
			health.AvgProgress,
		), nil
	}

	// Low progress warning
	return fmt.Sprintf(
		"%d active track(s) averaging %.0f%% progress (threshold: %d%%); "+
			"consider making progress on existing tracks before starting new ones",
		health.ActiveLoad, health.AvgProgress, minProgress,
	), nil
}
