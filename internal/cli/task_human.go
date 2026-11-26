// Package cli: `tlc task approve|reject` — the decisions on human-kind
// tasks that `track execute` waits on.
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var (
	taskApproveBy    string
	taskApproveNote  string
	taskRejectBy     string
	taskRejectReason string

	taskApproveNoVerify bool
	taskRejectNoVerify  bool
)

// TaskApproveCmd completes a human-kind task.
var TaskApproveCmd = &cobra.Command{
	Use:   "approve <task-id>",
	Short: "Approve a task in review (or a human task)",
	Long: `Approve a task. On a task in the review status, move it to the
completed status. On a human-kind task (recipe decision), record the
decision and complete it as described below.

Human-kind tasks: record a human decision in favor of the task and complete
it. The completion is forced past the state machine because a decision
is the task's whole work: it never passes through IN_PROGRESS. When the
task is the last open leaf of a recipe run, the run's subject completes
too.
The decision is recorded in the task's result as decision, by and note, so
a later step's when: reads results.<step>.decision == 'approved'.

Examples:
  tlc task approve T-0050
  tlc task approve T-0050 --dry-run
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

		task, err := loadAnyTask(ctx, s, args[0])
		if err != nil {
			return err
		}
		if task.EffectiveKind() != core.TaskKindHuman {
			return runMove(cmd, args, s, move{
				verb: "approve", past: "Approved", audit: "APPROVED",
				logAction: core.ActionApproved,
				toRole:    config.RoleCompleted, fromRole: roleReview,
				note: taskApproveNote, by: taskApproveBy, force: taskApproveNoVerify,
			})
		}
		wm := core.DefaultWorkflow()
		by := approvalBy(taskApproveBy)
		if err := core.DecideHumanTask(ctx, s, wm, task, core.HumanDecisionInput{
			Decision: core.HumanApproved, By: by, Note: taskApproveNote,
		}); err != nil {
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
	Short: "Reject a task in review (or a human task)",
	Long: `Reject a task. On a task in the review status, send it back to the
active status; --reason is required. On a human-kind task (recipe
decision), behavior is as described below.

Human-kind tasks: record a human decision against the task. The task stays in
its current status and is marked blocked with the reason, so 'tlc track
execute' leaves it and everything behind it alone until someone unblocks
or skips it. The decision is recorded in the task's result as decision, by
and reason.

Examples:
  tlc task reject T-0050 --reason "tests missing" --dry-run
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

		task, err := loadAnyTask(ctx, s, args[0])
		if err != nil {
			return err
		}
		if task.EffectiveKind() != core.TaskKindHuman {
			return runMove(cmd, args, s, move{
				verb: "reject", past: "Rejected", audit: "REJECTED",
				logAction: core.ActionRejected,
				toRole:    config.RoleActive, fromRole: roleReview,
				reason: taskRejectReason, by: taskRejectBy, force: taskRejectNoVerify,
			})
		}
		by := approvalBy(taskRejectBy)
		if err := core.DecideHumanTask(ctx, s, core.DefaultWorkflow(), task, core.HumanDecisionInput{
			Decision: core.HumanRejected, By: by, Reason: taskRejectReason,
		}); err != nil {
			return fmt.Errorf("reject %s: %w", formatTaskAlias(task), err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Rejected %s (by %s): %s\n", formatTaskAlias(task), by, taskRejectReason)
		return nil
	},
}

// loadAnyTask resolves a task reference to its task, whatever its kind.
func loadAnyTask(ctx context.Context, s *storage.SQLiteStorage, ref string) (*core.Task, error) {
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
	return task, nil
}

func init() {
	TaskApproveCmd.Flags().StringVar(&taskApproveBy, "by", "", "Who decided (default: current user)")
	TaskApproveCmd.Flags().StringVarP(&taskApproveNote, "note", "n", "", "Decision note")
	TaskApproveCmd.Flags().BoolVar(&taskApproveNoVerify, "no-verify", false, "Skip state machine validation (non-human tasks)")
	TaskRejectCmd.Flags().BoolVar(&taskRejectNoVerify, "no-verify", false, "Skip state machine validation (non-human tasks)")
	TaskRejectCmd.Flags().StringVar(&taskRejectBy, "by", "", "Who decided (default: current user)")
	TaskRejectCmd.Flags().StringVar(&taskRejectReason, "reason", "", "Why the task is rejected (required)")
	TaskCmd.AddCommand(TaskApproveCmd, TaskRejectCmd)
}

// approvalBy resolves the approver identity: the explicit flag when set,
// otherwise the current user.
func approvalBy(flag string) string {
	if flag != "" {
		return flag
	}
	return core.GetCurrentUser()
}

// cmdContext returns the cobra command's context, falling back to
// context.Background when nil (tests bypass cobra's full lifecycle).
func cmdContext(cmd *cobra.Command) context.Context {
	if c := cmd.Context(); c != nil {
		return c
	}
	return context.Background()
}
