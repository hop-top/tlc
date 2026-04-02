package flowtest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hop.top/tlc/internal/core"
	xrr "hop.top/xrr"
)

// SandboxAgentRunner implements core.AgentRunner by dispatching to the adapter
// resolved for each step. In record mode the shim proxies to the real binary and
// writes cassettes; in replay mode it serves from cassettes.
type SandboxAgentRunner struct {
	sandbox  *Sandbox
	mode     xrr.Mode
	run      *Run
	resolver *AdapterResolver
}

// NewSandboxAgentRunner creates a runner bound to the given sandbox, run, and
// resolver. Must be called before sandbox env vars are injected into the process.
// The sandbox is wired into the resolver so Probe is called on first resolution.
func NewSandboxAgentRunner(sb *Sandbox, mode xrr.Mode, run *Run, resolver *AdapterResolver) *SandboxAgentRunner {
	resolver.WithSandbox(sb)
	return &SandboxAgentRunner{
		sandbox:  sb,
		mode:     mode,
		run:      run,
		resolver: resolver,
	}
}

// CanHandle returns true for task steps with a TaskTemplate (i.e. steps that
// require agent dispatch). Structural steps (parallel/branch/join/retry/subflow)
// are not dispatched by this runner.
func (r *SandboxAgentRunner) CanHandle(step core.Step) bool {
	return step.Type == core.StepTypeTask && step.TaskTemplate != nil
}

// Run dispatches the agent for stepID and returns the parsed step output.
func (r *SandboxAgentRunner) Run(ctx context.Context, step core.Step, prompt string) (map[string]any, error) {
	adapter, cfg, operation, err := r.resolver.ResolveCtx(ctx, step)
	if err != nil {
		return nil, fmt.Errorf("sandbox agent runner: %w", err)
	}

	binaryPath := filepath.Join(r.sandbox.BinDir, adapter.Binary())

	// Pre-create per-step cassette dir so xrr.FileCassette can write to it.
	if r.run != nil {
		cassetteDir := filepath.Join(r.run.RecordDir, step.ID)
		if err := os.MkdirAll(cassetteDir, 0o755); err != nil {
			return nil, fmt.Errorf("sandbox agent runner: mkdir cassette dir: %w", err)
		}
	}

	env := r.sandbox.Env(step.ID, r.mode, r.run)
	env = adapter.BuildEnv(env, cfg)

	cmd := exec.CommandContext(ctx, binaryPath, adapter.BuildArgs(prompt, cfg, operation)...)
	cmd.Dir = r.sandbox.RepoDir
	cmd.Env = env
	// Explicit empty stdin so agents don't wait for piped input.
	cmd.Stdin = strings.NewReader("")

	modeLabel := "replay"
	if r.mode == xrr.ModeRecord {
		modeLabel = "record"
	}
	t0 := time.Now()
	fmt.Fprintf(os.Stderr, "  [%s] dispatching %s (%s)...\n", step.ID, adapter.Name(), modeLabel)

	combined, err := cmd.CombinedOutput()
	fmt.Fprintf(os.Stderr, "  [%s] done in %.1fs\n", step.ID, time.Since(t0).Seconds())
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("sandbox agent runner: step %q: %s exited %d: %s",
				step.ID, adapter.Name(), exitErr.ExitCode(), string(combined))
		}
		return nil, fmt.Errorf("sandbox agent runner: step %q: %w", step.ID, err)
	}

	return adapter.ParseOutput(combined)
}
