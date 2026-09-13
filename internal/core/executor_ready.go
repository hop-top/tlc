package core

import (
	"context"
	"fmt"
	"sort"
)

// taskClass is what one readiness pass decided for a task.
type taskClass int

const (
	classIgnore     taskClass = iota // not eligible or not ready
	classProgressed                  // state changed (skipped, blocked, timed out)
	classWaiting                     // ready human task awaiting a decision
	classDispatch                    // ready to run
)

// round performs one readiness pass: human timeouts, `when` decisions,
// claims and dispatches. It reports whether anything changed (so the
// caller recomputes) and which human tasks are ready and waiting.
func (e *Executor) round(ctx context.Context, tasks []*Task, roles execRoles, report *ExecReport) (bool, []*Task, error) {
	byID := make(map[string]*Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}

	progressed := false
	var waiting, dispatchable []*Task
	for _, t := range tasks {
		class, err := e.classify(ctx, t, byID, roles, report)
		if err != nil {
			return false, nil, err
		}
		switch class {
		case classProgressed:
			progressed = true
		case classWaiting:
			waiting = append(waiting, t)
		case classDispatch:
			dispatchable = append(dispatchable, t)
		case classIgnore:
		}
	}

	stale := e.staleClaims(tasks, roles)
	if len(dispatchable)+len(stale) == 0 {
		return progressed, waiting, nil
	}
	sort.Slice(dispatchable, func(i, j int) bool {
		a, b := dispatchable[i], dispatchable[j]
		if a.StepOrdinal != b.StepOrdinal {
			return a.StepOrdinal < b.StepOrdinal
		}
		return a.ID < b.ID
	})
	e.dispatchAll(ctx, dispatchable, stale, roles, report)
	return true, waiting, nil
}

// classify decides a task's fate for this round, applying the human
// timeout policy and `when` as side effects where they fire.
func (e *Executor) classify(ctx context.Context, t *Task, byID map[string]*Task, roles execRoles, report *ExecReport) (taskClass, error) {
	if !e.eligible(t) || t.Status != roles.initial {
		return classIgnore, nil
	}
	ready, err := e.depsSatisfied(ctx, t, byID)
	if err != nil || !ready {
		return classIgnore, err
	}
	if t.EffectiveKind() == TaskKindHuman {
		applied, err := e.applyHumanTimeout(ctx, t, roles, report)
		if err != nil {
			return classIgnore, err
		}
		if applied {
			return classProgressed, nil
		}
		return classWaiting, nil
	}
	verdict, err := e.evaluateWhen(ctx, t, roles, report)
	if err != nil {
		return classIgnore, err
	}
	if verdict != whenRun {
		return classProgressed, nil
	}
	return classDispatch, nil
}

// eligible applies the provenance filter: strict mode dispatches only
// recipe-born tasks; a blocked reason parks a task in either mode.
func (e *Executor) eligible(t *Task) bool {
	if t.BlockedReason != nil && *t.BlockedReason != "" {
		return false
	}
	return e.opts.Permissive || t.RunID != ""
}

// depsSatisfied reports whether every blocker is terminal. Blockers
// outside the listed set are looked up; unknown ids are ignored, as the
// dependency graph does.
func (e *Executor) depsSatisfied(ctx context.Context, t *Task, byID map[string]*Task) (bool, error) {
	for _, dep := range t.BlockedBy() {
		blocker, ok := byID[dep]
		if !ok {
			other, err := e.repo.GetTask(ctx, dep)
			if err != nil {
				return false, fmt.Errorf("executor: look up blocker %s of %s: %w", dep, t.ID, err)
			}
			if other == nil {
				continue
			}
			blocker = other
		}
		if !e.wm.IsTerminal(blocker.Status) {
			return false, nil
		}
	}
	return true, nil
}

// staleClaims returns eligible active-role tasks whose claim is older
// than Reclaim, whoever holds it.
func (e *Executor) staleClaims(tasks []*Task, roles execRoles) []*Task {
	if e.opts.Reclaim <= 0 {
		return nil
	}
	now := e.opts.Now()
	var out []*Task
	for _, t := range tasks {
		if !e.eligible(t) || t.Status != roles.active || t.ClaimedAt == nil {
			continue
		}
		if now.Sub(*t.ClaimedAt) >= e.opts.Reclaim && t.EffectiveKind() != TaskKindHuman {
			out = append(out, t)
		}
	}
	return out
}

type whenVerdict int

const (
	whenRun whenVerdict = iota
	whenSkipped
	whenBlocked
)

// evaluateWhen applies a task's `when:` against the results of its run.
// False skips the task; an evaluation error blocks it with the error as
// the reason, so a typo is visible rather than silently passing.
func (e *Executor) evaluateWhen(ctx context.Context, t *Task, roles execRoles, report *ExecReport) (whenVerdict, error) {
	if t.Spec == nil || t.Spec.When == "" {
		return whenRun, nil
	}
	env, err := e.resultsEnv(ctx, t)
	if err != nil {
		return whenRun, err
	}
	ok, evalErr := EvalConditionEnv(t.Spec.When, env)
	switch {
	case evalErr != nil:
		if err := e.block(ctx, t, "when: "+evalErr.Error(), report); err != nil {
			return whenRun, err
		}
		return whenBlocked, nil
	case ok:
		return whenRun, nil
	}
	return whenSkipped, e.skip(ctx, t, roles, "when false: "+t.Spec.When, report)
}

// resultsEnv builds the `when:` environment: results of every task in
// the same run keyed by step id, plus the run's vars.
func (e *Executor) resultsEnv(ctx context.Context, t *Task) (CondEnv, error) {
	env := CondEnv{Results: map[string]map[string]any{}, Vars: map[string]any{}}
	if t.RunID == "" {
		return env, nil
	}
	siblings, err := e.repo.ListTasks(ctx, Query{
		Filters:         []FieldFilter{{Field: filterRunID, Operator: OpEq, Value: t.RunID}},
		AllProjects:     true,
		IncludeArchived: true,
	})
	if err != nil {
		return env, fmt.Errorf("executor: list run %s tasks: %w", t.RunID, err)
	}
	for _, s := range siblings {
		if s.StepID == "" {
			continue
		}
		result := s.Result
		if result == nil {
			result = map[string]any{}
		}
		env.Results[s.StepID] = result
	}
	if e.runs != nil {
		if run, err := e.runs.GetRecipeRun(ctx, t.RunID); err == nil && run != nil && run.Vars != nil {
			env.Vars = run.Vars
		}
	}
	return env, nil
}
