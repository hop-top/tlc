package core

import (
	"context"
	"errors"
	"fmt"

	"hop.top/kit/domain"
)

// ValidateTrackTransition checks whether a track status transition is allowed.
// It delegates state-graph validation to the kit/domain TrackStateMachine
// and applies business-rule guards on top.
//
// Business rules enforced:
//   - pending → active: requires linkedTaskCount > 0
//   - pending → abandoned: always allowed
//   - active → completed: requires allTasksTerminal == true
//   - active → abandoned: always allowed
//   - completed → abandoned: always allowed
//   - completed/abandoned → archived: always allowed
//   - archived: terminal, no outgoing transitions
func ValidateTrackTransition(
	current, next TrackStatus,
	linkedTaskCount int,
	allTasksTerminal bool,
) error {
	if current == next {
		return nil
	}

	// Validate both statuses are known.
	if !ValidTrackStatus(current) {
		return fmt.Errorf(
			"cannot transition track from %s to %s: unknown status: %s",
			current, next, current,
		)
	}
	if !ValidTrackStatus(next) {
		return fmt.Errorf(
			"cannot transition track from %s to %s: unknown status: %s",
			current, next, next,
		)
	}

	// Validate state-graph edge via kit/domain StateMachine.
	sm := NewTrackStateMachine(nil)
	if err := sm.Transition(
		context.Background(),
		domain.State(current), domain.State(next), false,
	); err != nil {
		var te *domain.TransitionError
		if errors.As(err, &te) {
			allowed := make([]string, len(te.Allowed))
			for i, s := range te.Allowed {
				allowed[i] = string(s)
			}
			if len(allowed) > 0 {
				return fmt.Errorf(
					"cannot transition track from %s to %s: "+
						"transition not allowed; valid: %v",
					current, next, allowed,
				)
			}
			return fmt.Errorf(
				"cannot transition track from %s to %s: %s",
				current, next, err,
			)
		}
		return err
	}

	// Business-rule guards.
	switch {
	case current == TrackStatusPending && next == TrackStatusActive:
		if linkedTaskCount == 0 {
			return fmt.Errorf(
				"cannot transition track from %s to %s: "+
					"cannot activate track with 0 linked tasks; "+
					"link at least one task first",
				current, next,
			)
		}

	case current == TrackStatusActive && next == TrackStatusCompleted:
		if !allTasksTerminal {
			return fmt.Errorf(
				"cannot transition track from %s to %s: "+
					"cannot complete track with non-terminal tasks; "+
					"all linked tasks must be DONE or SKIPPED",
				current, next,
			)
		}
	}

	return nil
}
