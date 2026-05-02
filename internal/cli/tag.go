package cli

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/uri"
)

// taskIDPattern matches IDs like T-0001, T-42, abc/T-0001, tlc:// URIs.
var taskIDPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]*/)?[A-Z]-\d+$|^tlc://`)

// looksLikeTaskID returns true if s appears to be a task identifier.
func looksLikeTaskID(s string) bool {
	return taskIDPattern.MatchString(s)
}

// TagCmd is the top-level `tlc tag` command group.
var TagCmd = &cobra.Command{
	Use:   "tag",
	Short: "Tag operations",
	Long: `Tag operations.

Usage:
  tlc tag list              – list all unique tags
  tlc tag <task-id> <tag…>  – add tags to a task
  tlc tag <tag…>            – filter tasks by tags (AND/OR semantics)

AND/OR semantics for filter mode:
  Comma-separated values within one argument = OR (any match).
  Multiple arguments                         = AND (all must match).

  tlc tag feat,bug          → tasks tagged feat OR bug
  tlc tag feat bug          → tasks tagged feat AND bug
  tlc tag feat feat,bug     → tasks tagged feat AND (feat OR bug)`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		// Route: if first arg looks like a task ID and ≥2 args, run tag-add mode.
		if len(args) >= 2 && looksLikeTaskID(args[0]) {
			return runTagAdd(cmd, args[0], args[1:])
		}
		// Otherwise: filter mode.
		return runTagFilter(cmd, args)
	},
}

// TagListCmd lists all unique tags across all tasks.
var TagListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all unique tags",
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		tags, err := s.ListAllTags(ctx)
		if err != nil {
			return fmt.Errorf("failed to list tags: %w", err)
		}

		sort.Strings(tags)

		out := cmd.OutOrStdout()
		if len(tags) == 0 {
			_, _ = fmt.Fprintln(out, "No tags found.")
			return nil
		}
		for _, t := range tags {
			_, _ = fmt.Fprintln(out, t)
		}
		return nil
	},
}

// runTagAdd adds tags to a task given its ID.
func runTagAdd(cmd *cobra.Command, taskID string, newTags []string) error {
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	res, err := uri.NewResolver(s).ResolveTask(ctx, taskID)
	if err != nil {
		return err
	}
	task := res.Task
	if res.Storage != s {
		defer func() { _ = res.Storage.Close() }()
	}

	// Merge tags (dedup, sorted).
	tagSet := make(map[string]struct{}, len(task.Tags)+len(newTags))
	for _, t := range task.Tags {
		tagSet[t] = struct{}{}
	}
	for _, t := range newTags {
		tagSet[t] = struct{}{}
	}
	merged := make([]string, 0, len(tagSet))
	for t := range tagSet {
		merged = append(merged, t)
	}
	sort.Strings(merged)
	task.Tags = merged
	task.UpdatedAt = time.Now().UTC()

	if err := res.Storage.UpdateTask(ctx, task); err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Updated tags for %s: %s\n",
		task.ID, strings.Join(task.Tags, ", "))
	return nil
}

// runTagFilter filters/lists tasks by tags with AND(OR) semantics.
// Each arg is an OR-group (comma-separated values); multiple args are ANDed.
func runTagFilter(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	query := core.Query{
		Limit:         100,
		SortBy:        "created_at",
		SortDirection: "desc",
	}

	// Default: show IN_PROGRESS + TODO unless --all-statuses.
	if !tagFilterAllStatuses {
		query.Filters = append(query.Filters,
			core.FieldFilter{Field: "status", Value: "IN_PROGRESS"},
			core.FieldFilter{Field: "status", Value: "TODO"},
		)
	}

	// Parse OR-groups from args.
	type orGroup = []string
	var groups []orGroup
	for _, arg := range args {
		parts := strings.Split(arg, ",")
		trimmed := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				trimmed = append(trimmed, p)
			}
		}
		if len(trimmed) > 0 {
			groups = append(groups, trimmed)
		}
	}

	if len(groups) == 0 {
		return cmd.Help()
	}

	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	// The storage layer ORs multiple filters for the same field, so we cannot
	// push AND-semantics for tags into the query. Instead, push only the first
	// tag as a pre-filter (to reduce the result set), then apply full AND(OR)
	// logic in Go via filterTasksByTagGroups.
	//
	// Special case: single group with one tag → pure storage filter is exact.
	var tasks []*core.Task

	if len(groups) == 1 && len(groups[0]) == 1 {
		// Simple single-tag filter: storage handles it exactly.
		query.Filters = append(query.Filters,
			core.FieldFilter{Field: "tags", Operator: core.OpContains, Value: groups[0][0]})
		tasks, err = s.ListTasks(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}
	} else {
		// Multi-tag or OR-group: fetch all (status-filtered) then post-filter.
		all, lErr := s.ListTasks(ctx, query)
		if lErr != nil {
			return fmt.Errorf("failed to list tasks: %w", lErr)
		}
		tasks = filterTasksByTagGroups(all, groups)
	}

	return formatTasks(cmd, tasks, "")
}

// filterTasksByTagGroups returns tasks satisfying all OR-groups (AND of ORs).
func filterTasksByTagGroups(tasks []*core.Task, groups [][]string) []*core.Task {
	var result []*core.Task
	for _, task := range tasks {
		tagSet := make(map[string]struct{}, len(task.Tags))
		for _, t := range task.Tags {
			tagSet[t] = struct{}{}
		}
		satisfied := true
		for _, group := range groups {
			matched := false
			for _, t := range group {
				if _, has := tagSet[t]; has {
					matched = true
					break
				}
			}
			if !matched {
				satisfied = false
				break
			}
		}
		if satisfied {
			result = append(result, task)
		}
	}
	return result
}

var tagFilterAllStatuses bool

func init() {
	TagCmd.PersistentFlags().BoolVar(&tagFilterAllStatuses, "all-statuses", false,
		"Include tasks of all statuses (default: IN_PROGRESS + TODO only)")

	TagCmd.AddCommand(TagListCmd)

	RootCmd.AddCommand(TagCmd)
}
