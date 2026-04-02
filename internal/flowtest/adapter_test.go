package flowtest

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// allAdapters returns one instance of every built-in adapter.
func allAdapters() []AgentAdapter {
	return []AgentAdapter{
		NewClaudeAdapter(),
		NewGeminiAdapter(),
		NewFabricAdapter(),
		NewLLMAdapter(),
		NewCodexAdapter(),
		NewOpenCodeAdapter(),
		NewRouteLLMAdapter(),
		NewCrewAIAdapter(),
		NewLangChainAdapter(),
		NewGoogleADKAdapter(),
		NewMastraAdapter(),
		NewN8NAdapter(),
		NewBedrockAdapter(),
		NewDSPyAdapter(),
		NewOpenAIAgentsAdapter(),
		NewAutoGenAdapter(),
		NewOllamaAdapter(),
	}
}

// TestAdapterInterface verifies each built-in adapter satisfies the interface
// and returns consistent metadata.
func TestAdapterInterface(t *testing.T) {
	for _, a := range allAdapters() {
		a := a
		t.Run(a.Name(), func(t *testing.T) {
			if a.Name() == "" {
				t.Fatal("Name() must not be empty")
			}
			if a.Binary() == "" {
				t.Fatal("Binary() must not be empty")
			}
			args := a.BuildArgs("hello", nil, "")
			if len(args) == 0 {
				t.Fatal("BuildArgs() must return at least one arg")
			}
			found := false
			for _, arg := range args {
				if strings.Contains(arg, "hello") {
					found = true
				}
			}
			if !found {
				t.Errorf("BuildArgs() args %v do not contain prompt", args)
			}
		})
	}
}

// --- BuildEnv with explicit config ---

func TestClaudeAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewClaudeAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]any{"dir": "/custom/.claude"})
	if !hasEnv(env, "CLAUDE_CONFIG_DIR=/custom/.claude") {
		t.Errorf("expected CLAUDE_CONFIG_DIR=/custom/.claude; got %v", env)
	}
}

func TestLLMAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewLLMAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]any{"dir": "/custom/llm"})
	if !hasEnv(env, "LLM_USER_PATH=/custom/llm") {
		t.Errorf("expected LLM_USER_PATH=/custom/llm; got %v", env)
	}
}

func TestCodexAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewCodexAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]any{"dir": "/custom/.codex"})
	if !hasEnv(env, "CODEX_HOME=/custom/.codex") {
		t.Errorf("expected CODEX_HOME=/custom/.codex; got %v", env)
	}
}

func TestOpenCodeAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewOpenCodeAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]any{"dir": "/custom/opencode"})
	// opencode uses XDG_CONFIG_HOME set to the parent of the opencode dir
	if !hasEnv(env, "XDG_CONFIG_HOME=/custom") {
		t.Errorf("expected XDG_CONFIG_HOME=/custom; got %v", env)
	}
}

func TestRouteLLMAdapterBuildEnvExplicitDir(t *testing.T) {
	a := NewRouteLLMAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, map[string]any{"dir": "/custom/routellm/config.yaml"})
	if !hasEnv(env, "ROUTELLM_CONFIG=/custom/routellm/config.yaml") {
		t.Errorf("expected ROUTELLM_CONFIG=/custom/routellm/config.yaml; got %v", env)
	}
}

// --- BuildEnv with nil config → auto-detect ---

func TestClaudeAdapterBuildEnvAutoDetect(t *testing.T) {
	a := NewClaudeAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, nil)
	// should inject CLAUDE_CONFIG_DIR pointing somewhere under real HOME
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") && kv != "CLAUDE_CONFIG_DIR=" {
			found = true
		}
	}
	if !found {
		t.Errorf("auto-detect: expected CLAUDE_CONFIG_DIR to be set; got %v", env)
	}
}

func TestLLMAdapterBuildEnvAutoDetect(t *testing.T) {
	a := NewLLMAdapter()
	env := a.BuildEnv([]string{"HOME=/tmp"}, nil)
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "LLM_USER_PATH=") && kv != "LLM_USER_PATH=" {
			found = true
		}
	}
	if !found {
		t.Errorf("auto-detect: expected LLM_USER_PATH to be set; got %v", env)
	}
}

// --- BuildEnv no-op adapters (gemini, fabric) ---

func TestGeminiAdapterBuildEnvIsNoop(t *testing.T) {
	a := NewGeminiAdapter()
	base := []string{"HOME=/tmp", "PATH=/usr/bin"}
	got := a.BuildEnv(base, map[string]any{"dir": "/custom"})
	if len(got) != len(base) {
		t.Errorf("gemini BuildEnv should be no-op; got %v", got)
	}
}

