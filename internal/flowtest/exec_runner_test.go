package flowtest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func newExecStep(id string, exec *core.ExecStep) core.Step {
	return core.Step{
		ID:    id,
		Type:  core.StepTypeExec,
		Title: id,
		Exec:  exec,
	}
}

func TestExecAgentRunnerCanHandle(t *testing.T) {
	r := NewExecAgentRunner("")
	if !r.CanHandle(core.Step{Type: core.StepTypeExec}) {
		t.Error("CanHandle(exec) = false, want true")
	}
	if r.CanHandle(core.Step{Type: core.StepTypeTask}) {
		t.Error("CanHandle(task) = true, want false")
	}
}

// TestExecAgentRunnerHappyPath covers (a) exit 0 + non-empty stdout.
func TestExecAgentRunnerHappyPath(t *testing.T) {
	r := NewExecAgentRunner("")
	step := newExecStep("happy", &core.ExecStep{Argv: []string{"sh", "-c", "echo hello"}})

	out, err := r.Run(context.Background(), step, "")
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	assertExit(t, out, 0)
	if got := out["stdout"].(string); got != "hello\n" {
		t.Errorf("stdout = %q, want %q", got, "hello\n")
	}
	if out["truncated"].(bool) {
		t.Error("truncated = true, want false")
	}
	if dur, _ := out["duration_ms"].(int64); dur < 0 {
		t.Errorf("duration_ms negative: %d", dur)
	}
}

// TestExecAgentRunnerNonzeroExitFails covers (b) without allow_nonzero_exit.
func TestExecAgentRunnerNonzeroExitFails(t *testing.T) {
	r := NewExecAgentRunner("")
	step := newExecStep("fail", &core.ExecStep{Argv: []string{"sh", "-c", "exit 7"}})

	out, err := r.Run(context.Background(), step, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if out == nil {
		t.Fatal("expected output map even on failure, got nil")
	}
	assertExit(t, out, 7)
}

// TestExecAgentRunnerNonzeroExitAllowed covers (b) with allow_nonzero_exit=true.
func TestExecAgentRunnerNonzeroExitAllowed(t *testing.T) {
	r := NewExecAgentRunner("")
	step := newExecStep("ok-fail", &core.ExecStep{
		Argv:             []string{"sh", "-c", "exit 7"},
		AllowNonzeroExit: true,
	})

	out, err := r.Run(context.Background(), step, "")
	if err != nil {
		t.Fatalf("Run err = %v (allow_nonzero_exit should swallow it)", err)
	}
	assertExit(t, out, 7)
}

// TestExecAgentRunnerTimeout covers (c) timeout behavior.
func TestExecAgentRunnerTimeout(t *testing.T) {
	r := NewExecAgentRunner("")
	step := newExecStep("slow", &core.ExecStep{
		Argv:       []string{"sh", "-c", "sleep 5"},
		TimeoutSec: 1,
	})

	t0 := time.Now()
	_, err := r.Run(context.Background(), step, "")
	elapsed := time.Since(t0)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want 'timed out' substring", err)
	}
	if elapsed > 4*time.Second {
		t.Errorf("timeout did not interrupt promptly: %v", elapsed)
	}
}

// TestExecAgentRunnerStdoutTruncation covers (d) stdout truncation.
func TestExecAgentRunnerStdoutTruncation(t *testing.T) {
	r := NewExecAgentRunner("")
	// Print 4 KiB; cap at 1 KiB.
	step := newExecStep("big", &core.ExecStep{
		Argv:           []string{"sh", "-c", "head -c 4096 /dev/zero | tr '\\0' 'x'"},
		StdoutMaxBytes: 1024,
	})

	out, err := r.Run(context.Background(), step, "")
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	gotStdout := out["stdout"].(string)
	if len(gotStdout) != 1024 {
		t.Errorf("stdout len = %d, want 1024", len(gotStdout))
	}
	if !out["truncated"].(bool) {
		t.Error("truncated = false, want true")
	}
}

