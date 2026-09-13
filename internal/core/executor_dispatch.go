package core

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// dispatchAll runs the ready set under the concurrency bound. With a
// bound of 1 the loop is strictly sequential in the given order.
func (e *Executor) dispatchAll(ctx context.Context, ready, stale []*Task, roles execRoles, report *ExecReport) {
	type job struct {
		task  *Task
		stale bool
	}
	jobs := make([]job, 0, len(ready)+len(stale))
	for _, t := range stale {
		jobs = append(jobs, job{task: t, stale: true})
	}
	for _, t := range ready {
		jobs = append(jobs, job{task: t})
	}

	if e.opts.Concurrency == 1 {
		for _, j := range jobs {
			if ctx.Err() != nil {
				return
			}
			e.runOne(ctx, j.task, j.stale, roles, report)
		}
		return
	}

	sem := make(chan struct{}, e.opts.Concurrency)
	var wg sync.WaitGroup
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			e.runOne(ctx, j.task, j.stale, roles, report)
		}(j)
	}
	wg.Wait()
}

// runOne takes the task (claim or reclaim), dispatches it by kind and
// applies the outcome.
func (e *Executor) runOne(ctx context.Context, t *Task, stale bool, roles execRoles, report *ExecReport) {
	task, ok := e.take(ctx, t, stale, roles, report)
	if !ok {
		return
	}

	dispatcher, found := e.dispatch[task.EffectiveKind()]
	if !found {
		if err := e.block(ctx, task, fmt.Sprintf("no dispatcher registered for kind %s", task.EffectiveKind()), report); err != nil {
			e.printf("! %s: %v\n", FormatTaskAlias(task), err)
		}
		return
	}

	e.printf("→ %s %s (%s)\n", FormatTaskAlias(task), task.Title, task.EffectiveKind())
	res, err := dispatcher.Dispatch(ctx, task)

	// Reload: an agent may have edited the task while it ran.
	task = e.reload(ctx, task)
	if res != nil && res.Result != nil {
		task.Result = res.Result
	}
	summary, failed := dispatchOutcome(res, err)
	if !failed && task.Spec != nil && task.Spec.Gate != nil {
		if gateErr := e.gate(ctx, task.Spec.Gate, task.Result, e.opts.EvaKey); gateErr != nil {
			failed = true
			summary = fmt.Sprintf("gate %s: %v", task.Spec.Gate.Contract, gateErr)
		}
	}
	if failed {
		e.fail(ctx, task, roles, summary, report)
		return
	}
	e.complete(ctx, task, roles, summary, report)
}

// take claims a ready task through the compare-and-set, or takes over a
// stale claim in place. Returns the reloaded task and whether we own it.
func (e *Executor) take(ctx context.Context, t *Task, stale bool, roles execRoles, report *ExecReport) (*Task, bool) {
	now := e.opts.Now()
	actor := e.opts.Actor
	if stale {
		prevOwner, prevClaim := "", now
		if t.AssignedTo != nil {
			prevOwner = *t.AssignedTo
		}
		if t.ClaimedAt != nil {
			prevClaim = *t.ClaimedAt
		}
		t.AssignedTo = &actor
		t.ClaimedAt = &now
		t.UpdatedAt = now
		entry := e.logEntry(t, ActionReclaimed, fmt.Sprintf("reclaimed from %s after %s", prevOwner, now.Sub(prevClaim).Round(time.Second)))
		if err := e.repo.UpdateTaskWithLog(ctx, t, entry); err != nil {
			e.printf("! %s reclaim failed: %v\n", FormatTaskAlias(t), err)
			return nil, false
		}
		e.note(&report.Reclaimed, t.ID)
		return t, true
	}

	won, err := e.claims.ClaimTask(ctx, t.ID, projectOf(t), roles.initial, roles.active, actor, now)
	if err != nil {
		e.printf("! %s claim failed: %v\n", FormatTaskAlias(t), err)
		return nil, false
	}
	if !won {
		return nil, false // another actor took it since the listing
	}
	task := e.reload(ctx, t)
	entry := e.logEntry(task, ActionClaimed, fmt.Sprintf("(%s → %s, assigned to @%s)", roles.initial, roles.active, actor))
	if err := e.repo.UpdateTaskWithLog(ctx, task, entry); err != nil {
		e.printf("! %s claim log failed: %v\n", FormatTaskAlias(task), err)
	}
	return task, true
}

// dispatchOutcome folds a dispatcher's result and error into a summary
// and a pass/fail verdict.
func dispatchOutcome(res *DispatchResult, err error) (string, bool) {
	if err != nil {
		return err.Error(), true
	}
	if res == nil {
		return "dispatcher returned no result", true
	}
	if res.Status != AgentStatusSucceeded {
		summary := res.Summary
		if summary == "" {
			summary = string(res.Status)
		}
		return summary, true
	}
	return res.Summary, false
}

