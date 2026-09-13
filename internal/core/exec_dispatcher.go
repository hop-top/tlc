package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ExecDispatcher runs exec-kind tasks: the task's argv, literally, on
// the host through RunArgv. A nonzero exit is a failed dispatch; a
// timeout is reported as such with the partial output kept.
type ExecDispatcher struct {
	// DefaultCwd resolves a relative exec.cwd; empty means the process
	// working directory.
	DefaultCwd string
}

// NewExecDispatcher returns a dispatcher whose relative cwds resolve
// against defaultCwd.
func NewExecDispatcher(defaultCwd string) *ExecDispatcher {
	return &ExecDispatcher{DefaultCwd: defaultCwd}
}

// Dispatch implements Dispatcher.
func (d *ExecDispatcher) Dispatch(ctx context.Context, task *Task) (*DispatchResult, error) {
	if task.Spec == nil || task.Spec.Exec == nil || len(task.Spec.Exec.Argv) == 0 {
		return nil, fmt.Errorf("task %s has no exec argv; set spec.exec.argv or change its kind", task.ID)
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
			return nil, fmt.Errorf("task %s: exec timeout: %w", task.ID, err)
		}
		opts.Timeout = timeout
	}

	res, err := RunArgv(ctx, opts)
	switch {
	case errors.Is(err, ErrArgvTimeout):
		return &DispatchResult{Status: AgentStatusTimeout, Summary: err.Error(), Result: res.Map()}, nil
	case err != nil:
		return nil, err
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

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

var _ Dispatcher = (*ExecDispatcher)(nil)
