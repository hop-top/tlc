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

func ValidateTransition(current, next TaskStatus) error {
	if current == next {
		return nil
	}

	// Forbidden: DONE -> anything, SKIPPED -> anything
	if current == StatusDone || current == StatusSkipped {
		return ErrInvalidTransition{From: current, To: next, Msg: "terminal states are immutable"}
	}

	// Forbidden: TODO -> DONE (must pass through IN_PROGRESS)
	if current == StatusTodo && next == StatusDone {
		return ErrInvalidTransition{From: current, To: next, Msg: "must pass through IN_PROGRESS for audit clarity"}
	}

	switch current {
	case StatusTodo:
		if next == StatusInProgress || next == StatusSkipped {
			return nil
		}
	case StatusInProgress:
		if next == StatusDone || next == StatusTodo || next == StatusSkipped {
			return nil
		}
	}

	return ErrInvalidTransition{From: current, To: next, Msg: "transition not allowed by state machine"}
}

func (t *Task) Transition(next TaskStatus, by string, note string) (*LogEntry, error) {
	if err := ValidateTransition(t.Status, next); err != nil {
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
