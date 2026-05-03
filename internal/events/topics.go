// Package events defines topic constants and payload types for the
// tlc event bus.
package events

import (
	"time"

	"hop.top/kit/go/runtime/bus"
)

// Typed topic constants (bus.Topic) per domain-events.md.
const (
	TopicTaskCreated   bus.Topic = "tlc.task.created"
	TopicTaskClaimed   bus.Topic = "tlc.task.claimed"
	TopicTaskCompleted bus.Topic = "tlc.task.completed"
	TopicTaskReopened  bus.Topic = "tlc.task.reopened"

	TopicTrackCreated   bus.Topic = "tlc.track.created"
	TopicTrackActivated bus.Topic = "tlc.track.activated"
	TopicTrackCompleted bus.Topic = "tlc.track.completed"
)

// Subscription topics for cross-app events.
const TopicApsProfileAll bus.Topic = "aps.profile.*"

// Legacy string constants (aliases for backward compat with audit.go).
const (
	TaskCreated       = string(TopicTaskCreated)
	TaskClaimed       = string(TopicTaskClaimed)
	TaskCompleted     = string(TopicTaskCompleted)
	TaskStatusChanged = "tlc.task.status-changed"

	TrackCreated   = string(TopicTrackCreated)
	TrackActivated = string(TopicTrackActivated)
	TrackCompleted = string(TopicTrackCompleted)

	FlowStarted       = "tlc.flow.started"
	FlowStepCompleted = "tlc.flow.step-completed"
	FlowCompleted     = "tlc.flow.completed"
	FlowFailed        = "tlc.flow.failed"
)

// State-machine transition topics, composed by domain.WithSMTopicPrefix
// from the per-entity prefixes wired in internal/core/domain_statemachine.go.
// Pre topics fire before the transition; subscriber error vetoes it.
// Post topics fire fire-and-forget after the transition committed.
const (
	TaskStatusPreTransitioned  = "tlc.task.status.pre_transitioned"
	TaskStatusPostTransitioned = "tlc.task.status.post_transitioned"

	TrackStatusPreTransitioned  = "tlc.track.status.pre_transitioned"
	TrackStatusPostTransitioned = "tlc.track.status.post_transitioned"

	FlowStatusPreTransitioned  = "tlc.flow.status.pre_transitioned"
	FlowStatusPostTransitioned = "tlc.flow.status.post_transitioned"
)

// --- Typed payloads per domain-events.md ---

// TaskCreatedPayload is published after a task is created.
type TaskCreatedPayload struct {
	TaskID     string   `json:"task_id"`
	Title      string   `json:"title"`
	TrackID    string   `json:"track_id,omitempty"`
	AssignedTo string   `json:"assigned_to,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

// TaskClaimedPayload is published after a task is claimed.
type TaskClaimedPayload struct {
	TaskID    string `json:"task_id"`
	ClaimedBy string `json:"claimed_by"`
	TrackID   string `json:"track_id,omitempty"`
}

// TaskCompletedPayload is published after a task is completed.
type TaskCompletedPayload struct {
	TaskID      string `json:"task_id"`
	CompletedBy string `json:"completed_by"`
	TrackID     string `json:"track_id,omitempty"`
	DurationSec int64  `json:"duration_sec,omitempty"`
}

// TaskReopenedPayload is published after a task is reopened.
type TaskReopenedPayload struct {
	TaskID     string `json:"task_id"`
	ReopenedBy string `json:"reopened_by"`
	Note       string `json:"note,omitempty"`
}

// TrackCreatedPayload is published after a track is created.
type TrackCreatedPayload struct {
	TrackID string `json:"track_id"`
	Title   string `json:"title"`
	Type    string `json:"type"`
}

// TrackActivatedPayload is published when a track becomes active.
type TrackActivatedPayload struct {
	TrackID       string `json:"track_id"`
	TriggerTaskID string `json:"trigger_task_id,omitempty"`
}

// TrackCompletedPayload is published after a track is completed.
type TrackCompletedPayload struct {
	TrackID     string `json:"track_id"`
	TaskCount   int    `json:"task_count"`
	DurationSec int64  `json:"duration_sec,omitempty"`
}

// EntityEvent is the generic payload for all lifecycle events.
// Retained for backward compat with audit subscriber.
type EntityEvent struct {
	ID        string         `json:"id"`
	OldState  string         `json:"old_state,omitempty"`
	NewState  string         `json:"new_state,omitempty"`
	Actor     string         `json:"actor"`
	Timestamp time.Time      `json:"timestamp"`
	Meta      map[string]any `json:"meta,omitempty"`
}
