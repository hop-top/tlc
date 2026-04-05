package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestClarifyInline_HappyPath(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("complete T-42\n")

	result, err := clarifyInline(
		context.Background(),
		"mark the auth task done",
		"Which task? T-0042 (JWT refresh) or T-0068 (login page)",
		&buf, input,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Aborted {
		t.Fatal("expected non-aborted result")
	}
	if len(result.Commands) == 0 {
		t.Fatal("expected resolved commands")
	}
	if result.Commands[0].Args[0] != "complete" {
		t.Errorf("expected complete, got %s", result.Commands[0].Args[0])
	}
}

func TestClarifyInline_JustTaskID(t *testing.T) {
	var buf bytes.Buffer
	// User replies with just "show 42" after being asked which task.
	input := strings.NewReader("show 42\n")

	result, err := clarifyInline(
		context.Background(),
		"tell me about the task",
		"Which task?",
		&buf, input,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Aborted {
		t.Fatal("expected non-aborted result")
	}
	if len(result.Commands) == 0 {
		t.Fatal("expected resolved commands")
	}
}

func TestClarifyInline_EmptyAnswer(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("\n")

	result, err := clarifyInline(
		context.Background(),
		"do something",
		"What do you mean?",
		&buf, input,
	)
	if err != nil {
		t.Fatal("expected no error on empty answer")
	}
	if !result.Aborted {
		t.Fatal("expected aborted on empty answer")
	}
}

func TestClarifyInline_EOF(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("") // EOF immediately

	result, err := clarifyInline(
		context.Background(),
		"do something",
		"What do you mean?",
		&buf, input,
	)
	if err != nil {
		t.Fatal("expected no error on EOF")
	}
	if !result.Aborted {
		t.Fatal("expected aborted on EOF")
	}
}

func TestClarifyInline_Unresolvable(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("something completely unrelated\n")

	_, err := clarifyInline(
		context.Background(),
		"do something",
		"What do you mean?",
		&buf, input,
	)
	if err == nil {
		t.Fatal("expected error for unresolvable input")
	}
	if !strings.Contains(err.Error(), "could not resolve") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestClarifyREPL_ResolveOnSecondTurn(t *testing.T) {
	var buf bytes.Buffer
	// First turn unresolvable, second turn resolves.
	input := strings.NewReader("hmm\ncomplete T-42\n")

	result, err := clarifyREPL(
		context.Background(),
		"do something with auth",
		&buf, input,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Aborted {
		t.Fatal("expected non-aborted result")
	}
	if len(result.Commands) == 0 {
		t.Fatal("expected resolved commands")
	}
	// Should have shown "Still unclear" for first turn.
	if !strings.Contains(buf.String(), "Still unclear") {
		t.Error("expected 'Still unclear' message for first turn")
	}
}

func TestClarifyREPL_QuitCommand(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("quit\n")

	result, err := clarifyREPL(
		context.Background(),
		"do something",
		&buf, input,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aborted {
		t.Fatal("expected aborted on quit")
	}
}

func TestClarifyREPL_ExitCommand(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("exit\n")

	result, err := clarifyREPL(
		context.Background(),
		"do something",
		&buf, input,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aborted {
		t.Fatal("expected aborted on exit")
	}
}

func TestClarifyREPL_EOF(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("")

	result, err := clarifyREPL(
		context.Background(),
		"do something",
		&buf, input,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Aborted {
		t.Fatal("expected aborted on EOF")
	}
}

func TestClarifyREPL_EmptyLinesSkipped(t *testing.T) {
	var buf bytes.Buffer
	input := strings.NewReader("\n\ncomplete T-42\n")

	result, err := clarifyREPL(
		context.Background(),
		"do something",
		&buf, input,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Aborted {
		t.Fatal("expected non-aborted")
	}
	if len(result.Commands) == 0 {
		t.Fatal("expected resolved commands")
	}
}