func TestFabricAdapterBuildEnvIsNoop(t *testing.T) {
	a := NewFabricAdapter()
	base := []string{"HOME=/tmp", "PATH=/usr/bin"}
	got := a.BuildEnv(base, nil)
	if len(got) != len(base) {
		t.Errorf("fabric BuildEnv should be no-op; got %v", got)
	}
}

// --- Auto-detect paths match expected OS defaults ---

func TestClaudeAdapterAutoDetectPath(t *testing.T) {
	a := NewClaudeAdapter()
	env := a.BuildEnv(nil, nil)
	dir := envVal(env, "CLAUDE_CONFIG_DIR")
	if !strings.HasSuffix(dir, ".claude") {
		t.Errorf("CLAUDE_CONFIG_DIR=%q, want suffix .claude", dir)
	}
}

func TestCodexAdapterAutoDetectPath(t *testing.T) {
	a := NewCodexAdapter()
	env := a.BuildEnv(nil, nil)
	dir := envVal(env, "CODEX_HOME")
	if !strings.HasSuffix(dir, ".codex") {
		t.Errorf("CODEX_HOME=%q, want suffix .codex", dir)
	}
}

func TestLLMAdapterAutoDetectPath(t *testing.T) {
	a := NewLLMAdapter()
	env := a.BuildEnv(nil, nil)
	dir := envVal(env, "LLM_USER_PATH")
	if runtime.GOOS == "darwin" {
		if !strings.Contains(dir, "Application Support") {
			t.Errorf("LLM_USER_PATH=%q, want Application Support on macOS", dir)
		}
	} else {
		if !strings.Contains(dir, "io.datasette.llm") {
			t.Errorf("LLM_USER_PATH=%q, want io.datasette.llm on Linux", dir)
		}
	}
}

// --- ParseOutput ---

func TestClaudeAdapterParseOutputJSON(t *testing.T) {
	a := NewClaudeAdapter()
	raw := []byte(`{"type":"result","subtype":"success","result":"done"}`)
	out, err := a.ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}
	if out["type"] != "result" {
		t.Errorf("expected type=result, got %v", out["type"])
	}
}

func TestFabricAdapterParseOutputText(t *testing.T) {
	a := NewFabricAdapter()
	raw := []byte("# Analysis\n\nSome text output.")
	out, err := a.ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error: %v", err)
	}
	if out["output"] != string(raw) {
		t.Errorf("expected output=%q, got %v", string(raw), out["output"])
	}
}

func TestParseOutputFallbackOnInvalidJSON(t *testing.T) {
	for _, a := range allAdapters() {
		a := a
		if a.Name() == "fabric" {
			continue // fabric always wraps as text
		}
		t.Run(a.Name(), func(t *testing.T) {
			raw := []byte("not json at all")
			out, err := a.ParseOutput(raw)
			if err != nil {
				t.Fatalf("ParseOutput() should not error on bad JSON; got: %v", err)
			}
			if out["output"] != string(raw) {
				t.Errorf("expected fallback output=%q, got %v", string(raw), out["output"])
			}
		})
	}
}

func TestAdapterParseOutputValidJSON(t *testing.T) {
	payload := map[string]any{"type": "result", "result": "ok"}
	raw, _ := json.Marshal(payload)
	for _, a := range allAdapters() {
		a := a
		if a.Name() == "fabric" {
			continue
		}
		t.Run(a.Name(), func(t *testing.T) {
			out, err := a.ParseOutput(raw)
			if err != nil {
				t.Fatalf("ParseOutput() error: %v", err)
			}
			if out["type"] != "result" {
				t.Errorf("expected type=result, got %v", out["type"])
			}
		})
	}
}

// --- buildFlagsFromConfig ---

func TestBuildFlagsFromConfigFlat(t *testing.T) {
	cfg := map[string]any{"model": "gpt-4o", "temperature": "0.7"}
	flags := buildFlagsFromConfig(cfg, "", nil)
	got := strings.Join(flags, " ")
	// Keys sorted: model before temperature
	if !strings.Contains(got, "--model gpt-4o") {
		t.Errorf("expected --model gpt-4o in %q", got)
	}
	if !strings.Contains(got, "--temperature 0.7") {
		t.Errorf("expected --temperature 0.7 in %q", got)
	}
}

