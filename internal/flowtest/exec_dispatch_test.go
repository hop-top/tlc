package flowtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	xrr "hop.top/xrr"
)

func newTestSandbox(t *testing.T) *Sandbox {
	t.Helper()
	sb, err := NewSandbox(findRepoRoot(t))
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	t.Cleanup(sb.Teardown)
	return sb
}

func execTask(id string, spec *core.ExecSpec) *core.Task {
	return &core.Task{ID: "task_" + id, Title: id, StepID: id, Kind: core.TaskKindExec, Spec: &core.TaskSpec{Exec: spec}}
}

func shell(script string) *core.ExecSpec {
	return &core.ExecSpec{Argv: []string{"sh", "-c", script}}
}

// TestSandboxArgvRunnerMergesSandboxEnv: the sandbox step env reaches the
// command, the spec env is layered on top, and PATH leads with bin/.
func TestSandboxArgvRunnerMergesSandboxEnv(t *testing.T) {
	sb := newTestSandbox(t)
	run := &Run{RecordDir: t.TempDir(), Passthrough: []string{"docker"}}
	r := &SandboxArgvRunner{Sandbox: sb, Mode: xrr.ModeReplay, Fixture: run, StepID: "lint"}

	res, err := r.Run(t.Context(), core.ArgvOpts{
		Argv: []string{"sh", "-c", `echo "$TLC_FLOW_TEST_STEP|$TLC_FLOW_TEST_MODE|$TLC_FLOW_TEST_CASSETTE_DIR|$TLC_FLOW_TEST_PASSTHROUGH|$HOME|$SPEC_ONLY|$PATH"`},
		Env:  map[string]string{"SPEC_ONLY": "yes", "TLC_FLOW_TEST_PASSTHROUGH": "spec-wins"},
	})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	fields := strings.Split(strings.TrimSpace(res.Stdout), "|")
	if len(fields) != 7 {
		t.Fatalf("stdout = %q, want 7 fields", res.Stdout)
	}
	want := []string{"lint", "replay", filepath.Join(run.RecordDir, "lint"), "spec-wins", sb.HomeDir, "yes"}
	for i, w := range want {
		if fields[i] != w {
			t.Errorf("field %d = %q, want %q", i, fields[i], w)
		}
	}
	if !strings.HasPrefix(fields[6], sb.BinDir+string(os.PathListSeparator)) {
		t.Errorf("PATH = %q, want it to lead with the sandbox bin dir", fields[6])
	}
}

// TestSandboxArgvRunnerDefaultsCwdToRepo: a relative or empty cwd resolves
// under the sandbox repo, never the caller's working directory.
func TestSandboxArgvRunnerDefaultsCwdToRepo(t *testing.T) {
	sb := newTestSandbox(t)
	r := &SandboxArgvRunner{Sandbox: sb, Mode: xrr.ModeReplay, Fixture: &Run{RecordDir: t.TempDir()}, StepID: "s"}

	res, err := r.Run(t.Context(), core.ArgvOpts{Argv: []string{"sh", "-c", "pwd -P"}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	realRepo, _ := filepath.EvalSymlinks(sb.RepoDir)
	if got := strings.TrimSpace(res.Stdout); got != realRepo {
		t.Errorf("cwd = %q, want %q", got, realRepo)
	}
}

// TestSandboxExecDispatcherHappyPath dispatches an exec task through the
// sandbox and reports the exec result schema.
func TestSandboxExecDispatcherHappyPath(t *testing.T) {
	sb := newTestSandbox(t)
	d := NewSandboxExecDispatcher(sb, xrr.ModeReplay, &Run{RecordDir: t.TempDir()})

	res, err := d.Dispatch(t.Context(), execTask("hello", shell(`echo "hello from $TLC_FLOW_TEST_STEP"`)))
	if err != nil {
		t.Fatalf("Dispatch err = %v", err)
	}
	if res.Status != core.AgentStatusSucceeded {
		t.Errorf("Status = %q, want succeeded", res.Status)
	}
	if got, _ := res.Result["stdout"].(string); got != "hello from hello\n" {
		t.Errorf("stdout = %q, want the step id rendered from the sandbox env", got)
	}
	if got, _ := res.Result[core.ResultKeyExitCode].(int); got != 0 {
		t.Errorf("exit_code = %v, want 0", res.Result[core.ResultKeyExitCode])
	}
}

// TestSandboxExecDispatcherNonzeroExitFails keeps the exec dispatcher's
// verdict: a nonzero exit is a failed dispatch with the result kept.
func TestSandboxExecDispatcherNonzeroExitFails(t *testing.T) {
	sb := newTestSandbox(t)
	d := NewSandboxExecDispatcher(sb, xrr.ModeReplay, &Run{RecordDir: t.TempDir()})

	res, err := d.Dispatch(t.Context(), execTask("fail", shell(`echo 'tlc-flow-test: cassette miss for "gh"' >&2; exit 2`)))
	if err != nil {
		t.Fatalf("Dispatch err = %v", err)
	}
	if res.Status != core.AgentStatusFailed {
		t.Errorf("Status = %q, want failed", res.Status)
	}
	if got, _ := res.Result[core.ResultKeyExitCode].(int); got != 2 {
		t.Errorf("exit_code = %v, want 2", res.Result[core.ResultKeyExitCode])
	}
	if !IsCassetteMiss(res.Result) {
		t.Errorf("result %v should read as a cassette miss", res.Result)
	}
}

// TestSandboxExecDispatcherRecordModeCreatesCassetteDir pre-creates the
// per-step cassette dir so the shims can write into it.
func TestSandboxExecDispatcherRecordModeCreatesCassetteDir(t *testing.T) {
	sb := newTestSandbox(t)
	run := &Run{RecordDir: filepath.Join(t.TempDir(), "record")}
	d := NewSandboxExecDispatcher(sb, xrr.ModeRecord, run)

	if _, err := d.Dispatch(t.Context(), execTask("rec", shell("true"))); err != nil {
		t.Fatalf("Dispatch err = %v", err)
	}
	if st, err := os.Stat(filepath.Join(run.RecordDir, "rec")); err != nil || !st.IsDir() {
		t.Errorf("record dir for the step not created: %v", err)
	}
}

// TestSandboxExecDispatcherMissingArgv verifies a task without exec.argv is
// rejected as an error, not run.
func TestSandboxExecDispatcherMissingArgv(t *testing.T) {
	sb := newTestSandbox(t)
	d := NewSandboxExecDispatcher(sb, xrr.ModeReplay, &Run{RecordDir: t.TempDir()})

	if _, err := d.Dispatch(t.Context(), execTask("x", &core.ExecSpec{})); err == nil {
		t.Fatal("expected error for empty argv")
	}
}
