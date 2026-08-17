package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uri"
)

type relatedTaskSummary struct {
	Ref    string
	Title  string
	Status core.TaskStatus
}

// blockerJSON is one entry of the top-level "blocked_by" array added to
// `task show --format json`.
//
// The raw durable IDs remain available under meta.blocked_by for
// backwards compatibility; this view resolves each ref to its display
// alias, title and status so dependencies are usable from scripts
// without a second lookup per edge.
type blockerJSON struct {
	// Ref is the resolved display alias (T-NNNN), project-qualified for
	// cross-project blockers. Falls back to the stored raw ref when the
	// blocker cannot be resolved.
	Ref string `json:"ref"`
	// ID is the durable task ID, empty when unresolved.
	ID string `json:"id,omitempty"`
	// Title is the blocker's title, empty when unresolved.
	Title string `json:"title,omitempty"`
	// Status is the blocker's status, empty when unresolved.
	Status core.TaskStatus `json:"status,omitempty"`
	// Met reports whether the blocker is satisfied (terminal status).
	// An unresolved blocker is never met.
	Met bool `json:"met"`
	// Missing flags a stored ref that resolves to no task.
	Missing bool `json:"missing,omitempty"`
}

// taskShowJSON wraps a task so the JSON/YAML view can carry derived
// fields alongside every original key. Embedding keeps the existing
// payload shape byte-for-byte and only adds keys.
type taskShowJSON struct {
	*core.Task
	BlockedBy []blockerJSON `json:"blocked_by,omitempty"`
}

// blockerSummariesToJSON converts resolved blocker summaries into the
// JSON view, marking each as met/unmet.
func blockerSummariesToJSON(refs []string, summaries []relatedTaskSummary) []blockerJSON {
	wm := core.DefaultWorkflow()
	out := make([]blockerJSON, 0, len(summaries))
	for i, s := range summaries {
		entry := blockerJSON{Ref: s.Ref, Title: s.Title, Status: s.Status}
		if s.Status == "" {
			// Unresolved: keep the stored ref so the edge stays visible.
			entry.Missing = true
			entry.Title = ""
			if i < len(refs) {
				entry.Ref = refs[i]
			}
		} else {
			if i < len(refs) {
				entry.ID = refs[i]
			}
			entry.Met = wm.IsTerminal(s.Status)
		}
		out = append(out, entry)
	}
	return out
}

