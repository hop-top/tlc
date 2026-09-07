package cli

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/workspace"
)

var TaskListCmd = &cobra.Command{
	Use:   "list [query]",
	Short: "List tasks with filters",
	Long: `List tasks across the active project (or all projects with --all),
filtered by status, assignee, tag, priority, track, due/overdue, and more.

Defaults to active statuses (IN_PROGRESS + TODO) unless --status or
--archived is explicitly set. Supports temporal filters (--due-before,
--due-after, --overdue, --no-due) and aps profile / squad resolution.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fromConfig, err := applyConfigDefaults(cmd)
		if err != nil {
			return err
		}

		query := core.Query{
			Limit:           taskListLimit,
			Offset:          taskListOffset,
			SortBy:          taskListSortBy,
			SortDirection:   taskListSortDirection,
			IncludeArchived: taskListArchived,
			AllProjects:     taskListAllProjects,
		}

		// Aggregate formats count the match set, never a page of it.
		// Pagination is dropped from the query before it reaches the
		// store: a truncated count carries no signal that it is
		// truncated, so honoring --limit here under-reports silently
		// and JSON/YAML consumers never see a warning. --limit and an
		// aggregate format are contradictory; the aggregate wins, and
		// an explicit --limit is reported as ignored on stderr.
		aggregate := resolveAggregateFormat()
		if aggregate != "" {
			if cmd.Flags().Changed("limit") || cmd.Flags().Changed("offset") {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"note: --limit/--offset ignored with --%s; aggregates count the full match set\n",
					aggregate)
			}
			query.Limit = 0
			query.Offset = 0
		}

		if len(args) > 0 {
			query.Search = args[0]
		}

		if taskListMine || taskListAssignedTo == "me" {
			taskListAssignedTo = core.GetCurrentUser()
		}

		// --profile: resolve aps profile ID and filter by assignee.
		if taskListProfile != "" {
			resolved := core.GetGlobalResolver().Resolve(taskListProfile)
			taskListAssignedTo = resolved
		}

		// --squad: resolve squad members and filter by any of them.
		if taskListSquad != "" {
			members, err := core.ResolveSquadMembers(taskListSquad)
			if err != nil {
				return fmt.Errorf("failed to resolve squad %q: %w", taskListSquad, err)
			}
			if len(members) == 0 {
				return fmt.Errorf("squad %q has no members", taskListSquad)
			}
			query.Filters = append(query.Filters, core.FieldFilter{
				Field:    "assigned_to",
				Operator: core.OpIn,
				Value:    strings.Join(members, ","),
			})
		}

		statusFlags := taskListStatus
		statusProvided := cmd.Flags().Changed("status") || fromConfig["status"]
		defaultStatusFilter := !statusProvided && !cmd.Flags().Changed("archived") && !fromConfig["archived"]
		if defaultStatusFilter {
			statusFlags = []string{"IN_PROGRESS", "TODO"}
			query.StatusPriority = string(core.StatusInProgress)
		}
		for _, st := range statusFlags {
			normalized, ok := NormalizeStatus(st)
			if !ok {
				return fmt.Errorf("unknown status %q; valid values: TODO, IN_PROGRESS, DONE, SKIPPED", st)
			}
			query.Filters = append(query.Filters, core.FieldFilter{Field: "status", Value: normalized})
		}
		if taskListAssignedTo != "" {
			query.Filters = append(query.Filters, core.FieldFilter{Field: "assigned_to", Value: taskListAssignedTo})
		}
		for _, tag := range taskListTag {
			query.Filters = append(query.Filters, core.FieldFilter{Field: "tags", Operator: core.OpContains, Value: tag})
		}
		for _, p := range taskListPriority {
			normalized, ok := NormalizePriority(p)
			if !ok {
				return fmt.Errorf("unknown priority %q; valid values: P0, P1, P2, P3", p)
			}
			query.Filters = append(query.Filters, core.FieldFilter{Field: "priority", Value: normalized})
		}

		// Resolve --track IDs via fuzzy match before building
		// query filters. Uses a temporary storage handle so the
		// workspace branch is not forced to open local storage.
		var trackQIDs []core.QualifiedTrackID
		if taskListTrack == "-" || taskListTrack == "null" ||
			(cmd.Flags().Changed("track") && taskListTrack == "") {
			// --track - / --track null / --track "" → untracked tasks only.
			query.Filters = append(query.Filters, core.FieldFilter{
				Field: "track_id", Operator: core.OpEq, Value: nil,
			})
		} else if taskListTrack != "" {
			trackQIDs = core.ParseMultiTrackIDs(taskListTrack)
			hasLocal := false
			for _, q := range trackQIDs {
				if q.IsLocal() {
					hasLocal = true
					break
				}
			}
			if hasLocal {
				rs, rsErr := getStorageRaw()
				if rsErr != nil {
					return rsErr
				}
				for i, q := range trackQIDs {
					if q.IsLocal() {
						resolved, rErr := resolveTrackID(
							ctx, rs, q.TrackID,
						)
						if rErr != nil {
							_ = rs.Close()
							return rErr
						}
						trackQIDs[i].TrackID = resolved
					}
				}
				_ = rs.Close()
			}
			if len(trackQIDs) == 1 && trackQIDs[0].IsLocal() {
				query.Filters = append(query.Filters, core.FieldFilter{
					Field: "track_id", Operator: core.OpEq,
					Value: trackQIDs[0].TrackID,
				})
				trackQIDs = nil
			} else if len(trackQIDs) > 0 {
				var ids []string
				for _, q := range trackQIDs {
					ids = append(ids, q.TrackID)
				}
				query.Filters = append(query.Filters, core.FieldFilter{
					Field:    "track_id",
					Operator: core.OpIn,
					Value:    strings.Join(ids, ","),
				})
			}
		}

		// Temporal filter validation + parsing (T-0908).
		// Mutual exclusion is checked here, before opening storage, so
		// errors surface against the user's flags without I/O side
		// effects. Parsing uses util.ParseUntil per
		// docs/temporal-spec-0.1.md §5 (forward-looking expressions).
		if err := applyTemporalFilters(cmd, &query); err != nil {
			return err
		}

		// Workspace mode: query across workspace projects.
		if cmd.Flags().Changed("workspace") {
			return runTaskListWorkspace(cmd, ctx, query)
		}

		// Default: single-project query.
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		// Aggregate fast path: count in SQL over the full match set so
		// no rows are materialized. Only valid when every filter is
		// expressible in the query — the post-query filters below
		// (--stale/--blocked/--blocked-by/--overdue, qualified track
		// IDs) are not, so those fall through to counting the full,
		// unpaginated slice instead.
		if aggregate != "" && canCountInStore(aggregate, query, trackQIDs) {
			counts, cErr := s.CountTasksByStatus(ctx, query)
			if cErr != nil {
				return fmt.Errorf("failed to count tasks: %w", cErr)
			}
			return renderAggregate(cmd, aggregate, counts)
		}

		tasks, err := s.ListTasks(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Load stale config once; apply project default timeout + auto-fire hooks.
		var taskCfg config.TaskConfig
		_ = viper.UnmarshalKey("task", &taskCfg) //nolint:errcheck // best-effort config load
		_ = taskCfg.Validate()                   //nolint:errcheck // best-effort validation

		// Apply project default stale timeout to all tasks with nil StaleTimeout.
		for _, t := range tasks {
			if t.StaleTimeout == nil && taskCfg.Stale.DefaultTimeout > 0 {
				d := taskCfg.Stale.DefaultTimeout
				t.StaleTimeout = &d
			}
		}

		// Auto-fire stale hooks once per crossing (StaleFiredAt == nil guards re-fire).
		if len(taskCfg.Stale.Hooks) > 0 {
			now := time.Now().UTC()
			for _, t := range tasks {
				if t.IsStale() && t.StaleFiredAt == nil {
					_ = core.RunStaleHooks(t, taskCfg.Stale.Hooks) //nolint:errcheck // best-effort stale hook
					t.StaleFiredAt = &now
					_ = s.UpdateTask(ctx, t) //nolint:errcheck // best-effort stale timestamp persist
				}
			}
		}

		// Post-query filter for --stale, --blocked, --blocked-by, --overdue.
		if taskListStale || taskListBlocked || len(taskListBlockedBy) > 0 || taskListOverdue {
			filtered := tasks[:0]
			for _, t := range tasks {
				if taskListStale && !t.IsStale() {
					continue
				}
				if taskListBlocked && !t.IsBlocked() {
					continue
				}
				if len(taskListBlockedBy) > 0 && !taskBlockedByAny(t, taskListBlockedBy) {
					continue
				}
				if taskListOverdue {
					if t.Status == core.StatusDone || t.Status == core.StatusSkipped {
						continue
					}
				}
				filtered = append(filtered, t)
			}
			tasks = filtered
		}

		// Post-query filter for qualified track IDs (project-scoped).
		if len(trackQIDs) > 0 {
			tasks = filterByQualifiedTracks(tasks, trackQIDs)
		}

		format := viper.GetString("output.format")
		if aggregate != "" {
			format = aggregate
		}
		return formatTasks(cmd, tasks, format, statusProvided)
	},
}

// resolveAggregateFormat returns the aggregate output format selected by
// --summary / --counters, or "" when ordinary list output is wanted.
// Aggregates are resolved before the store query because they change the
// query itself: pagination must be dropped so the counts cover the whole
// match set. --counters wins over --summary, matching the flag precedence
// the tail-end format resolution has always applied.
func resolveAggregateFormat() string {
	if taskListCounters {
		return formatCounters
	}
	if taskListSummary {
		return formatSummary
	}
	return ""
}

// canCountInStore reports whether the aggregate can be computed as a
// single SQL COUNT over the query, materializing no rows.
//
// Two things disqualify it. First, filters applied in Go after the store
// query returns (--stale, --blocked, --blocked-by, --overdue, qualified
// track IDs): a store-side COUNT would include rows those are about to
// discard. Second, --summary groups by project, a dimension flat status
// counts do not carry, so it can only be served this way when the query
// is already scoped to one project.
//
// Disqualification is not a fallback to the buggy behavior: the slice path
// still runs with pagination cleared, so the count covers the full match
// set either way. It just materializes the rows to get there.
func canCountInStore(format string, query core.Query, trackQIDs []core.QualifiedTrackID) bool {
	if taskListStale || taskListBlocked || taskListOverdue ||
		len(taskListBlockedBy) > 0 || len(trackQIDs) > 0 {
		return false
	}
	if format == formatSummary && query.AllProjects {
		return false
	}
	return true
}

// renderAggregate writes store-computed status counts in the selected
// aggregate format.
func renderAggregate(cmd *cobra.Command, format string, counts map[string]int) error {
	out := cmd.OutOrStdout()
	switch format {
	case formatCounters:
		renderCountersFromCounts(out, counts)
	case formatSummary:
		renderSummaryFromCounts(out, map[string]map[string]int{
			aggregateProjectLabel(): counts,
		})
	default:
		return fmt.Errorf("unsupported aggregate format %q", format)
	}
	return nil
}

// aggregateProjectLabel names the project bucket for a grouped summary
// built from store counts, which carry no project dimension of their own.
// Only reached when the query is scoped to one project — see
// canCountInStore.
func aggregateProjectLabel() string {
	if proj := core.DetectProject(); proj != nil && proj.InProject && proj.ProjectID != "" {
		return proj.ProjectID
	}
	return noProject
}

// filesystemOpener implements workspace.SourceOpener for local SQLite DBs.
type filesystemOpener struct{}

func (f *filesystemOpener) Open(project core.RegisteredProject) (workspace.ProjectSource, error) {
	return workspace.NewFilesystemSource(project.DBPath), nil
}

// runTaskListWorkspace queries tasks across all projects in a workspace.
func runTaskListWorkspace(cmd *cobra.Command, ctx context.Context, query core.Query) error {
	var workspaces []config.WorkspaceConfig
	if err := viper.UnmarshalKey("workspaces", &workspaces); err != nil {
		return fmt.Errorf("failed to read workspace config: %w", err)
	}

	cfg := &config.Config{Workspaces: workspaces}

	var ws *config.WorkspaceConfig
	if taskListWorkspace == "" {
		ws = cfg.DefaultWorkspace()
	} else {
		ws = cfg.FindWorkspace(taskListWorkspace)
	}
	if ws == nil {
		return fmt.Errorf("workspace not found: %q", taskListWorkspace)
	}

	store, err := getStorageRaw()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer func() { _ = store.Close() }()

	reg := workspace.NewRegistry()
	reg.Register(workspace.NewFilesystemAdapter(store))

	projects, err := workspace.ListProjects(*ws, reg)
	if err != nil {
		log.Warn("Some spaces had errors", "error", err)
	}

	// Filter by --space if set.
	if taskListSpace != "" {
		var filtered []core.RegisteredProject
		for _, p := range projects {
			if p.SpaceURI == taskListSpace {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}

	if len(projects) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No projects found in workspace")
		return nil
	}

	tasks, err := workspace.QueryAcross(ctx, projects, query, &filesystemOpener{})
	if err != nil {
		return fmt.Errorf("workspace query failed: %w", err)
	}

	// QueryAcross ignores StatusPriority; reintroduce IN_PROGRESS-first
	// ordering via a post-merge stable sort so the base sort is preserved.
	if query.StatusPriority != "" {
		prio := core.TaskStatus(query.StatusPriority)
		sort.SliceStable(tasks, func(i, j int) bool {
			ip := tasks[i].Status == prio
			jp := tasks[j].Status == prio
			return ip && !jp
		})
	}

	format := viper.GetString("output.format")
	if agg := resolveAggregateFormat(); agg != "" {
		format = agg
	}
	return formatWorkspaceTasks(cmd, tasks, format)
}

// projectLabel returns a short label from a project ID.
// e.g. "hop-top/tlc" -> "tlc", "my-project" -> "my-project"
func projectLabel(projectID string) string {
	return path.Base(projectID)
}

// formatWorkspaceTasks renders tasks with project context.
func formatWorkspaceTasks(cmd *cobra.Command, tasks []*core.Task, format string) error {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON, formatYAML:
		return formatTasks(cmd, tasks, format, false)
	case "tls":
		for _, t := range tasks {
			label := ""
			if t.ProjectID != nil && *t.ProjectID != "" {
				label = projectLabel(*t.ProjectID)
			}
			if label != "" {
				_, _ = fmt.Fprintf(out, "[%s] %s\n", label, formatTLS(t))
			} else {
				_, _ = fmt.Fprintln(out, formatTLS(t))
			}
		}
	case formatSummary:
		renderSummary(out, tasks)
	case formatCounters:
		renderCounters(out, tasks)
	case formatVtodo:
		return writeVtodo(cmd, tasks, nil, taskListOutput, taskListIncludeLogs)
	default:
		renderWorkspaceTable(out, tasks)
	}
	return nil
}

func init() {
	TaskListCmd.Flags().StringSliceVarP(&taskListStatus, "status", "s", []string{}, "Filter by status")
	TaskListCmd.Flags().StringVarP(&taskListAssignedTo, "assigned-to", "a", "", "Filter by assignee")
	TaskListCmd.Flags().StringSliceVar(&taskListTag, "tag", []string{}, "Filter by tag")
	TaskListCmd.Flags().BoolVar(&taskListMine, "mine", false, "Filter by current user")
	TaskListCmd.Flags().BoolVar(&taskListArchived, "archived", false, "Show archived tasks")
	TaskListCmd.Flags().BoolVar(&taskListAllProjects, "all-projects", false, "Show tasks from all projects")
	TaskListCmd.Flags().StringVar(&taskListSortBy, "sort-by", "created_at", "Sort field")
	TaskListCmd.Flags().StringVar(&taskListSortDirection, "sort-direction", "desc", "Sort direction (asc, desc)")
	TaskListCmd.Flags().IntVarP(&taskListLimit, "limit", "n", 100, "Limit results")
	TaskListCmd.Flags().IntVar(&taskListOffset, "offset", 0, "Skip results")
	TaskListCmd.Flags().BoolVar(&taskListSummary, "summary", false, "Show status summary instead of task list")
	TaskListCmd.Flags().BoolVar(&taskListCounters, "counters", false, "Show flat status counters")
	TaskListCmd.Flags().StringVar(&taskListWorkspace, "workspace", "", "Query across workspace projects")
	TaskListCmd.Flags().StringVar(&taskListSpace, "space", "", "Filter to specific space within workspace")
	TaskListCmd.Flags().StringVar(&taskListProfile, "profile", "", "Filter by aps profile")
	TaskListCmd.Flags().StringVar(&taskListSquad, "squad", "", "Filter by aps squad members")
	TaskListCmd.Flags().BoolVar(&taskListStale, "stale", false, "Show only stale tasks")
	TaskListCmd.Flags().BoolVar(&taskListBlocked, "blocked", false, "Show only blocked tasks")
	TaskListCmd.Flags().StringSliceVar(&taskListPriority, "priority", []string{}, "Filter by priority (P0, P1, P2, P3)")
	TaskListCmd.Flags().StringSliceVar(&taskListBlockedBy, "blocked-by", []string{}, "Show only tasks blocked by the given task IDs")
	TaskListCmd.Flags().StringVar(&taskListTrack, "track", "", "Filter by track ID")
	TaskListCmd.Flags().StringVar(&taskListOutput, "output", "", "Write output to a file instead of stdout")
	TaskListCmd.Flags().BoolVar(&taskListIncludeLogs, "include-logs", false, "Include audit log entries (vtodo: emit VJOURNAL components)")

	// Temporal filters (T-0908). Values for --due-before/--due-after are
	// parsed with util.ParseUntil per docs/temporal-spec-0.1.md §5.
	TaskListCmd.Flags().StringVar(&taskListDueBefore, "due-before", "", "Show tasks with due_at before the given time (e.g. tomorrow, 2026-05-12, +24h)")
	TaskListCmd.Flags().StringVar(&taskListDueAfter, "due-after", "", "Show tasks with due_at after the given time")
	TaskListCmd.Flags().BoolVar(&taskListOverdue, "overdue", false, "Show overdue tasks (due_at < now AND status not in DONE,SKIPPED)")
	TaskListCmd.Flags().BoolVar(&taskListNoDue, "no-due", false, "Show only tasks with no due date set")
}

// applyTemporalFilters validates and applies --due-before, --due-after,
// --overdue, and --no-due to the query. Mutually-exclusive combinations
// are rejected with a clear error. Time strings are parsed via
// util.ParseUntil per docs/temporal-spec-0.1.md §5.
func applyTemporalFilters(cmd *cobra.Command, query *core.Query) error {
	hasDueBefore := cmd.Flags().Changed("due-before")
	hasDueAfter := cmd.Flags().Changed("due-after")

	// Mutual exclusion: --overdue conflicts with --due-before / --due-after.
	if taskListOverdue && (hasDueBefore || hasDueAfter) {
		return fmt.Errorf("--overdue is mutually exclusive with --due-before / --due-after")
	}
	// Mutual exclusion: --no-due cannot be combined with the others.
	if taskListNoDue && (hasDueBefore || hasDueAfter || taskListOverdue) {
		return fmt.Errorf("--no-due is mutually exclusive with --due-before / --due-after / --overdue")
	}

	if hasDueBefore && taskListDueBefore != "" {
		t, err := util.ParseUntil(taskListDueBefore)
		if err != nil {
			return fmt.Errorf("invalid --due-before %q: %w", taskListDueBefore, err)
		}
		query.DueBefore = &t
	}
	if hasDueAfter && taskListDueAfter != "" {
		t, err := util.ParseUntil(taskListDueAfter)
		if err != nil {
			return fmt.Errorf("invalid --due-after %q: %w", taskListDueAfter, err)
		}
		query.DueAfter = &t
	}
	if taskListOverdue {
		now := time.Now().UTC()
		query.DueBefore = &now
		// Status exclusion (DONE / SKIPPED) is applied post-query in
		// the same way --stale / --blocked filter results — adding it
		// to query.Filters would merge under the existing OR-joined
		// status group and silently widen the result set.
	}
	if taskListNoDue {
		f := false
		query.HasDue = &f
	}
	return nil
}

// taskBlockedByAny reports whether t is blocked by any of the given IDs.
func taskBlockedByAny(t *core.Task, ids []string) bool {
	blockers := t.BlockedBy()
	for _, want := range ids {
		for _, b := range blockers {
			if strings.EqualFold(b, want) {
				return true
			}
		}
	}
	return false
}

// filterByQualifiedTracks returns tasks matching any of the qualified track IDs.
// Local QIDs match any task with that track_id regardless of project.
// Qualified QIDs match only tasks whose track_id and project_id both match.
func filterByQualifiedTracks(tasks []*core.Task, qids []core.QualifiedTrackID) []*core.Task {
	filtered := tasks[:0]
	for _, t := range tasks {
		if t.TrackID == nil || *t.TrackID == "" {
			continue
		}
		for _, q := range qids {
			if *t.TrackID != q.TrackID {
				continue
			}
			if q.IsLocal() {
				filtered = append(filtered, t)
				break
			}
			// Qualified: must also match project.
			projID := q.ProjectID()
			if t.ProjectID != nil && *t.ProjectID == projID {
				filtered = append(filtered, t)
				break
			}
		}
	}
	return filtered
}