// TestExecAgentRunnerCwdRespected covers (e) cwd is honored.
func TestExecAgentRunnerCwdRespected(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")
	if err := os.WriteFile(marker, []byte("here"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// Step cwd absolute.
	r := NewExecAgentRunner("/tmp")
	step := newExecStep("cwd", &core.ExecStep{
		Argv: []string{"sh", "-c", "cat marker.txt"},
		Cwd:  dir,
	})
	out, err := r.Run(context.Background(), step, "")
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got := out["stdout"].(string); got != "here" {
		t.Errorf("stdout = %q, want 'here' (cwd not respected)", got)
	}

	// Default cwd applied when step.Cwd empty.
	r2 := NewExecAgentRunner(dir)
	step2 := newExecStep("dflt-cwd", &core.ExecStep{
		Argv: []string{"sh", "-c", "cat marker.txt"},
	})
	out2, err := r2.Run(context.Background(), step2, "")
	if err != nil {
		t.Fatalf("Run (default cwd) err = %v", err)
	}
	if got := out2["stdout"].(string); got != "here" {
		t.Errorf("default cwd stdout = %q, want 'here'", got)
	}
}

// TestExecAgentRunnerEnvPropagated covers (f) env propagation.
func TestExecAgentRunnerEnvPropagated(t *testing.T) {
	r := NewExecAgentRunner("")
	step := newExecStep("env", &core.ExecStep{
		Argv: []string{"sh", "-c", "echo $TLC_EXEC_TEST_VAR"},
		Env:  map[string]string{"TLC_EXEC_TEST_VAR": "propagated"},
	})

	out, err := r.Run(context.Background(), step, "")
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got := strings.TrimSpace(out["stdout"].(string)); got != "propagated" {
		t.Errorf("stdout = %q, want 'propagated'", got)
	}
}

// TestExecAgentRunnerEnvUnset verifies that empty value unsets a parent env var.
func TestExecAgentRunnerEnvUnset(t *testing.T) {
	t.Setenv("TLC_EXEC_UNSET_ME", "should-be-gone")

	r := NewExecAgentRunner("")
	step := newExecStep("unset", &core.ExecStep{
		Argv: []string{"sh", "-c", "echo before=${TLC_EXEC_UNSET_ME-MISSING}"},
		Env:  map[string]string{"TLC_EXEC_UNSET_ME": ""},
	})

	out, err := r.Run(context.Background(), step, "")
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if got := strings.TrimSpace(out["stdout"].(string)); got != "before=MISSING" {
		t.Errorf("stdout = %q, want 'before=MISSING'", got)
	}
}

// TestExecAgentRunnerMissingExecConfig verifies a step without exec.argv is rejected.
func TestExecAgentRunnerMissingExecConfig(t *testing.T) {
	r := NewExecAgentRunner("")

	_, err := r.Run(context.Background(), core.Step{ID: "x", Type: core.StepTypeExec}, "")
	if err == nil {
		t.Fatal("expected error for missing exec config")
	}

	_, err = r.Run(context.Background(), newExecStep("x", &core.ExecStep{}), "")
	if err == nil {
		t.Fatal("expected error for empty argv")
	}
}

// TestExecAgentRunnerBinaryNotFound verifies a missing binary surfaces as a
// runner error (not a non-zero exit).
func TestExecAgentRunnerBinaryNotFound(t *testing.T) {
	r := NewExecAgentRunner("")
	step := newExecStep("nope", &core.ExecStep{Argv: []string{"tlc-no-such-binary-xyzzy"}})

	_, err := r.Run(context.Background(), step, "")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

// assertExit checks the exit_code field in a step output map.
func assertExit(t *testing.T, out map[string]any, want int) {
	t.Helper()
	got, ok := out["exit_code"].(int)
	if !ok {
		t.Fatalf("exit_code not int: %T %v", out["exit_code"], out["exit_code"])
	}
	if got != want {
		t.Errorf("exit_code = %d, want %d", got, want)
	}
}
