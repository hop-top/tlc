package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Defaults for an argv run, mirroring the exec step contract.
const (
	defaultArgvTimeout   = 60 * time.Second
	defaultArgvStdoutMax = 1 << 20 // 1 MiB
)

// ErrArgvTimeout is returned (alongside the partial result) when a run
// exceeds its timeout. The whole process group is killed first, so a
// shell child's grandchildren cannot keep the pipes open.
var ErrArgvTimeout = errors.New("argv run timed out")

// ArgvOpts describes one literal command run: an exec-kind task or a
// flow exec step.
type ArgvOpts struct {
	Argv []string
	// Cwd is the working directory; relative values resolve against
	// DefaultCwd. Empty means DefaultCwd, and both empty means the
	// process working directory.
	Cwd        string
	DefaultCwd string
	// Env overrides the parent environment; an empty value unsets the key.
	Env map[string]string
	// Timeout bounds the run; zero means defaultArgvTimeout.
	Timeout time.Duration
	// StdoutMax caps captured stdout and stderr each; zero means
	// defaultArgvStdoutMax. Output past the cap is dropped, not an error.
	StdoutMax int
}

// ArgvResult is what a run leaves behind. ExitCode is reported, not
// judged: the caller decides whether nonzero is a failure.
type ArgvResult struct {
	ExitCode   int
	Stdout     string
	Stderr     string
	DurationMs int64
	Truncated  bool
	TimedOut   bool
}

// Map renders the result in the exec output schema — the keys `when:`
// expressions, eva gates and the flow adapter read.
func (r *ArgvResult) Map() map[string]any {
	return map[string]any{
		"exit_code":   r.ExitCode,
		"stdout":      r.Stdout,
		"stderr":      r.Stderr,
		"duration_ms": r.DurationMs,
		"truncated":   r.Truncated,
	}
}

// ArgvRunner runs one literal argv and reports an ArgvResult, wherever
// the command actually runs: HostArgvRunner on this machine,
// PodArgvRunner inside a pod. The result shape is the same either way.
type ArgvRunner interface {
	Run(ctx context.Context, opts ArgvOpts) (*ArgvResult, error)
}

// HostArgvRunner runs argv on the host through RunArgv.
type HostArgvRunner struct{}

// Run implements ArgvRunner.
func (HostArgvRunner) Run(ctx context.Context, opts ArgvOpts) (*ArgvResult, error) {
	return RunArgv(ctx, opts)
}

var _ ArgvRunner = HostArgvRunner{}

// RunArgv executes opts.Argv literally. Spawn failures (binary not found,
// bad cwd) return a nil result and an error; a run that started returns
// its result, with ErrArgvTimeout when the deadline killed it.
func RunArgv(ctx context.Context, opts ArgvOpts) (*ArgvResult, error) {
	timeout, maxBytes, err := argvLimits(opts)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// exec.Command rather than CommandContext so the kill strategy is ours:
	// CommandContext SIGKILLs only the direct child on deadline, and a shell
	// child's grandchildren then keep the stdout/stderr pipes open, so Wait
	// blocks past the timeout. Setpgid puts the child in its own process
	// group and the watcher kills the whole group.
	cmd := exec.Command(opts.Argv[0], opts.Argv[1:]...) //nolint:gosec,noctx // caller-supplied argv is the contract; ctx is enforced by the watcher + killProcessGroup, not CommandContext
	cmd.Dir = argvCwd(opts)
	cmd.Env = mergeExecEnv(os.Environ(), opts.Env)
	setProcessGroup(cmd)

	stdoutBuf := &cappedBuffer{limit: maxBytes}
	stderrBuf := &cappedBuffer{limit: maxBytes}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	t0 := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("run %q: %w", opts.Argv[0], err)
	}

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

	res := &ArgvResult{
		Stdout:     stdoutBuf.String(),
		Stderr:     stderrBuf.String(),
		DurationMs: time.Since(t0).Milliseconds(),
		Truncated:  stdoutBuf.truncated || stderrBuf.truncated,
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		return res, fmt.Errorf("%w after %s", ErrArgvTimeout, timeout)
	}
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			// Spawn / IO error — distinct from "ran and exited nonzero".
			return nil, fmt.Errorf("run %q: %w", opts.Argv[0], runErr)
		}
		res.ExitCode = exitErr.ExitCode()
	}
	return res, nil
}

// argvLimits validates and defaults the timeout and output cap.
func argvLimits(opts ArgvOpts) (time.Duration, int, error) {
	if len(opts.Argv) == 0 {
		return 0, 0, errors.New("argv run: empty argv")
	}
	timeout := opts.Timeout
	switch {
	case timeout == 0:
		timeout = defaultArgvTimeout
	case timeout < 0:
		return 0, 0, fmt.Errorf("argv run: negative timeout %s", timeout)
	}
	maxBytes := opts.StdoutMax
	switch {
	case maxBytes == 0:
		maxBytes = defaultArgvStdoutMax
	case maxBytes < 0:
		return 0, 0, fmt.Errorf("argv run: negative stdout cap %d", maxBytes)
	}
	return timeout, maxBytes, nil
}

// argvCwd resolves Cwd against DefaultCwd per the ArgvOpts contract.
func argvCwd(opts ArgvOpts) string {
	switch {
	case opts.Cwd == "":
		return opts.DefaultCwd
	case !filepath.IsAbs(opts.Cwd) && opts.DefaultCwd != "":
		return filepath.Join(opts.DefaultCwd, opts.Cwd)
	default:
		return opts.Cwd
	}
}

// mergeExecEnv returns base merged with overrides. An empty override
// value unsets the matching key. Order: base first, then overrides.
func mergeExecEnv(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	byKey := make(map[string]int, len(base))
	for _, kv := range base {
		key := envVarKey(kv)
		if _, dup := byKey[key]; dup {
			continue
		}
		byKey[key] = len(out)
		out = append(out, kv)
	}
	for k, v := range overrides {
		idx, present := byKey[k]
		switch {
		case v == "" && present:
			// Mark for removal; compact once below so indexes stay valid.
			out[idx] = ""
		case v == "":
		case present:
			out[idx] = k + "=" + v
		default:
			byKey[k] = len(out)
			out = append(out, k+"="+v)
		}
	}
	compact := out[:0]
	for _, kv := range out {
		if kv != "" {
			compact = append(compact, kv)
		}
	}
	return compact
}

// envVarKey returns the KEY of a KEY=value entry.
func envVarKey(kv string) string {
	if i := strings.IndexByte(kv, '='); i >= 0 {
		return kv[:i]
	}
	return kv
}

// cappedBuffer buffers up to limit bytes and silently drops the rest,
// recording that truncation occurred. Write always reports the full
// length so the child never sees EPIPE or a short write.
type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	remaining := c.limit - c.buf.Len()
	switch {
	case remaining <= 0:
		c.truncated = true
	case len(p) <= remaining:
		_, _ = c.buf.Write(p)
	default:
		_, _ = c.buf.Write(p[:remaining])
		c.truncated = true
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string {
	return c.buf.String()
}

var _ io.Writer = (*cappedBuffer)(nil)
