// Package events defines topic constants and payload types for the
// tlc event bus.
package events

import "time"

// Topic constants follow dot-separated convention: tlc.<entity>.<action>.
const (
	// Task lifecycle topics.
	TaskCreated       = "tlc.task.created"
	TaskClaimed       = "tlc.task.claimed"
	TaskCompleted     = "tlc.task.completed"
	TaskStatusChanged = "tlc.task.status-changed"

	// Track lifecycle topics.
	TrackCreated   = "tlc.track.created"
	TrackActivated = "tlc.track.activated"
	TrackCompleted = "tlc.track.completed"

	// Flow lifecycle topics.
	FlowStarted       = "tlc.flow.started"
	FlowStepCompleted = "tlc.flow.step-completed"
	FlowCompleted      = "tlc.flow.completed"
	FlowFailed         = "tlc.flow.failed"
)

// EntityEvent is the standard payload for all lifecycle events.
type EntityEvent struct {
	// ID of the affected entity (task ID, track ID, flow run ID).
	ID string `json:"id"`
	// OldState before the event (empty for creation events).
	OldState string `json:"old_state,omitempty"`
	// NewState after the event.
	NewState string `json:"new_state,omitempty"`
	// Actor who triggered the event.
	Actor string `json:"actor"`
	// Timestamp of the event.
	Timestamp time.Time `json:"timestamp"`
	// Meta carries additional event-specific key/value pairs.
	Meta map[string]any `json:"meta,omitempty"`
}
