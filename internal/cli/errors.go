package cli

import (
	"errors"
	"fmt"
)

// Process exit codes used by tlc, classified per docs/exit-codes.md.
//
// 0 — success (no error)
// 1 — generic failure (default for any unmapped error)
// 2 — usage error (cobra default; unknown flag / bad args)
// 3 — not found (task/track/flow/project missing)
// 4 — conflict (policy denial, duplicate ID, state-machine refusal)
// 5 — unauthorized (auth failure, sync 401/403)
//
// exitCodeFor in root.go maps errors to these codes. New error sites
// should wrap one of the sentinels below (or use a typed error that
// signals the right class via Is()) so the mapping picks them up
// automatically.
const (
	ExitOK           = 0
	ExitGeneric      = 1
	ExitUsage        = 2
	ExitNotFound     = 3
	ExitConflict     = 4
	ExitUnauthorized = 5
)

// ErrNotFound is the shared sentinel for "the thing you asked for
// doesn't exist". Callers use errors.Is(err, cli.ErrNotFound) (or wrap
// it via fmt.Errorf("...: %w", cli.ErrNotFound)) so exitCodeFor can
// classify the error as ExitNotFound (3).
//
// Existing typed errors (uri.ErrTaskNotFound, cli.ErrTrackNotFound)
// implement Is() against this sentinel so callers don't need to know
// every flavour.
var ErrNotFound = errors.New("not found")

// ErrUnauthorized is the shared sentinel for auth-failure paths
// (login refusal, sync 401/403, missing credential). Maps to
// ExitUnauthorized (5).
var ErrUnauthorized = errors.New("unauthorized")

// ExitCodeError wraps a process exit code so callers can request a
// specific code without rewiring the sentinel match.
type ExitCodeError struct {
	Code    int
	Message string
}

func (e *ExitCodeError) Error() string { return e.Message }

// errTaskNotFound returns an actionable error for a missing task.
// Tells the agent which command to run to see available tasks.
// Wraps ErrNotFound so callers + exitCodeFor classify the error
// uniformly via errors.Is.
func errTaskNotFound(id string) error {
	return fmt.Errorf("task %s not found; run 'tlc task list' to see available tasks: %w", id, ErrNotFound)
}

// errProjectNotFound returns an actionable error for an unregistered project.
// Tells the agent how to register the project.
func errProjectNotFound(projectID string) error {
	return fmt.Errorf(
		"project %q not found in registry; run 'tlc init' in the project root to register it: %w",
		projectID, ErrNotFound,
	)
}

// errTransitionNotAllowed returns an actionable error for a blocked state-machine transition.
// Lists the valid next statuses and the escape hatch.
func errTransitionNotAllowed(from, to string, allowed []string) error {
	if len(allowed) == 0 {
		return fmt.Errorf(
			"cannot transition task from %s to %s: no transitions defined for %s; "+
				"use --force to bypass the state machine",
			from, to, from,
		)
	}
	return fmt.Errorf(
		"cannot transition task from %s to %s: allowed transitions are %v; "+
			"use --force to bypass the state machine",
		from, to, allowed,
	)
}

// errTerminalState returns an actionable error when trying to mutate a terminal task.
func errTerminalState(taskID, status string) error {
	return fmt.Errorf(
		"task %s is in terminal state %s and cannot be transitioned; "+
			"run 'tlc task reopen %s --note \"<reason>\"' to reopen it first",
		taskID, status, taskID,
	)
}

// errNoteRequired returns an actionable error for commands that require a note.
func errNoteRequired(cmdHint string) error {
	return fmt.Errorf("--note is required; re-run with: %s --note \"<reason>\"", cmdHint)
}

// errDeleteRequiresYes returns an actionable error when --yes is missing for delete.
func errDeleteRequiresYes(taskID string) error {
	return fmt.Errorf(
		"task delete requires confirmation; re-run with: tlc task delete %s --yes",
		taskID,
	)
}
