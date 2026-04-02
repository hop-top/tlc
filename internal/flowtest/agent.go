package flowtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hop.top/tlc/internal/core"
	xrr "hop.top/xrr"
)

// SandboxAgentRunner implements core.AgentRunner by spawning the claude shim
// inside the sandbox. In record mode the shim proxies to the real binary and
// writes cassettes; in replay mode it serves from cassettes.
type SandboxAgentRunner struct {
	sandbox      *Sandbox
	mode         xrr.Mode
	run          *Run
	realClaudeDir string // captured before sandbox HOME override
}

// NewSandboxAgentRunner creates a runner bound to the given sandbox and run.
// Must be called before sandbox env vars are injected into the process.
func NewSandboxAgentRunner(sb *Sandbox, mode xrr.Mode, run *Run) *SandboxAgentRunner {
	realHome := os.Getenv("HOME")
	return &SandboxAgentRunner{
		sandbox:       sb,
		mode:          mode,
		run:           run,
		realClaudeDir: filepath.Join(realHome, ".claude"),
	}
}

// Run dispatches the agent for stepID and returns the parsed step output.
func (r *SandboxAgentRunner) Run(ctx context.Context, step core.Step, prompt string) (map[string]any, error) {
	// Pre-create per-step cassette dir so xrr.FileCassette can write to it.
	if r.run != nil {
		cassetteDir := filepath.Join(r.run.RecordDir, step.ID)
		if err := os.MkdirAll(cassetteDir, 0o755); err != nil {
			return nil, fmt.Errorf("sandbox agent runner: mkdir cassette dir: %w", err)
		}
	}

	env := r.sandbox.Env(step.ID, r.mode, r.run)

	// Always route through the claude shim. In record mode the shim calls the
	// real binary (via findReal which skips shimDir) and writes the cassette.
	// In replay mode the shim serves the response from the cassette without any
	// real API call. CLAUDE_CONFIG_DIR is only needed for the real binary path
	// (shim passes it through as part of the recorded env).
	claudePath := filepath.Join(r.sandbox.BinDir, "claude")
	// In record mode, the shim will invoke the real claude — pass CLAUDE_CONFIG_DIR
	// so it can authenticate. The shim inherits env, so set it here.
	agentEnv := replaceEnvVar(env, "CLAUDE_CONFIG_DIR", r.realClaudeDir)

	cmd := exec.CommandContext(ctx, claudePath, "-p", prompt,
		"--print", "--output-format", "json", "--dangerously-skip-permissions",
		"--max-budget-usd", "0.10")
	cmd.Dir = r.sandbox.RepoDir
	cmd.Env = agentEnv
	// Explicit empty stdin so claude doesn't wait 3s for piped input.
	cmd.Stdin = strings.NewReader("")

	modeLabel := "replay"
	if r.mode == xrr.ModeRecord {
		modeLabel = "record"
	}
	t0 := time.Now()
	fmt.Fprintf(os.Stderr, "  [%s] dispatching claude (%s)...\n", step.ID, modeLabel)

	combined, err := cmd.CombinedOutput()
	fmt.Fprintf(os.Stderr, "  [%s] done in %.1fs\n", step.ID, time.Since(t0).Seconds())
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("sandbox agent runner: step %q: claude exited %d: %s",
				step.ID, exitErr.ExitCode(), string(combined))
		}
		return nil, fmt.Errorf("sandbox agent runner: step %q: %w", step.ID, err)
	}
	stdout := combined

	// --output-format json returns {"type":"result","subtype":"success","result":"...","..."}
	// Pass the full parsed object as step output; fallback to raw wrap on parse error.
	var result map[string]any
	if jsonErr := json.Unmarshal(stdout, &result); jsonErr != nil {
		result = map[string]any{"output": string(stdout)}
	}
	return result, nil
}

// replaceEnvVar returns env with the value of key replaced by val.
// If key is not present, it appends "key=val".
func replaceEnvVar(env []string, key, val string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	found := false
	for _, kv := range env {
		if len(kv) >= len(prefix) && kv[:len(prefix)] == prefix {
			out = append(out, prefix+val)
			found = true
		} else {
			out = append(out, kv)
		}
	}
	if !found {
		out = append(out, prefix+val)
	}
	return out
}

