package core

import "time"

// IsStale reports whether the task has exceeded its stale threshold.
// Returns false if StaleTimeout is nil (no threshold configured).
func (t *Task) IsStale() bool {
	if t.StaleTimeout == nil {
		return false
	}
	return time.Since(t.UpdatedAt) > *t.StaleTimeout
}

// StaleSince returns how long the task has been stale, or nil if not stale.
func (t *Task) StaleSince() *time.Duration {
	if !t.IsStale() {
		return nil
	}
	d := time.Since(t.UpdatedAt) - *t.StaleTimeout
	return &d
}

// IsBlocked reports whether the task has a blocked reason set.
func (t *Task) IsBlocked() bool {
	return t.BlockedReason != nil && *t.BlockedReason != ""
}
