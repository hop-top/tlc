package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"hop.top/kit/llm"
)

// ---------------------------------------------------------------------------
// Mock provider
// ---------------------------------------------------------------------------

// mockProvider implements llm.Provider + llm.Completer for testing.
type mockProvider struct {
	response llm.Response
	err      error
	closed   bool
}

func (m *mockProvider) Close() error {
	m.closed = true
	return nil
}

func (m *mockProvider) Complete(_ context.Context, _ llm.Request) (llm.Response, error) {
	if m.err != nil {
		return llm.Response{}, m.err
	}
	return m.response, nil
}

// ---------------------------------------------------------------------------
// parseRouterResponse tests
// ---------------------------------------------------------------------------

func TestParseRouterResponse_HappyPath(t *testing.T) {
	raw := `{"commands": [{"cmd": "task", "args": ["complete", "T-0042"]}], "confidence": 0.95}`

	cmds, err := parseRouterResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Cmd != "task" {
		t.Errorf("cmd = %q, want %q", cmds[0].Cmd, "task")
	}
	if len(cmds[0].Args) != 2 || cmds[0].Args[0] != "complete" || cmds[0].Args[1] != "T-0042" {
		t.Errorf("args = %v, want [complete T-0042]", cmds[0].Args)
	}
	if cmds[0].Confidence != 0.95 {
		t.Errorf("confidence = %f, want 0.95", cmds[0].Confidence)
	}
}

func TestParseRouterResponse_MultipleCommands(t *testing.T) {
	raw := `{
		"commands": [
			{"cmd": "task", "args": ["claim", "T-0001"]},
			{"cmd": "task", "args": ["claim", "T-0002"]}
		],
		"confidence": 0.9
	}`

	cmds, err := parseRouterResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	if cmds[1].Args[1] != "T-0002" {
		t.Errorf("second command task ID = %q, want T-0002", cmds[1].Args[1])
	}
}

func TestParseRouterResponse_MalformedJSON(t *testing.T) {
	raw := `not valid json at all`

	_, err := parseRouterResponse(raw)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "failed to parse LLM response as JSON") {
		t.Errorf("error = %q, want substring %q", got, "failed to parse LLM response as JSON")
	}
}

func TestParseRouterResponse_EmptyCommands(t *testing.T) {
	raw := `{"commands": [], "confidence": 0.5}`

	_, err := parseRouterResponse(raw)
	if err == nil {
		t.Fatal("expected error for empty commands, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "LLM returned no commands") {
		t.Errorf("error = %q, want substring %q", got, "LLM returned no commands")
	}
}

func TestParseRouterResponse_Clarification(t *testing.T) {
	raw := `{
		"commands": [{"cmd": "task", "args": ["show", "T-0001"]}],
		"confidence": 0.5,
		"clarification": "Did you mean task T-0001 or T-0010?"
	}`

	cmds, err := parseRouterResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Confidence != 0.5 {
		t.Errorf("confidence = %f, want 0.5", cmds[0].Confidence)
	}
}

func TestParseRouterResponse_DestructiveConfidenceCap(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantConf   float64
		wantCapped bool
	}{
		{
			name:       "delete at 0.95 capped to 0.9",
			raw:        `{"commands": [{"cmd": "task", "args": ["delete", "T-0042"]}], "confidence": 0.95}`,
			wantConf:   0.9,
			wantCapped: true,
		},
		{
			name:       "unclaim at 1.0 capped to 0.9",
			raw:        `{"commands": [{"cmd": "task", "args": ["unclaim", "T-0042"]}], "confidence": 1.0}`,
			wantConf:   0.9,
			wantCapped: true,
		},
		{
			name:       "unassign at 0.92 capped to 0.9",
			raw:        `{"commands": [{"cmd": "task", "args": ["unassign", "T-0042"]}], "confidence": 0.92}`,
			wantConf:   0.9,
			wantCapped: true,
		},
		{
			name:     "delete at 0.8 stays 0.8",
			raw:      `{"commands": [{"cmd": "task", "args": ["delete", "T-0042"]}], "confidence": 0.8}`,
			wantConf: 0.8,
		},
		{
			name:     "non-destructive at 0.95 stays 0.95",
			raw:      `{"commands": [{"cmd": "task", "args": ["complete", "T-0042"]}], "confidence": 0.95}`,
			wantConf: 0.95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds, err := parseRouterResponse(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cmds[0].Confidence != tt.wantConf {
				t.Errorf("confidence = %f, want %f", cmds[0].Confidence, tt.wantConf)
			}
		})
	}
}

