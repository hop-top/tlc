package cli

import (
	"context"
	"fmt"
	"io"
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

var TaskShowCmd = &cobra.Command{
	Use:   "show <task-id>...",
	Short: "Show task details",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()

		var errs []string
		for i, id := range args {
			res, resolveErr := uri.NewResolver(s).ResolveTask(ctx, id)
			if resolveErr != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", id, resolveErr))
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
					errs = append(errs, fmt.Sprintf("%s: failed to get logs: %v", id, err))
					continue
				}
			}

			if i > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}

			format := viper.GetString("output.format")
			if format == formatJSON || format == formatYAML {
				printTask(cmd, task, logs, format)
				continue
			}

			blockedBy, blocking := collectTaskRelations(ctx, s, res.Storage, task)
			renderTaskDetail(cmd.OutOrStdout(), task, logs)
			renderTaskRelations(cmd.OutOrStdout(), blockedBy, blocking)
		}

		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
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
		if strings.Contains(ref, "/") || strings.Contains(ref, "://") {
			resolverStorage = registryStorage
		}

		resolved, err := uri.NewResolver(resolverStorage).ResolveTask(ctx, ref)
		if err != nil {
			summaries = append(summaries, relatedTaskSummary{Ref: ref, Title: "(missing)"})
			continue
		}

		summaries = append(summaries, relatedTaskSummary{
			Ref:    ref,
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
			if !matchesTaskReference(candidate.BlockedBy(), candidates) {
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
	if task.ProjectID != nil && *task.ProjectID != "" {
		return *task.ProjectID + "/" + task.ID
	}
	return task.ID
}

func sameProject(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func renderTaskRelations(w io.Writer, blockedBy, blocking []relatedTaskSummary) {
	if len(blockedBy) > 0 {
		_, _ = fmt.Fprintln(w, "\nBlocked By:")
		for _, item := range blockedBy {
			status := ""
			if item.Status != "" {
				status = fmt.Sprintf(" [%s]", item.Status)
			}
			_, _ = fmt.Fprintf(w, "  - %s%s %s\n", item.Ref, status, item.Title)
		}
	}

	if len(blocking) > 0 {
		_, _ = fmt.Fprintln(w, "\nBlocking:")
		for _, item := range blocking {
			status := ""
			if item.Status != "" {
				status = fmt.Sprintf(" [%s]", item.Status)
			}
			_, _ = fmt.Fprintf(w, "  - %s%s %s\n", item.Ref, status, item.Title)
		}
	}
}

func init() {
	TaskShowCmd.Flags().BoolVar(&taskShowLogs, "logs", false, "Include audit logs")
	TaskShowCmd.Flags().StringVar(&taskShowLogSortDirection, "log-sort-direction", "", "Log sort direction (asc, desc)")
}
