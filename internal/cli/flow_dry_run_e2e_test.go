package cli

import (
	"bytes"
	"testing"
)

// TestFlowDryRun_E2E_HappyPath verifies that --dry-run walks a 4-step
// flow in dependency order, printing each step, without dispatching agents.
func TestFlowDryRun_E2E_HappyPath(t *testing.T) {
	flowPath := findFixture(t, "dry-run/happy-path/flow.yaml")

	cmd := newTestCmd()
	cmd.AddCommand(FlowCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"flow", "run", "--dry-run", flowPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected exit 0; got error: %v\noutput:\n%s", err, buf.String())
	}

	out := buf.String()

	// All 4 steps must appear.
	for _, id := range []string{"gather", "design", "implement", "verify"} {
		if !contains(out, id) {
			t.Errorf("expected step %q in output; got:\n%s", id, out)
		}
	}

	// Verify ordering: gather before design before implement before verify.
	steps := []string{"gather", "design", "implement", "verify"}
	prev := -1
	for _, s := range steps {
		idx := indexOf(out, s)
		if idx < 0 {
			t.Errorf("step %q not found in output", s)
			continue
		}
		if idx <= prev {
			t.Errorf("step %q appeared before a prior step in output", s)
		}
		prev = idx
	}

	// Agent info present.
	if !contains(out, "agent: claude") {
		t.Errorf("expected flow-level agent 'claude'; got:\n%s", out)
	}
	if !contains(out, "agent: codex") {
		t.Errorf("expected step-level agent 'codex'; got:\n%s", out)
	}

	// Must declare no agents dispatched.
	if !contains(out, "No agents dispatched") {
		t.Errorf("expected dry-run notice; got:\n%s", out)
	}
}

// TestFlowDryRun_E2E_InvalidFlow verifies that --dry-run with an
// invalid flow exits 1.
func TestFlowDryRun_E2E_InvalidFlow(t *testing.T) {
	flowPath := findFixture(t, "validate/invalid-cycle/flow.yaml")

	cmd := newTestCmd()
	cmd.AddCommand(FlowCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"flow", "run", "--dry-run", flowPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected exit 1 for cycle; got success\noutput:\n%s", buf.String())
	}

	if !contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error; got: %v", err)
	}
}

// indexOf returns the byte offset of substr in s, or -1.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
