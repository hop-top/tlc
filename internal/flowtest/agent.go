package flowtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hop.top/tlc/internal/core"
	xrr "hop.top/xrr"
)

// SandboxAgentExec runs agent-kind tasks through the adapter shim resolved
// for each step. In record mode the shim proxies to the real binary and
// writes cassettes; in replay mode it serves from cassettes. The CLI wraps
// Run into the executor's agent dispatcher.
type SandboxAgentExec struct {
	sandbox  *Sandbox
	mode     xrr.Mode
	run      *Run
	resolver *AdapterResolver
	// Log receives one line per dispatch; nil discards.
	Log io.Writer
}

// NewSandboxAgentExec binds a sandbox, run and resolver. The sandbox is
// wired into the resolver so Probe runs on first resolution.
func NewSandboxAgentExec(sb *Sandbox, mode xrr.Mode, run *Run, resolver *AdapterResolver) *SandboxAgentExec {
	resolver.WithSandbox(sb)
	return &SandboxAgentExec{sandbox: sb, mode: mode, run: run, resolver: resolver, Log: io.Discard}
}

// Run dispatches the adapter for step with prompt and reports the outcome.
// A nonzero exit is a failed result that keeps the exit code and the raw
// output; only a spawn failure or an unresolvable adapter is an error.
func (x *SandboxAgentExec) Run(ctx context.Context, step StepRef, prompt string) (*core.AgentResult, error) {
	adapter, cfg, operation, err := x.resolver.ResolveCtx(ctx, step)
	if err != nil {
		return nil, fmt.Errorf("sandbox agent exec: %w", err)
	}
	if err := ensureCassetteDir(x.run, step.ID, x.mode); err != nil {
		return nil, fmt.Errorf("sandbox agent exec: %w", err)
	}

	// The binary is the adapter's own name under the sandbox bin dir, and
	// the args come from the adapter, not from user input.
	cmd := exec.CommandContext(ctx, filepath.Join(x.sandbox.BinDir, adapter.Binary()), //nolint:gosec // resolved shim path, adapter-built args
		adapter.BuildArgs(prompt, cfg, operation)...)
	cmd.Dir = x.sandbox.RepoDir
	cmd.Env = adapter.BuildEnv(x.sandbox.Env(step.ID, x.mode, x.run), cfg)
	// Explicit empty stdin so agents don't wait for piped input.
	cmd.Stdin = strings.NewReader("")

	t0 := time.Now()
	fmt.Fprintf(x.log(), "  [%s] dispatching %s (%s)...\n", step.ID, adapter.Name(), x.mode)
	combined, runErr := cmd.CombinedOutput()
	elapsed := time.Since(t0)
	fmt.Fprintf(x.log(), "  [%s] done in %.1fs\n", step.ID, elapsed.Seconds())

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, fmt.Errorf("sandbox agent exec: step %q: %s: %w", step.ID, adapter.Name(), runErr)
		}
		exitCode = exitErr.ExitCode()
	}
	return agentResult(adapter, combined, exitCode, elapsed, t0), nil
}

func (x *SandboxAgentExec) log() io.Writer {
	if x.Log == nil {
		return io.Discard
	}
	return x.Log
}

// agentResult folds the adapter's parsed output and the process exit code
// into the result the executor stores on the task. exit_code is always
// present so `when:` and eva gates read agent steps like exec steps.
func agentResult(adapter AgentAdapter, combined []byte, exitCode int, elapsed time.Duration, startedAt time.Time) *core.AgentResult {
	outputs, err := adapter.ParseOutput(combined)
	if err != nil || outputs == nil {
		outputs = map[string]any{resultKeyOutput: string(combined)}
	}
	outputs[core.ResultKeyExitCode] = exitCode
	res := &core.AgentResult{
		Version:   core.AgentResultVersion,
		Status:    core.AgentStatusSucceeded,
		ExitCode:  exitCode,
		Summary:   fmt.Sprintf("%s ok in %.1fs", adapter.Name(), elapsed.Seconds()),
		Agent:     adapter.Name(),
		StartedAt: startedAt.UTC().Format(time.RFC3339),
		EndedAt:   startedAt.Add(elapsed).UTC().Format(time.RFC3339),
		Outputs:   outputs,
	}
	if exitCode != 0 {
		res.Status = core.AgentStatusFailed
		res.Summary = fmt.Sprintf("%s exited %d: %s", adapter.Name(), exitCode, firstLine(string(combined)))
	}
	return res
}

// ensureCassetteDir creates the step's cassette dir in record mode so the
// shim can write into it; replay reads a missing dir as a miss already.
func ensureCassetteDir(run *Run, stepID string, mode xrr.Mode) error {
	if run == nil || mode != xrr.ModeRecord {
		return nil
	}
	dir := filepath.Join(run.RecordDir, stepID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mkdir cassette dir %s: %w", dir, err)
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
