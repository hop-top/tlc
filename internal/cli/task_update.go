package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/uri"
)

var TaskUpdateCmd = &cobra.Command{
	Use:   "update <task-id>",
	Short: "Update task fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		res, err := uri.NewResolver(s).ResolveTask(ctx, id)
		if err != nil {
			return err
		}
		task := res.Task
		if res.Storage != s {
			defer func() { _ = res.Storage.Close() }()
		}

		changed := false
		now := time.Now().UTC()

		if cmd.Flags().Changed("title") {
			task.Title = taskUpdateTitle
			changed = true
		}
		if cmd.Flags().Changed("description") {
			task.Description = taskUpdateDescription
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
			nextStatus := core.TaskStatus(taskUpdateStatus)
			wm := core.DefaultWorkflow()
			log, err := task.TransitionWithWorkflow(
				nextStatus, core.GetCurrentUser(), "Manual update", wm, taskUpdateForce,
			)
			if err != nil {
				return fmt.Errorf("failed to transition task: %w", err)
			}
			if err := res.Storage.AddLog(ctx, log); err != nil {
				fmt.Printf("Warning: failed to write log: %v\n", err)
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
				return err
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

		if changed {
			// Config-driven validation for update.
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
				return err
			}

			task.UpdatedAt = now

			if task.OriginSystem != nil && *task.OriginSystem != "" {
				if err := updateSyncedTask(ctx, task, res.Storage); err != nil {
					return err
				}
			} else {
				if err := res.Storage.UpdateTask(ctx, task); err != nil {
					return fmt.Errorf("failed to update task: %w", err)
				}
				fmt.Printf("Updated task %s\n", task.ID)
			}

			return syncTODOAll()
		}
		fmt.Println("No changes specified")
		return nil
	},
}

var TaskDeleteCmd = &cobra.Command{
	Use:   "delete <task-id>",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		res, err := uri.NewResolver(s).ResolveTask(ctx, id)
		if err != nil {
			return err
		}
		task := res.Task
		if res.Storage != s {
			defer func() { _ = res.Storage.Close() }()
		}

		if !taskDeleteYes {
			if !deletePromptInteractive(cmd) {
				return fmt.Errorf("task delete requires --yes in non-interactive mode")
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
				return fmt.Errorf("delete aborted")
			}
		}

		// Config-driven validation for delete.
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
			return err
		}

		if task.OriginSystem != nil && *task.OriginSystem != "" {
			if err := deleteSyncedTask(ctx, task, res.Storage); err != nil {
				return err
			}
		}

		if err := res.Storage.DeleteTask(ctx, task.ID); err != nil {
			return fmt.Errorf("failed to delete task: %w", err)
		}

		fmt.Printf("Deleted task %s\n", task.ID)
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
