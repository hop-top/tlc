package core

import (
	"context"
	"testing"

	"hop.top/kit/go/runtime/domain"
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

func TestNewTrackStateMachine_AllowedTransitions(t *testing.T) {
	sm := NewTrackStateMachine(nil)
	ctx := context.Background()

	allowed := []struct{ from, to domain.State }{
		{"pending", "active"},
		{"pending", "abandoned"},
		{"active", "completed"},
		{"active", "abandoned"},
		{"completed", "archived"},
		{"completed", "abandoned"},
		{"abandoned", "archived"},
	}
	for _, tt := range allowed {
		if err := sm.Transition(ctx, tt.from, tt.to, false); err != nil {
			t.Errorf("Transition(%s -> %s) unexpected error: %v",
				tt.from, tt.to, err)
		}
	}
}

func TestNewTrackStateMachine_DisallowedTransitions(t *testing.T) {
	sm := NewTrackStateMachine(nil)
	ctx := context.Background()

	disallowed := []struct{ from, to domain.State }{
		{"archived", "active"},
		{"archived", "pending"},
		{"active", "pending"},
		{"completed", "active"},
		{"abandoned", "active"},
		{"pending", "completed"},
	}
	for _, tt := range disallowed {
		if err := sm.Transition(ctx, tt.from, tt.to, false); err == nil {
			t.Errorf("Transition(%s -> %s) expected error, got nil",
				tt.from, tt.to)
		}
	}
}

func TestNewTrackStateMachine_ForceBypass(t *testing.T) {
	sm := NewTrackStateMachine(nil)
	ctx := context.Background()

	// Force bypasses rules.
	if err := sm.Transition(ctx, "archived", "active", true); err != nil {
		t.Errorf("forced Transition(archived -> active) unexpected error: %v", err)
	}
}

func TestNewFlowStateMachine_AllowedTransitions(t *testing.T) {
	sm := NewFlowStateMachine(nil)
	ctx := context.Background()

	allowed := []struct{ from, to domain.State }{
		{"queued", "running"},
		{"running", "succeeded"},
		{"running", "failed"},
		{"running", "canceled"},
		{"running", "paused"},
		{"paused", "running"},
		{"paused", "canceled"},
	}
	for _, tt := range allowed {
		if err := sm.Transition(ctx, tt.from, tt.to, false); err != nil {
			t.Errorf("Transition(%s -> %s) unexpected error: %v",
				tt.from, tt.to, err)
		}
	}
}

func TestNewFlowStateMachine_DisallowedTransitions(t *testing.T) {
	sm := NewFlowStateMachine(nil)
	ctx := context.Background()

	disallowed := []struct{ from, to domain.State }{
		{"queued", "succeeded"},
		{"queued", "failed"},
		{"succeeded", "running"},
		{"failed", "running"},
		{"canceled", "running"},
		{"paused", "succeeded"},
	}
	for _, tt := range disallowed {
		if err := sm.Transition(ctx, tt.from, tt.to, false); err == nil {
			t.Errorf("Transition(%s -> %s) expected error, got nil",
				tt.from, tt.to)
		}
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
