package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunArgv_HappyPath(t *testing.T) {
	res, err := RunArgv(context.Background(), ArgvOpts{Argv: []string{"sh", "-c", "echo hello"}})
	if err != nil {
		t.Fatalf("RunArgv err = %v", err)
	}
	if res.ExitCode != 0 || res.Stdout != "hello\n" || res.Truncated || res.TimedOut || res.DurationMs < 0 {
		t.Errorf("result = %+v; want exit 0, stdout hello, not truncated/timed out", res)
	}
}

// TestRunArgv_NonzeroExitIsResultNotError: the runner reports the exit
// code and leaves the success policy to the caller (the executor marks
// nonzero as failed).
func TestRunArgv_NonzeroExitIsResultNotError(t *testing.T) {
	res, err := RunArgv(context.Background(), ArgvOpts{Argv: []string{"sh", "-c", "exit 7"}})
	if err != nil {
		t.Fatalf("RunArgv err = %v; want nil with exit code in result", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("ExitCode = %d; want 7", res.ExitCode)
	}
}

// TestRunArgv_Timeout: the deadline kills the whole process group (the
// `sleep` grandchild included) so Wait returns promptly, and the partial
// result comes back alongside the sentinel.
func TestRunArgv_Timeout(t *testing.T) {
	t0 := time.Now()
	res, err := RunArgv(context.Background(), ArgvOpts{
		Argv:    []string{"sh", "-c", "echo partial; sleep 5"},
		Timeout: time.Second,
	})
	elapsed := time.Since(t0)

	if !errors.Is(err, ErrArgvTimeout) {
		t.Fatalf("err = %v; want ErrArgvTimeout", err)
	}
	if res == nil || !res.TimedOut {
		t.Fatalf("result = %+v; want partial result with TimedOut", res)
	}
	if res.Stdout != "partial\n" {
		t.Errorf("partial stdout = %q; want %q", res.Stdout, "partial\n")
	}
	if elapsed > 4*time.Second {
		t.Errorf("timeout did not interrupt promptly: %v", elapsed)
	}
}

func TestRunArgv_StdoutTruncation(t *testing.T) {
	res, err := RunArgv(context.Background(), ArgvOpts{
		Argv:      []string{"sh", "-c", "head -c 4096 /dev/zero | tr '\\0' 'x'"},
		StdoutMax: 1024,
	})
	if err != nil {
		t.Fatalf("RunArgv err = %v", err)
	}
	if len(res.Stdout) != 1024 || !res.Truncated {
		t.Errorf("stdout len = %d, truncated = %v; want 1024, true", len(res.Stdout), res.Truncated)
	}
}

func TestRunArgv_CwdResolution(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, p := range []string{filepath.Join(dir, "marker.txt"), filepath.Join(sub, "marker.txt")} {
		if err := os.WriteFile(p, []byte(filepath.Base(filepath.Dir(p))), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	cat := []string{"sh", "-c", "cat marker.txt"}

	cases := []struct {
		name string
		opts ArgvOpts
		want string
	}{
		{"absolute cwd", ArgvOpts{Argv: cat, Cwd: dir, DefaultCwd: "/tmp"}, filepath.Base(dir)},
		{"default cwd when empty", ArgvOpts{Argv: cat, DefaultCwd: dir}, filepath.Base(dir)},
		{"relative cwd joins default", ArgvOpts{Argv: cat, Cwd: "sub", DefaultCwd: dir}, "sub"},
	}
	for _, tc := range cases {
		res, err := RunArgv(context.Background(), tc.opts)
		if err != nil {
			t.Fatalf("%s: RunArgv err = %v", tc.name, err)
		}
		if res.Stdout != tc.want {
			t.Errorf("%s: stdout = %q; want %q", tc.name, res.Stdout, tc.want)
		}
	}
}

func TestRunArgv_Env(t *testing.T) {
	t.Setenv("TLC_EXEC_UNSET_ME", "should-be-gone")
	res, err := RunArgv(context.Background(), ArgvOpts{
		Argv: []string{"sh", "-c", "echo $TLC_EXEC_TEST_VAR ${TLC_EXEC_UNSET_ME-MISSING}"},
		Env:  map[string]string{"TLC_EXEC_TEST_VAR": "propagated", "TLC_EXEC_UNSET_ME": ""},
	})
	if err != nil {
		t.Fatalf("RunArgv err = %v", err)
	}
	if got := strings.TrimSpace(res.Stdout); got != "propagated MISSING" {
		t.Errorf("stdout = %q; want %q (override propagated, empty value unsets)", got, "propagated MISSING")
	}
}

func TestRunArgv_InvalidOpts(t *testing.T) {
	cases := map[string]ArgvOpts{
		"empty argv":         {},
		"negative timeout":   {Argv: []string{"true"}, Timeout: -time.Second},
		"negative stdoutMax": {Argv: []string{"true"}, StdoutMax: -1},
	}
	for name, opts := range cases {
		if _, err := RunArgv(context.Background(), opts); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// TestRunArgv_BinaryNotFound: a spawn failure is an error, not a nonzero
// exit — the caller must not treat "could not run" as "ran and failed".
func TestRunArgv_BinaryNotFound(t *testing.T) {
	res, err := RunArgv(context.Background(), ArgvOpts{Argv: []string{"tlc-no-such-binary-xyzzy"}})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if res != nil {
		t.Errorf("result = %+v; want nil on spawn failure", res)
	}
}

// TestArgvResult_Map pins the output schema exec-kind tasks store as their
// result — the keys `when:` expressions and eva gates read.
func TestArgvResult_Map(t *testing.T) {
	m := (&ArgvResult{ExitCode: 3, Stdout: "o", Stderr: "e", DurationMs: 12, Truncated: true}).Map()
	if m["exit_code"] != 3 || m["stdout"] != "o" || m["stderr"] != "e" || m["duration_ms"] != int64(12) || m["truncated"] != true {
		t.Errorf("Map() = %v", m)
	}
	if _, ok := m["exit_code"].(int); !ok {
		t.Errorf("exit_code is %T; want int", m["exit_code"])
	}
}
