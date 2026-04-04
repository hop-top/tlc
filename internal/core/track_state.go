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
//   - blocked: 1+ tasks, all non-terminal tasks have unresolved blocked_by
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

	// Blocked check: all non-terminal tasks must have unresolved blocked_by.
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

// isTrackBlocked returns true when every non-terminal task has a non-empty
// blocked_by list.
func isTrackBlocked(tasks []*Task) bool {
	wm := DefaultWorkflow()
	nonTerminalCount := 0
	blockedCount := 0

	for _, t := range tasks {
		if wm.IsTerminal(t.Status) {
			continue
		}
		nonTerminalCount++
		if len(t.BlockedBy()) > 0 {
			blockedCount++
		}
	}

	// Need at least one non-terminal task, and all must be blocked.
	return nonTerminalCount > 0 && blockedCount == nonTerminalCount
}
