package cli

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/workspace"
)

var TaskListCmd = &cobra.Command{
	Use:   "list [query]",
	Short: "List tasks with filters",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		query := core.Query{
			Limit:           taskListLimit,
			Offset:          taskListOffset,
			SortBy:          taskListSortBy,
			SortDirection:   taskListSortDirection,
			IncludeArchived: taskListArchived,
			AllProjects:     taskListAllProjects,
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
		if !cmd.Flags().Changed("status") && !cmd.Flags().Changed("archived") {
			statusFlags = []string{"IN_PROGRESS", "TODO"}
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

		tasks, err := s.ListTasks(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Load stale config once; apply project default timeout + auto-fire hooks.
		var taskCfg config.TaskConfig
		_ = viper.UnmarshalKey("task", &taskCfg)
		_ = taskCfg.Validate()

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
					_ = core.RunStaleHooks(t, taskCfg.Stale.Hooks)
					t.StaleFiredAt = &now
					_ = s.UpdateTask(ctx, t)
				}
			}
		}

		// Post-query filter for --stale, --blocked, --blocked-by.
		if taskListStale || taskListBlocked || len(taskListBlockedBy) > 0 {
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
				filtered = append(filtered, t)
			}
			tasks = filtered
		}

		// Post-query filter for qualified track IDs (project-scoped).
		if len(trackQIDs) > 0 {
			tasks = filterByQualifiedTracks(tasks, trackQIDs)
		}

		if !cmd.Flags().Changed("status") && !cmd.Flags().Changed("archived") {
			sort.SliceStable(tasks, func(i, j int) bool {
				iIP := tasks[i].Status == "IN_PROGRESS"
				jIP := tasks[j].Status == "IN_PROGRESS"
				return iIP && !jIP
			})
		}

		format := viper.GetString("output.format")
		if taskListSummary {
			format = formatSummary
		}
		if taskListCounters {
			format = formatCounters
		}
		formatTasks(cmd, tasks, format)
		return nil
	},
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

	if !cmd.Flags().Changed("status") && !cmd.Flags().Changed("archived") {
		sort.SliceStable(tasks, func(i, j int) bool {
			iIP := tasks[i].Status == "IN_PROGRESS"
			jIP := tasks[j].Status == "IN_PROGRESS"
			return iIP && !jIP
		})
	}

	format := viper.GetString("output.format")
	if taskListSummary {
		format = formatSummary
	}
	if taskListCounters {
		format = formatCounters
	}
	formatWorkspaceTasks(cmd, tasks, format)
	return nil
}

// projectLabel returns a short label from a project ID.
// e.g. "hop-top/tlc" -> "tlc", "my-project" -> "my-project"
func projectLabel(projectID string) string {
	return path.Base(projectID)
}

// formatWorkspaceTasks renders tasks with project context.
func formatWorkspaceTasks(cmd *cobra.Command, tasks []*core.Task, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON:
		formatTasks(cmd, tasks, format)
	case formatYAML:
		formatTasks(cmd, tasks, format)
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
	default:
		renderWorkspaceTable(out, tasks)
	}
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
