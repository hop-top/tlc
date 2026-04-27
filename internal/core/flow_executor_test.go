package core

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// TestDependencySatisfied is the unit-level truth table for the
// depends_on satisfaction predicate. SUCCEEDED and SKIPPED satisfy;
// every other state blocks.
func TestDependencySatisfied(t *testing.T) {
	cases := []struct {
		status StepStatus
		want   bool
	}{
		{StepStatusSucceeded, true},
		{StepStatusSkipped, true},
		{StepStatusFailed, false},
		{StepStatusPending, false},
		{StepStatusQueued, false},
		{StepStatusRunning, false},
		{StepStatusCanceled, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := dependencySatisfied(tc.status); got != tc.want {
				t.Errorf("dependencySatisfied(%s) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// runSchedulerWithSeeded drives the scheduler loop after pre-seeding
// predecessor statuses. The descendant ("dst") depends on the listed
// preds. Predecessors are NOT materialized as Flow.Steps — only their
// status is seeded — so the scheduler treats them as opaque upstream
// nodes that have already reached the seeded state. Returns whether
// the descendant ran and any scheduler error.
func runSchedulerWithSeeded(
	t *testing.T,
	preds map[string]StepStatus,
	dstDepends []string,
) (ran bool, err error) {
	t.Helper()

	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	exec := NewFlowExecutor(repo, logRepo)

	ctx := context.Background()
	repo.CreateTask(ctx, &Task{ID: "T-DST", Title: "Descendant", Status: StatusTodo})

	steps := map[string]Step{
		"dst": {
			ID:        "dst",
			Type:      StepTypeTask,
			Title:     "Descendant",
			TaskRef:   "T-DST",
			DependsOn: dstDepends,
		},
	}

	flow := &Flow{
		ID:        "flow:test:depends",
		Name:      "depends_on test",
		EntryStep: "dst",
		Steps:     steps,
	}

	run := &FlowRun{ID: "run:test", FlowID: flow.ID, Status: FlowStatusRunning}
	if err := repo.CreateFlowRun(ctx, run); err != nil {
		t.Fatalf("create flow run: %v", err)
	}

	statuses := make(map[string]StepStatus)
	statuses["dst"] = StepStatusPending
	for id, s := range preds {
		statuses[id] = s
	}

	stepOutputs := make(map[string]map[string]any)
	mu := &sync.Mutex{}
	isChild := make(map[string]bool)

	err = exec.runSequential(ctx, flow, run, statuses, stepOutputs, mu, isChild, "test")
	ran = statuses["dst"] == StepStatusSucceeded
	return ran, err
}

// All predecessors SUCCEEDED → descendant runs.
func TestRunSequential_DependsOn_AllSucceeded(t *testing.T) {
	ran, err := runSchedulerWithSeeded(t,
		map[string]StepStatus{
			"p1": StepStatusSucceeded,
			"p2": StepStatusSucceeded,
		},
		[]string{"p1", "p2"},
	)
	if err != nil {
		t.Fatalf("scheduler error: %v", err)
	}
	if !ran {
		t.Errorf("expected descendant to run when all preds SUCCEEDED")
	}
}

// One SKIPPED + one SUCCEEDED → descendant runs.
func TestRunSequential_DependsOn_MixedSkippedSucceeded(t *testing.T) {
	ran, err := runSchedulerWithSeeded(t,
		map[string]StepStatus{
			"p1": StepStatusSucceeded,
			"p2": StepStatusSkipped,
		},
		[]string{"p1", "p2"},
	)
	if err != nil {
		t.Fatalf("scheduler error: %v", err)
	}
	if !ran {
		t.Errorf("expected descendant to run when preds are SUCCEEDED+SKIPPED")
	}
}

// All predecessors SKIPPED → descendant runs.
func TestRunSequential_DependsOn_AllSkipped(t *testing.T) {
	ran, err := runSchedulerWithSeeded(t,
		map[string]StepStatus{
			"p1": StepStatusSkipped,
			"p2": StepStatusSkipped,
		},
		[]string{"p1", "p2"},
	)
	if err != nil {
		t.Fatalf("scheduler error: %v", err)
	}
	if !ran {
		t.Errorf("expected descendant to run when all preds SKIPPED")
	}
}

// Predecessor FAILED → descendant does not run. The scheduler reports
// either a terminal-failure abort (if the FAILED step is materialized
// in flow.Steps) or a deadlock (if the FAILED status is opaque). Both
// outcomes satisfy "FAILED blocks descendants".
func TestRunSequential_DependsOn_PredecessorFailed(t *testing.T) {
	ran, err := runSchedulerWithSeeded(t,
		map[string]StepStatus{
			"p1": StepStatusSucceeded,
			"p2": StepStatusFailed,
		},
		[]string{"p1", "p2"},
	)
	if err == nil {
		t.Fatal("expected scheduler error for FAILED predecessor")
	}
	if ran {
		t.Errorf("descendant must NOT run when a predecessor FAILED")
	}
}

// Mixed FAILED + SKIPPED → FAILED wins; descendant does not run.
func TestRunSequential_DependsOn_MixedFailedSkipped(t *testing.T) {
	ran, err := runSchedulerWithSeeded(t,
		map[string]StepStatus{
			"p1": StepStatusSkipped,
			"p2": StepStatusFailed,
		},
		[]string{"p1", "p2"},
	)
	if err == nil {
		t.Fatal("expected scheduler error for FAILED predecessor")
	}
	if ran {
		t.Errorf("descendant must NOT run when a predecessor FAILED (even with SKIPPED siblings)")
	}
}

// Predecessor RUNNING/PENDING → scheduler deadlocks (cannot make
// progress because the predecessor is not a managed flow step here, so
// nothing will advance it). This matches existing behaviour: if no
// step is ready to fire and not all are done, the scheduler reports a
// deadlock. Descendant must not run.
func TestRunSequential_DependsOn_PredecessorPending(t *testing.T) {
	ran, err := runSchedulerWithSeeded(t,
		map[string]StepStatus{
			"p1": StepStatusSucceeded,
			"p2": StepStatusPending,
		},
		[]string{"p1", "p2"},
	)
	// Scheduler should not advance "dst": pending pred fails the
	// dependency check, and "p2" itself has no ready upstream that
	// can transition it, so the loop reports deadlock.
	if err == nil {
		t.Fatal("expected deadlock error when a predecessor is PENDING with no path to ready")
	}
	if !strings.Contains(err.Error(), "deadlock") {
		t.Errorf("expected deadlock error, got %v", err)
	}
	if ran {
		t.Errorf("descendant must NOT run while a predecessor is still PENDING")
	}
}

// stepFlow builds a tiny two-step flow where step "gated" carries the given
// condition and depends on the always-running entry step. Used as the standard
// fixture for condition-evaluation tests below.
func stepFlow(condition string) *Flow {
	return &Flow{
		ID:        "flow:cond:test",
		Name:      "Cond Test",
		EntryStep: "entry",
		Steps: map[string]Step{
			"entry": {
				ID:      "entry",
				Type:    StepTypeTask,
				Title:   "Entry",
				TaskRef: "T-1",
			},
			"gated": {
				ID:        "gated",
				Type:      StepTypeTask,
				Title:     "Gated",
				TaskRef:   "T-2",
				DependsOn: []string{"entry"},
				Condition: condition,
			},
		},
	}
}

func runWithInputs(t *testing.T, condition string, inputs map[string]any) (map[string]StepStatus, error) {
	t.Helper()
	repo := NewMockRepository()
	logRepo := NewMockLogRepository()
	ctx := context.Background()

	repo.CreateTask(ctx, &Task{ID: "T-1", Title: "T1", Status: StatusTodo})
	repo.CreateTask(ctx, &Task{ID: "T-2", Title: "T2", Status: StatusTodo})

	executor := NewFlowExecutor(repo, logRepo).WithInputs(inputs)
	flow := stepFlow(condition)

	_, _, statuses, err := executor.ExecuteWithStatuses(ctx, flow, "test-user")
	return statuses, err
}

func TestStepCondition_Empty_RunsNormally(t *testing.T) {
	statuses, err := runWithInputs(t, "", map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if statuses["gated"] != StepStatusSucceeded {
		t.Errorf("gated step: want succeeded, got %s", statuses["gated"])
	}
}

func TestStepCondition_TrueWithInputsPrefix_Runs(t *testing.T) {
	statuses, err := runWithInputs(t, "inputs.target == 'staging'", map[string]any{"target": "staging"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if statuses["gated"] != StepStatusSucceeded {
		t.Errorf("gated step: want succeeded, got %s", statuses["gated"])
	}
}

func TestStepCondition_TrueBareKey_Runs(t *testing.T) {
	statuses, err := runWithInputs(t, "env == 'staging'", map[string]any{"env": "staging"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if statuses["gated"] != StepStatusSucceeded {
		t.Errorf("gated step: want succeeded, got %s", statuses["gated"])
	}
}

func TestStepCondition_False_Skipped(t *testing.T) {
	statuses, err := runWithInputs(t, "inputs.target == 'staging'", map[string]any{"target": "production"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if statuses["gated"] != StepStatusSkipped {
		t.Errorf("gated step: want skipped, got %s", statuses["gated"])
	}
}

func TestStepCondition_NotEqual_True_Runs(t *testing.T) {
	statuses, err := runWithInputs(t, "inputs.target != 'production'", map[string]any{"target": "staging"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if statuses["gated"] != StepStatusSucceeded {
		t.Errorf("gated step: want succeeded, got %s", statuses["gated"])
	}
}

func TestStepCondition_NotEqual_False_Skipped(t *testing.T) {
	statuses, err := runWithInputs(t, "target != 'staging'", map[string]any{"target": "staging"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if statuses["gated"] != StepStatusSkipped {
		t.Errorf("gated step: want skipped, got %s", statuses["gated"])
	}
}

func TestStepCondition_UnknownKey_Fails(t *testing.T) {
	_, err := runWithInputs(t, "inputs.missing == 'x'", map[string]any{"target": "staging"})
	if err == nil {
		t.Fatal("Execute: expected error for unknown input key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown input key") {
		t.Errorf("Execute: error %q does not mention unknown input key", err.Error())
	}
}

func TestStepCondition_DoubleQuotedRHS_Works(t *testing.T) {
	statuses, err := runWithInputs(t, `inputs.target == "staging"`, map[string]any{"target": "staging"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if statuses["gated"] != StepStatusSucceeded {
		t.Errorf("gated step: want succeeded, got %s", statuses["gated"])
	}
}

func TestStepCondition_BareRHS_Works(t *testing.T) {
	statuses, err := runWithInputs(t, "inputs.target == staging", map[string]any{"target": "staging"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if statuses["gated"] != StepStatusSucceeded {
		t.Errorf("gated step: want succeeded, got %s", statuses["gated"])
	}
}

func TestStepCondition_NoOperator_Fails(t *testing.T) {
	_, err := runWithInputs(t, "inputs.target", map[string]any{"target": "staging"})
	if err == nil {
		t.Fatal("Execute: expected error for missing operator, got nil")
	}
}

// TestEvalCondition_Unit covers the parser standalone for whitespace + edges
// not hit by the executor flow tests.
func TestEvalCondition_Unit(t *testing.T) {
	cases := []struct {
		name    string
		expr    string
		inputs  map[string]any
		want    bool
		wantErr bool
	}{
		{"empty is true", "", nil, true, false},
		{"whitespace pad", "  inputs.x  ==  'a'  ", map[string]any{"x": "a"}, true, false},
		{"int coerced via Sprintf", "n == '7'", map[string]any{"n": 7}, true, false},
		{"bare key bare value", "env == prod", map[string]any{"env": "prod"}, true, false},
		{"missing operator errors", "inputs.x", map[string]any{"x": "a"}, false, true},
		{"empty lhs errors", " == 'a'", map[string]any{"x": "a"}, false, true},
		{"empty rhs errors", "inputs.x == ", map[string]any{"x": "a"}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EvalCondition(tc.expr, tc.inputs)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (got=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}
