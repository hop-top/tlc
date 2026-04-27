package core

import (
	"strings"
	"sync"
	"testing"
)

// TestAggregateJoinOutput_Empty covers a join with zero predecessors:
// summary is empty string, steps map is empty (not nil).
func TestAggregateJoinOutput_Empty(t *testing.T) {
	step := Step{ID: "j", Type: StepTypeJoin}
	statuses := map[string]StepStatus{}
	stepOutputs := map[string]map[string]any{}

	out := aggregateJoinOutput(step, statuses, stepOutputs, &sync.Mutex{})

	if out["summary"] != "" {
		t.Errorf("expected empty summary, got %q", out["summary"])
	}
	steps, ok := out["steps"].(map[string]any)
	if !ok {
		t.Fatalf("expected steps map, got %T", out["steps"])
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(steps))
	}
}

// TestAggregateJoinOutput_SinglePredecessor verifies summary line + state
// uppercasing for one wait_for entry.
func TestAggregateJoinOutput_SinglePredecessor(t *testing.T) {
	step := Step{ID: "j", Type: StepTypeJoin, WaitFor: []string{"lint"}}
	statuses := map[string]StepStatus{"lint": StepStatusSucceeded}
	stepOutputs := map[string]map[string]any{
		"lint": {"warnings": 0},
	}

	out := aggregateJoinOutput(step, statuses, stepOutputs, &sync.Mutex{})

	if out["summary"] != "lint: SUCCEEDED" {
		t.Errorf("expected 'lint: SUCCEEDED', got %q", out["summary"])
	}
	steps := out["steps"].(map[string]any)
	lint := steps["lint"].(map[string]any)
	if lint["state"] != "SUCCEEDED" {
		t.Errorf("expected state SUCCEEDED, got %v", lint["state"])
	}
	if lint["output"] == nil {
		t.Error("expected output preserved, got nil")
	}
}

// TestAggregateJoinOutput_MultiPredecessor covers a join with 3+ predecessors
// across mixed states (SUCCEEDED, FAILED, SKIPPED). Order in summary follows
// the declared WaitFor list (deterministic for test assertions).
func TestAggregateJoinOutput_MultiPredecessor(t *testing.T) {
	step := Step{
		ID:      "j",
		Type:    StepTypeJoin,
		WaitFor: []string{"lint", "test", "security-scan"},
	}
	statuses := map[string]StepStatus{
		"lint":          StepStatusSucceeded,
		"test":          StepStatusFailed,
		"security-scan": StepStatusSkipped,
	}
	stepOutputs := map[string]map[string]any{
		"lint": {"ok": true},
		"test": {"failures": 3},
	}

	out := aggregateJoinOutput(step, statuses, stepOutputs, &sync.Mutex{})

	want := "lint: SUCCEEDED\ntest: FAILED\nsecurity-scan: SKIPPED"
	if out["summary"] != want {
		t.Errorf("summary mismatch:\n got %q\nwant %q", out["summary"], want)
	}

	steps := out["steps"].(map[string]any)
	if len(steps) != 3 {
		t.Errorf("expected 3 steps in map, got %d", len(steps))
	}

	// security-scan had no output → output should be nil (preserved as key).
	scan := steps["security-scan"].(map[string]any)
	if scan["output"] != nil {
		t.Errorf("expected nil output for security-scan, got %v", scan["output"])
	}
}

// TestAggregateJoinOutput_FallbackDependsOn confirms that when WaitFor is
// empty the function falls back to DependsOn for predecessor identification.
func TestAggregateJoinOutput_FallbackDependsOn(t *testing.T) {
	step := Step{
		ID:        "j",
		Type:      StepTypeJoin,
		DependsOn: []string{"a", "b"},
	}
	statuses := map[string]StepStatus{
		"a": StepStatusSucceeded,
		"b": StepStatusSucceeded,
	}
	stepOutputs := map[string]map[string]any{}

	out := aggregateJoinOutput(step, statuses, stepOutputs, &sync.Mutex{})

	want := "a: SUCCEEDED\nb: SUCCEEDED"
	if out["summary"] != want {
		t.Errorf("summary mismatch:\n got %q\nwant %q", out["summary"], want)
	}
}

// TestAggregateJoinOutput_MissingState renders a missing predecessor as
// PENDING (defensive — should never happen in real flow execution because
// the scheduler waits for terminal state, but covers the edge).
func TestAggregateJoinOutput_MissingState(t *testing.T) {
	step := Step{ID: "j", Type: StepTypeJoin, WaitFor: []string{"ghost"}}
	out := aggregateJoinOutput(step, map[string]StepStatus{}, map[string]map[string]any{}, &sync.Mutex{})

	if !strings.Contains(out["summary"].(string), "ghost: PENDING") {
		t.Errorf("expected PENDING fallback, got %q", out["summary"])
	}
}

// TestAggregateJoinOutput_OutputPreservation confirms structured per-step
// output is preserved verbatim under steps[<id>].output (for richer
// assertions beyond the flat summary).
func TestAggregateJoinOutput_OutputPreservation(t *testing.T) {
	nested := map[string]any{
		"test_report": map[string]any{
			"passed": 42,
			"failed": 0,
		},
	}
	step := Step{ID: "j", Type: StepTypeJoin, WaitFor: []string{"test"}}
	statuses := map[string]StepStatus{"test": StepStatusSucceeded}
	stepOutputs := map[string]map[string]any{"test": nested}

	out := aggregateJoinOutput(step, statuses, stepOutputs, &sync.Mutex{})

	got := out["steps"].(map[string]any)["test"].(map[string]any)["output"]
	gotMap, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T", got)
	}
	report, _ := gotMap["test_report"].(map[string]any)
	if report["passed"] != 42 {
		t.Errorf("expected nested passed=42, got %v", report["passed"])
	}
}