var TaskShowCmd = &cobra.Command{
	Use:   "show <task-id>...",
	Short: "Show task details",
	Long: `Show full details for one or more tasks, including description,
status, assignee, dependencies, scheduling, and audit log entries.

Accepts task aliases (T-NNNN), durable TypeIDs, or cross-project URIs
(tlc://<project>/<id>). With --logs or a non-table --format, includes
the full log history sorted by configured direction.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()

		var errs []error
		for i, id := range args {
			canonical, parseErr := parseTaskRefForCLI(ctx, s, id)
			if parseErr != nil {
				errs = append(errs, fmt.Errorf("%s: %w", id, parseErr))
				continue
			}
			res, resolveErr := uri.NewResolver(s).ResolveTask(ctx, canonical)
			if resolveErr != nil {
				errs = append(errs, fmt.Errorf("%s: %w", id, resolveErr))
				continue
			}
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			var logs []*core.LogEntry
			if taskShowLogs || viper.GetString("output.format") != formatTable {
				direction := taskShowLogSortDirection
				if direction == "" {
					direction = viper.GetString("ui.log_sort_direction")
				}
				if direction == "" {
					direction = "desc"
				}
				logs, err = res.Storage.GetLogs(ctx, task.ID, direction)
				if err != nil {
					errs = append(errs, fmt.Errorf("%s: failed to get logs: %w", id, err))
					continue
				}
			}

			if i > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}

			format := viper.GetString("output.format")
			if format == formatJSON || format == formatYAML {
				// Resolve blockers so the structured view carries
				// seq IDs, titles and met/unmet state rather than
				// only the opaque IDs under meta.blocked_by.
				refs := task.BlockedBy()
				printTaskWithBlockers(
					cmd, task, logs, format,
					blockerSummariesToJSON(refs, resolveBlockedBySummaries(
						ctx, s, res.Storage, refs,
					)),
				)
				continue
			}
			if format == formatVtodo {
				if err := writeVtodo(cmd, []*core.Task{task}, nil, taskShowOutput, taskShowIncludeLogs); err != nil {
					return err
				}
				continue
			}

			blockedBy, blocking := collectTaskRelations(ctx, s, res.Storage, task)
			renderTaskDetail(cmd.OutOrStdout(), task, logs)

			currentProjectID := ""
			if task.ProjectID != nil && *task.ProjectID != "" {
				currentProjectID = *task.ProjectID
			} else if proj := core.DetectProject(); proj != nil {
				currentProjectID = proj.ProjectID
			}
			renderTaskRelations(cmd.OutOrStdout(), blockedBy, blocking, currentProjectID)
		}

		if len(errs) > 0 {
			// errors.Join preserves each wrapped sentinel so exitCodeFor
			// classifies the result (e.g. all not-found → ExitNotFound).
			joined := errors.Join(errs...)
			if len(args) == 1 {
				return joined
			}
			return fmt.Errorf("some tasks failed: %w", joined)
		}
		return nil
	},
}

func collectTaskRelations(ctx context.Context, registryStorage, taskStorage *storage.SQLiteStorage, task *core.Task) ([]relatedTaskSummary, []relatedTaskSummary) {
	return resolveBlockedBySummaries(ctx, registryStorage, taskStorage, task.BlockedBy()), findBlockingTasks(ctx, registryStorage, task)
}

func resolveBlockedBySummaries(ctx context.Context, registryStorage, taskStorage *storage.SQLiteStorage, refs []string) []relatedTaskSummary {
	if len(refs) == 0 {
		return nil
	}

	summaries := make([]relatedTaskSummary, 0, len(refs))
	for _, ref := range refs {
		resolverStorage := taskStorage
		crossProject := strings.Contains(ref, "/") || strings.Contains(ref, "://")
		if crossProject {
			resolverStorage = registryStorage
		}

		resolved, err := uri.NewResolver(resolverStorage).ResolveTask(ctx, ref)
		if err != nil {
			// The user-supplied ref is the only thing we have when
			// resolution fails. Render it as-is rather than pretending
			// to convert a typeid we cannot look up.
			summaries = append(summaries, relatedTaskSummary{Ref: ref, Title: "(missing)"})
			continue
		}

		// Render with the canonical display alias (T-NNNN) rather than
		// the raw typeid in `ref`. Qualify with the resolved task's
		// project when crossing project boundaries so the formatter can
		// shorten/elide as appropriate.
		displayRef := taskDisplayRef(resolved.Task)
		if crossProject && resolved.Task.ProjectID != nil && *resolved.Task.ProjectID != "" {
			displayRef = *resolved.Task.ProjectID + "/" + displayRef
		}

		summaries = append(summaries, relatedTaskSummary{
			Ref:    displayRef,
			Title:  resolved.Task.Title,
			Status: resolved.Task.Status,
		})
		if resolved.Storage != resolverStorage {
			_ = resolved.Storage.Close()
		}
	}
	return summaries
}

func findBlockingTasks(ctx context.Context, registryStorage *storage.SQLiteStorage, task *core.Task) []relatedTaskSummary {
	candidates := make(map[string]struct{})
	candidates[task.ID] = struct{}{}
	if task.ProjectID != nil && *task.ProjectID != "" {
		candidates[*task.ProjectID+"/"+task.ID] = struct{}{}
	}

	type storageHandle struct {
		key     string
		storage *storage.SQLiteStorage
		close   bool
	}

	handles := []storageHandle{{key: "current", storage: registryStorage}}
	seenHandles := map[string]struct{}{"current": {}}

	projects, err := registryStorage.ListAllProjects(ctx)
	if err == nil {
		for _, project := range projects {
			if _, ok := seenHandles[project.DBPath]; ok {
				continue
			}
			projectStorage, openErr := storage.NewSQLiteStorage(project.DBPath)
			if openErr != nil {
				continue
			}
			seenHandles[project.DBPath] = struct{}{}
			handles = append(handles, storageHandle{
				key:     project.DBPath,
				storage: projectStorage,
				close:   true,
			})
		}
	}

	// Build a project-qualified-only subset of candidates for
	// cross-project matching. Bare task IDs (e.g. "T-0001") must
	// only match within the same project; cross-project refs must
	// use the "project/T-NNNN" form.
	qualifiedOnly := make(map[string]struct{})
	for c := range candidates {
		if strings.Contains(c, "/") {
			qualifiedOnly[c] = struct{}{}
		}
	}

	found := make([]relatedTaskSummary, 0)
	seenRefs := make(map[string]struct{})
	for _, handle := range handles {
		tasks, listErr := handle.storage.ListTasks(ctx, core.Query{AllProjects: true})
		if handle.close {
			_ = handle.storage.Close()
		}
		if listErr != nil {
			continue
		}

		for _, candidate := range tasks {
			if candidate.ID == task.ID && sameProject(candidate.ProjectID, task.ProjectID) {
				continue
			}

			// Use the full candidate set for same-project tasks,
			// but only project-qualified refs for cross-project
			// tasks. This prevents bare T-ID collisions (T-0436).
			matchSet := candidates
			if !sameProject(candidate.ProjectID, task.ProjectID) {
				matchSet = qualifiedOnly
			}
			if !matchesTaskReference(candidate.BlockedBy(), matchSet) {
				continue
			}

			ref := relatedTaskRef(candidate)
			if _, ok := seenRefs[ref]; ok {
				continue
			}
			seenRefs[ref] = struct{}{}
			found = append(found, relatedTaskSummary{
				Ref:    ref,
				Title:  candidate.Title,
				Status: candidate.Status,
			})
		}
	}

	return found
}

func matchesTaskReference(blockedBy []string, candidates map[string]struct{}) bool {
	for _, ref := range blockedBy {
		if _, ok := candidates[ref]; ok {
			return true
		}
	}
	return false
}

func relatedTaskRef(task *core.Task) string {
	display := taskDisplayRef(task)
	if task.ProjectID != nil && *task.ProjectID != "" {
		return *task.ProjectID + "/" + display
	}
	return display
}

// taskDisplayRef returns the human-readable identifier for a task,
// matching the precedence in formatTaskAlias: a legacy non-typeid
// Task.ID (e.g. "T-0042" from pre-typeid rows) wins so we keep parity
// with the seq the row was originally created under; otherwise we
// synthesise the T-NNNN alias from Seq via core.FormatTaskDisplay,
// which itself falls back to the raw ID when Seq is unset.
func taskDisplayRef(t *core.Task) string {
	if t == nil {
		return ""
	}
	if t.ID != "" && !core.IsTaskID(t.ID) {
		return t.ID
	}
	return core.FormatTaskDisplay(t)
}

func sameProject(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func renderTaskRelations(w io.Writer, blockedBy, blocking []relatedTaskSummary, currentProjectID string) {
	if len(blockedBy) > 0 {
		_, _ = fmt.Fprintln(w, "\nBlocked By:")
		for _, item := range blockedBy {
			status := ""
			if item.Status != "" {
				status = fmt.Sprintf(" [%s]", item.Status)
			}
			ref := shortenTaskRef(item.Ref, currentProjectID)
			_, _ = fmt.Fprintf(w, "  - %s%s %s\n", ref, status, item.Title)
		}
	}

	if len(blocking) > 0 {
		_, _ = fmt.Fprintln(w, "\nBlocking:")
		for _, item := range blocking {
			status := ""
			if item.Status != "" {
				status = fmt.Sprintf(" [%s]", item.Status)
			}
			ref := shortenTaskRef(item.Ref, currentProjectID)
			_, _ = fmt.Fprintf(w, "  - %s%s %s\n", ref, status, item.Title)
		}
	}
}

// bareTaskIDRe matches bare task IDs like T-0013, GH-1, ABC-42.
var bareTaskIDRe = regexp.MustCompile(`^[A-Z]+-\d+$`)

// shortenTaskRef returns a display-friendly ref. Same-project refs are
// shortened to the bare task ID (e.g. "T-0013"); cross-project refs
// use the "project#T-NNNN" convention. Legacy task:// URIs are normalised.
// Non-task refs (e.g. HTTP URLs) are returned unchanged.
func shortenTaskRef(ref, currentProjectID string) string {
	// Strip known URI schemes.
	stripped := ref
	for _, prefix := range []string{"tlc://", "task://"} {
		if strings.HasPrefix(stripped, prefix) {
			stripped = strings.TrimPrefix(stripped, prefix)
			// Remove leading slash from tlc:///T-NNNN form.
			stripped = strings.TrimPrefix(stripped, "/")
			break
		}
	}

	// Non-task URI schemes — return unchanged.
	if strings.Contains(stripped, "://") {
		return ref
	}

	// Bare task ID — already short.
	if !strings.Contains(stripped, "/") && !strings.Contains(stripped, "#") {
		return stripped
	}

	// Split "org/repo/T-NNNN" or "org/repo#T-NNNN" into project + task.
	var projectID, taskID string
	if idx := strings.LastIndex(stripped, "#"); idx >= 0 {
		projectID = stripped[:idx]
		taskID = stripped[idx+1:]
	} else if idx := strings.LastIndex(stripped, "/"); idx >= 0 {
		projectID = stripped[:idx]
		taskID = stripped[idx+1:]
	}

	if taskID == "" {
		return ref
	}

	// Guard: only rewrite when tail looks like a task ID.
	if !bareTaskIDRe.MatchString(taskID) {
		return ref
	}

	// Same project → short form.
	if currentProjectID != "" && projectID == currentProjectID {
		return taskID
	}

	// Cross project → project#task convention.
	return projectID + "#" + taskID
}

func init() {
	TaskShowCmd.Flags().BoolVar(&taskShowLogs, "logs", false, "Include audit logs")
	TaskShowCmd.Flags().StringVar(&taskShowLogSortDirection, "log-sort-direction", "", "Log sort direction (asc, desc)")
	TaskShowCmd.Flags().StringVar(&taskShowOutput, "output", "", "Write output to a file instead of stdout")
	TaskShowCmd.Flags().BoolVar(&taskShowIncludeLogs, "include-logs", false, "Include audit log entries (vtodo: emit VJOURNAL components)")
}
