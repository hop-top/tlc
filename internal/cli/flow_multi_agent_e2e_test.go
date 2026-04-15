package cli

import (
	"bytes"
	"testing"
)

// TestFlowMultiAgent_E2E_DryRunShowsAgents verifies that dry-run output
// displays the correct agent name for each step in a multi-agent flow.
func TestFlowMultiAgent_E2E_DryRunShowsAgents(t *testing.T) {
	flowPath := findFixture(t, "../multi-agent.yaml")

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

	// All 3 steps must appear.
	for _, id := range []string{"plan", "implement", "review"} {
		if !contains(out, id) {
			t.Errorf("expected step %q in output; got:\n%s", id, out)
		}
	}

	// Each agent must appear.
	for _, agent := range []string{"claude", "codex", "gemini"} {
		if !contains(out, "agent: "+agent) {
			t.Errorf("expected agent %q in output; got:\n%s", agent, out)
		}
	}

	// Verify step ordering: plan before implement before review.
	steps := []string{"plan", "implement", "review"}
	prev := -1
	for _, s := range steps {
		idx := indexOf(out, "["+s+"]")
		if idx < 0 {
			t.Errorf("step marker [%s] not found", s)
			continue
		}
		if idx <= prev {
			t.Errorf("step [%s] appeared before prior step in output", s)
		}
		prev = idx
	}
}
