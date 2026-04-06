package core

import "fmt"

// ErrInvalidTrackTransition describes an illegal track status transition.
type ErrInvalidTrackTransition struct {
	From    TrackStatus
	To      TrackStatus
	Msg     string
	Allowed []TrackStatus
}

func (e ErrInvalidTrackTransition) Error() string {
	base := fmt.Sprintf(
		"cannot transition track from %s to %s: %s",
		e.From, e.To, e.Msg,
	)
	if len(e.Allowed) > 0 {
		return fmt.Sprintf("%s; valid transitions from %s: %v", base, e.From, e.Allowed)
	}
	return base
}

// trackTransitionRules defines the allowed status transitions.
// Each key maps to the set of statuses reachable from it.
var trackTransitionRules = map[TrackStatus][]TrackStatus{
	TrackStatusPending:   {TrackStatusActive, TrackStatusAbandoned},
	TrackStatusActive:    {TrackStatusCompleted, TrackStatusAbandoned},
	TrackStatusCompleted: {TrackStatusArchived, TrackStatusAbandoned},
	TrackStatusAbandoned: {TrackStatusArchived},
	// archived is terminal — no outgoing transitions
}

// ValidateTrackTransition checks whether a track status transition is allowed.
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
		return ErrInvalidTrackTransition{
			From: current, To: next,
			Msg: fmt.Sprintf("unknown status: %s", current),
		}
	}
	if !ValidTrackStatus(next) {
		return ErrInvalidTrackTransition{
			From: current, To: next,
			Msg: fmt.Sprintf("unknown status: %s", next),
		}
	}

	// Archived is terminal.
	if current == TrackStatusArchived {
		return ErrInvalidTrackTransition{
			From: current, To: next,
			Msg: "archived is terminal; no transitions allowed",
		}
	}

	// Check transition is in the allowed set.
	allowed, ok := trackTransitionRules[current]
	if !ok {
		return ErrInvalidTrackTransition{
			From: current, To: next,
			Msg: "no transition rules defined for current status",
		}
	}

	found := false
	for _, a := range allowed {
		if a == next {
			found = true
			break
		}
	}
	if !found {
		return ErrInvalidTrackTransition{
			From: current, To: next,
			Msg:     "transition not allowed",
			Allowed: allowed,
		}
	}

	// Business-rule guards.
	switch {
	case current == TrackStatusPending && next == TrackStatusActive:
		if linkedTaskCount == 0 {
			return ErrInvalidTrackTransition{
				From: current, To: next,
				Msg: "cannot activate track with 0 linked tasks; " +
					"link at least one task first",
			}
		}

	case current == TrackStatusActive && next == TrackStatusCompleted:
		if !allTasksTerminal {
			return ErrInvalidTrackTransition{
				From: current, To: next,
				Msg: "cannot complete track with non-terminal tasks; " +
					"all linked tasks must be DONE or SKIPPED",
			}
		}
	}

	return nil
}
