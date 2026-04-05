package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/kit/llm"

	// Register adapters for resolvePromptLLM tests.
	_ "hop.top/kit/llm/anthropic"
	_ "hop.top/kit/llm/ollama"
	_ "hop.top/kit/llm/openai"
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

	cmds, clarification, err := parseRouterResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clarification != "" {
		t.Errorf("clarification = %q, want empty", clarification)
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

	cmds, _, err := parseRouterResponse(raw)
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

	_, _, err := parseRouterResponse(raw)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "failed to parse LLM response as JSON") {
		t.Errorf("error = %q, want substring %q", got, "failed to parse LLM response as JSON")
	}
}

func TestParseRouterResponse_EmptyCommands(t *testing.T) {
	raw := `{"commands": [], "confidence": 0.5}`

	_, _, err := parseRouterResponse(raw)
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

	cmds, clarification, err := parseRouterResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Confidence != 0.5 {
		t.Errorf("confidence = %f, want 0.5", cmds[0].Confidence)
	}
	if clarification != "Did you mean task T-0001 or T-0010?" {
		t.Errorf("clarification = %q, want %q", clarification, "Did you mean task T-0001 or T-0010?")
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
			cmds, _, err := parseRouterResponse(tt.raw)
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

	cmds, _, err := parseRouterResponse(raw)
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

	cmds, clarification, err := routePromptWithProvider(context.Background(), "finish task 42", []byte(`[]`), mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Args[1] != "T-0042" {
		t.Errorf("task ID = %q, want T-0042", cmds[0].Args[1])
	}
	if clarification != "" {
		t.Errorf("clarification = %q, want empty", clarification)
	}
	if !mock.closed {
		t.Error("expected provider to be closed")
	}
}

func TestRoutePrompt_LLMError(t *testing.T) {
	mock := &mockProvider{
		err: errors.New("connection refused"),
	}

	_, _, err := routePromptWithProvider(context.Background(), "do something", []byte(`[]`), mock)
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

	_, _, err := routePromptWithProvider(context.Background(), "do stuff", []byte(`[]`), mock)
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
// resolvePromptLLM: env var → provider auth
// ---------------------------------------------------------------------------

// TestResolvePromptLLM_ViperURI_UsesLoadConfig verifies that when
// prompt.llm_provider is a full URI (e.g. "openai://gpt-4o"), the
// resolver uses llm.LoadConfig to merge env var API keys — not just
// llm.Resolve which ignores env vars.
func TestResolvePromptLLM_ViperURI_UsesLoadConfig(t *testing.T) {
	// Set a full URI in viper + API key in env.
	viper.Set("prompt.llm_provider", "openai://gpt-4o")
	t.Setenv("OPENAI_API_KEY", "sk-test-key-for-tdd")
	t.Setenv("TLC_PROMPT_LLM", "")
	t.Setenv("LLM_PROVIDER", "")
	defer viper.Set("prompt.llm_provider", "")

	provider, err := resolvePromptLLM()
	if err != nil {
		t.Fatalf("resolvePromptLLM failed: %v", err)
	}
	defer provider.Close()

	// If the provider resolved without auth error, the API key
	// was picked up from the env var via LoadConfig.
	// (anthropic would fail at New() if key missing; openai
	// succeeds at New() but we can at least confirm no error.)
}

// TestResolvePromptLLM_BareScheme_NormalizesToURI verifies that a
// bare scheme name like "openai" (without "://") stored in config
// is normalized to a valid URI before resolution.
func TestResolvePromptLLM_BareScheme_NormalizesToURI(t *testing.T) {
	viper.Set("prompt.llm_provider", "openai")
	t.Setenv("OPENAI_API_KEY", "sk-test-key-for-tdd")
	t.Setenv("TLC_PROMPT_LLM", "")
	t.Setenv("LLM_PROVIDER", "")
	defer viper.Set("prompt.llm_provider", "")

	provider, err := resolvePromptLLM()
	if err != nil {
		// Currently fails: "missing scheme in URI "openai""
		t.Fatalf("resolvePromptLLM should handle bare scheme: %v", err)
	}
	defer provider.Close()
}

// TestResolvePromptLLM_EnvOverride_UsesLoadConfig verifies that
// TLC_PROMPT_LLM env var also goes through LoadConfig for key merge.
func TestResolvePromptLLM_EnvOverride_UsesLoadConfig(t *testing.T) {
	viper.Set("prompt.llm_provider", "")
	t.Setenv("TLC_PROMPT_LLM", "openai://gpt-4o")
	t.Setenv("OPENAI_API_KEY", "sk-test-key-for-tdd")
	t.Setenv("LLM_PROVIDER", "")

	provider, err := resolvePromptLLM()
	if err != nil {
		t.Fatalf("resolvePromptLLM with TLC_PROMPT_LLM failed: %v", err)
	}
	defer provider.Close()
}

// TestResolvePromptLLM_Anthropic_EnvKey verifies that anthropic
// adapter (which checks key at New()) picks up ANTHROPIC_API_KEY.
// This fails if resolvePromptLLM uses llm.Resolve instead of LoadConfig,
// because Resolve doesn't read env vars.
func TestResolvePromptLLM_Anthropic_EnvKey(t *testing.T) {
	viper.Set("prompt.llm_provider", "anthropic://claude-sonnet-4-20250514")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("TLC_PROMPT_LLM", "")
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("LLM_API_KEY", "")
	defer viper.Set("prompt.llm_provider", "")

	provider, err := resolvePromptLLM()
	if err != nil {
		// Anthropic New() requires API key. If this fails with
		// "API key required", LoadConfig isn't being used.
		t.Fatalf("resolvePromptLLM should pick up ANTHROPIC_API_KEY: %v", err)
	}
	defer provider.Close()
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
) ([]ResolvedCommand, string, error) {
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
		return nil, "", fmt.Errorf("llm completion failed: %w", err)
	}

	return parseRouterResponse(resp.Content)
}