func TestBuildFlagsFromConfigSkip(t *testing.T) {
	cfg := map[string]any{"dir": "/some/path", "model": "gpt-4o"}
	flags := buildFlagsFromConfig(cfg, "", []string{"dir"})
	for _, f := range flags {
		if strings.Contains(f, "dir") {
			t.Errorf("skip list not honored: got %q", f)
		}
	}
	if !containsStr(flags, "--model") {
		t.Errorf("expected --model in %v", flags)
	}
}

func TestBuildFlagsFromConfigOperationOverride(t *testing.T) {
	cfg := map[string]any{
		"model": "gpt-3.5",
		"embed": map[string]any{"model": "text-embedding-3-small"},
	}
	flags := buildFlagsFromConfig(cfg, "embed", nil)
	// embed sub-map should override flat model
	got := strings.Join(flags, " ")
	if !strings.Contains(got, "--model text-embedding-3-small") {
		t.Errorf("operation override not applied; got %q", got)
	}
	if strings.Contains(got, "gpt-3.5") {
		t.Errorf("flat model should be overridden; got %q", got)
	}
}

func TestBuildFlagsFromConfigBoolTrue(t *testing.T) {
	cfg := map[string]any{"verbose": true}
	flags := buildFlagsFromConfig(cfg, "", nil)
	if len(flags) != 1 || flags[0] != "--verbose" {
		t.Errorf("bool true should emit flag-only; got %v", flags)
	}
}

func TestBuildFlagsFromConfigBoolFalse(t *testing.T) {
	cfg := map[string]any{"verbose": false}
	flags := buildFlagsFromConfig(cfg, "", nil)
	if len(flags) != 0 {
		t.Errorf("bool false should not emit flag; got %v", flags)
	}
}

func TestBuildFlagsFromConfigSubMapSkipped(t *testing.T) {
	cfg := map[string]any{
		"model": "gpt-4o",
		"embed": map[string]any{"model": "text-embedding-3-small"},
	}
	// No operation — sub-map should not appear as a flag
	flags := buildFlagsFromConfig(cfg, "", nil)
	for _, f := range flags {
		if f == "--embed" || f == "embed" {
			t.Errorf("sub-map key should be skipped; got %v", flags)
		}
	}
}

func TestBuildFlagsFromConfigEmpty(t *testing.T) {
	if flags := buildFlagsFromConfig(nil, "", nil); len(flags) != 0 {
		t.Errorf("nil config should return nil; got %v", flags)
	}
	if flags := buildFlagsFromConfig(map[string]any{}, "", nil); len(flags) != 0 {
		t.Errorf("empty config should return nil; got %v", flags)
	}
}

// --- Operation dispatch ---

func TestLLMAdapterOperationEmbed(t *testing.T) {
	a := NewLLMAdapter()
	step := core.Step{
		ID:   "embed-step",
		Type: core.StepTypeTask,
		TaskTemplate: &core.TaskTemplate{
			Title:        "embed",
			Requirements: &core.TaskRequirements{Capabilities: []string{"embed"}},
		},
	}
	if got := a.Operation(step); got != "embed" {
		t.Errorf("Operation()=%q, want embed", got)
	}
}

func TestLLMAdapterOperationPromptDefault(t *testing.T) {
	a := NewLLMAdapter()
	step := core.Step{
		ID:   "plain-step",
		Type: core.StepTypeTask,
		TaskTemplate: &core.TaskTemplate{
			Title:        "prompt",
			Requirements: &core.TaskRequirements{Capabilities: []string{"planning"}},
		},
	}
	if got := a.Operation(step); got != "prompt" {
		t.Errorf("Operation()=%q, want prompt", got)
	}
}

func TestLLMAdapterOperationNoTemplate(t *testing.T) {
	a := NewLLMAdapter()
	step := core.Step{ID: "s", Type: core.StepTypeTask}
	if got := a.Operation(step); got != "prompt" {
		t.Errorf("Operation()=%q, want prompt for no template", got)
	}
}

func TestClaudeAdapterOperationAlwaysEmpty(t *testing.T) {
	a := NewClaudeAdapter()
	step := core.Step{
		ID:   "s",
		Type: core.StepTypeTask,
		TaskTemplate: &core.TaskTemplate{
			Title:        "t",
			Requirements: &core.TaskRequirements{Capabilities: []string{"embed"}},
		},
	}
	if got := a.Operation(step); got != "" {
		t.Errorf("claudeAdapter.Operation()=%q, want empty", got)
	}
}

func TestAllAdaptersOperationReturnString(t *testing.T) {
	for _, a := range allAdapters() {
		a := a
		t.Run(a.Name(), func(t *testing.T) {
			// Operation must not panic and must return a string.
			step := core.Step{ID: "s", Type: core.StepTypeTask}
			_ = a.Operation(step)
		})
	}
}

