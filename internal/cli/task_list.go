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

Defaults to unfinished work — every status your config gives the
"initial" or "active" role — unless --status or --archived is explicitly
set. Supports temporal filters (--due-before, --due-after, --overdue,
--no-due) and aps profile / squad resolution.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		fromConfig, err := applyConfigDefaults(cmd)
		if err != nil {
			return err
		}

		// Validated before storage opens, so a mistyped dimension fails
		// against the user's flags with no I/O side effects. The flag
		// enum already rejects unknown values at parse time; this is the
		// second gate for the config-supplied path, which never reaches
		// cobra's parser.
		if taskListGroupBy != "" && !ValidGroupByKey(taskListGroupBy) {
			return unknownGroupByError(taskListGroupBy)
		}

		// --group-limit caps rows WITHIN a group, so without a grouping
		// dimension there is nothing for it to cap. Accepting it there
		// would leave the user believing their listing was capped when
		// it was not: the flag would parse, exit 0, and do nothing.
		if taskListGroupLimit != 0 && taskListGroupBy == "" {
			return fmt.Errorf("--group-limit caps rows within each group and needs --group-by; " +
				"add --group-by <dimension>, or use --limit to cap the whole listing")
		}

		query := core.Query{
			Limit:           taskListLimit,
			Offset:          taskListOffset,
			SortBy:          taskListSortBy,
			SortDirection:   taskListSortDirection,
			IncludeArchived: taskListArchived,
			AllProjects:     taskListAllProjects,
			// Rank order for --sort-by priority and --sort-by effort.
			// The store cannot read config, so the configured
			// vocabularies travel with the query; without them those
			// sorts would mean alphabetical, which is right for P0..P3
			// by accident and right for XS..XL never.
			PriorityOrder: core.ConfiguredPriorityStrings(),
			EffortOrder:   core.ConfiguredEffortStrings(),
		}

		// Aggregate formats count the match set, never a page of it.
		// Pagination is dropped from the query before it reaches the
		// store: a truncated count carries no signal that it is
		// truncated, so honoring --limit here under-reports silently
		// and JSON/YAML consumers never see a warning. --limit and an
		// aggregate format are contradictory; the aggregate wins.
		//
		// Resolved once, here, and threaded onward: every branch below
		// must agree on whether this run is an aggregate, and a second
		// resolution is a second chance to disagree.
		aggregate := aggregateFormat()

		// --group-by and an aggregate format both claim the output
		// shape: one partitions the match set into row tables, the other
		// collapses it into counts. There is no reading of the pair that
		// satisfies either, so it is rejected rather than resolved —
		// silently picking a winner hands the user output for a command
		// they did not type.
		//
		// Gated on the RESOLVED aggregate, never on taskListSummary /
		// taskListCounters. An aggregate has three spellings and the
		// bools see only one of them; `-f summary` and a config
		// `output.format: summary` leave both false. Keying on the bools
		// is the same mistake aggregateFormat's doc comment records from
		// the pagination path, and here it would let exactly those two
		// spellings through the check.
		if aggregate != "" && taskListGroupBy != "" {
			return fmt.Errorf(
				"--group-by cannot be combined with --%s: --group-by lists rows in one table per %s, "+
					"--%s counts the whole match set; pick one",
				aggregate, taskListGroupBy, aggregate)
		}

		paginationRequested := cmd.Flags().Changed("limit") ||
			cmd.Flags().Changed("offset") ||
			fromConfig["limit"] || fromConfig["offset"]
		if aggregate != "" {
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
		// The default filter is "not-yet-finished work", derived from the
		// effective vocabulary's ROLES rather than named. Naming it — the
		// IN_PROGRESS + TODO literal this replaces — made bare `task list`
		// fail outright on any project whose `task.statuses` declares no
		// IN_PROGRESS: the default filter was invalid under the very
		// config it was filtering.
		if defaultStatusFilter {
			statusFlags = core.UnfinishedTaskStatuses()
			query.StatusPriority = core.PrimaryActiveTaskStatus()
		}
		for _, st := range statusFlags {
			normalized, ok := NormalizeStatus(st)
			if !ok {
				return unknownStatusError(st)
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
				return unknownPriorityError(p)
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

		// --blocked is a column predicate (blocked_reason non-empty),
		// so it pushes into the WHERE fragment rather than filtering
		// the returned slice — keeping --counters --blocked on the
		// counting path.
		if taskListBlocked {
			blocked := true
			query.Blocked = &blocked
		}

		// Temporal filter validation + parsing.
		// Mutual exclusion is checked here, before opening storage, so
		// errors surface against the user's flags without I/O side
		// effects. Parsing uses util.ParseUntil per
		// docs/temporal-spec-0.1.md §5 (forward-looking expressions).
		if err := applyTemporalFilters(cmd, &query); err != nil {
			return err
		}

		// Workspace mode: query across workspace projects.
		if cmd.Flags().Changed("workspace") {
			return runTaskListWorkspace(cmd, ctx, query, aggregate)
		}

		// Default: single-project query.
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		// Aggregate fast path: count in SQL over the full match set so
		// no rows are materialized. Valid exactly when nothing is
		// filtered in Go afterwards — see taskListPostFilters, which
		// is both the gate and the filter list, so the two cannot
		// drift. Counts come grouped by project because that is the
		// dimension --summary renders; --counters sums across them.
		postFilters := taskListPostFilters(trackQIDs)
		if aggregate != "" && len(postFilters) == 0 {
			byProject, cErr := s.CountTasksByProjectAndStatus(ctx, query)
			if cErr != nil {
				return fmt.Errorf("failed to count tasks: %w; retry, or drop --%s for a row listing", cErr, aggregate)
			}
			noteIgnoredPagination(cmd, aggregate, paginationRequested,
				totalProjectCounts(byProject))
			return renderAggregateCounts(cmd, aggregate, byProject)
		}

		tasks, err := s.ListTasks(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Stale bookkeeping — the project default timeout and the
		// auto-fired hooks — is for row output only. An aggregate
		// wants numbers; it does not read StaleFiredAt, and firing a
		// hook plus a write transaction per stale task off a command
		// annotated read-only is a side effect nobody asked for. The
		// boundary is documented in docs/task-crud-spec-0.1.md.
		if aggregate == "" {
			applyStaleDefaults(ctx, s, tasks)
		}

		tasks = applyPostFilters(tasks, postFilters)

		if aggregate != "" {
			noteIgnoredPagination(cmd, aggregate, paginationRequested, len(tasks))
			return formatTasks(cmd, tasks, aggregate, statusProvided)
		}
		// --group-by travels as an option into the single chokepoint
		// rather than branching here: `task list` reaches formatTasks
		// from several paths and each extra branch is a chance for two
		// of them to render differently. Aggregate formats above are
		// deliberately excluded — they emit counts, not rows.
		// A match set that came back exactly at the limit is
		// indistinguishable from one that was truncated by it, so it is
		// treated as truncated. Over-disclosing costs one stderr-shaped
		// note on an exact-fit listing; under-disclosing prints per-group
		// totals that are not the groups' totals, with nothing on screen
		// to say so — the asymmetry noteIgnoredPagination settles the
		// same way.
		partial := taskListLimit > 0 && len(tasks) >= taskListLimit

		// --group-limit caps what a screen shows; a structured payload
		// carries every task instead, so the flag does not apply there.
		// It is not dropped silently: exiting 0 with output that ignored
		// a typed flag is how a user comes to trust a cap that never ran.
		format := viper.GetString("output.format")
		if taskListGroupLimit > 0 && taskListGroupBy != "" && structuredFormat(format) {
			noteGroupLimitIgnored(cmd.ErrOrStderr(), format)
		}
		// Headings read as NAMES. The labeler is built HERE, where the
		// store handle lives, and threaded in: resolving a track title
		// needs a read, and formatTasks is a rendering chokepoint that
		// deliberately holds no storage. Built once per run — the track
		// lookup is a single batched pass, not a query per section.
		// Nil for every dimension whose keys are already names.
		labeler := groupLabelerFor(ctx, taskListGroupBy, s)

		return formatTasks(cmd, tasks, format, statusProvided,
			withGroupBy(taskListGroupBy),
			withGroupLimit(taskListGroupLimit),
			withPartialMatchSet(partial),
			withGroupLabeler(labeler))
	},
}

// filesystemOpener implements workspace.SourceOpener for local SQLite DBs.
type filesystemOpener struct{}

func (f *filesystemOpener) Open(project core.RegisteredProject) (workspace.ProjectSource, error) {
	return workspace.NewFilesystemSource(project.DBPath), nil
}

// runTaskListWorkspace queries tasks across all projects in a workspace.
func runTaskListWorkspace(
	cmd *cobra.Command, ctx context.Context, query core.Query, aggregate string,
) error {
	var workspaces []config.WorkspaceConfig
	if err := unmarshalConfigKey("workspaces", &workspaces); err != nil {
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
	if aggregate != "" {
		format = aggregate
	}
	// Grouping travels the same way the single-project path sends it —
	// as options into the one chokepoint. The labeler is nil here on
	// purpose: it resolves track titles out of the LOCAL store, and a
	// cross-project listing's track ids belong to other stores, so a
	// lookup would either miss or, worse, rename a group after an
	// unrelated project's track that happens to share an id.
	if taskListGroupLimit > 0 && taskListGroupBy != "" && structuredFormat(format) {
		noteGroupLimitIgnored(cmd.ErrOrStderr(), format)
	}
	partial := taskListLimit > 0 && len(tasks) >= taskListLimit
	return formatWorkspaceTasks(cmd, tasks, format,
		withGroupBy(taskListGroupBy),
		withGroupLimit(taskListGroupLimit),
		withPartialMatchSet(partial))
}

// projectLabel returns a short label from a project ID.
// e.g. "hop-top/tlc" -> "tlc", "my-project" -> "my-project"
func projectLabel(projectID string) string {
	return path.Base(projectID)
}

// formatWorkspaceTasks renders tasks with project context.
//
// Every arm delegates to formatTasks. The table arm used to be the
// exception — a private row struct with five fixed columns and its own
// renderer — which is why --cols, --group-by, --group-limit, truncation
// and priority-driven column dropping all stopped at the --workspace
// boundary while exiting 0. The only thing the cross-project view
// actually needs is one more column, and that is a column-registry key
// now, so it travels as an option instead of a second renderer.
//
// The tls arm stays local: it prefixes each line with the project label,
// which the shared formatter has no notion of.
func formatWorkspaceTasks(
	cmd *cobra.Command, tasks []*core.Task, format string, opts ...listOption,
) error {
	out := cmd.OutOrStdout()
	if format == "tls" {
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
		return nil
	}
	// Appended, not prepended: a caller that explicitly passed
	// withProjectColumn(false) — a test pinning the un-injected shape —
	// would otherwise be silently overridden by the default.
	return formatTasks(cmd, tasks, format, false,
		append([]listOption{withProjectColumn(true)}, opts...)...)
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
	TaskListCmd.Flags().StringSliceVar(&taskListPriority, "priority", []string{}, "Filter by priority")
	TaskListCmd.Flags().StringSliceVar(&taskListBlockedBy, "blocked-by", []string{}, "Show only tasks blocked by the given task IDs")
	TaskListCmd.Flags().StringVar(&taskListTrack, "track", "", "Filter by track ID")
	TaskListCmd.Flags().StringVar(&taskListOutput, "output", "", "Write output to a file instead of stdout")
	TaskListCmd.Flags().BoolVar(&taskListIncludeLogs, "include-logs", false, "Include audit log entries (vtodo: emit VJOURNAL components)")
	// Values are NOT spelled out in the usage string: the flag-enum
	// registration in registerFlagEnums is what renders them, into help,
	// the parse rejection and shell completion alike. See GroupByKeys.
	TaskListCmd.Flags().StringVar(&taskListGroupBy, "group-by", "", "Group results into one table per value of the given dimension")
	// Separate knob from --limit on purpose: --limit caps the match set
	// fetched from the store, this caps rows rendered within each group.
	// Zero means no cap, so an unset flag forwards unconditionally.
	TaskListCmd.Flags().IntVar(&taskListGroupLimit, "group-limit", 0, "Cap rows rendered within each --group-by group (0 = no cap)")

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
		// One definition of overdue, in SQL: due_at < now AND status
		// NOT IN (DONE, SKIPPED). core.Query.Overdue carries both
		// halves so the status exclusion cannot merge into the
		// OR-joined Filters status group and widen the result set —
		// and so `task list --overdue`, `status`, and
		// `--counters --overdue` all mean the same rows.
		now := time.Now().UTC()
		query.Overdue = &now
	}
	if taskListNoDue {
		f := false
		query.HasDue = &f
	}
	return nil
}
