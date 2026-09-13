package core

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"hop.top/tlc/internal/config"
)

// Dispatcher runs one task of a kind and reports how it went. The
// executor owns every state transition; a dispatcher only executes.
type Dispatcher interface {
	Dispatch(ctx context.Context, task *Task) (*DispatchResult, error)
}

// DispatchFunc adapts a function to Dispatcher.
type DispatchFunc func(ctx context.Context, task *Task) (*DispatchResult, error)

// Dispatch implements Dispatcher.
func (f DispatchFunc) Dispatch(ctx context.Context, task *Task) (*DispatchResult, error) {
	return f(ctx, task)
}

// DispatchResult is a dispatcher's verdict. Result is stored on the task
// and read by downstream `when:` expressions and eva gates.
type DispatchResult struct {
	Status  AgentResultStatus
	Summary string
	Result  map[string]any
}

// ExecutorOpts tunes one run. Sleep and Now are injectable so tests run
// without wall-clock waits.
type ExecutorOpts struct {
	// Actor is recorded as the claimant and log author.
	Actor string
	// Concurrency bounds in-flight dispatches; values below 1 mean 1, and
	// 1 runs strictly in ordinal order.
	Concurrency int
	// Permissive also dispatches tasks without recipe provenance (no
	// run_id). Strict mode is the default.
	Permissive bool
	// Reclaim, when positive, re-dispatches tasks whose claim is older
	// than this — the leftovers of an actor that died.
	Reclaim time.Duration
	// Wait keeps polling every Poll instead of returning when only human
	// tasks are ready.
	Wait bool
	Poll time.Duration
	// EvaKey is passed to eva gates.
	EvaKey string
	// Out receives one progress line per event; nil discards.
	Out   io.Writer
	Sleep func(ctx context.Context, d time.Duration) error
	Now   func() time.Time
}

// ExecReport lists what a run did, by task id.
type ExecReport struct {
	Done         []string
	Skipped      []string
	Failed       []string // failed attempts that will be retried
	Blocked      []string
	Reclaimed    []string
	WaitingHuman []string
}

// Executor is the single engine behind `track execute`: it recomputes
// readiness from the store each round, claims, dispatches by kind, and
// applies the outcome to the task.
type Executor struct {
	repo     Repository
	claims   TaskClaimer
	runs     RecipeRunStore // may be nil: no results env, no subject completion
	wm       *WorkflowManager
	dispatch map[TaskKind]Dispatcher
	opts     ExecutorOpts
	gate     func(ctx context.Context, gate *StepGate, output map[string]any, apiKey string) error

	mu sync.Mutex // guards report appends from concurrent dispatches
}

// NewExecutor wires an executor. Register a Dispatcher per kind before
// running; kinds without one are blocked, not silently completed.
func NewExecutor(repo Repository, claims TaskClaimer, runs RecipeRunStore, wm *WorkflowManager, opts ExecutorOpts) *Executor {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}
	if opts.Poll <= 0 {
		opts.Poll = 5 * time.Second
	}
	if opts.Sleep == nil {
		opts.Sleep = ctxSleep
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Actor == "" {
		opts.Actor = GetCurrentUser()
	}
	return &Executor{
		repo:     repo,
		claims:   claims,
		runs:     runs,
		wm:       wm,
		dispatch: map[TaskKind]Dispatcher{},
		opts:     opts,
		gate:     RunEvaGate,
	}
}

// Register installs the dispatcher for a kind, replacing any previous one.
func (e *Executor) Register(kind TaskKind, d Dispatcher) {
	e.dispatch[kind] = d
}

// execRoles are the workflow statuses the executor moves tasks between,
// resolved by role so a custom vocabulary works unchanged.
type execRoles struct {
	initial, active, completed TaskStatus
	skipped                    TaskStatus // "" when the vocabulary declares none
}

func (e *Executor) roles() (execRoles, error) {
	var r execRoles
	var err error
	if r.initial, err = e.wm.StatusForRole(config.RoleInitial); err != nil {
		return r, fmt.Errorf("executor: %w", err)
	}
	if r.active, err = e.wm.StatusForRole(config.RoleActive); err != nil {
		return r, fmt.Errorf("executor: %w", err)
	}
	if r.completed, err = e.wm.StatusForRole(config.RoleCompleted); err != nil {
		return r, fmt.Errorf("executor: %w", err)
	}
	if s, res := e.wm.SkippedStatus(); res != SkippedUnresolved {
		r.skipped = s
	}
	return r, nil
}

// RunTrack executes the track's tasks until nothing is ready. It returns
// early, with the report so far, when the context is canceled.
func (e *Executor) RunTrack(ctx context.Context, trackID, projectID string) (*ExecReport, error) {
	return e.run(ctx, func(ctx context.Context) ([]*Task, error) {
		tasks, err := e.repo.ListTasks(ctx, Query{
			Filters: []FieldFilter{
				{Field: filterTrackID, Operator: OpEq, Value: trackID},
				{Field: filterProjectID, Operator: OpEq, Value: projectID},
			},
			AllProjects:     true,
			IncludeArchived: true,
		})
		if err != nil {
			return nil, fmt.Errorf("list track %s tasks: %w", trackID, err)
		}
		return tasks, nil
	})
}

// RunTasks executes an explicit set of tasks (the `task execute` and
// `--run` scopes) under the same rules as RunTrack.
func (e *Executor) RunTasks(ctx context.Context, ids []string) (*ExecReport, error) {
	return e.run(ctx, func(ctx context.Context) ([]*Task, error) {
		out := make([]*Task, 0, len(ids))
		for _, id := range ids {
			t, err := e.repo.GetTask(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("get task %s: %w", id, err)
			}
			if t == nil {
				return nil, fmt.Errorf("task %s not found; run 'tlc task list' to see available tasks", id)
			}
			out = append(out, t)
		}
		return out, nil
	})
}

func (e *Executor) run(ctx context.Context, scope func(context.Context) ([]*Task, error)) (*ExecReport, error) {
	report := &ExecReport{}
	roles, err := e.roles()
	if err != nil {
		return report, err
	}
	for {
		if ctx.Err() != nil {
			return report, nil //nolint:nilerr // cancellation is a clean stop; the report so far is the result
		}
		tasks, err := scope(ctx)
		if err != nil {
			return report, fmt.Errorf("executor: list tasks: %w", err)
		}
		progressed, waiting, err := e.round(ctx, tasks, roles, report)
		if err != nil {
			return report, err
		}
		if progressed {
			continue
		}
		report.WaitingHuman = taskIDs(waiting)
		if len(waiting) == 0 || !e.opts.Wait {
			return report, nil
		}
		for _, t := range waiting {
			e.printf("⏸ waiting on %s (human)\n", FormatTaskAlias(t))
		}
		if err := e.opts.Sleep(ctx, e.opts.Poll); err != nil {
			return report, nil //nolint:nilerr // an interrupted poll is a clean stop
		}
	}
}

func (e *Executor) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(e.opts.Out, format, args...)
}

func (e *Executor) note(list *[]string, id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	*list = append(*list, id)
}

func taskIDs(tasks []*Task) []string {
	out := make([]string, len(tasks))
	for i, t := range tasks {
		out[i] = t.ID
	}
	return out
}

// ctxSleep waits d or until ctx is done.
func ctxSleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("sleep interrupted: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
