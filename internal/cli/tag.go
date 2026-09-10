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

// taskIDPattern matches display-alias forms: T-0001, T-42, abc/T-0001,
// and tlc:// URIs. Case-insensitive on the alias so `t-0001` routes like
// `T-0001` — every other task-argument command accepts that spelling, and
// routing it to filter mode instead produced "no tasks tagged t-0001".
var taskIDPattern = regexp.MustCompile(`(?i)^([a-z][a-z0-9_-]*/)?[a-z]-\d+$|^tlc://`)

// looksLikeTaskID returns true if s appears to be a task identifier.
//
// This is a ROUTER, not a validator: it decides whether `tlc tag X …`
// means "add tags to task X" or "filter by tags X …". It therefore has to
// recognize every form a user may address a task by, including the
// durable TypeID — which the alias pattern above cannot match, and which
// was consequently misrouted into filter mode and answered with an empty
// result rather than tagging the task.
func looksLikeTaskID(s string) bool {
	return core.IsTaskID(s) || taskIDPattern.MatchString(s)
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
	Annotations: map[string]string{
		"kit/top-level-verb": "true",
	},
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
	Long:    "List every unique tag value across all tasks in the active storage.",
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
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
	// `tlc tag <id> <tag…>` writes tags without going through
	// applyTaskFieldChanges, so it needs the gate of its own — it is the
	// most direct tag write in the tool and would otherwise be the hole
	// the policy leaks through.
	//
	// BEFORE the task is resolved, deliberately: the gate is a statement
	// about the TAGS, and nothing about the task can make a disallowed
	// tag allowed. Rejecting on the tag before spending a lookup is both
	// cheaper and a better message — "this tag is not permitted" rather
	// than a not-found for a task the user addressed correctly.
	//
	// Only the NEW tags are checked, for the reason applyTaskFieldChanges
	// spells out — a task already carrying a since-disallowed tag must
	// stay editable.
	if err := core.ValidateTags(newTags); err != nil {
		return err
	}

	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	// Resolve through the same path `task show` and every other
	// task-argument command uses. T-NNNN is a DISPLAY ALIAS rendered on
	// demand from (project_id, seq) and never stored; the durable
	// identity is the TypeID. uri.Resolver looks up stored identifiers
	// only, so calling it directly — as this command used to — could not
	// find the one form users actually type, and `tlc tag T-0001 x`
	// returned NOT_FOUND for a task that plainly existed.
	//
	// parseTaskRefForCLI does the project-scoped seq lookup and returns
	// the input unchanged when it is not a short form, so URI and
	// cross-project refs still fall through to the resolver untouched.
	canonical, err := parseTaskRefForCLI(ctx, s, taskID)
	if err != nil {
		return err
	}
	res, err := uri.NewResolver(s).ResolveTask(ctx, canonical)
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

	// Default: unfinished work only, unless --all-statuses.
	//
	// Role-derived rather than named, matching `task list`. The literals
	// this replaces were RAW STRINGS — not even the core constants — so
	// they were config-blind twice over, and silently: on a vocabulary
	// declaring neither name the filter matched nothing and the command
	// printed an empty result at exit 0.
	if !tagFilterAllStatuses {
		for _, st := range core.UnfinishedTaskStatuses() {
			query.Filters = append(query.Filters,
				core.FieldFilter{Field: "status", Value: st})
		}
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

	return formatTasks(cmd, tasks, "", false)
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
	// Worded, not named. This runs at package init — long before any
	// config file is read — so naming the default's statuses here can
	// only ever name the ones this package happens to know, and a help
	// string listing statuses the user's config does not declare is its
	// own small lie. Reading config to fill it in would be worse: a
	// pre-argv config read pins the memoising DefaultWorkflow* singleton
	// and silently discards later `-c key=value` overrides.
	TagCmd.PersistentFlags().BoolVar(&tagFilterAllStatuses, "all-statuses", false,
		"Include tasks of all statuses (default: unfinished work only)")

	TagCmd.AddCommand(TagListCmd)

	RootCmd.AddCommand(TagCmd)
}
