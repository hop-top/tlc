package core

import (
	"fmt"
	"time"
)

type ErrInvalidTransition struct {
	From TaskStatus
	To   TaskStatus
	Msg  string
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid transition from %s to %s: %s", e.From, e.To, e.Msg)
}

// ValidateTransition checks if a status transition is allowed using the
// default workflow. Kept for backward compatibility; new code should use
// WorkflowManager.ValidateTransition directly.
func ValidateTransition(current, next TaskStatus) error {
	return DefaultWorkflow().ValidateTransition(current, next, false)
}

// TransitionWithWorkflow transitions the task using the given WorkflowManager
// and records a log entry. Set force=true to bypass transition rules.
func (t *Task) TransitionWithWorkflow(next TaskStatus, by string, note string, wm *WorkflowManager, force bool) (*LogEntry, error) {
	if err := wm.ValidateTransition(t.Status, next, force); err != nil {
		return nil, err
	}
	oldStatus := t.Status
	t.Status = next
	t.UpdatedAt = time.Now().UTC()
	return &LogEntry{
		TaskID:    t.ID,
		Timestamp: t.UpdatedAt,
		By:        by,
		Action:    string(next),
		Note:      fmt.Sprintf("Status changed from %s to %s: %s", oldStatus, next, note),
	}, nil
}

// Transition transitions the task using the default workflow.
// Deprecated: use TransitionWithWorkflow for explicit workflow control.
func (t *Task) Transition(next TaskStatus, by string, note string) (*LogEntry, error) {
	return t.TransitionWithWorkflow(next, by, note, DefaultWorkflow(), false)
}

func (t *Task) NeedsPush() bool {
	if t.OriginSystem == nil || *t.OriginSystem == "" {
		return false
	}
	if t.LastSyncAt == nil {
		return true
	}
	// Use a small buffer to avoid jitter issues with time precision
	return t.UpdatedAt.After(t.LastSyncAt.Add(time.Millisecond))
}
