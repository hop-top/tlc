package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const podCreatedJSON = `{"name":"pod-1","id":"abc","status":"running","provider":"docker"}`

// podRunResults is the canned pod CLI conversation of one full run:
// create, cp, exec, destroy.
func podRunResults(exec mockResult) []mockResult {
	return []mockResult{{Stdout: podCreatedJSON}, {}, exec, {}}
}

func podArgsAt(t *testing.T, r *mockRunner, i int) []string {
	t.Helper()
	if i >= len(r.calls) {
		t.Fatalf("pod call %d missing; calls = %v", i, r.calls)
	}
	return r.calls[i].Args
}

// stallingRunner answers like mockRunner except that the exec call parks
// until its context ends, the way a hung remote command parks the pod
// client; it also records whether destroy ran under a live context.
type stallingRunner struct {
	mockRunner
	destroyed     bool
	destroyCtxErr error
}

func (s *stallingRunner) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	switch {
	case len(args) > 0 && args[0] == "exec":
		s.calls = append(s.calls, mockCall{Name: name, Args: args})
		<-ctx.Done()
		return "partial", "", -1, nil
	case len(args) > 0 && args[0] == "destroy":
		s.destroyed = true
		s.destroyCtxErr = ctx.Err()
	}
	return s.mockRunner.Run(ctx, name, args...)
}

// assertArgsContain checks the i-th pod call's argv contains every want.
func assertArgsContain(t *testing.T, r *mockRunner, i int, wants ...string) {
	t.Helper()
	got := strings.Join(podArgsAt(t, r, i), " ")
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("pod call %d %q lacks %q", i, got, want)
		}
	}
}

// assertArgsEqual checks the i-th pod call's argv joined by spaces.
func assertArgsEqual(t *testing.T, r *mockRunner, i int, want string) {
	t.Helper()
	if got := strings.Join(podArgsAt(t, r, i), " "); got != want {
		t.Errorf("pod call %d = %q; want %q", i, got, want)
	}
}

// TestPodArgvRunner_RunSequence pins the protocol conversation of one
// run: create with the image, env and labels; cp of the workspace to the
// in-pod root; exec; destroy.
func TestPodArgvRunner_RunSequence(t *testing.T) {
	r := &mockRunner{results: podRunResults(mockResult{Stdout: "hello\n"})}
	runner := &PodArgvRunner{
		Shell:     NewPodShell(r),
		Image:     "ghcr.io/example/tools:1",
		Workspace: "/home/me/repo",
		Labels:    map[string]string{"tlc.kind": "exec"},
	}
	res, err := runner.Run(context.Background(), ArgvOpts{
		Argv: []string{"echo", "hello"},
		Env:  map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.ExitCode != 0 || res.Stdout != "hello\n" || res.Truncated || res.TimedOut || res.DurationMs < 0 {
		t.Errorf("result = %+v; want exit 0, stdout hello, not truncated/timed out", res)
	}
	if len(r.calls) != 4 {
		t.Fatalf("calls = %v; want create, cp, exec, destroy", r.calls)
	}
	assertArgsContain(t, r, 0, "create --format json", "--image ghcr.io/example/tools:1", "--env FOO=bar", "--label tlc.kind=exec")
	assertArgsEqual(t, r, 1, "cp /home/me/repo pod-1:/workspace")
	assertArgsContain(t, r, 2, "exec pod-1 -- sh -c")
	assertArgsEqual(t, r, 3, "destroy pod-1 --yes")
	if !runner.Shell.Destroyed() {
		t.Error("pod not destroyed after the run")
	}
}

// TestPodArgvRunner_ExecArgvShape: the literal argv rides behind a sh
// prologue that only enters the cwd — argv is never re-parsed.
func TestPodArgvRunner_ExecArgvShape(t *testing.T) {
	r := &mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {}, {}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img"}
	if _, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"echo", "a b"}, Cwd: "sub"}); err != nil {
		t.Fatalf("Run err = %v", err)
	}
	exec := podArgsAt(t, r, 1)
	want := []string{"exec", "pod-1", "--", "sh", "-c", exec[5], "tlc-exec", "/workspace/sub", "echo", "a b"}
	if strings.Join(exec, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("exec args = %q; want %q", exec, want)
	}
	for _, fragment := range []string{`cd -- "$1"`, "shift", `exec "$@"`} {
		if !strings.Contains(exec[5], fragment) {
			t.Errorf("prologue %q lacks %q", exec[5], fragment)
		}
	}
}

