package core

import (
	"context"
	"strings"
	"testing"
)

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
