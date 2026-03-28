package cli

import "fmt"

// errTaskNotFound returns an actionable error for a missing task.
// Tells the agent which command to run to see available tasks.
func errTaskNotFound(id string) error {
	return fmt.Errorf("task %s not found; run 'tlc task list' to see available tasks", id)
}

// errProjectNotFound returns an actionable error for an unregistered project.
// Tells the agent how to register the project.
func errProjectNotFound(projectID string) error {
	return fmt.Errorf(
		"project %q not found in registry; run 'tlc init' in the project root to register it",
		projectID,
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
