package cli

// End-to-end tests for flow test-run with contracts (story 078, T-0628).
//
// Verifies that `flow invoke` works for flows designed for test-run
// scenarios, and that the contracts fixture directory structure is
// properly set up for `flow test` to consume.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestFlowTestRun_InvokesSuccessfully verifies that a flow intended for
// test-run scenarios can be invoked normally. The contracts directory
// exists alongside the fixture but does not block `flow invoke`.
func TestFlowTestRun_InvokesSuccessfully(t *testing.T) {
	withTestLock(func() {
		tmpDir, _ := setupProjectScopedTestDir(t, "tlc-flow-testrun-", "test/project")

		flowPath := filepath.Join(tmpDir, "testrun-flow.yaml")
		flowYAML := `flow_id: "flow:testrun:1.0"
name: "Test Run Flow"
version: "1.0"
entry_step: "validate"
steps:
  validate:
    step_id: "validate"
    type: "task"
    title: "Validate output"
    task_template:
      title: "Validate step output"
      description: "Run validation against step output."
    next: "report"
  report:
    step_id: "report"
    type: "task"
    title: "Generate report"
    task_template:
      title: "Generate test report"
      description: "Produce pass/fail report from validation."
`
		if err := os.WriteFile(flowPath, []byte(flowYAML), 0o644); err != nil {
			t.Fatalf("WriteFile flow yaml: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(FlowCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"flow", "invoke", flowPath})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("flow invoke for test-run flow: %v\noutput:\n%s",
				err, buf.String())
		}

		out := buf.String()
		if !contains(out, "invoked successfully") {
			t.Errorf("expected success message; got:\n%s", out)
		}
	})
}

// TestFlowTestRun_ContractFixtureStructure verifies the expected
// directory layout for `flow test` consumption: fixtures/<name>/
// happy-path/contracts/*.yaml exist on disk.
func TestFlowTestRun_ContractFixtureStructure(t *testing.T) {
	t.Parallel()

	fixtureBase := filepath.Join(
		"..", "..", "examples", "flows", "fixtures",
		"test-run", "happy-path",
	)

	// Contracts directory must exist.
	contractsDir := filepath.Join(fixtureBase, "contracts")
	info, err := os.Stat(contractsDir)
	if err != nil {
		t.Fatalf("contracts dir missing: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("contracts path is not a directory")
	}

	// At least one contract YAML must exist.
	entries, err := os.ReadDir(contractsDir)
	if err != nil {
		t.Fatalf("ReadDir contracts: %v", err)
	}
	var yamlCount int
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			yamlCount++
		}
	}
	if yamlCount == 0 {
		t.Error("no .yaml contract files in contracts directory")
	}

	// Record directory must exist (for cassettes).
	recordDir := filepath.Join(fixtureBase, "record")
	if _, err := os.Stat(recordDir); err != nil {
		t.Fatalf("record dir missing: %v", err)
	}
}
