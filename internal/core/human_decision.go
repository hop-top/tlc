package core

import (
	"context"
	"fmt"

	"hop.top/tlc/internal/config"
)

// Decisions a human task can take. They are what the task's result
// records under "decision", so a downstream `when:` reads them as
// results.<step>.decision.
const (
	HumanApproved = "approved"
	HumanRejected = "rejected"
)

// HumanDecisionInput is one decision on a human-kind task. Note belongs to
// an approval, Reason to a rejection.
type HumanDecisionInput struct {
	Decision string
	By       string
	Note     string
	Reason   string
}

// DecideHumanTask applies a human decision to a human-kind task and
// records it in the task's result.
//
// Approving completes the task, forced past the state machine because a
// decision is the task's whole work: it never passes through the active
// status. Rejecting leaves the task in its current status and marks it
// blocked with the reason, so the executor leaves it and everything
// behind it alone until someone unblocks or skips it.
//
// Both `tlc task approve|reject` and the recipe test harness decide
// through this function.
func DecideHumanTask(ctx context.Context, repo Repository, wm *WorkflowManager, task *Task, in HumanDecisionInput) error {
	if task.EffectiveKind() != TaskKindHuman {
		return fmt.Errorf("decide %s: not a human task", task.ID)
	}
	switch in.Decision {
	case HumanApproved:
		return approveHumanTask(ctx, repo, wm, task, in)
	case HumanRejected:
		return rejectHumanTask(ctx, repo, task, in)
	}
	return fmt.Errorf("decide %s: unknown decision %q (want %s or %s)", task.ID, in.Decision, HumanApproved, HumanRejected)
}

func approveHumanTask(ctx context.Context, repo Repository, wm *WorkflowManager, task *Task, in HumanDecisionInput) error {
	completed, err := wm.StatusForRole(config.RoleCompleted)
	if err != nil {
		return fmt.Errorf("workflow has no completed status: %w", err)
	}
	note := "approved"
	if in.Note != "" {
		note += ": " + in.Note
	}
	entry, err := task.TransitionWithWorkflow(completed, in.By, note, wm, true)
	if err != nil {
		return err
	}
	entry.Action = ActionApproved
	task.ClaimedAt = nil
	task.Result = map[string]any{"decision": HumanApproved, "by": in.By, "note": in.Note}
	return repo.UpdateTaskWithLog(ctx, task, entry) //nolint:wrapcheck // callers name the task and verb
}

func rejectHumanTask(ctx context.Context, repo Repository, task *Task, in HumanDecisionInput) error {
	reason := "rejected: " + in.Reason
	task.BlockedReason = &reason
	task.Result = map[string]any{"decision": HumanRejected, "by": in.By, "reason": in.Reason}
	entry := &LogEntry{TaskID: task.ID, Timestamp: task.UpdatedAt, By: in.By, Action: ActionRejected, Note: reason}
	return repo.UpdateTaskWithLog(ctx, task, entry) //nolint:wrapcheck // callers name the task and verb
}