// TestPodArgvRunner_DestroyFailureIsReported: a pod that will not tear
// down is not leaked silently; the run's own outcome stays attached.
func TestPodArgvRunner_DestroyFailureIsReported(t *testing.T) {
	r := &mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {Stdout: "done"}, {Stderr: "busy", ExitCode: 1}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img"}
	res, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"true"}})
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("err = %v; want the destroy failure", err)
	}
	if res == nil || res.Stdout != "done" {
		t.Errorf("result = %+v; want the run's output kept", res)
	}
}

// TestPodArgvRunner_CwdResolvesInPod: exec.cwd resolves against the
// in-pod root, never against the host DefaultCwd.
func TestPodArgvRunner_CwdResolvesInPod(t *testing.T) {
	cases := []struct {
		name, workDir, cwd, defaultCwd, want string
	}{
		{"empty is the root", "", "", "", "/workspace"},
		{"relative joins the root", "", "pkg/x", "", "/workspace/pkg/x"},
		{"absolute wins", "", "/opt/app", "", "/opt/app"},
		{"custom root", "/src", "sub", "", "/src/sub"},
		{"host default cwd is ignored", "", "", "/home/me/repo", "/workspace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {}, {}}}
			runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img", WorkDir: tc.workDir}
			if _, err := runner.Run(context.Background(), ArgvOpts{
				Argv: []string{"true"}, Cwd: tc.cwd, DefaultCwd: tc.defaultCwd,
			}); err != nil {
				t.Fatalf("Run err = %v", err)
			}
			exec := podArgsAt(t, r, 1)
			if got := exec[len(exec)-2]; got != tc.want {
				t.Errorf("in-pod cwd = %q; want %q", got, tc.want)
			}
		})
	}
}

// TestPodArgvRunner_CopyTargetIsWorkDir: the workspace lands on the
// same root relative cwds resolve against.
func TestPodArgvRunner_CopyTargetIsWorkDir(t *testing.T) {
	r := &mockRunner{results: podRunResults(mockResult{})}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img", Workspace: "/repo", WorkDir: "/src"}
	if _, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"true"}}); err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got := strings.Join(podArgsAt(t, r, 1), " "); got != "cp /repo pod-1:/src" {
		t.Errorf("cp args = %q", got)
	}
}

// TestPodArgvRunner_EnvAtCreateDropsEmpty: env travels on pod create; an
// empty value means "unset" on the host but the protocol cannot unset an
// image variable, so the key is dropped rather than set to "".
func TestPodArgvRunner_EnvAtCreateDropsEmpty(t *testing.T) {
	r := &mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {}, {}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img"}
	if _, err := runner.Run(context.Background(), ArgvOpts{
		Argv: []string{"true"}, Env: map[string]string{"A": "1", "B": ""},
	}); err != nil {
		t.Fatalf("Run err = %v", err)
	}
	create := strings.Join(podArgsAt(t, r, 0), " ")
	if !strings.Contains(create, "--env A=1") {
		t.Errorf("create args %q lack --env A=1", create)
	}
	if strings.Contains(create, "--env B=") {
		t.Errorf("create args %q set the empty B", create)
	}
}

func TestPodArgvRunner_NonzeroExitIsResultNotError(t *testing.T) {
	r := &mockRunner{results: podRunResults(mockResult{Stderr: "boom", ExitCode: 7})}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img", Workspace: "/repo"}
	res, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"false"}})
	if err != nil {
		t.Fatalf("Run err = %v; want nil with the exit code in the result", err)
	}
	if res.ExitCode != 7 || res.Stderr != "boom" {
		t.Errorf("result = %+v; want exit 7, stderr boom", res)
	}
}

// TestPodArgvRunner_OutputCapIsClientSide: the protocol returns the whole
// output; the cap is applied afterwards on the host and reported.
func TestPodArgvRunner_OutputCapIsClientSide(t *testing.T) {
	r := &mockRunner{results: podRunResults(mockResult{
		Stdout: strings.Repeat("x", 4096), Stderr: strings.Repeat("y", 2048),
	})}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img", Workspace: "/repo"}
	res, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"yes"}, StdoutMax: 1024})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if len(res.Stdout) != 1024 || len(res.Stderr) != 1024 || !res.Truncated {
		t.Errorf("stdout %d, stderr %d, truncated %v; want 1024, 1024, true", len(res.Stdout), len(res.Stderr), res.Truncated)
	}
}

func TestPodArgvRunner_UnderCapIsNotTruncated(t *testing.T) {
	r := &mockRunner{results: podRunResults(mockResult{Stdout: "short"})}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img", Workspace: "/repo"}
	res, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"true"}, StdoutMax: 1024})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.Stdout != "short" || res.Truncated {
		t.Errorf("result = %+v; want untouched output", res)
	}
}

