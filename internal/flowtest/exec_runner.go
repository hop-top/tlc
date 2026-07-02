package flowtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"hop.top/tlc/internal/core"
)

// Default values for ExecStep, mirroring task-flow-spec-0.1-dev §"Step Type: exec".
const (
	defaultExecTimeoutSec     = 60
	defaultExecStdoutMaxBytes = 1 << 20 // 1 MiB
)

// ExecAgentRunner implements core.AgentRunner for `type: exec` steps. It runs
// step.Exec.Argv literally via os/exec, captures stdout/stderr/exit_code/
// duration, and returns a structured map matching the spec's output schema.
//
// In flow-test mode the sandbox prepends the catchall shim to PATH, so exec
// steps that invoke real binaries are intercepted and cassetted via xrr
// transparently — no extra wiring required here. Tests that exercise the
// runner directly (without a sandbox) bypass the shim and exec the binary
// directly, which is the intended unit-test path.
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

	timeoutSec := step.Exec.TimeoutSec
	if timeoutSec == 0 {
		timeoutSec = defaultExecTimeoutSec
	}
	if timeoutSec < 0 {
		return nil, fmt.Errorf("exec runner: step %q has negative timeout_sec=%d", step.ID, timeoutSec)
	}

	maxBytes := step.Exec.StdoutMaxBytes
	if maxBytes == 0 {
		maxBytes = defaultExecStdoutMaxBytes
	}
	if maxBytes < 0 {
		return nil, fmt.Errorf("exec runner: step %q has negative stdout_max_bytes=%d", step.ID, maxBytes)
	}

	cwd := step.Exec.Cwd
	if cwd == "" {
		cwd = r.DefaultCwd
	} else if !filepath.IsAbs(cwd) && r.DefaultCwd != "" {
		cwd = filepath.Join(r.DefaultCwd, cwd)
	}

	// Build env: parent + step.Exec.Env overrides; empty value unsets.
	env := mergeExecEnv(os.Environ(), step.Exec.Env)

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Use exec.Command (not CommandContext) so we control the kill strategy.
	// exec.CommandContext sends SIGKILL only to the direct child on context
	// deadline; if the child is a shell, its grandchildren survive and keep
	// the stdout/stderr pipes open, so cmd.Wait blocks past the timeout
	// (observed as a CI -race flake: "5.003s elapsed" against a 4s budget
	// for `sh -c "sleep 5"` with TimeoutSec=1). Setting Setpgid puts the
	// child in its own process group; on deadline we kill the whole group
	// via negative pid (-pgid), reliably reaping descendants.
	cmd := exec.Command(step.Exec.Argv[0], step.Exec.Argv[1:]...) //nolint:gosec,noctx // user-supplied argv is the contract; ctx managed via runCtx watcher goroutine + killProcessGroup, not via CommandContext (which kills only the direct child)
	cmd.Dir = cwd
	cmd.Env = env
	setProcessGroup(cmd)

	stdoutBuf := &cappedBuffer{limit: maxBytes}
	stderrBuf := &cappedBuffer{limit: maxBytes}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec runner: step %q: %w", step.ID, err)
	}

	// Watcher goroutine: if the deadline trips before Wait returns, kill the
	// whole process group. Closed `done` signals the watcher to exit cleanly
	// on normal completion.
	done := make(chan struct{})
	go func() {
		select {
		case <-runCtx.Done():
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				killProcessGroup(cmd)
			}
		case <-done:
		}
	}()

	runErr := cmd.Wait()
	close(done)
	elapsed := time.Since(t0)

	// Timeout always fails, regardless of allow_nonzero_exit.
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("exec runner: step %q timed out after %ds", step.ID, timeoutSec)
	}

	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(runErr, &exitErr):
			exitCode = exitErr.ExitCode()
		default:
			// Spawn / not-found / IO error — distinct from "ran and exited nonzero".
			return nil, fmt.Errorf("exec runner: step %q: %w", step.ID, runErr)
		}
	}

	out := map[string]any{
		"exit_code":   exitCode,
		"stdout":      stdoutBuf.String(),
		"stderr":      stderrBuf.String(),
		"duration_ms": elapsed.Milliseconds(),
		"truncated":   stdoutBuf.truncated || stderrBuf.truncated,
	}

	if exitCode != 0 && !step.Exec.AllowNonzeroExit {
		return out, fmt.Errorf("exec runner: step %q exited %d", step.ID, exitCode)
	}

	return out, nil
}

// mergeExecEnv returns base merged with overrides. An empty override value
// unsets the matching key. Order: base first, then overrides; later wins.
func mergeExecEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		// Defensive copy so callers can mutate.
		out := make([]string, len(base))
		copy(out, base)
		return out
	}

	// Index base by key.
	byKey := make(map[string]int, len(base))
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		key := envKey(kv)
		byKey[key] = len(out)
		out = append(out, kv)
	}

	// Apply overrides.
	for k, v := range overrides {
		if v == "" {
			// unset — drop existing entry if present.
			if idx, ok := byKey[k]; ok {
				out = append(out[:idx], out[idx+1:]...)
				// re-index after deletion.
				byKey = make(map[string]int, len(out))
				for i, kv := range out {
					byKey[envKey(kv)] = i
				}
			}
			continue
		}
		entry := k + "=" + v
		if idx, ok := byKey[k]; ok {
			out[idx] = entry
		} else {
			byKey[k] = len(out)
			out = append(out, entry)
		}
	}
	return out
}

// cappedBuffer is an io.Writer that buffers up to limit bytes and silently
// drops the rest, recording whether truncation occurred.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

// Write implements io.Writer. It always returns len(p), nil so the underlying
// command does not see EPIPE / short-write — truncation is reported via the
// truncated flag instead.
func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.limit <= 0 {
		c.truncated = true
		return len(p), nil
	}
	remaining := c.limit - c.buf.Len()
	if remaining <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) <= remaining {
		_, _ = c.buf.Write(p)
		return len(p), nil
	}
	_, _ = c.buf.Write(p[:remaining])
	c.truncated = true
	return len(p), nil
}

// String returns the buffered content as a string.
func (c *cappedBuffer) String() string {
	return c.buf.String()
}

// Compile-time assertions: ExecAgentRunner satisfies core.AgentRunner and
// cappedBuffer satisfies io.Writer.
var (
	_ core.AgentRunner = (*ExecAgentRunner)(nil)
	_ io.Writer        = (*cappedBuffer)(nil)
)
