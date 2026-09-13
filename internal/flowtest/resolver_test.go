package flowtest

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// step builds a StepRef for resolver tests; agent is the task's own agent.
func step(id, agent string) StepRef {
	return StepRef{ID: id, Title: id, Kind: "agent", Agent: agent}
}

// builtins returns a registry with all standard adapters.
func builtins() map[string]AgentAdapter {
	return map[string]AgentAdapter{
		"claude":     NewClaudeAdapter(),
		"gemini":     NewGeminiAdapter(),
		"fabric":     NewFabricAdapter(),
		"llm":        NewLLMAdapter(),
		"codex":      NewCodexAdapter(),
		"opencode":   NewOpenCodeAdapter(),
		"crewai":     NewCrewAIAdapter(),
		"langchain":  NewLangChainAdapter(),
		"google-adk": NewGoogleADKAdapter(),
		"n8n":        NewN8NAdapter(),
		"bedrock":    NewBedrockAdapter(),
	}
}

// resolve is a helper that asserts no error and returns adapter + config.
func resolve(t *testing.T, r *AdapterResolver, s StepRef) (AgentAdapter, map[string]any) {
	t.Helper()
	a, cfg, err := r.Resolve(s)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	return a, cfg
}

func globalDefault(name string) *GlobalAdapterConfig {
	return &GlobalAdapterConfig{Adapters: AdapterDefaults{Default: name}}
}

// --- Resolution step 1: the task's own agent ---

func TestResolverTaskAgentWins(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Recipe{Agent: "claude"}, globalDefault("codex"))
	a, _ := resolve(t, r, step("s", "gemini"))
	if a.Name() != "gemini" {
		t.Errorf("got %q, want gemini", a.Name())
	}
}

// --- Resolution step 2: recipe.Agent ---

func TestResolverRecipeAgent(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Recipe{Agent: "codex"}, globalDefault("fabric"))
	a, _ := resolve(t, r, step("s", ""))
	if a.Name() != "codex" {
		t.Errorf("got %q, want codex", a.Name())
	}
}

// --- Resolution step 3: global default ---

func TestResolverGlobalDefault(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Recipe{}, globalDefault("opencode"))
	a, _ := resolve(t, r, step("s", ""))
	if a.Name() != "opencode" {
		t.Errorf("got %q, want opencode", a.Name())
	}
}

func TestResolverNilRecipeFallsThroughToGlobal(t *testing.T) {
	r := NewAdapterResolver(builtins(), nil, globalDefault("llm"))
	a, _ := resolve(t, r, step("s", ""))
	if a.Name() != "llm" {
		t.Errorf("got %q, want llm", a.Name())
	}
}

// --- Resolution step 4: fatal ---

func TestResolverFatalNoAdapter(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Recipe{}, nil)
	_, _, err := r.Resolve(step("my-step", ""))
	if err == nil {
		t.Fatal("Resolve() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "my-step") {
		t.Errorf("error %q does not mention the step id", err.Error())
	}
}

func TestResolverUnknownAdapterName(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Recipe{}, nil)
	_, _, err := r.Resolve(step("s", "unknown-agent"))
	if err == nil {
		t.Fatal("Resolve() expected error for unknown adapter, got nil")
	}
	if !strings.Contains(err.Error(), "unknown-agent") {
		t.Errorf("error %q does not mention the adapter name", err.Error())
	}
}

func TestResolverUnknownRecipeAgentIsNotMaskedByGlobal(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Recipe{Agent: "nope"}, globalDefault("claude"))
	_, _, err := r.Resolve(step("s", ""))
	if err == nil {
		t.Fatal("Resolve() expected error: a named but unregistered agent must not fall through")
	}
}

// --- Config resolution chain ---

// The default adapter still reads its config from Configs, keyed by name.
func TestResolverConfigGlobalDefaultUsesConfigs(t *testing.T) {
	global := &GlobalAdapterConfig{Adapters: AdapterDefaults{
		Default: "claude",
		Configs: map[string]map[string]any{"claude": {"dir": "/configs/.claude"}},
	}}
	r := NewAdapterResolver(builtins(), &core.Recipe{}, global)
	_, cfg := resolve(t, r, step("s", ""))
	if cfg["dir"] != "/configs/.claude" {
		t.Errorf("cfg[dir]=%q, want /configs/.claude", cfg["dir"])
	}
}

// global.Adapters.Configs[name] for an adapter named elsewhere.
func TestResolverConfigGlobalConfigs(t *testing.T) {
	global := &GlobalAdapterConfig{Adapters: AdapterDefaults{
		Configs: map[string]map[string]any{"claude": {"dir": "/global-configs/.claude"}},
	}}
	r := NewAdapterResolver(builtins(), &core.Recipe{Agent: "claude"}, global)
	_, cfg := resolve(t, r, step("s", ""))
	if cfg["dir"] != "/global-configs/.claude" {
		t.Errorf("cfg[dir]=%q, want /global-configs/.claude", cfg["dir"])
	}
}

// Config step 3: nil when no config anywhere (adapter auto-detects).
func TestResolverConfigNilWhenNone(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Recipe{Agent: "claude"}, nil)
	_, cfg := resolve(t, r, step("s", ""))
	if cfg != nil {
		t.Errorf("cfg=%v, want nil (auto-detect)", cfg)
	}
}

// --- Operation ---

func TestResolverCtxReturnsAdapterOperation(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Recipe{Agent: "codex"}, nil)
	_, _, op, err := r.ResolveCtx(t.Context(), step("s", ""))
	if err != nil {
		t.Fatalf("ResolveCtx() error: %v", err)
	}
	if op != "exec" {
		t.Errorf("operation = %q, want exec (codex is exec-mode)", op)
	}
}
