package cli

import (
	"errors"
	"fmt"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// sortByUsage is the --sort-by help text. Rendered from the storage
// allowlist, so the help cannot list a field the store rejects or omit
// one it accepts.
func sortByUsage() string {
	return fmt.Sprintf("Sort field, or a comma-separated list of them (%s)",
		core.VocabularyList(storage.TaskFieldNames()))
}

// validateTaskListSort rejects an unsupported --sort-by or
// --sort-direction before any storage is opened, naming the flag the
// value came from. The store enforces the same allowlist on every read;
// this is the early, flag-aware gate, and the only one a value supplied
// through config defaults would otherwise meet late.
func validateTaskListSort() error {
	err := storage.ValidateTaskSort(taskListSortBy, taskListSortDirection)
	switch {
	case errors.Is(err, core.ErrInvalidSortField):
		return fmt.Errorf("--sort-by: %w", err)
	case errors.Is(err, core.ErrInvalidSortDirection):
		return fmt.Errorf("--sort-direction: %w", err)
	}
	return err
}
