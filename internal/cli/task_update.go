package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

var TaskUpdateCmd = &cobra.Command{
	Use:   "update <task-id|pattern>...",
	Short: "Update task fields",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		resolved, needsConfirm, err := resolveTaskIDs(ctx, args, s, core.Query{})
		if err != nil {
			return err
		}
		if needsConfirm {
			if err := confirmBatch(cmd, resolved, args[0]); err != nil {
				return err
			}
		}

		if cmd.Flags().Changed("title") && len(resolved) > 1 {
			return fmt.Errorf("--title can only be set when updating a single task")
		}

		now := time.Now().UTC()
		var errs []string

		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			changed := false

			if cmd.Flags().Changed("title") {
				task.Title = taskUpdateTitle
				changed = true
			}
			if cmd.Flags().Changed("description") {
				task.Description = unescapeMarkdown(taskUpdateDescription)
				changed = true
			}
			if cmd.Flags().Changed("assigned-to") {
				if taskUpdateAssignedTo == "null" || taskUpdateAssignedTo == "-" {
					task.AssignedTo = nil
				} else {
					task.AssignedTo = &taskUpdateAssignedTo
				}
				changed = true
			}

			if cmd.Flags().Changed("status") {
				normalized, ok := NormalizeStatus(taskUpdateStatus)
				if !ok {
					errs = append(errs, fmt.Sprintf("%s: unknown status %q; valid values: TODO, IN_PROGRESS, DONE, SKIPPED", task.ID, taskUpdateStatus))
					continue
				}
				nextStatus := core.TaskStatus(normalized)
				wm := core.DefaultWorkflow()
				log, err := task.TransitionWithWorkflow(
					nextStatus, core.GetCurrentUser(), "Manual update", wm, taskUpdateForce,
				)
				if err != nil {
					errs = append(errs, fmt.Sprintf("%s: failed to transition: %v", task.ID, err))
					continue
				}
				if err := res.Storage.AddLog(ctx, log); err != nil {
					fmt.Printf("Warning: failed to write log for %s: %v\n", task.ID, err)
				}
				changed = true
			}

			if cmd.Flags().Changed("effort") {
				if !core.ValidEffort(core.Effort(taskUpdateEffort)) {
					return fmt.Errorf("invalid effort %q: must be one of XS, S, M, L, XL", taskUpdateEffort)
				}
				task.Effort = core.Effort(taskUpdateEffort)
				changed = true
			}

			if cmd.Flags().Changed("priority") {
				if !core.ValidPriority(core.Priority(taskUpdatePriority)) {
					return fmt.Errorf("invalid priority %q: must be one of P0, P1, P2, P3", taskUpdatePriority)
				}
				task.Priority = core.Priority(taskUpdatePriority)
				changed = true
			}

			if taskUpdateClearBlockedBy {
				task.SetBlockedBy(nil)
				changed = true
			}

			if len(taskUpdateAddBlockedBy) > 0 {
				validated, err := validateBlockedByRefs(ctx, s, res.Storage, taskUpdateAddBlockedBy)
				if err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
					continue
				}
				task.AddBlockedBy(validated)
				changed = true
			}

			if len(taskUpdateRemoveBlockedBy) > 0 {
				task.RemoveBlockedBy(taskUpdateRemoveBlockedBy)
				changed = true
			}

			if len(taskUpdateAddTags) > 0 || len(taskUpdateRemoveTags) > 0 {
				tagMap := make(map[string]bool)
				for _, t := range task.Tags {
					tagMap[t] = true
				}
				for _, t := range taskUpdateAddTags {
					tagMap[t] = true
				}
				for _, t := range taskUpdateRemoveTags {
					delete(tagMap, t)
				}
				newTags := []string{}
				for t := range tagMap {
					newTags = append(newTags, t)
				}
				task.Tags = newTags
				changed = true
			}

			if cmd.Flags().Changed("blocked") {
				task.BlockedReason = &taskUpdateBlocked
				changed = true
			}
			if cmd.Flags().Changed("unblock") && taskUpdateUnblock {
				task.BlockedReason = nil
				changed = true
			}
			if cmd.Flags().Changed("timeout") {
				d, err := time.ParseDuration(taskUpdateTimeout)
				if err != nil {
					return fmt.Errorf("invalid --timeout %q: %w", taskUpdateTimeout, err)
				}
				task.StaleTimeout = &d
				changed = true
			}

			if cmd.Flags().Changed("track") {
				if taskUpdateTrack == "-" || taskUpdateTrack == "" {
					task.TrackID = nil
				} else {
					resolved, trackErr := resolveTrackID(ctx, res.Storage, taskUpdateTrack)
					if trackErr != nil {
						errs = append(errs, fmt.Sprintf(
							"%s: %v", task.ID, trackErr,
						))
						continue
					}
					task.TrackID = &resolved
				}
				changed = true
			}

			if !changed {
				continue
			}

			task.StaleFiredAt = nil // reset stale crossing state on any change
			var assignedTo string
			if task.AssignedTo != nil {
				assignedTo = *task.AssignedTo
			}
			valCfg := getValidationConfig()
			if err := valCfg.ValidateTaskOp(config.ValidationOpUpdate, config.TaskFields{
				Title:       task.Title,
				Description: task.Description,
				Status:      string(task.Status),
				AssignedTo:  assignedTo,
				Effort:      string(task.Effort),
				Priority:    string(task.Priority),
				Tags:        task.Tags,
				Reference:   task.Reference,
			}); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
				continue
			}

			task.UpdatedAt = now

			if task.OriginSystem != nil && *task.OriginSystem != "" {
				if err := updateSyncedTask(ctx, task, res.Storage); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
					continue
				}
			} else {
				if err := res.Storage.UpdateTask(ctx, task); err != nil {
					errs = append(errs, fmt.Sprintf("%s: failed to update: %v", task.ID, err))
					continue
				}
				fmt.Printf("Updated task %s\n", task.ID)
			}
		}

		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}

		if len(resolved) > 0 {
			return syncTODOAll()
		}
		fmt.Println("No changes specified; use --title, --description, --status, --assigned-to, or other flags to update")
		return nil
	},
}

