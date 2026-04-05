package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// stubRunCommand replaces runCommandFn for testing. It records calls and
// optionally returns an error for a specific call index.
type stubRunCommand struct {
	calls   []ResolvedCommand
	failAt  int // -1 = never fail
	failErr error
}

func (s *stubRunCommand) run(_ context.Context, cmd ResolvedCommand) error {
	s.calls = append(s.calls, cmd)
	if s.failAt >= 0 && len(s.calls)-1 == s.failAt {
		return s.failErr
	}
	return nil
}

func newStub() *stubRunCommand {
	return &stubRunCommand{failAt: -1}
}

func newStubFailAt(idx int, err error) *stubRunCommand {
	return &stubRunCommand{failAt: idx, failErr: err}
}

// setConfirmResponse sets readConfirmFn to always return the given value.
func setConfirmResponse(accept bool) func() {
	old := readConfirmFn
	readConfirmFn = func() bool { return accept }
	return func() { readConfirmFn = old }
}

// withStub installs a stub as runCommandFn and returns a cleanup function.
func withStub(s *stubRunCommand) func() {
	old := runCommandFn
	runCommandFn = s.run
	return func() { runCommandFn = old }
}

func TestExecuteCommands_EmptyList(t *testing.T) {
	result := executeCommands(context.Background(), nil, ExecuteOptions{})
	if result.Error != nil {
		t.Fatalf("expected no error, got %v", result.Error)
	}
	if len(result.Executed) != 0 {
		t.Fatalf("expected no executed commands")
	}
	if result.NeedsClarification {
		t.Fatal("should not need clarification for empty list")
	}
}

func TestExecuteCommands_HighConfidence_AutoExecutes(t *testing.T) {
	stub := newStub()
	defer withStub(stub)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"complete", "T-0042"}, Confidence: 1.0},
		{Cmd: "task", Args: []string{"claim", "T-0043"}, Confidence: 0.95},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

	if result.Error != nil {
		t.Fatalf("expected no error, got %v", result.Error)
	}
	if len(result.Executed) != 2 {
		t.Fatalf("expected 2 executed, got %d", len(result.Executed))
	}
	if result.Failed != nil {
		t.Fatal("expected no failure")
	}
	if len(stub.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(stub.calls))
	}
}

func TestExecuteCommands_MediumConfidence_Prompts(t *testing.T) {
	stub := newStub()
	defer withStub(stub)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"claim", "T-0042"}, Confidence: 0.8},
	}

	t.Run("rejected", func(t *testing.T) {
		defer setConfirmResponse(false)()
		var buf bytes.Buffer
		result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

		if result.Error == nil || !strings.Contains(result.Error.Error(), "aborted") {
			t.Fatalf("expected aborted error, got %v", result.Error)
		}
		if len(stub.calls) != 0 {
			t.Fatalf("expected no calls when rejected, got %d", len(stub.calls))
		}
	})

	t.Run("accepted", func(t *testing.T) {
		stub2 := newStub()
		defer withStub(stub2)()
		defer setConfirmResponse(true)()
		var buf bytes.Buffer
		result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

		if result.Error != nil {
			t.Fatalf("expected no error, got %v", result.Error)
		}
		if len(stub2.calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(stub2.calls))
		}
	})
}

func TestExecuteCommands_MediumConfidence_ExecuteOverride(t *testing.T) {
	stub := newStub()
	defer withStub(stub)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"claim", "T-0042"}, Confidence: 0.75},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{
		Execute: true,
		Writer:  &buf,
	})

	if result.Error != nil {
		t.Fatalf("expected no error, got %v", result.Error)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(stub.calls))
	}
}

func TestExecuteCommands_LowConfidence_Clarification(t *testing.T) {
	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"claim", "T-0042"}, Confidence: 0.5},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

	if !result.NeedsClarification {
		t.Fatal("expected clarification needed")
	}
	if result.Clarification == "" {
		t.Fatal("expected clarification message")
	}
	if len(result.Remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(result.Remaining))
	}
}

func TestExecuteCommands_DryRun(t *testing.T) {
	stub := newStub()
	defer withStub(stub)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"complete", "T-0042"}, Confidence: 1.0},
		{Cmd: "task", Args: []string{"claim", "T-0043"}, Confidence: 1.0},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{
		DryRun: true,
		Writer: &buf,
	})

	if result.Error != nil {
		t.Fatalf("expected no error, got %v", result.Error)
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no execution in dry-run, got %d calls", len(stub.calls))
	}
	if len(result.Remaining) != 2 {
		t.Fatalf("expected 2 remaining in dry-run, got %d", len(result.Remaining))
	}

	out := buf.String()
	if !strings.Contains(out, "dry-run") {
		t.Fatalf("expected dry-run output, got %q", out)
	}
	if !strings.Contains(out, "complete T-0042") {
		t.Fatalf("expected command listed in output, got %q", out)
	}
}

func TestExecuteCommands_DestructiveGuard_Delete(t *testing.T) {
	stub := newStub()
	defer withStub(stub)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"delete", "T-0042"}, Confidence: 1.0},
	}

	t.Run("rejected", func(t *testing.T) {
		s := newStub()
		defer withStub(s)()
		defer setConfirmResponse(false)()
		var buf bytes.Buffer
		result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

		if result.Error == nil || !strings.Contains(result.Error.Error(), "destructive") {
			t.Fatalf("expected destructive rejection, got %v", result.Error)
		}
		if len(s.calls) != 0 {
			t.Fatalf("expected no calls, got %d", len(s.calls))
		}
	})

	t.Run("accepted", func(t *testing.T) {
		s := newStub()
		defer withStub(s)()
		defer setConfirmResponse(true)()
		var buf bytes.Buffer
		result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

		if result.Error != nil {
			t.Fatalf("expected no error, got %v", result.Error)
		}
		if len(s.calls) != 1 {
			t.Fatalf("expected 1 call, got %d", len(s.calls))
		}
	})
}

