package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/uri"
)

var TaskClaimCmd = &cobra.Command{
	Use:   "claim <task-id>",
	Short: "Claim a task for work",
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

		user := core.GetCurrentUser()
		prevStatus := task.Status
		task.AssignedTo = &user

		wm := core.DefaultWorkflow()
		activeStatus, wmErr := wm.StatusForRole("active")
		if wmErr != nil {
			return fmt.Errorf("workflow has no active status: %w", wmErr)
		}
		log, err := task.TransitionWithWorkflow(
			activeStatus, user, taskClaimNote, wm, false,
		)
		if err == nil {
			details := fmt.Sprintf("(%s → %s, assigned to @%s)", prevStatus, activeStatus, user)
			appendAuditLog(task, user, "CLAIMED", details, taskClaimNote, task.UpdatedAt)
		}
		if err != nil {
			return fmt.Errorf("failed to transition task: %w", err)
		}

		if err := saveTaskWithLog(ctx, cmd, task, log, res.Storage); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed task %s\n", task.ID)
		return syncTODOAll()
	},
}

var TaskUnclaimCmd = &cobra.Command{
	Use:   "unclaim <task-id>",
	Short: "Release a claimed task",
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

		user := core.GetCurrentUser()
		prevStatus := task.Status
		task.AssignedTo = nil

		wm := core.DefaultWorkflow()
		initialStatus, wmErr := wm.StatusForRole("initial")
		if wmErr != nil {
			return fmt.Errorf("workflow has no initial status: %w", wmErr)
		}
		log, err := task.TransitionWithWorkflow(
			initialStatus, user, taskUnclaimNote, wm, false,
		)
		if err == nil {
			details := fmt.Sprintf("(%s → %s, unassigned)", prevStatus, initialStatus)
			appendAuditLog(task, user, "UNCLAIMED", details, taskUnclaimNote, task.UpdatedAt)
		}
		if err != nil {
			return fmt.Errorf("failed to transition task: %w", err)
		}

		if err := saveTaskWithLog(ctx, cmd, task, log, res.Storage); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Unclaimed task %s\n", task.ID)
		return syncTODOAll()
	},
}

var TaskAssignCmd = &cobra.Command{
	Use:   "assign <task-id> <assignee>",
	Short: "Assign a task to someone",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		assignee := args[1]
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

		user := core.GetCurrentUser()
		prevAssignee := ""
		if task.AssignedTo != nil {
			prevAssignee = *task.AssignedTo
		}

		task.AssignedTo = &assignee
		task.UpdatedAt = time.Now().UTC()

		details := fmt.Sprintf("(assigned to @%s)", assignee)
		if prevAssignee != "" {
			details = fmt.Sprintf("(reassigned from @%s to @%s)", prevAssignee, assignee)
		}
		appendAuditLog(task, user, "ASSIGNED", details, taskAssignNote, task.UpdatedAt)

		logEntry := &core.LogEntry{
			TaskID:    task.ID,
			Timestamp: task.UpdatedAt,
			By:        user,
			Action:    core.ActionReassigned,
			Note:      taskAssignNote,
		}

		if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assigned task %s to %s\n", task.ID, assignee)
		return syncTODOAll()
	},
}

var TaskUnassignCmd = &cobra.Command{
	Use:   "unassign <task-id>",
	Short: "Remove assignee from a task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskUnassignNote == "" {
			return fmt.Errorf("--note is required when unassigning a task")
		}

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

		prevAssignee := ""
		if task.AssignedTo != nil {
			prevAssignee = *task.AssignedTo
		}

		user := core.GetCurrentUser()
		task.AssignedTo = nil
		task.UpdatedAt = time.Now().UTC()

		details := "(unassigned)"
		if prevAssignee != "" {
			details = fmt.Sprintf("(unassigned from @%s)", prevAssignee)
		}
		appendAuditLog(task, user, "UNASSIGNED", details, taskUnassignNote, task.UpdatedAt)

		logEntry := &core.LogEntry{
			TaskID:    task.ID,
			Timestamp: task.UpdatedAt,
			By:        user,
			Action:    core.ActionReassigned,
			Note:      taskUnassignNote,
		}

		if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Unassigned task %s\n", task.ID)
		return syncTODOAll()
	},
}

var TaskCompleteCmd = &cobra.Command{
	Use:   "complete <task-id>",
	Short: "Mark a task as done",
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

		user := core.GetCurrentUser()
		if task.AssignedTo == nil || *task.AssignedTo == "" {
			task.AssignedTo = &user
		}

		prevStatus := task.Status
		wm := core.DefaultWorkflow()
		completedStatus, wmErr := wm.StatusForRole("completed")
		if wmErr != nil {
			return fmt.Errorf("workflow has no completed status: %w", wmErr)
		}
		logEntry, err := task.TransitionWithWorkflow(
			completedStatus, user, taskCompleteNote, wm, taskCompleteNoVerify,
		)
		if err == nil {
			details := fmt.Sprintf("(%s → %s)", prevStatus, completedStatus)
			appendAuditLog(task, user, "COMPLETED", details, taskCompleteNote, task.UpdatedAt)
		}
		if err != nil {
			return fmt.Errorf("failed to transition task: %w", err)
		}

		if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Completed task %s\n", task.ID)
		return syncTODOAll()
	},
}

var TaskReopenCmd = &cobra.Command{
	Use:   "reopen <task-id>",
	Short: "Reopen a completed or skipped task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskReopenNote == "" {
			return fmt.Errorf("--note is required when reopening a task")
		}

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

		wm := core.DefaultWorkflow()
		if !wm.IsTerminal(task.Status) {
			return fmt.Errorf("task %s is not in a terminal state (status: %s)", task.ID, task.Status)
		}

		initialStatus, wmErr := wm.StatusForRole("initial")
		if wmErr != nil {
			return fmt.Errorf("workflow has no initial status: %w", wmErr)
		}

		user := core.GetCurrentUser()
		details := fmt.Sprintf("(%s → %s)", task.Status, initialStatus)
		appendAuditLog(task, user, "REOPENED", details, taskReopenNote, time.Now().UTC())
		logEntry, err := task.TransitionWithWorkflow(
			initialStatus, user, taskReopenNote, wm, true,
		)
		if err != nil {
			return fmt.Errorf("failed to reopen task: %w", err)
		}

		if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reopened task %s\n", task.ID)
		return syncTODOAll()
	},
}

func init() {
	TaskClaimCmd.Flags().StringVarP(&taskClaimNote, "note", "n", "", "Claim note")

	TaskUnclaimCmd.Flags().StringVarP(&taskUnclaimNote, "note", "n", "", "Unclaim note")

	TaskAssignCmd.Flags().StringVarP(&taskAssignNote, "note", "n", "", "Assignment note")

	TaskUnassignCmd.Flags().StringVarP(&taskUnassignNote, "note", "n", "", "Reason for unassigning (required)")

	TaskCompleteCmd.Flags().StringVarP(&taskCompleteNote, "note", "n", "", "Completion note")
	TaskCompleteCmd.Flags().BoolVar(&taskCompleteNoVerify, "no-verify", false, "Skip state machine validation")

	TaskReopenCmd.Flags().StringVarP(&taskReopenNote, "note", "n", "", "Reopen note")
}