// TestPodArgvRunner_TimeoutKillsClientAndStillDestroys: the deadline
// kills the local pod client (the protocol cannot signal the remote
// command); the partial output comes back with the sentinel and the pod
// is still torn down, under a context the deadline did not end.
func TestPodArgvRunner_TimeoutKillsClientAndStillDestroys(t *testing.T) {
	r := &stallingRunner{mockRunner: mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {}}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img"}
	res, err := runner.Run(context.Background(), ArgvOpts{
		Argv: []string{"sleep", "60"}, Timeout: 20 * time.Millisecond,
	})
	if !errors.Is(err, ErrArgvTimeout) {
		t.Fatalf("err = %v; want ErrArgvTimeout", err)
	}
	if res == nil || !res.TimedOut || res.Stdout != "partial" {
		t.Fatalf("result = %+v; want partial result with TimedOut", res)
	}
	if !r.destroyed {
		t.Fatal("pod not destroyed after the timeout")
	}
	if r.destroyCtxErr != nil {
		t.Errorf("destroy ran under an ended context: %v", r.destroyCtxErr)
	}
}

func TestPodArgvRunner_CanceledContextIsAnError(t *testing.T) {
	r := &stallingRunner{mockRunner: mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {}}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := runner.Run(ctx, ArgvOpts{Argv: []string{"sleep", "60"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v; want context.Canceled", err)
	}
	if res != nil {
		t.Errorf("result = %+v; want nil on cancellation", res)
	}
	if !r.destroyed || r.destroyCtxErr != nil {
		t.Errorf("destroyed = %v under ctx err %v; want teardown under a live context", r.destroyed, r.destroyCtxErr)
	}
}

func TestPodArgvRunner_NoWorkspaceSkipsCopy(t *testing.T) {
	r := &mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {}, {}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img"}
	if _, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"true"}}); err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if len(r.calls) != 3 || r.calls[1].Args[0] != "exec" {
		t.Errorf("calls = %v; want create, exec, destroy", r.calls)
	}
}

func TestPodArgvRunner_CreateFailureIsError(t *testing.T) {
	r := &mockRunner{results: []mockResult{{Stderr: "no such image", ExitCode: 1}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img"}
	res, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"true"}})
	if err == nil || !strings.Contains(err.Error(), "no such image") {
		t.Fatalf("err = %v; want the pod create failure", err)
	}
	if res != nil || len(r.calls) != 1 {
		t.Errorf("result = %+v, calls = %v; want nil result and no further calls", res, r.calls)
	}
}

func TestPodArgvRunner_CopyFailureDestroys(t *testing.T) {
	r := &mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {Stderr: "cp: denied", ExitCode: 1}, {}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img", Workspace: "/repo"}
	res, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"true"}})
	if err == nil || !strings.Contains(err.Error(), "cp: denied") {
		t.Fatalf("err = %v; want the pod cp failure", err)
	}
	if res != nil {
		t.Errorf("result = %+v; want nil", res)
	}
	if len(r.calls) != 3 || r.calls[2].Args[0] != "destroy" {
		t.Errorf("calls = %v; want create, cp, destroy", r.calls)
	}
}

func TestPodArgvRunner_ExecTransportFailureIsError(t *testing.T) {
	r := &mockRunner{results: []mockResult{{Stdout: podCreatedJSON}, {Err: errors.New("pod: connection reset")}, {}}}
	runner := &PodArgvRunner{Shell: NewPodShell(r), Image: "img"}
	res, err := runner.Run(context.Background(), ArgvOpts{Argv: []string{"true"}})
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("err = %v; want the transport failure", err)
	}
	if res != nil || !runner.Shell.Destroyed() {
		t.Errorf("result = %+v, destroyed = %v; want nil and destroyed", res, runner.Shell.Destroyed())
	}
}

func TestPodArgvRunner_ValidatesBeforeTouchingThePod(t *testing.T) {
	cases := []struct {
		name   string
		runner *PodArgvRunner
		opts   ArgvOpts
		want   string
	}{
		{"empty argv", &PodArgvRunner{Image: "img"}, ArgvOpts{}, "empty argv"},
		{"negative timeout", &PodArgvRunner{Image: "img"}, ArgvOpts{Argv: []string{"true"}, Timeout: -1}, "negative timeout"},
		{"no image", &PodArgvRunner{}, ArgvOpts{Argv: []string{"true"}}, "image"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &mockRunner{}
			tc.runner.Shell = NewPodShell(r)
			res, err := tc.runner.Run(context.Background(), tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v; want %q", err, tc.want)
			}
			if res != nil || len(r.calls) != 0 {
				t.Errorf("result = %+v, calls = %v; want nothing run", res, r.calls)
			}
		})
	}
}
