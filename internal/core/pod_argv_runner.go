package core

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"
)

// podWorkDir is the in-pod root exec-kind tasks run under: where the
// workspace is copied and what a relative exec.cwd resolves against. It
// matches the /workspace convention agent images follow.
const podWorkDir = "/workspace"

// podCwdPrologue is the POSIX sh script that enters the working
// directory before exec'ing the literal argv, since the pod exec
// protocol carries no working directory of its own. argv arrives as
// positional parameters, so the shell never re-parses it.
const podCwdPrologue = `cd -- "$1" && shift && exec "$@"`

// PodArgvRunner runs each argv inside a fresh pod through PodShell:
// create (image, env, labels), copy the workspace to WorkDir, exec the
// argv, destroy. It reports the same ArgvResult as the host path, within
// what the pod protocol can express:
//
//   - exec.env is set at pod creation; an empty value cannot unset a
//     variable the image provides, so such keys are dropped.
//   - exec.cwd is honored through a `sh -c` prologue, so the image needs
//     a POSIX sh.
//   - exec.timeout ends the wait on the host and kills the local pod
//     client; the protocol cannot signal the remote command, which dies
//     with the pod when it is destroyed right after.
//   - exec.stdout_max is applied on the host after the transfer; the
//     protocol cannot cap output at the source, so the pod client still
//     buffers the whole output before it is cut.
//   - the working tree is copied, not mounted: files the command writes
//     stay in the pod and are gone once it is destroyed.
type PodArgvRunner struct {
	Shell *PodShell
	// Image is the container image every run starts from.
	Image string
	// Workspace is the host directory copied to WorkDir before the run;
	// empty skips the copy.
	Workspace string
	// WorkDir is the in-pod root: the copy target and what a relative
	// exec.cwd resolves against. Empty means podWorkDir. ArgvOpts.DefaultCwd
	// is a host path and is ignored here.
	WorkDir string
	// Labels are attached to every pod created.
	Labels map[string]string
}

// Run implements ArgvRunner. A pod that fails to tear down is reported
// in err alongside whatever the run itself produced.
func (r *PodArgvRunner) Run(ctx context.Context, opts ArgvOpts) (res *ArgvResult, err error) {
	timeout, maxBytes, err := argvLimits(opts)
	if err != nil {
		return nil, err
	}
	if r.Shell == nil || r.Image == "" {
		return nil, errors.New("pod argv run: a pod shell and an image are required")
	}

	info, err := r.Shell.Create(ctx, PodCreateOpts{Image: r.Image, EnvVars: podEnv(opts.Env), Labels: r.Labels})
	if err != nil {
		return nil, fmt.Errorf("pod argv run: %w", err)
	}
	// Teardown must outlive the run's deadline and the caller's cancel.
	defer func() {
		if derr := r.Shell.Destroy(context.WithoutCancel(ctx), info.Name); derr != nil {
			err = errors.Join(err, fmt.Errorf("pod argv run: %w", derr))
		}
	}()

	if r.Workspace != "" {
		if err := r.Shell.CopyTo(ctx, info.Name, r.Workspace, r.workDir()); err != nil {
			return nil, fmt.Errorf("pod argv run: copy workspace %s: %w", r.Workspace, err)
		}
	}

	argv := podArgv(podCwd(opts.Cwd, r.workDir()), opts.Argv)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t0 := time.Now()
	stdout, stderr, code, err := r.Shell.Exec(runCtx, info.Name, argv[0], argv[1:]...)
	res = &ArgvResult{DurationMs: time.Since(t0).Milliseconds()}
	res.Stdout, res.Stderr, res.Truncated = capOutput(stdout, stderr, maxBytes)
	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		res.TimedOut = true
		return res, fmt.Errorf("%w after %s", ErrArgvTimeout, timeout)
	case runCtx.Err() != nil:
		return nil, fmt.Errorf("pod argv run %q: %w", opts.Argv[0], runCtx.Err())
	case err != nil:
		return nil, fmt.Errorf("pod argv run %q: %w", opts.Argv[0], err)
	}
	res.ExitCode = code
	return res, nil
}

func (r *PodArgvRunner) workDir() string {
	if r.WorkDir != "" {
		return r.WorkDir
	}
	return podWorkDir
}

// podEnv keeps the overrides pod create can express: an empty value
// would set the variable to "" rather than unset it, so it is dropped.
func podEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// podCwd resolves an exec.cwd against the in-pod root.
func podCwd(cwd, workDir string) string {
	switch {
	case cwd == "":
		return workDir
	case path.IsAbs(cwd):
		return cwd
	default:
		return path.Join(workDir, cwd)
	}
}

// podArgv wraps argv in the cwd prologue: sh -c <prologue> tlc-exec
// <cwd> argv..., where "tlc-exec" is the $0 sh reports errors under.
func podArgv(cwd string, argv []string) []string {
	out := make([]string, 0, len(argv)+5)
	out = append(out, "sh", "-c", podCwdPrologue, "tlc-exec", cwd)
	return append(out, argv...)
}

// capOutput cuts stdout and stderr to limit bytes each and reports
// whether anything was dropped.
func capOutput(stdout, stderr string, limit int) (string, string, bool) {
	truncated := false
	if len(stdout) > limit {
		stdout, truncated = stdout[:limit], true
	}
	if len(stderr) > limit {
		stderr, truncated = stderr[:limit], true
	}
	return stdout, stderr, truncated
}

var _ ArgvRunner = (*PodArgvRunner)(nil)
