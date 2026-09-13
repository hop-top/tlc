package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ExecDispatcher runs exec-kind tasks: the task's argv, literally,
// through an ArgvRunner — the host by default, a pod when `track execute
// --with-pod` asks for one. A nonzero exit is a failed dispatch; a
// timeout is reported as such with the partial output kept.
type ExecDispatcher struct {
	// DefaultCwd resolves a relative exec.cwd on the host; empty means
	// the process working directory. The pod runner resolves against its
	// own WorkDir instead.
	DefaultCwd string
	// Runner executes the argv; nil means HostArgvRunner.
	Runner ArgvRunner
}

// NewExecDispatcher returns a host dispatcher whose relative cwds
// resolve against defaultCwd.
func NewExecDispatcher(defaultCwd string) *ExecDispatcher {
	return &ExecDispatcher{DefaultCwd: defaultCwd}
}

// NewPodExecDispatcher returns a dispatcher that runs every exec-kind
// task inside a fresh pod through runner.
func NewPodExecDispatcher(runner *PodArgvRunner) *ExecDispatcher {
	return &ExecDispatcher{Runner: runner}
}

func (d *ExecDispatcher) runner() ArgvRunner {
	if d.Runner != nil {
		return d.Runner
	}
	return HostArgvRunner{}
}

// Dispatch implements Dispatcher.
func (d *ExecDispatcher) Dispatch(ctx context.Context, task *Task) (*DispatchResult, error) {
	opts, err := d.argvOpts(task)
	if err != nil {
		return nil, err
	}

	res, err := d.runner().Run(ctx, opts)
	switch {
	case errors.Is(err, ErrArgvTimeout):
		return &DispatchResult{Status: AgentStatusTimeout, Summary: err.Error(), Result: res.Map()}, nil
	case err != nil:
		return nil, fmt.Errorf("task %s: %w", task.ID, err)
	}
	if res.ExitCode != 0 {
		return &DispatchResult{
			Status:  AgentStatusFailed,
			Summary: fmt.Sprintf("exit %d: %s", res.ExitCode, firstLine(res.Stderr)),
			Result:  res.Map(),
		}, nil
	}
	return &DispatchResult{
		Status:  AgentStatusSucceeded,
		Summary: fmt.Sprintf("exit 0 in %dms", res.DurationMs),
		Result:  res.Map(),
	}, nil
}

// argvOpts builds the run options from the task's exec spec.
func (d *ExecDispatcher) argvOpts(task *Task) (ArgvOpts, error) {
	if task.Spec == nil || task.Spec.Exec == nil || len(task.Spec.Exec.Argv) == 0 {
		return ArgvOpts{}, fmt.Errorf("task %s has no exec argv; set spec.exec.argv or change its kind", task.ID)
	}
	spec := task.Spec.Exec
	opts := ArgvOpts{
		Argv:       spec.Argv,
		Cwd:        spec.Cwd,
		DefaultCwd: d.DefaultCwd,
		Env:        spec.Env,
		StdoutMax:  spec.StdoutMax,
	}
	if spec.Timeout != "" {
		timeout, err := ParseRecipeDuration(spec.Timeout)
		if err != nil {
			return ArgvOpts{}, fmt.Errorf("task %s: exec timeout: %w", task.ID, err)
		}
		opts.Timeout = timeout
	}
	return opts, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

var _ Dispatcher = (*ExecDispatcher)(nil)
