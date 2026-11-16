package core

import (
	"strings"
	"testing"
)

func TestErrInvalidTransition_WithAllowed(t *testing.T) {
	err := ErrInvalidTransition{
		From:    TaskStatus("todo"),
		To:      TaskStatus("done"),
		Msg:     "transition not allowed by state machine",
		Allowed: []string{"in_progress", "cancelled"},
	}
	msg := err.Error()

	if !strings.Contains(msg, "todo") {
		t.Errorf("expected From status in error: %q", msg)
	}
	if !strings.Contains(msg, "done") {
		t.Errorf("expected To status in error: %q", msg)
	}
	if !strings.Contains(msg, "in_progress") {
		t.Errorf("expected allowed transitions in error: %q", msg)
	}
	if !strings.Contains(msg, "--force") {
		t.Errorf("expected '--force' hint in error: %q", msg)
	}
}

func TestErrInvalidTransition_NoAllowed(t *testing.T) {
	err := ErrInvalidTransition{
		From: TaskStatus("done"),
		To:   TaskStatus("todo"),
		Msg:  "terminal states are immutable; run 'tlc task reopen <id> --note \"<reason>\"' first",
	}
	msg := err.Error()

	if !strings.Contains(msg, "done") {
		t.Errorf("expected From status in error: %q", msg)
	}
	if !strings.Contains(msg, "--force") {
		t.Errorf("expected '--force' hint in error: %q", msg)
	}
}

func TestWorkflow_TransitionError_IncludesAllowed(t *testing.T) {
	wm := DefaultWorkflow()

	// TODO -> DONE is not directly allowed; should list [IN_PROGRESS SKIPPED]
	err := wm.ValidateTransition("todo", "done", false)
	if err == nil {
		t.Fatal("expected error for TODO -> DONE transition")
	}

	msg := err.Error()
	if !strings.Contains(msg, "in_progress") {
		t.Errorf("expected allowed transition IN_PROGRESS in error: %q", msg)
	}
	if !strings.Contains(msg, "--force") {
		t.Errorf("expected '--force' hint in error: %q", msg)
	}
}

func TestWorkflow_TerminalTransitionError_HasReopenHint(t *testing.T) {
	wm := DefaultWorkflow()

	err := wm.ValidateTransition("done", "todo", false)
	if err == nil {
		t.Fatal("expected error for DONE -> TODO transition")
	}

	msg := err.Error()
	if !strings.Contains(msg, "reopen") {
		t.Errorf("expected 'reopen' hint in terminal transition error: %q", msg)
	}
}
