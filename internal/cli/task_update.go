package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
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
	RunE: func(_ *cobra.Command, args []string) error {
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
				fmt.Println("Aborted")
				return nil
			}
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
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAddTags, "add-tag", []string{}, "Add tags")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateRemoveTags, "remove-tag", []string{}, "Remove tags")
	TaskUpdateCmd.Flags().BoolVar(&taskUpdateForce, "force", false, "Force status transition (bypass workflow rules)")

	TaskDeleteCmd.Flags().BoolVarP(&taskDeleteYes, "yes", "y", false, "Skip confirmation")
}
