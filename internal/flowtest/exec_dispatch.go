package flowtest

import (
	"context"
	"fmt"

	"hop.top/tlc/internal/core"
	xrr "hop.top/xrr"
)

// SandboxArgvRunner runs one exec-kind task's argv inside the sandbox: the
// step env (cassette dir, mode, HOME, PATH leading with bin/) under the
// task's own exec.env, and a relative cwd resolved against the sandbox
// repo. The sandbox prepends the shims to PATH, so exec steps that invoke
// shimmed binaries are cassetted transparently.
//
// ArgvOpts carries no step identity, so one runner is built per task.
type SandboxArgvRunner struct {
	Sandbox *Sandbox
	Mode    xrr.Mode
	Fixture *Run
	StepID  string
}

// Run implements core.ArgvRunner.
func (r *SandboxArgvRunner) Run(ctx context.Context, opts core.ArgvOpts) (*core.ArgvResult, error) {
	env := r.Sandbox.Overrides(r.StepID, r.Mode, r.Fixture)
	for k, v := range opts.Env {
		env[k] = v
	}
	opts.Env = env
	if opts.DefaultCwd == "" {
		opts.DefaultCwd = r.Sandbox.RepoDir
	}
	return core.RunArgv(ctx, opts) //nolint:wrapcheck // the host runner's error already names the argv
}

var _ core.ArgvRunner = (*SandboxArgvRunner)(nil)

// SandboxExecDispatcher runs exec-kind tasks inside the sandbox through
// core.ExecDispatcher, one SandboxArgvRunner per task so the cassette dir
// follows the task's step id.
type SandboxExecDispatcher struct {
	sandbox *Sandbox
	mode    xrr.Mode
	run     *Run
}

// NewSandboxExecDispatcher binds a sandbox and run.
func NewSandboxExecDispatcher(sb *Sandbox, mode xrr.Mode, run *Run) *SandboxExecDispatcher {
	return &SandboxExecDispatcher{sandbox: sb, mode: mode, run: run}
}

// Dispatch implements core.Dispatcher.
func (d *SandboxExecDispatcher) Dispatch(ctx context.Context, task *core.Task) (*core.DispatchResult, error) {
	if err := ensureCassetteDir(d.run, task.StepID, d.mode); err != nil {
		return nil, fmt.Errorf("sandbox exec: task %s: %w", task.ID, err)
	}
	inner := &core.ExecDispatcher{
		DefaultCwd: d.sandbox.RepoDir,
		Runner:     &SandboxArgvRunner{Sandbox: d.sandbox, Mode: d.mode, Fixture: d.run, StepID: task.StepID},
	}
	return inner.Dispatch(ctx, task) //nolint:wrapcheck // the exec dispatcher's error already names the task
}

var _ core.Dispatcher = (*SandboxExecDispatcher)(nil)
