package flowtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	xrr "hop.top/xrr"
)

// writeFakeBinary plants an executable script under the sandbox bin dir.
func writeFakeBinary(t *testing.T, sb *Sandbox, script string) {
	const name = "llm" // the adapter every sandbox agent test resolves to
	t.Helper()
	p := filepath.Join(sb.BinDir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
}

func newLLMExec(t *testing.T, run *Run, recipe *core.Recipe) (*Sandbox, *SandboxAgentExec) {
	t.Helper()
	sb, err := NewSandbox(findRepoRoot(t))
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	t.Cleanup(sb.Teardown)
	adapters := map[string]AgentAdapter{"claude": NewClaudeAdapter(), "llm": NewLLMAdapter()}
	return sb, NewSandboxAgentExec(sb, xrr.ModeReplay, run, NewAdapterResolver(adapters, recipe, nil))
}

// TestSandboxAgentExecDispatchesViaResolver verifies that Run() delegates
// binary selection to the resolver rather than hardcoding "claude".
func TestSandboxAgentExecDispatchesViaResolver(t *testing.T) {
	sb, x := newLLMExec(t, nil, &core.Recipe{Agent: "llm"})
	writeFakeBinary(t, sb, `echo '{"type":"result","adapter":"llm"}'`)

	res, err := x.Run(t.Context(), step("test-step", ""), "hello")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res.Status != core.AgentStatusSucceeded {
		t.Errorf("Status = %q, want succeeded", res.Status)
	}
	if res.Outputs["adapter"] != "llm" {
		t.Errorf("expected adapter=llm in outputs, got %v", res.Outputs)
	}
	if got, _ := res.Outputs[core.ResultKeyExitCode].(int); got != 0 {
		t.Errorf("outputs.exit_code = %v, want 0", res.Outputs[core.ResultKeyExitCode])
	}
	if res.Agent != "llm" || res.ExitCode != 0 || res.Summary == "" {
		t.Errorf("result metadata = %+v, want agent llm, exit 0 and a summary", res)
	}
}

// TestSandboxAgentExecRunsInsideTheSandbox verifies the step env, cwd and
// stdin the adapter binary sees.
func TestSandboxAgentExecRunsInsideTheSandbox(t *testing.T) {
	run := &Run{RecordDir: t.TempDir()}
	sb, x := newLLMExec(t, run, &core.Recipe{Agent: "llm"})
	writeFakeBinary(t, sb,
		`printf '{"step":"%s","mode":"%s","cassette":"%s","home":"%s","cwd":"%s","stdin":"%s"}' `+
			`"$TLC_FLOW_TEST_STEP" "$TLC_FLOW_TEST_MODE" "$TLC_FLOW_TEST_CASSETTE_DIR" "$HOME" "$(pwd -P)" "$(cat)"`)

	res, err := x.Run(t.Context(), step("plan", ""), "hello")
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	want := map[string]string{
		"step": "plan", "mode": "replay", "cassette": filepath.Join(run.RecordDir, "plan"),
		"home": sb.HomeDir, "stdin": "",
	}
	for k, v := range want {
		if res.Outputs[k] != v {
			t.Errorf("outputs[%s] = %v, want %q", k, res.Outputs[k], v)
		}
	}
	realRepo, _ := filepath.EvalSymlinks(sb.RepoDir)
	if got, _ := res.Outputs["cwd"].(string); got != realRepo {
		t.Errorf("cwd = %q, want the sandbox repo %q", got, realRepo)
	}
}

// TestSandboxAgentExecNonzeroExitFails maps a nonzero exit to a failed
// result that still carries the exit code and the raw output.
func TestSandboxAgentExecNonzeroExitFails(t *testing.T) {
	sb, x := newLLMExec(t, nil, &core.Recipe{Agent: "llm"})
	writeFakeBinary(t, sb, `echo 'tlc-flow-test: cassette miss for "llm"' >&2; exit 2`)

	res, err := x.Run(t.Context(), step("s", ""), "hello")
	if err != nil {
		t.Fatalf("Run() error: %v (a nonzero exit is a result, not an error)", err)
	}
	if res.Status != core.AgentStatusFailed || res.ExitCode != 2 {
		t.Errorf("result = %+v, want failed with exit 2", res)
	}
	if !strings.Contains(res.Summary, "exited 2") {
		t.Errorf("Summary = %q, want the exit code named", res.Summary)
	}
	if !IsCassetteMiss(res.Outputs) {
		t.Errorf("outputs %v should read as a cassette miss", res.Outputs)
	}
}

// TestSandboxAgentExecResolverFatalPropagates ensures a resolution failure
// surfaces as an error from Run(), not a panic.
func TestSandboxAgentExecResolverFatalPropagates(t *testing.T) {
	_, x := newLLMExec(t, nil, &core.Recipe{})
	if _, err := x.Run(t.Context(), step("no-agent-step", ""), "hello"); err == nil {
		t.Fatal("Run() expected error for unresolvable adapter, got nil")
	}
}

// TestSandboxAgentExecMissingBinaryIsAnError distinguishes a spawn failure
// from a run that exited nonzero.
func TestSandboxAgentExecMissingBinaryIsAnError(t *testing.T) {
	_, x := newLLMExec(t, nil, &core.Recipe{Agent: "llm"})
	if _, err := x.Run(t.Context(), step("s", ""), "hello"); err == nil {
		t.Fatal("Run() expected error when the adapter binary is absent from the sandbox")
	}
}

// TestSandboxAgentExecRecordModeCreatesCassetteDir pre-creates the per-step
// cassette dir so the shim can write into it.
func TestSandboxAgentExecRecordModeCreatesCassetteDir(t *testing.T) {
	run := &Run{RecordDir: filepath.Join(t.TempDir(), "record")}
	sb, err := NewSandbox(findRepoRoot(t))
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	t.Cleanup(sb.Teardown)
	writeFakeBinary(t, sb, `echo '{}'`)
	adapters := map[string]AgentAdapter{"llm": NewLLMAdapter()}
	x := NewSandboxAgentExec(sb, xrr.ModeRecord, run, NewAdapterResolver(adapters, &core.Recipe{Agent: "llm"}, nil))

	if _, err := x.Run(t.Context(), step("rec", ""), "hello"); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if st, err := os.Stat(filepath.Join(run.RecordDir, "rec")); err != nil || !st.IsDir() {
		t.Errorf("record dir for the step not created: %v", err)
	}
}

// findRepoRoot walks up from the test file to locate the module root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}