func TestParseRouterResponse_CodeFenceStripping(t *testing.T) {
	raw := "```json\n" +
		`{"commands": [{"cmd": "task", "args": ["show", "T-0001"]}], "confidence": 0.9}` +
		"\n```"

	cmds, err := parseRouterResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Args[0] != "show" {
		t.Errorf("args[0] = %q, want %q", cmds[0].Args[0], "show")
	}
}

// ---------------------------------------------------------------------------
// routePrompt integration tests (with mock)
// ---------------------------------------------------------------------------

func TestRoutePrompt_HappyPath(t *testing.T) {
	mock := &mockProvider{
		response: llm.Response{
			Content: `{"commands": [{"cmd": "task", "args": ["complete", "T-0042"]}], "confidence": 0.95}`,
		},
	}

	cmds, err := routePromptWithProvider(context.Background(), "finish task 42", []byte(`[]`), mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Args[1] != "T-0042" {
		t.Errorf("task ID = %q, want T-0042", cmds[0].Args[1])
	}
	if !mock.closed {
		t.Error("expected provider to be closed")
	}
}

func TestRoutePrompt_LLMError(t *testing.T) {
	mock := &mockProvider{
		err: errors.New("connection refused"),
	}

	_, err := routePromptWithProvider(context.Background(), "do something", []byte(`[]`), mock)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "llm completion failed") {
		t.Errorf("error = %q, want substring %q", got, "llm completion failed")
	}
}

func TestRoutePrompt_MalformedResponse(t *testing.T) {
	mock := &mockProvider{
		response: llm.Response{Content: "I don't understand your request"},
	}

	_, err := routePromptWithProvider(context.Background(), "do stuff", []byte(`[]`), mock)
	if err == nil {
		t.Fatal("expected error for malformed response, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "failed to parse LLM response as JSON") {
		t.Errorf("error = %q, want substring %q", got, "failed to parse LLM response as JSON")
	}
}

// ---------------------------------------------------------------------------
// resolvePromptLLM error test
// ---------------------------------------------------------------------------

func TestResolvePromptLLM_NoProvider(t *testing.T) {
	// Clear all env vars that could configure a provider.
	t.Setenv("TLC_PROMPT_LLM", "")
	t.Setenv("LLM_PROVIDER", "")

	_, err := resolvePromptLLM()
	if err == nil {
		t.Fatal("expected error when no provider configured, got nil")
	}
	want := "no LLM provider configured; set TLC_PROMPT_LLM (e.g. ollama://llama3.2)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// routePromptWithProvider is a test-only variant that accepts an explicit
// provider, bypassing resolvePromptLLM.
func routePromptWithProvider(
	ctx context.Context,
	prompt string,
	schemaJSON []byte,
	provider llm.Provider,
) ([]ResolvedCommand, error) {
	defer provider.Close()

	client := llm.NewClient(provider)

	systemMsg := strings.ReplaceAll(systemPromptTemplate, "{schema}", string(schemaJSON))

	req := llm.Request{
		Messages: []llm.Message{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
		MaxTokens:   1024,
	}

	resp, err := client.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm completion failed: %w", err)
	}

	return parseRouterResponse(resp.Content)
}

