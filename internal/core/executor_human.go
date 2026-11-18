package core

import (
	"context"
	"fmt"
)

// applyHumanTimeout enforces a human task's on_timeout policy once its
// timeout has elapsed since creation. It runs lazily — at the next
// execute — so no process has to stay alive to watch the clock. Returns
// whether the policy fired.
func (e *Executor) applyHumanTimeout(ctx context.Context, task *Task, roles execRoles, report *ExecReport) (bool, error) {
	if task.Spec == nil || task.Spec.Human == nil || task.Spec.Human.Timeout == "" {
		return false, nil
	}
	timeout, err := ParseRecipeDuration(task.Spec.Human.Timeout)
	if err != nil {
		return false, nil //nolint:nilerr // validated at recipe load; an unparsable value never fires
	}
	elapsed := e.opts.Now().Sub(task.CreatedAt)
	if elapsed < timeout {
		return false, nil
	}
	switch HumanTimeoutAction(task.Spec.Human.OnTimeout) {
	case HumanTimeoutApprove:
		note := fmt.Sprintf("auto-approved: no decision after %s", timeout)
		entry, err := task.TransitionWithWorkflow(roles.completed, e.opts.Actor, note, e.wm, true)
		if err != nil {
			return false, fmt.Errorf("executor: auto-approve %s: %w", task.ID, err)
		}
		entry.Action = ActionApproved
		if err := e.repo.UpdateTaskWithLog(ctx, task, entry); err != nil {
			return false, fmt.Errorf("executor: auto-approve %s: %w", task.ID, err)
		}
		e.note(&report.Done, task.ID)
		e.printf("✓ %s auto-approved after %s\n", FormatTaskAlias(task), timeout)
		if err := e.maybeCompleteSubject(ctx, task); err != nil {
			e.printf("! subject of %s: %v\n", FormatTaskAlias(task), err)
		}
		return true, nil
	case HumanTimeoutReject:
		reason := fmt.Sprintf("rejected: no decision after %s timeout", timeout)
		task.BlockedReason = &reason
		task.UpdatedAt = e.opts.Now()
		if err := e.repo.UpdateTaskWithLog(ctx, task, e.logEntry(task, ActionRejected, reason)); err != nil {
			return false, fmt.Errorf("executor: auto-reject %s: %w", task.ID, err)
		}
		e.note(&report.Blocked, task.ID)
		e.printf("✗ %s %s\n", FormatTaskAlias(task), reason)
		return true, nil
	}
	return false, nil
}