func TestExecuteCommands_DestructiveGuard_Unclaim(t *testing.T) {
	stub := newStub()
	defer withStub(stub)()
	defer setConfirmResponse(false)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"unclaim", "T-0042"}, Confidence: 1.0},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

	if result.Error == nil || !strings.Contains(result.Error.Error(), "destructive") {
		t.Fatalf("expected destructive rejection for unclaim, got %v", result.Error)
	}
}

func TestExecuteCommands_DestructiveGuard_Unassign(t *testing.T) {
	stub := newStub()
	defer withStub(stub)()
	defer setConfirmResponse(false)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"unassign", "T-0042"}, Confidence: 1.0},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

	if result.Error == nil || !strings.Contains(result.Error.Error(), "destructive") {
		t.Fatalf("expected destructive rejection for unassign, got %v", result.Error)
	}
}

func TestExecuteCommands_NoPromptSkipsDestructiveGuard(t *testing.T) {
	stub := newStub()
	defer withStub(stub)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"delete", "T-0042"}, Confidence: 1.0},
		{Cmd: "task", Args: []string{"unclaim", "T-0043"}, Confidence: 1.0},
		{Cmd: "task", Args: []string{"unassign", "T-0044"}, Confidence: 1.0},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{
		NoPrompt: true,
		Writer:   &buf,
	})

	if result.Error != nil {
		t.Fatalf("expected no error with --no-prompt, got %v", result.Error)
	}
	if len(stub.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(stub.calls))
	}
}

func TestExecuteCommands_PartialFailure(t *testing.T) {
	stub := newStubFailAt(2, fmt.Errorf("task T-0044 not found"))
	defer withStub(stub)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"complete", "T-0042"}, Confidence: 1.0},
		{Cmd: "task", Args: []string{"claim", "T-0043"}, Confidence: 1.0},
		{Cmd: "task", Args: []string{"complete", "T-0044"}, Confidence: 1.0},
		{Cmd: "task", Args: []string{"complete", "T-0045"}, Confidence: 1.0},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

	if len(result.Executed) != 2 {
		t.Fatalf("expected 2 executed, got %d", len(result.Executed))
	}
	if result.Failed == nil {
		t.Fatal("expected a failed command")
	}
	if result.Failed.Args[1] != "T-0044" {
		t.Fatalf("expected T-0044 to fail, got %s", result.Failed.Args[1])
	}
	if result.Error == nil || !strings.Contains(result.Error.Error(), "T-0044") {
		t.Fatalf("expected error about T-0044, got %v", result.Error)
	}
	if len(result.Remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(result.Remaining))
	}
	if result.Remaining[0].Args[1] != "T-0045" {
		t.Fatalf("expected T-0045 remaining, got %s", result.Remaining[0].Args[1])
	}
}

func TestIsDestructive(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"delete", "T-0042"}, true},
		{[]string{"unclaim", "T-0042"}, true},
		{[]string{"unassign", "T-0042"}, true},
		{[]string{"complete", "T-0042"}, false},
		{[]string{"claim", "T-0042"}, false},
		{[]string{"show", "T-0042"}, false},
		{[]string{"list"}, false},
		{nil, false},
	}

	for _, tt := range tests {
		got := isDestructiveArgs(tt.args)
		if got != tt.want {
			t.Errorf("isDestructiveArgs(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestExecuteCommands_MixedConfidenceWithLow(t *testing.T) {
	// If any command is low confidence, return clarification before executing any.
	stub := newStub()
	defer withStub(stub)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"complete", "T-0042"}, Confidence: 1.0},
		{Cmd: "task", Args: []string{"claim", "T-0043"}, Confidence: 0.5},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{Writer: &buf})

	if !result.NeedsClarification {
		t.Fatal("expected clarification for mixed batch with low confidence")
	}
	if len(stub.calls) != 0 {
		t.Fatalf("expected no calls when clarification needed, got %d", len(stub.calls))
	}
}

func TestExecuteCommands_DestructiveWithExecuteFlag(t *testing.T) {
	// --execute skips confidence prompt but NOT destructive guard.
	stub := newStub()
	defer withStub(stub)()
	defer setConfirmResponse(false)()

	cmds := []ResolvedCommand{
		{Cmd: "task", Args: []string{"delete", "T-0042"}, Confidence: 0.8},
	}

	var buf bytes.Buffer
	result := executeCommands(context.Background(), cmds, ExecuteOptions{
		Execute: true,
		Writer:  &buf,
	})

	// Execute skips confidence prompt but destructive guard still fires.
	if result.Error == nil || !strings.Contains(result.Error.Error(), "destructive") {
		t.Fatalf("expected destructive guard even with --execute, got %v", result.Error)
	}
}

func TestFormatCmd(t *testing.T) {
	cmd := ResolvedCommand{Cmd: "task", Args: []string{"complete", "T-0042"}}
	got := formatCmd(cmd)
	if got != "task complete T-0042" {
		t.Fatalf("expected 'task complete T-0042', got %q", got)
	}
}
