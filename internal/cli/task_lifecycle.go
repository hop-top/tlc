package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

// taskDisplayID renders the alias (T-NNNN) when seq is populated,
// falling back to task.ID. Centralizes the lifecycle command output
// so users see the human-friendly form, not the typeid PK.
func taskDisplayID(t *core.Task) string {
	if alias := core.FormatTaskAlias(t); alias != "" {
		return alias
	}
	return t.ID
}

var TaskClaimCmd = &cobra.Command{
	Use:   "claim <task-id|pattern>...",
	Short: "Claim a task for work",
	Long: `Claim one or more tasks for the current user.

Transitions each matched task to the workflow's active status, assigns
it to the caller, and auto-transitions a linked track from pending to
active when applicable. Re-running claim with the same arguments
converges (assignee + status already set), so the operation is idempotent.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(1),
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
				errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", formatTaskAlias(task), transErr))
				continue
			}

			if err := saveTaskWithLog(ctx, cmd, task, log, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}

			// The transition above has already been validated, so the
			// preview reports exactly the change a real run would make
			// — and stops short of the track auto-transition below,
			// which is a second write and not the one being previewed.
			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "claim",
					fmt.Sprintf("%s → %s, assign to @%s", prevStatus, activeStatus, user))
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

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claimed task %s\n", taskDisplayID(task))
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			// Nothing was written, so the projection already describes
			// the store. Rewriting it here would be a side effect of
			// the flag that suppresses side effects.
			return nil
		}
		return writeProjection()
	},
}

var TaskUnclaimCmd = &cobra.Command{
	Use:   "unclaim <task-id|pattern>...",
	Short: "Release a claimed task",
	Long: `Release one or more tasks the caller previously claimed.

Clears the assignee and transitions each task back to the workflow's
initial status. Local-only side effect; re-running on already-released
tasks converges.`,
	Annotations: map[string]string{
		"kit/side-effect": "destructive-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(1),
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
				errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", formatTaskAlias(task), transErr))
				continue
			}

			if err := saveTaskWithLog(ctx, cmd, task, log, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}
			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "unclaim",
					fmt.Sprintf("%s → %s, unassign", prevStatus, initialStatus))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Unclaimed task %s\n", taskDisplayID(task))
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			return nil
		}
		return writeProjection()
	},
}

var TaskAssignCmd = &cobra.Command{
	Use:   "assign <assignee> <task-id|pattern>...",
	Short: "Assign tasks to someone",
	Long: `Assign one or more tasks to the named assignee.

Records a reassignment audit log entry if the task already had an
assignee. Re-running with the same assignee converges to the same
state, so the operation is idempotent.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(2),
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
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}

			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "assign", assignDryRunDetail(prevAssignee, assignee))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Assigned task %s to %s\n", taskDisplayID(task), assignee)
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			return nil
		}
		return writeProjection()
	},
}

var TaskUnassignCmd = &cobra.Command{
	Use:   "unassign <task-id|pattern>...",
	Short: "Remove assignee from a task",
	Long: `Clear the assignee on one or more tasks.

Requires --note explaining the reason; the note is appended to the
task audit log. Re-running on an already-unassigned task converges.`,
	Annotations: map[string]string{
		"kit/side-effect": "destructive-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(1),
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
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}
			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "unassign", unassignDryRunDetail(prevAssignee))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Unassigned task %s\n", taskDisplayID(task))
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			return nil
		}
		return writeProjection()
	},
}

var TaskCompleteCmd = &cobra.Command{
	Use:   "complete <task-id|pattern>...",
	Short: "Mark a task as done",
	Long: `Transition one or more tasks to the workflow completed status.

Auto-assigns the caller when the task has no assignee. Use --no-verify
to skip state-machine validation. Re-running on an already-completed
task converges (no transition fires).`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(1),
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
				errs = append(errs, fmt.Sprintf("%s: failed to transition task: %v", formatTaskAlias(task), transErr))
				continue
			}

			if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}
			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "complete",
					fmt.Sprintf("%s → %s", prevStatus, completedStatus))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Completed task %s\n", formatTaskAlias(task))
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			return nil
		}
		return writeProjection()
	},
}

var TaskReopenCmd = &cobra.Command{
	Use:   "reopen <task-id|pattern>...",
	Short: "Reopen a completed or skipped task",
	Long: `Transition one or more terminal (DONE/SKIPPED) tasks back to the
workflow's initial status.

Requires --note explaining the reason. Fails on non-terminal tasks;
use 'tlc task update --status --force' to force-transition mid-flight
tasks instead.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MinimumNArgs(1),
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
				alias := formatTaskAlias(task)
				errs = append(errs, fmt.Sprintf(
					"%s: cannot be reopened: current status %s is not terminal "+
						"(terminal statuses: DONE, SKIPPED); use 'tlc task update %s --status <status> --force' "+
						"to force a status change instead",
					alias, task.Status, alias,
				))
				continue
			}

			initialStatus, wmErr := wm.StatusForRole("initial")
			if wmErr != nil {
				return fmt.Errorf("workflow has no initial status: %w", wmErr)
			}

			user := core.GetCurrentUser()
			prevStatus := task.Status
			details := fmt.Sprintf("(%s → %s)", prevStatus, initialStatus)
			appendAuditLog(task, user, "REOPENED", details, taskReopenNote, time.Now().UTC())
			logEntry, transErr := task.TransitionWithWorkflow(
				initialStatus, user, taskReopenNote, wm, true,
			)
			if transErr != nil {
				errs = append(errs, fmt.Sprintf("%s: failed to reopen task: %v", formatTaskAlias(task), transErr))
				continue
			}

			if err := saveTaskWithLog(ctx, cmd, task, logEntry, res.Storage); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", formatTaskAlias(task), err))
				continue
			}
			if dryRunSkipsWrite(cmd) {
				printTaskDryRun(cmd, task, "reopen",
					fmt.Sprintf("%s → %s", prevStatus, initialStatus))
				continue
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reopened task %s\n", taskDisplayID(task))
		}
		if len(errs) > 0 {
			return fmt.Errorf("some tasks failed:\n%s", strings.Join(errs, "\n"))
		}
		if dryRunSkipsWrite(cmd) {
			return nil
		}
		return writeProjection()
	},
}

// assignDryRunDetail and unassignDryRunDetail phrase the assignee change
// a preview would make. They mirror the wording of the audit-log details
// the real path records, so a preview and the trail it predicts read the
// same way — and they name the PREVIOUS assignee, which is the part a
// reader cannot see from the command line they just typed.
func assignDryRunDetail(prevAssignee, assignee string) string {
	if prevAssignee != "" {
		return fmt.Sprintf("reassign from @%s to @%s", prevAssignee, assignee)
	}
	return fmt.Sprintf("assign to @%s", assignee)
}

func unassignDryRunDetail(prevAssignee string) string {
	if prevAssignee != "" {
		return fmt.Sprintf("unassign from @%s", prevAssignee)
	}
	return "already unassigned"
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
