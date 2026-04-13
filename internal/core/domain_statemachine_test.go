package core

import (
	"context"
	"testing"

	"hop.top/kit/domain"
	"hop.top/tlc/internal/config"
)

func TestNewTaskStateMachine_DefaultConfig(t *testing.T) {
	cfg := &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
	}
	sm := NewTaskStateMachine(cfg, nil)
	ctx := context.Background()

	// Allowed transitions.
	allowed := []struct{ from, to domain.State }{
		{"TODO", "IN_PROGRESS"},
		{"TODO", "SKIPPED"},
		{"IN_PROGRESS", "DONE"},
		{"IN_PROGRESS", "TODO"},
		{"IN_PROGRESS", "SKIPPED"},
	}
	for _, tt := range allowed {
		if err := sm.Transition(ctx, tt.from, tt.to, false); err != nil {
			t.Errorf("Transition(%s -> %s) unexpected error: %v",
				tt.from, tt.to, err)
		}
	}

	// Disallowed transitions.
	disallowed := []struct{ from, to domain.State }{
		{"TODO", "DONE"},
		{"DONE", "TODO"},
	}
	for _, tt := range disallowed {
		if err := sm.Transition(ctx, tt.from, tt.to, false); err == nil {
			t.Errorf("Transition(%s -> %s) expected error, got nil",
				tt.from, tt.to)
		}
	}
}

func TestNewTaskStateMachine_ForceBypass(t *testing.T) {
	cfg := &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
	}
	sm := NewTaskStateMachine(cfg, nil)
	ctx := context.Background()

	// Force bypasses rules.
	if err := sm.Transition(ctx, "DONE", "TODO", true); err != nil {
		t.Errorf("forced Transition(DONE -> TODO) unexpected error: %v", err)
	}
}

func TestNewTaskStateMachineFromWorkflow(t *testing.T) {
	wm := DefaultWorkflow()
	sm := NewTaskStateMachineFromWorkflow(wm, nil)
	ctx := context.Background()

	if err := sm.Transition(ctx, "TODO", "IN_PROGRESS", false); err != nil {
		t.Errorf("Transition(TODO -> IN_PROGRESS) unexpected error: %v", err)
	}
	if err := sm.Transition(ctx, "TODO", "DONE", false); err == nil {
		t.Error("Transition(TODO -> DONE) expected error, got nil")
	}
}

func TestNewTaskStateMachine_NilConfig(t *testing.T) {
	cfg := &config.TaskConfig{} // no statuses, no state machine
	sm := NewTaskStateMachine(cfg, nil)
	ctx := context.Background()

	// Should fall back to defaults.
	if err := sm.Transition(ctx, "TODO", "IN_PROGRESS", false); err != nil {
		t.Errorf("Transition(TODO -> IN_PROGRESS) unexpected error: %v", err)
	}
}