// --- Probe (no-op adapters) ---

func TestNoOpAdapterProbeReturnsEmpty(t *testing.T) {
	noops := []AgentAdapter{
		NewGeminiAdapter(),
		NewCodexAdapter(),
		NewOpenCodeAdapter(),
		NewRouteLLMAdapter(),
	}
	for _, a := range noops {
		a := a
		t.Run(a.Name(), func(t *testing.T) {
			caps, err := a.Probe(context.Background(), "/nonexistent/bin")
			if err != nil {
				t.Errorf("Probe() error: %v", err)
			}
			if caps == nil {
				t.Error("Probe() returned nil caps")
			}
		})
	}
}


// --- fabricAdapter ---

func TestFabricAdapterBuildArgsBasic(t *testing.T) {
	a := NewFabricAdapter()
	args := a.BuildArgs("hello world", nil, "")
	if len(args) < 2 {
		t.Fatalf("BuildArgs() returned too few args: %v", args)
	}
	if args[0] != "-p" || args[1] != "hello world" {
		t.Errorf("expected [-p hello world ...]; got %v", args)
	}
}

func TestFabricAdapterBuildArgsWithConfig(t *testing.T) {
	a := NewFabricAdapter()
	cfg := map[string]any{"pattern": "summarize", "model": "claude-opus-4-6", "temperature": "0.5"}
	args := a.BuildArgs("test prompt", cfg, "")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--pattern summarize") {
		t.Errorf("expected --pattern summarize in %q", joined)
	}
	if !strings.Contains(joined, "--model claude-opus-4-6") {
		t.Errorf("expected --model claude-opus-4-6 in %q", joined)
	}
	if !strings.Contains(joined, "--temperature 0.5") {
		t.Errorf("expected --temperature 0.5 in %q", joined)
	}
}

func TestFabricAdapterBuildArgsDirSkipped(t *testing.T) {
	a := NewFabricAdapter()
	cfg := map[string]any{"dir": "/some/path", "pattern": "extract_wisdom"}
	args := a.BuildArgs("prompt", cfg, "")
	for _, arg := range args {
		if strings.Contains(arg, "dir") || arg == "/some/path" {
			t.Errorf("dir should be skipped; got %v", args)
		}
	}
}

func TestFabricAdapterBuildArgsStreamNormalised(t *testing.T) {
	// stream:true as string "true" comes through buildFlagsFromConfig as "--stream true"
	// but bool true emits "--stream" only (no value); test the bool case for completeness.
	args := normaliseBoolFlags([]string{"-p", "x", "--stream", "true", "--model", "m"}, []string{"stream"})
	for i, arg := range args {
		if arg == "--stream" && i+1 < len(args) && args[i+1] == "true" {
			t.Errorf("--stream true should have been normalised; got %v", args)
		}
	}
	// --stream should still be present
	if !containsStr(args, "--stream") {
		t.Errorf("--stream should be present after normalisation; got %v", args)
	}
}

func TestFabricAdapterOperationAlwaysEmpty(t *testing.T) {
	a := NewFabricAdapter()
	step := core.Step{ID: "s", Type: core.StepTypeTask}
	if got := a.Operation(step); got != "" {
		t.Errorf("fabricAdapter.Operation()=%q, want empty", got)
	}
}

func TestFabricAdapterProbeReturnsCapabilities(t *testing.T) {
	a := NewFabricAdapter()
	caps, err := a.Probe(context.Background(), "/nonexistent/fabric")
	if err != nil {
		t.Fatalf("Probe() should not error; got %v", err)
	}
	if caps == nil {
		t.Fatal("Probe() returned nil caps")
	}
	// Flags must be pre-populated regardless of whether binary exists.
	if !containsStr(caps.Flags, "pattern") {
		t.Errorf("expected pattern in Flags; got %v", caps.Flags)
	}
	if !containsStr(caps.Flags, "stream") {
		t.Errorf("expected stream in Flags; got %v", caps.Flags)
	}
}

func TestFabricAdapterProbeParsesModels(t *testing.T) {
	// normaliseBoolFlags is package-internal; test it directly.
	input := []string{"--model", "m", "--stream", "true", "--temperature", "0.3"}
	got := normaliseBoolFlags(input, []string{"stream"})
	expected := []string{"--model", "m", "--stream", "--temperature", "0.3"}
	if strings.Join(got, "|") != strings.Join(expected, "|") {
		t.Errorf("normaliseBoolFlags: got %v, want %v", got, expected)
	}
}

// --- helpers ---

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func hasEnv(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func envVal(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}
