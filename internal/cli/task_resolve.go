package cli

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"charm.land/huh/v2"
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

// taskRefAliasRe and taskRefDigitsRe gate which inputs we route through
// core.ParseTaskRef. Anything else (cross-project URIs, scheme-prefixed
// refs, etc.) keeps falling through to uri.NewResolver below.
var (
	taskRefAliasRe  = regexp.MustCompile(`(?i)^@?T-\d+$`)
	taskRefDigitsRe = regexp.MustCompile(`^\d+$`)
)

// parseTaskRefForCLI maps a user-facing task reference to its durable
// typeid using core.ParseTaskRef. Inputs that aren't one of the
// supported short forms (task_<typeid>, T-NNNN, bare digits) are
// returned unchanged so legacy URI/cross-project refs fall through to
// the existing uri.Resolver path. When core.ParseTaskRef cannot resolve
// a short-form ref, the original input is returned so the legacy
// resolver gets a chance to find tasks whose IDs literally match the
// alias (e.g. tasks fixtures inserted as `T-0046` without a matching
// seq column).
func parseTaskRefForCLI(ctx context.Context, s *storage.SQLiteStorage, input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input, nil
	}
	// Strip a single leading "@" so "@T-0001" resolves like "T-0001".
	candidate := strings.TrimPrefix(trimmed, "@")

	switch {
	case core.IsTaskID(candidate),
		taskRefAliasRe.MatchString(candidate),
		taskRefDigitsRe.MatchString(candidate):
		projectID := ""
		if proj := core.DetectProject(); proj != nil && proj.InProject {
			projectID = proj.ProjectID
		}
		resolved, err := core.ParseTaskRef(ctx, s, projectID, candidate)
		if err != nil {
			// Fall through to legacy resolver — it normalises bare
			// digits and zero-padded refs into T-NNNN and looks up
			// the task by its literal stored ID. This keeps fixtures
			// without an allocated seq (tests, legacy data) working.
			return input, nil
		}
		return resolved, nil
	}
	return input, nil
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
		canonical, err := parseTaskRefForCLI(ctx, s, id)
		if err != nil {
			return nil, false, err
		}
		res, err := resolver.ResolveTask(ctx, canonical)
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

	// Without a usable terminal, huh would crash opening /dev/tty. Require an
	// explicit --no-prompt instead of prompting.
	if !interactiveAvailable(cmd) {
		return fmt.Errorf(
			"%d tasks matched %q: %s; "+
				"pass --no-prompt to proceed without confirmation",
			len(tasks), pattern, strings.Join(ids, ", "),
		)
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
