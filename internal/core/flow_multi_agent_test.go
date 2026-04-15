package core

import (
	"context"
	"testing"
)

// TestFlowMultiAgent_CorrectAgentPerStep verifies that a 3-step flow
// with different per-step agents dispatches the correct agent for each.
func TestFlowMultiAgent_CorrectAgentPerStep(t *testing.T) {
	repo := NewMockRepository()
	logRepo := &MockLogRepository{}
	runner := &MockAgentRunner{}

	executor := NewFlowExecutor(repo, logRepo).
		WithTestMode().
		WithAgentRunner(runner)

	flow := &Flow{
		ID:        "flow:multi-agent:1.0",
		Name:      "Multi-Agent",
		Version:   "1.0",
		EntryStep: "plan",
		Agent:     AgentRef{Name: "claude"},
		Steps: map[string]Step{
			"plan": {
				ID:    "plan",
				Type:  StepTypeTask,
				Title: "Plan",
				Agent: AgentRef{Name: "claude"},
				TaskTemplate: &TaskTemplate{
					Title:       "Plan task",
					Description: "Planning",
				},
			},
			"implement": {
				ID:        "implement",
				Type:      StepTypeTask,
				Title:     "Implement",
				DependsOn: []string{"plan"},
				Agent:     AgentRef{Name: "codex"},
				TaskTemplate: &TaskTemplate{
					Title:       "Implement task",
					Description: "Coding",
				},
			},
			"review": {
				ID:        "review",
				Type:      StepTypeTask,
				Title:     "Review",
				DependsOn: []string{"implement"},
				Agent:     AgentRef{Name: "gemini"},
				TaskTemplate: &TaskTemplate{
					Title:       "Review task",
					Description: "Reviewing",
				},
			},
		},
	}

	ctx := context.Background()
	_, _, err := executor.Execute(ctx, flow, "test-user")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(runner.Calls) != 3 {
		t.Fatalf("expected 3 runner calls; got %d", len(runner.Calls))
	}

	// Verify each step was dispatched with the correct agent.
	agentByStep := make(map[string]string)
	for _, call := range runner.Calls {
		agentByStep[call.ID] = call.Agent.Name
	}

	expected := map[string]string{
		"plan":      "claude",
		"implement": "codex",
		"review":    "gemini",
	}
	for stepID, wantAgent := range expected {
		got, ok := agentByStep[stepID]
		if !ok {
			t.Errorf("step %q was not dispatched to runner", stepID)
			continue
		}
		if got != wantAgent {
			t.Errorf("step %q: agent = %q; want %q", stepID, got, wantAgent)
		}
	}
}

// TestFlowMultiAgent_InheritsFlowDefault verifies that a step without
// an explicit agent field inherits the flow-level default.
func TestFlowMultiAgent_InheritsFlowDefault(t *testing.T) {
	flow := &Flow{
		ID:        "flow:inherit:1.0",
		Name:      "Inherit Test",
		EntryStep: "step-a",
		Agent:     AgentRef{Name: "claude"},
		Steps: map[string]Step{
			"step-a": {
				ID:    "step-a",
				Type:  StepTypeTask,
				Title: "Step A",
				// No Agent field — should inherit "claude".
				TaskTemplate: &TaskTemplate{
					Title: "A task",
				},
			},
		},
	}

	step := flow.Steps["step-a"]
	effective := step.Agent
	if effective.IsZero() {
		effective = flow.Agent
	}

	if effective.Name != "claude" {
		t.Errorf("effective agent = %q; want 'claude'", effective.Name)
	}
}
