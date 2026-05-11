package cli

// Regression coverage for T-0948: `tlc flow run --agent-local` must
// actually dispatch the agent declared on each step. Before the fix,
// buildFlowAgentRunner returned nil whenever --agent was unset, so the
// flow executor fell back to executeTemplateStep's DB-only path and
// the step completed as a no-op (no agent invocation, no results).
//
// These tests exercise the end-to-end CLI path with a shell-script
// "agent" binary that records every invocation to a sentinel file.

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeFlowAgentLocalFixture seeds tmpDir with:
//   - a sentinel-recording agent script at .tlc/fake-agent.sh
//   - .tlc/agents.yaml pointing agent "claude" at that script
//   - a flow YAML at flow.yaml that declares agent: claude on its
//     single template step
//
// Returns (flowPath, sentinelPath). The sentinel file is touched by
// the script on every invocation; absence after a run proves the
// agent was never dispatched.
func writeFlowAgentLocalFixture(t *testing.T, tmpDir, status string, exitCode int) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script agent fixture is POSIX-only")
	}

	sentinel := filepath.Join(tmpDir, "agent-invoked.log")
	scriptPath := filepath.Join(tmpDir, ".tlc", "fake-agent.sh")

	// Status JSON line is what ResultCollector parses from stdout.
	script := "#!/bin/sh\n" +
		"echo invoked >> '" + sentinel + "'\n" +
		"echo '{\"version\":1,\"status\":\"" + status + "\",\"summary\":\"agent ran\",\"exit_code\":" +
		itoa(exitCode) + "}'\n" +
		"exit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake agent: %v", err)
	}

	agentsYAML := "agents:\n  claude:\n    binary: " + scriptPath + "\n"
	if err := os.WriteFile(
		filepath.Join(tmpDir, ".tlc", "agents.yaml"),
		[]byte(agentsYAML), 0o600,
	); err != nil {
		t.Fatalf("write agents.yaml: %v", err)
	}

	flowPath := filepath.Join(tmpDir, "flow.yaml")
	flowYAML := `flow_id: "flow:agent-local-test:1.0"
name: "Agent Local Regression"
version: "1.0"
entry_step: "do"
steps:
  do:
    step_id: "do"
    type: "task"
    title: "Run agent locally"
    agent: "claude"
    task_template:
      title: "Template step"
      description: "Should dispatch the local agent."
`
	if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
		t.Fatalf("write flow.yaml: %v", err)
	}
	return flowPath, sentinel
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// TestFlowRun_AgentLocal_DispatchesAgent is the primary regression for
// T-0948. With --agent-local (and no --agent), a flow step whose YAML
// declares `agent: claude` must actually run the configured binary
// rather than no-op via the DB-only completion path.
func TestFlowRun_AgentLocal_DispatchesAgent(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-agent-local-", "test/agent-local")
		flowPath, sentinel := writeFlowAgentLocalFixture(t, tmpDir, "succeeded", 0)

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"flow", "run", flowPath,
			"--agent-local",
			"--trust-project",
		})

		t.Cleanup(func() {
			flowRunAgent = ""
			flowRunAgentLocal = false
			flowRunTrustProject = false
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow run --agent-local: unexpected error: %v\noutput:\n%s",
				err, buf.String())
		}

		// The sentinel proves the agent binary was actually executed.
		// Before the T-0948 fix the runner was nil, so the script
		// never ran and this file would not exist.
		if _, err := os.Stat(sentinel); err != nil {
			t.Fatalf("expected agent sentinel %s to exist; got: %v\nflow output:\n%s",
				sentinel, err, buf.String())
		}

		if !contains(buf.String(), "succeeded") {
			t.Errorf("expected flow status succeeded; output:\n%s", buf.String())
		}
	})
}

// TestFlowRun_AgentLocal_FailurePropagates ensures step completion
// is gated on agent success, not on --agent-local alone. An agent
// that exits with status=failed must cause the flow run to error.
func TestFlowRun_AgentLocal_FailurePropagates(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-agent-local-fail-", "test/agent-local-fail")
		flowPath, sentinel := writeFlowAgentLocalFixture(t, tmpDir, "failed", 1)

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"flow", "run", flowPath,
			"--agent-local",
			"--trust-project",
		})

		t.Cleanup(func() {
			flowRunAgent = ""
			flowRunAgentLocal = false
			flowRunTrustProject = false
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error when agent fails; got success\noutput:\n%s",
				buf.String())
		}

		// Agent must still have been dispatched.
		if _, statErr := os.Stat(sentinel); statErr != nil {
			t.Errorf("expected agent sentinel %s to exist even on failure; got: %v",
				sentinel, statErr)
		}

		if !contains(err.Error(), "agent claude failed") {
			t.Errorf("expected error to mention agent failure; got: %v", err)
		}
	})
}
