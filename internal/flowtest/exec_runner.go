package flowtest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hop.top/tlc/internal/core"
)

// ExecAgentRunner adapts core.RunArgv to core.AgentRunner for `type: exec`
// flow steps, keeping the step contract: timeouts and spawn failures are
// errors, a nonzero exit is an error unless allow_nonzero_exit.
//
// In flow-test mode the sandbox prepends the catchall shim to PATH, so exec
// steps that invoke real binaries are intercepted and cassetted via xrr
// transparently — no extra wiring required here.
type ExecAgentRunner struct {
	// DefaultCwd is used when step.Exec.Cwd is empty. May be empty (= os.Getwd).
	DefaultCwd string
}

// NewExecAgentRunner constructs an ExecAgentRunner with the given default cwd.
// Pass the flow workdir (sandbox repo dir in flow-test mode); pass "" for
// "use the current process working dir at Run time".
func NewExecAgentRunner(defaultCwd string) *ExecAgentRunner {
	return &ExecAgentRunner{DefaultCwd: defaultCwd}
}

// CanHandle returns true for steps with Type == "exec".
func (r *ExecAgentRunner) CanHandle(step core.Step) bool {
	return step.Type == core.StepTypeExec
}

// Run executes step.Exec.Argv and returns the structured output map.
// The prompt argument is unused (exec steps have no LLM prompt).
func (r *ExecAgentRunner) Run(ctx context.Context, step core.Step, _ string) (map[string]any, error) {
	if step.Exec == nil {
		return nil, fmt.Errorf("exec runner: step %q has no exec config", step.ID)
	}
	if len(step.Exec.Argv) == 0 {
		return nil, fmt.Errorf("exec runner: step %q has empty argv", step.ID)
	}

	res, err := core.RunArgv(ctx, core.ArgvOpts{
		Argv:       step.Exec.Argv,
		Cwd:        step.Exec.Cwd,
		DefaultCwd: r.DefaultCwd,
		Env:        step.Exec.Env,
		Timeout:    time.Duration(step.Exec.TimeoutSec) * time.Second,
		StdoutMax:  step.Exec.StdoutMaxBytes,
	})
	switch {
	case errors.Is(err, core.ErrArgvTimeout):
		// Timeout always fails, regardless of allow_nonzero_exit.
		return nil, fmt.Errorf("exec runner: step %q timed out after %ds", step.ID, step.Exec.TimeoutSec)
	case err != nil:
		return nil, fmt.Errorf("exec runner: step %q: %w", step.ID, err)
	}

	out := res.Map()
	if res.ExitCode != 0 && !step.Exec.AllowNonzeroExit {
		return out, fmt.Errorf("exec runner: step %q exited %d", step.ID, res.ExitCode)
	}
	return out, nil
}

// Compile-time assertion: ExecAgentRunner satisfies core.AgentRunner.
var _ core.AgentRunner = (*ExecAgentRunner)(nil)
