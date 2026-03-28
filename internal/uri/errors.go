package uri

import "fmt"

// ErrTaskNotFound is returned when a task ID cannot be resolved.
// Provides the agent with actionable next steps.
type ErrTaskNotFound struct {
	ID        string
	ProjectID string // non-empty when a specific project was searched
}

func (e *ErrTaskNotFound) Error() string {
	if e.ProjectID != "" {
		return fmt.Sprintf(
			"task %q not found in project %q; run 'tlc task list' to see available tasks",
			e.ID, e.ProjectID,
		)
	}
	return fmt.Sprintf(
		"task %s not found; run 'tlc task list' to see available tasks",
		e.ID,
	)
}

// ErrProjectNotFound is returned when a project ID is not in the registry.
// Provides the agent with actionable next steps.
type ErrProjectNotFound struct {
	ProjectID string
}

func (e *ErrProjectNotFound) Error() string {
	return fmt.Sprintf(
		"project %q not found in registry; run 'tlc init' in the project root to register it",
		e.ProjectID,
	)
}
