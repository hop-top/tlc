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
// 3 — not found (task/track/recipe/project missing)
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
// doesn't exist". Callers use errors.Is(err, cli.ErrNotFound) (or
// wrap it via a typed error whose Unwrap() returns ErrNotFound — see
// trackNotFoundError in track_resolve.go for the pattern).
//
// exitCodeFor in root.go classifies an error as ExitNotFound (3) when
// any of the following matches:
//
//   - errors.Is(err, ErrNotFound)         (this sentinel)
//   - errors.Is(err, ErrTrackNotFound)    (legacy track-resolution sentinel)
//   - errors.As(err, &*uri.ErrTaskNotFound)
//   - errors.As(err, &*uri.ErrProjectNotFound)
//
// Don't wrap ErrNotFound via fmt.Errorf("...: %w", ErrNotFound)
// directly — that appends ": not found" to the displayed message.
// Prefer a small typed wrapper that returns the actionable string
// from Error() and exposes ErrNotFound via Unwrap().
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

// Note: errTaskNotFound + errProjectNotFound helpers used to live here
// but had zero call sites. The actual not-found errors are produced by
// internal/uri (ErrTaskNotFound, ErrProjectNotFound) which implement
// AsCLIError() for kit middleware classification, and by
// internal/cli.trackNotFoundError in track_resolve.go for tracks. If
// you need a new not-found path, follow the trackNotFoundError pattern
// rather than reintroducing fmt.Errorf wrappers around ErrNotFound.

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