// complete moves the task to the completed role and, if it belongs to a
// subject, checks whether the subject can complete too.
func (e *Executor) complete(ctx context.Context, task *Task, roles execRoles, summary string, report *ExecReport) {
	task.ClaimedAt = nil
	entry, err := e.transition(task, roles.completed, summary)
	if err != nil {
		e.printf("! %s complete failed: %v\n", FormatTaskAlias(task), err)
		return
	}
	entry.Action = ActionDone
	if err := e.repo.UpdateTaskWithLog(ctx, task, entry); err != nil {
		e.printf("! %s complete failed: %v\n", FormatTaskAlias(task), err)
		return
	}
	e.note(&report.Done, task.ID)
	e.printf("✓ %s done\n", FormatTaskAlias(task))
	if err := e.maybeCompleteSubject(ctx, task); err != nil {
		e.printf("! subject of %s: %v\n", FormatTaskAlias(task), err)
	}
}

// fail records the attempt, then either parks the task as blocked (attempts
// exhausted) or returns it to the initial role for another round, sleeping
// the retry backoff first.
func (e *Executor) fail(ctx context.Context, task *Task, roles execRoles, summary string, report *ExecReport) {
	task.Attempts++
	task.ClaimedAt = nil
	maxAttempts := 1
	var backoff time.Duration
	if task.Spec != nil && task.Spec.Retry != nil {
		if task.Spec.Retry.MaxAttempts > 0 {
			maxAttempts = task.Spec.Retry.MaxAttempts
		}
		if d, err := ParseRecipeDuration(task.Spec.Retry.Backoff); err == nil {
			backoff = d
		}
	}
	note := fmt.Sprintf("%s failed ×%d: %s", task.EffectiveKind(), task.Attempts, summary)

	entry, err := e.transition(task, roles.initial, note)
	if err != nil {
		e.printf("! %s release failed: %v\n", FormatTaskAlias(task), err)
		return
	}
	if task.Attempts >= maxAttempts {
		reason := note
		task.BlockedReason = &reason
		entry.Action = ActionBlocked
		if err := e.repo.UpdateTaskWithLog(ctx, task, entry); err != nil {
			e.printf("! %s block failed: %v\n", FormatTaskAlias(task), err)
			return
		}
		e.note(&report.Blocked, task.ID)
		e.printf("✗ %s blocked: %s\n", FormatTaskAlias(task), note)
		return
	}
	entry.Action = ActionRetry
	if err := e.repo.UpdateTaskWithLog(ctx, task, entry); err != nil {
		e.printf("! %s retry failed: %v\n", FormatTaskAlias(task), err)
		return
	}
	e.note(&report.Failed, task.ID)
	e.printf("↻ %s retry %d/%d: %s\n", FormatTaskAlias(task), task.Attempts, maxAttempts, summary)
	if backoff > 0 {
		if err := e.opts.Sleep(ctx, backoff); err != nil {
			e.printf("↻ %s backoff interrupted: %v\n", FormatTaskAlias(task), err)
		}
	}
}

// block parks a task with a reason without changing its status.
func (e *Executor) block(ctx context.Context, task *Task, reason string, report *ExecReport) error {
	task.BlockedReason = &reason
	task.UpdatedAt = e.opts.Now()
	if err := e.repo.UpdateTaskWithLog(ctx, task, e.logEntry(task, ActionBlocked, reason)); err != nil {
		return fmt.Errorf("executor: block %s: %w", task.ID, err)
	}
	e.note(&report.Blocked, task.ID)
	e.printf("✗ %s blocked: %s\n", FormatTaskAlias(task), reason)
	return nil
}

// skip moves a task to the skipped role; a vocabulary without one blocks
// the task instead, naming the missing role.
func (e *Executor) skip(ctx context.Context, task *Task, roles execRoles, note string, report *ExecReport) error {
	if roles.skipped == "" {
		return e.block(ctx, task, note+" (no status declares role skipped; declare one in task.statuses)", report)
	}
	entry, err := e.transition(task, roles.skipped, note)
	if err != nil {
		return fmt.Errorf("executor: skip %s: %w", task.ID, err)
	}
	entry.Action = ActionSkipped
	if err := e.repo.UpdateTaskWithLog(ctx, task, entry); err != nil {
		return fmt.Errorf("executor: skip %s: %w", task.ID, err)
	}
	e.note(&report.Skipped, task.ID)
	e.printf("↷ %s skipped: %s\n", FormatTaskAlias(task), note)
	return nil
}

// transition applies a role transition, falling back to force when the
// task's vocabulary does not allow it (the audit note still records why).
func (e *Executor) transition(task *Task, next TaskStatus, note string) (*LogEntry, error) {
	entry, err := task.TransitionWithWorkflow(next, e.opts.Actor, note, e.wm, false)
	if err != nil {
		entry, err = task.TransitionWithWorkflow(next, e.opts.Actor, note+" (forced: "+err.Error()+")", e.wm, true)
	}
	return entry, err
}

func (e *Executor) logEntry(task *Task, action, note string) *LogEntry {
	return &LogEntry{TaskID: task.ID, Timestamp: e.opts.Now(), By: e.opts.Actor, Action: action, Note: note}
}

// reload fetches the current row; on any failure the in-memory task is
// kept so the outcome is still applied.
func (e *Executor) reload(ctx context.Context, task *Task) *Task {
	if fresh, err := e.repo.GetTask(ctx, task.ID); err == nil && fresh != nil {
		return fresh
	}
	return task
}

func projectOf(t *Task) string {
	if t.ProjectID != nil {
		return *t.ProjectID
	}
	return ""
}
