package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	"hop.top/kit/domain"
)

func TestTransitionError_WithAllowed(t *testing.T) {
	te := &domain.TransitionError{
		From:    domain.State("TODO"),
		To:      domain.State("DONE"),
		Allowed: []domain.State{"IN_PROGRESS", "SKIPPED"},
	}
	msg := te.Error()

	if !strings.Contains(msg, "TODO") {
		t.Errorf("expected From status in error: %q", msg)
	}
	if !strings.Contains(msg, "DONE") {
		t.Errorf("expected To status in error: %q", msg)
	}
	if !strings.Contains(msg, "IN_PROGRESS") {
		t.Errorf("expected allowed transitions in error: %q", msg)
	}
}

func TestTransitionError_NoAllowed(t *testing.T) {
	te := &domain.TransitionError{
		From: domain.State("DONE"),
		To:   domain.State("TODO"),
	}
	msg := te.Error()

	if !strings.Contains(msg, "DONE") {
		t.Errorf("expected From status in error: %q", msg)
	}
}

func TestTransitionError_Is(t *testing.T) {
	te := &domain.TransitionError{
		From: domain.State("TODO"),
		To:   domain.State("DONE"),
	}
	if !errors.Is(te, domain.ErrInvalidTransition) {
		t.Error("expected TransitionError to match ErrInvalidTransition")
	}
}

func TestWorkflow_StateMachine_TransitionError_IncludesAllowed(t *testing.T) {
	wm := DefaultWorkflow()
	sm := wm.StateMachine()

	// TODO -> DONE is not directly allowed; should list allowed targets
	err := sm.Transition(context.Background(), "TODO", "DONE", false)
	if err == nil {
		t.Fatal("expected error for TODO -> DONE transition")
	}

	var te *domain.TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("expected *domain.TransitionError, got %T", err)
	}
	if len(te.Allowed) == 0 {
		t.Error("expected non-empty Allowed list")
	}

	msg := err.Error()
	if !strings.Contains(msg, "IN_PROGRESS") {
		t.Errorf("expected allowed transition IN_PROGRESS in error: %q", msg)
	}
}

func TestWorkflow_StateMachine_TerminalTransitionError(t *testing.T) {
	wm := DefaultWorkflow()
	sm := wm.StateMachine()

	err := sm.Transition(context.Background(), "DONE", "TODO", false)
	if err == nil {
		t.Fatal("expected error for DONE -> TODO transition")
	}
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got: %v", err)
	}
}
