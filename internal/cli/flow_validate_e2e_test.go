package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestFlowValidate_E2E_Valid verifies that a structurally sound flow
// exits 0 and prints "valid".
func TestFlowValidate_E2E_Valid(t *testing.T) {
	flowPath := findFixture(t, "validate/valid/flow.yaml")

	cmd := newTestCmd()
	cmd.AddCommand(FlowCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"flow", "validate", flowPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected exit 0 for valid flow; got error: %v\noutput:\n%s", err, buf.String())
	}

	if !contains(buf.String(), "valid") {
		t.Errorf("expected output to contain 'valid'; got:\n%s", buf.String())
	}
}

// TestFlowValidate_E2E_InvalidMissingDep verifies that a flow with a
// dangling depends_on exits 1 and reports the bad reference.
func TestFlowValidate_E2E_InvalidMissingDep(t *testing.T) {
	flowPath := findFixture(t, "validate/invalid-missing-dep/flow.yaml")

	cmd := newTestCmd()
	cmd.AddCommand(FlowCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"flow", "validate", flowPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected exit 1 for missing dep; got success\noutput:\n%s", buf.String())
	}

	if !contains(err.Error(), "non-existent step phantom") {
		t.Errorf("expected error to mention 'non-existent step phantom'; got: %v", err)
	}
}

// TestFlowValidate_E2E_InvalidCycle verifies that a flow with a
// dependency cycle exits 1 and reports the cycle.
func TestFlowValidate_E2E_InvalidCycle(t *testing.T) {
	flowPath := findFixture(t, "validate/invalid-cycle/flow.yaml")

	cmd := newTestCmd()
	cmd.AddCommand(FlowCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"flow", "validate", flowPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected exit 1 for cycle; got success\noutput:\n%s", buf.String())
	}

	if !contains(err.Error(), "cycle detected") {
		t.Errorf("expected error to mention 'cycle detected'; got: %v", err)
	}
}

// findFixture resolves a fixture path relative to examples/flows/fixtures/
// in the repo root.
func findFixture(t *testing.T, rel string) string {
	t.Helper()

	path := filepath.Join(testRepoRoot(t), "examples", "flows", "fixtures", rel)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture not found: %s", path)
	}
	return path
}
