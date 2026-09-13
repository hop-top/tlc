// Package cli: `tlc task approve|reject` — the decisions on human-kind
// tasks that `track execute` waits on.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var (
	taskApproveBy    string
	taskApproveNote  string
	taskRejectBy     string
	taskRejectReason string
)

// TaskApproveCmd completes a human-kind task.
var TaskApproveCmd = &cobra.Command{
	Use:   "approve <task-id>",
	Short: "Approve a human task",
	Long: `Record a human decision in favor of a human-kind task and complete
it. The completion is forced past the state machine because a decision
is the task's whole work: it never passes through IN_PROGRESS. When the
task is the last open leaf of a recipe run, the run's subject completes
too.

Examples:
  tlc task approve T-0050
  tlc task approve T-0050 --by lead --note "release notes look good"`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		ctx := cmdContext(cmd)

		task, err := loadHumanTask(ctx, s, args[0])
		if err != nil {
			return err
		}
		wm := core.DefaultWorkflow()
		completed, err := wm.StatusForRole("completed")
		if err != nil {
			return fmt.Errorf("workflow has no completed status: %w", err)
		}
		by := approvalBy(taskApproveBy)
		note := "approved"
		if taskApproveNote != "" {
			note += ": " + taskApproveNote
		}
		entry, err := task.TransitionWithWorkflow(completed, by, note, wm, true)
		if err != nil {
			return fmt.Errorf("approve %s: %w", formatTaskAlias(task), err)
		}
		entry.Action = core.ActionApproved
		task.ClaimedAt = nil
		if err := s.UpdateTaskWithLog(ctx, task, entry); err != nil {
			return fmt.Errorf("approve %s: %w", formatTaskAlias(task), err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Approved %s (by %s)\n", formatTaskAlias(task), by)

		if err := core.CompleteSubjectIfDone(ctx, s, s, wm, by, task); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subject of %s: %v\n", formatTaskAlias(task), err)
		}
		return nil
	},
}

// TaskRejectCmd blocks a human-kind task with the decision as reason.
var TaskRejectCmd = &cobra.Command{
	Use:   "reject <task-id>",
	Short: "Reject a human task",
	Long: `Record a human decision against a human-kind task. The task stays in
its current status and is marked blocked with the reason, so 'tlc track
execute' leaves it and everything behind it alone until someone unblocks
or skips it.

Examples:
  tlc task reject T-0050 --reason "scope creep; split the change"`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if taskRejectReason == "" {
			return fmt.Errorf("--reason is required; re-run with: tlc task reject %s --reason \"<why>\"", args[0])
		}
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()
		ctx := cmdContext(cmd)

		task, err := loadHumanTask(ctx, s, args[0])
		if err != nil {
			return err
		}
		by := approvalBy(taskRejectBy)
		reason := "rejected: " + taskRejectReason
		task.BlockedReason = &reason
		entry := &core.LogEntry{TaskID: task.ID, Timestamp: task.UpdatedAt, By: by, Action: core.ActionRejected, Note: reason}
		if err := s.UpdateTaskWithLog(ctx, task, entry); err != nil {
			return fmt.Errorf("reject %s: %w", formatTaskAlias(task), err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Rejected %s (by %s): %s\n", formatTaskAlias(task), by, taskRejectReason)
		return nil
	},
}

// loadHumanTask resolves the reference and refuses non-human tasks: a
// decision is not how agent or exec work gets done.
func loadHumanTask(ctx context.Context, s *storage.SQLiteStorage, ref string) (*core.Task, error) {
	taskID, err := parseTaskRefForCLI(ctx, s, ref)
	if err != nil {
		return nil, err
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load task %s: %w", ref, err)
	}
	if task == nil {
		return nil, fmt.Errorf("task %s not found; run 'tlc task list' to see available tasks", ref)
	}
	if kind := task.EffectiveKind(); kind != core.TaskKindHuman {
		return nil, fmt.Errorf("task %s is a %s task, not a human decision; use 'tlc task complete %s' or 'tlc task block %s' instead",
			formatTaskAlias(task), kind, formatTaskAlias(task), formatTaskAlias(task))
	}
	return task, nil
}

func init() {
	TaskApproveCmd.Flags().StringVar(&taskApproveBy, "by", "", "Who decided (default: current user)")
	TaskApproveCmd.Flags().StringVarP(&taskApproveNote, "note", "n", "", "Decision note")
	TaskRejectCmd.Flags().StringVar(&taskRejectBy, "by", "", "Who decided (default: current user)")
	TaskRejectCmd.Flags().StringVar(&taskRejectReason, "reason", "", "Why the task is rejected (required)")
	TaskCmd.AddCommand(TaskApproveCmd, TaskRejectCmd)
}
