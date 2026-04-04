package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var TaskClaimCmd = &cobra.Command{
	Use:   "claim <task-id|pattern>...",
	Short: "Claim a task for work",
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

		var errs []string
		for _, res := range resolved {
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
			log, transErr := task.TransitionWithWorkflow(
				activeStatus, user, taskClaimNote, wm, false,
			)
			if transErr == nil {
				details := fmt.Sprintf("(%s → %s, assigned to @%s)", prevStatus, activeStatus, user)
				appendAuditLog(task, user, "CLAIMED", details, taskClaimNote, task.UpdatedAt)
			}
			if transErr != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", task.ID, transErr))
				continue
			}

			if err := saveTaskWithLog(ctx, cmd, task, log, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
				continue
			}

			// Auto-transition track pending → active on claim.
			if task.TrackID != nil && *task.TrackID != "" {
				svc := core.NewTrackService(res.Storage, res.Storage)
				if transErr := svc.AutoTransitionOnTaskClaim(ctx, *task.TrackID); transErr != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStderr(),
						"Warning: track auto-transition failed: %v\n", transErr)
				}
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed task %s\n", task.ID)
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		return syncTODOAll()
	},
}

var TaskUnclaimCmd = &cobra.Command{
	Use:   "unclaim <task-id|pattern>...",
	Short: "Release a claimed task",
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

		var errs []string
		for _, res := range resolved {
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
			log, transErr := task.TransitionWithWorkflow(
				initialStatus, user, taskUnclaimNote, wm, false,
			)
			if transErr == nil {
				details := fmt.Sprintf("(%s → %s, unassigned)", prevStatus, initialStatus)
				appendAuditLog(task, user, "UNCLAIMED", details, taskUnclaimNote, task.UpdatedAt)
			}
			if transErr != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", task.ID, transErr))
				continue
			}

			if err := saveTaskWithLog(ctx, cmd, task, log, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Unclaimed task %s\n", task.ID)
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		return syncTODOAll()
	},
}

var TaskAssignCmd = &cobra.Command{
	Use:   "assign <assignee> <task-id|pattern>...",
	Short: "Assign tasks to someone",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		assignee := args[0]
		taskArgs := args[1:]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		ctx := context.Background()

		resolved, needsConfirm, err := resolveTaskIDs(ctx, taskArgs, s, core.Query{})
		if err != nil {
			return err
		}
		if needsConfirm {
			if err := confirmBatch(cmd, resolved, taskArgs[0]); err != nil {
				return err
			}
		}

		user := core.GetCurrentUser()
		var errs []string
		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

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
				errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
				continue
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assigned task %s to %s\n", task.ID, assignee)
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		return syncTODOAll()
	},
}

var TaskUnassignCmd = &cobra.Command{
	Use:   "unassign <task-id|pattern>...",
	Short: "Remove assignee from a task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskUnassignNote == "" {
			return errNoteRequired("tlc task unassign <task-id|pattern>...")
		}

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

		var errs []string
		for _, res := range resolved {
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
				errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Unassigned task %s\n", task.ID)
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		return syncTODOAll()
	},
}

var TaskCompleteCmd = &cobra.Command{
	Use:   "complete <task-id|pattern>...",
	Short: "Mark a task as done",
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

		var errs []string
		for _, res := range resolved {
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
			logEntry, transErr := task.TransitionWithWorkflow(
				completedStatus, user, taskCompleteNote, wm, taskCompleteNoVerify,
			)
			if transErr == nil {
				details := fmt.Sprintf("(%s → %s)", prevStatus, completedStatus)
				appendAuditLog(task, user, "COMPLETED", details, taskCompleteNote, task.UpdatedAt)
			}
			if transErr != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", task.ID, transErr))
				continue
			}

			if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Completed task %s\n", task.ID)
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		return syncTODOAll()
	},
}

var TaskReopenCmd = &cobra.Command{
	Use:   "reopen <task-id|pattern>...",
	Short: "Reopen a completed or skipped task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskReopenNote == "" {
			return errNoteRequired("tlc task reopen <task-id|pattern>...")
		}

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

		var errs []string
		for _, res := range resolved {
			task := res.Task
			if res.Storage != s {
				defer func() { _ = res.Storage.Close() }()
			}

			wm := core.DefaultWorkflow()
			if !wm.IsTerminal(task.Status) {
				errs = append(errs, fmt.Sprintf(
					"%s: cannot be reopened: current status %s is not terminal "+
						"(terminal statuses: DONE, SKIPPED); use 'tlc task update %s --status <status> --force' "+
						"to force a status change instead",
					task.ID, task.Status, task.ID,
				))
				continue
			}

			initialStatus, wmErr := wm.StatusForRole("initial")
			if wmErr != nil {
				return fmt.Errorf("workflow has no initial status: %w", wmErr)
			}

			user := core.GetCurrentUser()
			details := fmt.Sprintf("(%s → %s)", task.Status, initialStatus)
			appendAuditLog(task, user, "REOPENED", details, taskReopenNote, time.Now().UTC())
			logEntry, transErr := task.TransitionWithWorkflow(
				initialStatus, user, taskReopenNote, wm, true,
			)
			if transErr != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to reopen task: %v", task.ID, transErr))
				continue
			}

			if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", task.ID, err))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reopened task %s\n", task.ID)
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
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
