package core

import (
	"strings"
	"testing"
)

func TestErrInvalidTransition_WithAllowed(t *testing.T) {
	err := ErrInvalidTransition{
		From:    TaskStatus("TODO"),
		To:      TaskStatus("DONE"),
		Msg:     "transition not allowed by state machine",
		Allowed: []string{"IN_PROGRESS", "SKIPPED"},
	}
	msg := err.Error()

	if !strings.Contains(msg, "TODO") {
		t.Errorf("expected From status in error: %q", msg)
	}
	if !strings.Contains(msg, "DONE") {
		t.Errorf("expected To status in error: %q", msg)
	}
	if !strings.Contains(msg, "IN_PROGRESS") {
		t.Errorf("expected allowed transitions in error: %q", msg)
	}
	if !strings.Contains(msg, "--force") {
		t.Errorf("expected '--force' hint in error: %q", msg)
	}
}

func TestErrInvalidTransition_NoAllowed(t *testing.T) {
	err := ErrInvalidTransition{
		From: TaskStatus("DONE"),
		To:   TaskStatus("TODO"),
		Msg:  "terminal states are immutable; run 'tlc task reopen <id> --note \"<reason>\"' first",
	}
	msg := err.Error()

	if !strings.Contains(msg, "DONE") {
		t.Errorf("expected From status in error: %q", msg)
	}
	if !strings.Contains(msg, "--force") {
		t.Errorf("expected '--force' hint in error: %q", msg)
	}
}

func TestWorkflow_TransitionError_IncludesAllowed(t *testing.T) {
	wm := DefaultWorkflow()

	// TODO -> DONE is not directly allowed; should list [IN_PROGRESS SKIPPED]
	err := wm.ValidateTransition("TODO", "DONE", false)
	if err == nil {
		t.Fatal("expected error for TODO -> DONE transition")
	}

	msg := err.Error()
	if !strings.Contains(msg, "IN_PROGRESS") {
		t.Errorf("expected allowed transition IN_PROGRESS in error: %q", msg)
	}
	if !strings.Contains(msg, "--force") {
		t.Errorf("expected '--force' hint in error: %q", msg)
	}
}

func TestWorkflow_TerminalTransitionError_HasReopenHint(t *testing.T) {
	wm := DefaultWorkflow()

	err := wm.ValidateTransition("DONE", "TODO", false)
	if err == nil {
		t.Fatal("expected error for DONE -> TODO transition")
	}

	msg := err.Error()
	if !strings.Contains(msg, "reopen") {
		t.Errorf("expected 'reopen' hint in terminal transition error: %q", msg)
	}
}
