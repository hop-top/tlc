package cli

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uri"
)

// patternChars are characters that indicate an arg is a pattern, not an exact ID.
const patternChars = `[(*?+\.^${}|`

func isPattern(s string) bool {
	return strings.ContainsAny(s, patternChars)
}

// resolveTaskIDs resolves a slice of args (exact IDs or a single regex/glob pattern)
// into a ResolvedTask slice. Returns requiresConfirmation=true when a pattern
// matched more than one task.
func resolveTaskIDs(ctx context.Context, args []string, s *storage.SQLiteStorage, query core.Query) ([]*uri.ResolvedTask, bool, error) {
	hasPattern := false
	hasExact := false
	for _, a := range args {
		if isPattern(a) {
			hasPattern = true
		} else {
			hasExact = true
		}
	}

	if hasPattern && hasExact {
		return nil, false, fmt.Errorf("cannot mix exact IDs and patterns; use one or the other")
	}

	if hasPattern {
		if len(args) > 1 {
			return nil, false, fmt.Errorf("only one pattern argument is supported")
		}
		return resolveByPattern(ctx, args[0], s, query)
	}

	// All exact IDs — resolve one by one; no confirmation required.
	resolver := uri.NewResolver(s)
	resolved := make([]*uri.ResolvedTask, 0, len(args))
	for _, id := range args {
		res, err := resolver.ResolveTask(ctx, id)
		if err != nil {
			return nil, false, err
		}
		resolved = append(resolved, res)
	}
	return resolved, false, nil
}

// resolveByPattern lists all tasks matching pattern (regex; "*" treated as ".*").
// Returns requiresConfirmation=true when more than one task matched.
func resolveByPattern(ctx context.Context, pattern string, s *storage.SQLiteStorage, query core.Query) ([]*uri.ResolvedTask, bool, error) {
	if pattern == "*" {
		pattern = ".*"
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, false, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	q := query
	if q.Limit == 0 {
		q.Limit = 10000
	}
	tasks, err := s.ListTasks(ctx, q)
	if err != nil {
		return nil, false, fmt.Errorf("failed to list tasks: %w", err)
	}

	var matched []*uri.ResolvedTask
	for _, t := range tasks {
		if re.MatchString(t.ID) {
			matched = append(matched, &uri.ResolvedTask{Task: t, Storage: s})
		}
	}

	if len(matched) == 0 {
		return nil, false, fmt.Errorf("pattern %q matched no tasks", pattern)
	}

	requiresConfirmation := len(matched) > 1
	return matched, requiresConfirmation, nil
}

// confirmBatch shows an interactive confirmation prompt listing matched task IDs.
// Returns nil if confirmed, error if rejected or aborted.
// Skipped entirely when --no-prompt is set.
func confirmBatch(cmd *cobra.Command, tasks []*uri.ResolvedTask, pattern string) error {
	if taskNoPrompt {
		return nil
	}

	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.Task.ID
	}

	msg := fmt.Sprintf("%d tasks matched %q: %s\nProceed?",
		len(tasks), pattern, strings.Join(ids, ", "))

	var confirmed bool
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(msg).
				Value(&confirmed),
		),
	).WithOutput(cmd.OutOrStdout()).Run()

	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("aborted")
	}
	return nil
}