var TaskDeleteCmd = &cobra.Command{
	Use:   "delete <task-id|pattern>...",
	Short: "Delete a task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		resolved, needsConfirm, err := resolveTaskIDs(ctx, args, s, core.Query{})
		if err != nil {
			return err
		}

		// For delete: confirm if multiple targets unless --yes or --no-prompt.
		if (needsConfirm || len(resolved) > 1) && !taskDeleteYes && !taskNoPrompt {
			if err := confirmBatch(cmd, resolved, strings.Join(args, " ")); err != nil {
				return err
			}
		} else if len(resolved) == 1 && !taskDeleteYes && !taskNoPrompt {
			task := resolved[0].Task
			if !deletePromptInteractive(cmd) {
				return errDeleteRequiresYes(task.ID)
			}
			var confirm bool
			err := huh.NewConfirm().
				Title(fmt.Sprintf("Delete task %s (%s)?", task.ID, task.Title)).
				Description("This action cannot be undone.").
				Value(&confirm).
				Run()
			if err != nil {
				return fmt.Errorf("failed to run confirm dialog: %w", err)
			}
			if !confirm {
				return fmt.Errorf("delete aborted; task %s was not deleted", task.ID)
			}
		}

		var errs []string
		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			var assignedTo string
			if task.AssignedTo != nil {
				assignedTo = *task.AssignedTo
			}
			valCfg := getValidationConfig()
			if err := valCfg.ValidateTaskOp(config.ValidationOpDelete, config.TaskFields{
				Title:       task.Title,
				Description: task.Description,
				Status:      string(task.Status),
				AssignedTo:  assignedTo,
				Effort:      string(task.Effort),
				Priority:    string(task.Priority),
				Tags:        task.Tags,
				Reference:   task.Reference,
			}); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
				continue
			}

			if task.OriginSystem != nil && *task.OriginSystem != "" {
				if err := deleteSyncedTask(ctx, task, res.Storage); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
					continue
				}
			}

			if err := res.Storage.DeleteTask(ctx, task.ID); err != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to delete: %v", task.ID, err))
				continue
			}
			fmt.Printf("Deleted task %s\n", task.ID)
		}

		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		return syncTODOAll()
	},
}

func init() {
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateTitle, "title", "t", "", "New title")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateDescription, "description", "d", "", "New description")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateStatus, "status", "s", "", "New status")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateAssignedTo, "assigned-to", "a", "", "New assignee")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdateEffort, "effort", "e", "", "Effort estimate (XS, S, M, L, XL)")
	TaskUpdateCmd.Flags().StringVarP(&taskUpdatePriority, "priority", "p", "", "Priority (P0, P1, P2, P3)")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddBlockedBy, "add-blocked-by", []string{}, "Add blocking task IDs")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveBlockedBy, "remove-blocked-by", []string{}, "Remove blocking task IDs")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateClearBlockedBy, "clear-blocked-by", false, "Clear all blocking task IDs")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddTags, "add-tag", []string{}, "Add tags")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveTags, "remove-tag", []string{}, "Remove tags")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateForce, "force", false, "Force status transition (bypass workflow rules)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateBlocked, "blocked", "", "Set blocked reason")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateUnblock, "unblock", false, "Clear blocked reason")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateTimeout, "timeout", "", "Stale timeout (e.g. 2h, 30m)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateTrack, "track", "", "Link to track ID (use '-' to unlink)")

	TaskDeleteCmd.Flags().BoolVarP(&taskDeleteYes, "yes", "y", false, "Skip confirmation")
}

func deletePromptInteractive(cmd *cobra.Command) bool {
	stdin, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	stdout, ok := cmd.OutOrStdout().(*os.File)
	if !ok {
		return false
	}

	stdinInfo, err := stdin.Stat()
	if err != nil || (stdinInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	stdoutInfo, err := stdout.Stat()
	if err != nil || (stdoutInfo.Mode()&os.ModeCharDevice) == 0 {
		return false
	}

	return true
}
