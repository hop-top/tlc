package core

import (
	"context"
	"fmt"
)

// CompleteSubjectIfDone applies the executor's subject rule from outside
// a run — after `task approve` finishes the last human leaf, for one.
func CompleteSubjectIfDone(ctx context.Context, repo Repository, runs RecipeRunStore, wm *WorkflowManager, actor string, task *Task) error {
	e := NewExecutor(repo, nil, runs, wm, ExecutorOpts{Actor: actor})
	return e.maybeCompleteSubject(ctx, task)
}

// maybeCompleteSubject completes the subject a run decomposes once every
// leaf of the run is done. The subject is left alone when it is already
// terminal or claimed by another actor; the run's provenance names the
// recipe in the completion note.
func (e *Executor) maybeCompleteSubject(ctx context.Context, task *Task) error {
	prov := task.Provenance()
	if e.runs == nil || task.RunID == "" || prov.Subject == "" {
		return nil
	}
	done, err := e.runLeavesDone(ctx, task.RunID)
	if err != nil || !done {
		return err
	}
	subject, err := e.loadSubject(ctx, task, prov.Subject)
	if err != nil || !e.subjectOpen(subject) {
		return err
	}
	return e.completeSubject(ctx, subject, prov)
}

// subjectOpen reports whether the subject can be completed on the run's
// behalf: it exists, is not terminal, and is not held by another actor.
func (e *Executor) subjectOpen(subject *Task) bool {
	if subject == nil || e.wm.IsTerminal(subject.Status) {
		return false
	}
	owner := ""
	if subject.AssignedTo != nil {
		owner = *subject.AssignedTo
	}
	return owner == "" || owner == e.opts.Actor
}

// completeSubject moves the subject to the completed role with a note
// naming the recipe that decomposed it.
func (e *Executor) completeSubject(ctx context.Context, subject *Task, prov TaskProvenance) error {
	completed, err := e.wm.StatusForRole("completed")
	if err != nil {
		return fmt.Errorf("executor: %w", err)
	}
	note := "completed via recipe " + prov.Recipe
	if prov.RecipeVersion != "" {
		note += "@" + prov.RecipeVersion
	}
	entry, err := subject.TransitionWithWorkflow(completed, e.opts.Actor, note, e.wm, true)
	if err != nil {
		return fmt.Errorf("executor: complete subject %s: %w", subject.ID, err)
	}
	entry.Action = ActionDone
	if err := e.repo.UpdateTaskWithLog(ctx, subject, entry); err != nil {
		return fmt.Errorf("executor: complete subject %s: %w", subject.ID, err)
	}
	e.printf("✓ %s %s\n", FormatTaskAlias(subject), note)
	return nil
}

// runLeavesDone reports whether every leaf task of a run (no dependents
// within the run) is terminal.
func (e *Executor) runLeavesDone(ctx context.Context, runID string) (bool, error) {
	rows, err := e.runs.ListRecipeRunTasks(ctx, runID)
	if err != nil {
		return false, fmt.Errorf("executor: list run %s tasks: %w", runID, err)
	}
	runTasks := make([]*Task, 0, len(rows))
	for _, row := range rows {
		t, err := e.repo.GetTask(ctx, row.TaskID)
		if err != nil {
			return false, fmt.Errorf("executor: get run task %s: %w", row.TaskID, err)
		}
		if t != nil {
			runTasks = append(runTasks, t)
		}
	}
	graph, err := NewDepGraph(runTasks)
	if err != nil {
		return false, fmt.Errorf("executor: run %s graph: %w", runID, err)
	}
	for _, t := range runTasks {
		if len(graph.Dependents(t.ID)) == 0 && !e.wm.IsTerminal(t.Status) {
			return false, nil
		}
	}
	return true, nil
}

// loadSubject resolves the subject reference, accepting a typeid or a
// display alias. An unresolvable reference is not a run failure: it
// returns (nil, nil) and the subject is simply not completed.
func (e *Executor) loadSubject(ctx context.Context, task *Task, ref string) (*Task, error) {
	subject, err := e.repo.GetTask(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("executor: get subject %s: %w", ref, err)
	}
	if subject != nil {
		return subject, nil
	}
	id, refErr := ParseTaskRef(ctx, e.repo, projectOf(task), ref)
	if refErr != nil {
		return nil, nil //nolint:nilerr // an unresolvable subject ref only means nothing to complete
	}
	subject, err = e.repo.GetTask(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("executor: get subject %s: %w", id, err)
	}
	return subject, nil
}
