package core

import "time"

// TrackStateFlag represents a computed state indicator for a track.
type TrackStateFlag string

const (
	TrackStateStale    TrackStateFlag = "stale"
	TrackStateUnlinked TrackStateFlag = "unlinked"
	TrackStateBlocked  TrackStateFlag = "blocked"
	TrackStateHealthy  TrackStateFlag = "healthy"
)

// ComputeTrackState derives computed state flags for a track given its linked
// tasks and a staleness threshold.
//
// Rules:
//   - stale: status in {pending, active}, 1+ tasks, max(task.UpdatedAt) < now - threshold
//   - unlinked: 0 linked tasks
//   - blocked: 1+ non-terminal tasks blocked by a task outside this track
//   - healthy: none of the above (exclusive with other flags)
//
// Multiple non-healthy flags can coexist.
func ComputeTrackState(
	track *Track,
	tasks []*Task,
	staleThreshold time.Duration,
) []TrackStateFlag {
	var flags []TrackStateFlag

	// Unlinked: no tasks at all.
	if len(tasks) == 0 {
		return []TrackStateFlag{TrackStateUnlinked}
	}

	// Stale check: only for pending/active tracks.
	if track.Status == TrackStatusPending || track.Status == TrackStatusActive {
		maxUpdated := maxTaskUpdatedAt(tasks)
		if !maxUpdated.IsZero() && time.Since(maxUpdated) > staleThreshold {
			flags = append(flags, TrackStateStale)
		}
	}

	// Blocked check: any non-terminal task blocked by something outside this track.
	if isTrackBlocked(tasks) {
		flags = append(flags, TrackStateBlocked)
	}

	if len(flags) == 0 {
		return []TrackStateFlag{TrackStateHealthy}
	}
	return flags
}

// maxTaskUpdatedAt returns the most recent UpdatedAt among tasks.
func maxTaskUpdatedAt(tasks []*Task) time.Time {
	var max time.Time
	for _, t := range tasks {
		if t.UpdatedAt.After(max) {
			max = t.UpdatedAt
		}
	}
	return max
}

// isTrackBlocked returns true when at least one non-terminal task has a
// blocked_by entry that points to a task outside this track (external blocker).
// Intra-track dependencies (one track task blocking another) are sequencing,
// not blockers, and must not flag the track as blocked.
func isTrackBlocked(tasks []*Task) bool {
	// Build a set of all task IDs in this track for fast lookup.
	inTrack := make(map[string]struct{}, len(tasks))
	for _, t := range tasks {
		inTrack[t.ID] = struct{}{}
	}

	wm := DefaultWorkflow()
	for _, t := range tasks {
		if wm.IsTerminal(t.Status) {
			continue
		}
		for _, blocker := range t.BlockedBy() {
			if _, ok := inTrack[blocker]; !ok {
				return true
			}
		}
	}
	return false
}
