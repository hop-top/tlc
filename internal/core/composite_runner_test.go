package core

import (
	"context"
	"errors"
	"testing"
)

// TestCompositeAgentRunnerRoutesByStepType verifies that CompositeAgentRunner
// dispatches to the first runner whose CanHandle returns true.
func TestCompositeAgentRunnerRoutesByStepType(t *testing.T) {
	taskRunner := &MockAgentRunner{
		CanHandleFunc: func(s Step) bool { return s.Type == StepTypeTask },
		RunFunc: func(_ context.Context, _ Step, _ string) (map[string]any, error) {
			return map[string]any{"who": "task"}, nil
		},
	}
	execRunner := &MockAgentRunner{
		CanHandleFunc: func(s Step) bool { return s.Type == StepTypeExec },
		RunFunc: func(_ context.Context, _ Step, _ string) (map[string]any, error) {
			return map[string]any{"who": "exec"}, nil
		},
	}
	composite := NewCompositeAgentRunner(taskRunner, execRunner)

	if !composite.CanHandle(Step{Type: StepTypeTask}) {
		t.Error("CanHandle(task) = false, want true")
	}
	if !composite.CanHandle(Step{Type: StepTypeExec}) {
		t.Error("CanHandle(exec) = false, want true")
	}
	if composite.CanHandle(Step{Type: StepTypeBranch}) {
		t.Error("CanHandle(branch) = true, want false (no runner registered)")
	}

	out, err := composite.Run(context.Background(), Step{Type: StepTypeTask}, "")
	if err != nil {
		t.Fatalf("Run(task) err = %v", err)
	}
	if out["who"] != "task" {
		t.Errorf("Run(task) → %v, want who=task", out)
	}

	out, err = composite.Run(context.Background(), Step{Type: StepTypeExec}, "")
	if err != nil {
		t.Fatalf("Run(exec) err = %v", err)
	}
	if out["who"] != "exec" {
		t.Errorf("Run(exec) → %v, want who=exec", out)
	}
}

// TestCompositeAgentRunnerNoMatch verifies a clear error when no runner can
// handle the step.
func TestCompositeAgentRunnerNoMatch(t *testing.T) {
	composite := NewCompositeAgentRunner(&MockAgentRunner{
		CanHandleFunc: func(s Step) bool { return false },
	})

	_, err := composite.Run(context.Background(), Step{ID: "x", Type: StepTypeExec}, "")
	if err == nil {
		t.Fatal("expected error when no runner matches, got nil")
	}
}

// TestCompositeAgentRunnerOrderWins verifies that earlier runners take
// precedence on overlapping CanHandle.
func TestCompositeAgentRunnerOrderWins(t *testing.T) {
	first := &MockAgentRunner{
		CanHandleFunc: func(_ Step) bool { return true },
		RunFunc: func(_ context.Context, _ Step, _ string) (map[string]any, error) {
			return map[string]any{"who": "first"}, nil
		},
	}
	second := &MockAgentRunner{
		CanHandleFunc: func(_ Step) bool { return true },
		RunFunc: func(_ context.Context, _ Step, _ string) (map[string]any, error) {
			return nil, errors.New("second should not run")
		},
	}
	composite := NewCompositeAgentRunner(first, second)

	out, err := composite.Run(context.Background(), Step{Type: StepTypeTask}, "")
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if out["who"] != "first" {
		t.Errorf("Run → %v, want who=first", out)
	}
}
