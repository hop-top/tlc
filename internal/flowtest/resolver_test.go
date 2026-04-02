package flowtest

import (
	"testing"

	"hop.top/tlc/internal/core"
)

// makeStep builds a minimal core.Step for resolver tests.
func makeStep(id, agent string, capabilities ...string) core.Step {
	s := core.Step{ID: id, Agent: core.AgentRef{Name: agent}, Type: core.StepTypeTask, Title: id}
	if len(capabilities) > 0 {
		s.TaskTemplate = &core.TaskTemplate{
			Title:        id,
			Requirements: &core.TaskRequirements{Capabilities: capabilities},
		}
	}
	return s
}

// makeStepWithTools builds a step with the given tools in task_template requirements.
func makeStepWithTools(id string, tools ...string) core.Step {
	return core.Step{
		ID:    id,
		Type:  core.StepTypeTask,
		Title: id,
		TaskTemplate: &core.TaskTemplate{
			Title:        id,
			Requirements: &core.TaskRequirements{Tools: tools},
		},
	}
}

// makeStepWithCapsAndTools builds a step with both capabilities and tools.
func makeStepWithCapsAndTools(id string, caps []string, tools []string) core.Step {
	return core.Step{
		ID:    id,
		Type:  core.StepTypeTask,
		Title: id,
		TaskTemplate: &core.TaskTemplate{
			Title:        id,
			Requirements: &core.TaskRequirements{Capabilities: caps, Tools: tools},
		},
	}
}

// makeStepWithConfig builds a step with an explicit agent config.
func makeStepWithConfig(id, agent string, cfg map[string]any) core.Step {
	return core.Step{
		ID:    id,
		Type:  core.StepTypeTask,
		Title: id,
		Agent: core.AgentRef{Name: agent, Config: cfg},
	}
}

// builtins returns a registry with all standard adapters.
func builtins() map[string]AgentAdapter {
	return map[string]AgentAdapter{
		"claude":   NewClaudeAdapter(),
		"gemini":   NewGeminiAdapter(),
		"fabric":   NewFabricAdapter(),
		"llm":      NewLLMAdapter(),
		"codex":    NewCodexAdapter(),
		"opencode": NewOpenCodeAdapter(),
	}
}

// resolve is a helper that asserts no error and returns adapter + config.
func resolve(t *testing.T, r *AdapterResolver, step core.Step) (AgentAdapter, map[string]any) {
	t.Helper()
	a, cfg, err := r.Resolve(step)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	return a, cfg
}

// --- Resolution step 1: step.Agent explicit ---

func TestResolverStepAgentExplicit(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Flow{Agent: core.AgentRef{Name: "claude"}}, nil)
	a, _ := resolve(t, r, makeStep("s", "gemini"))
	if a.Name() != "gemini" {
		t.Errorf("got %q, want gemini", a.Name())
	}
}

// --- Resolution step 2: flow.Agent ---

func TestResolverFlowAgentExplicit(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Flow{Agent: core.AgentRef{Name: "codex"}}, nil)
	a, _ := resolve(t, r, makeStep("s", ""))
	if a.Name() != "codex" {
		t.Errorf("got %q, want codex", a.Name())
	}
}

// --- Resolution step 3: flow.Adapters.Mappings capability match ---

func TestResolverFlowMappingCapabilityMatch(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Capabilities: []string{"requirements-gathering"}, Agent: core.AgentRef{Name: "gemini"}},
		{Capabilities: []string{"technical-writing"}, Agent: core.AgentRef{Name: "fabric"}},
	}}}
	r := NewAdapterResolver(builtins(), flow, nil)
	a, _ := resolve(t, r, makeStep("s", "", "requirements-gathering"))
	if a.Name() != "gemini" {
		t.Errorf("got %q, want gemini", a.Name())
	}
}

func TestResolverFlowMappingSecondRule(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Capabilities: []string{"requirements-gathering"}, Agent: core.AgentRef{Name: "gemini"}},
		{Capabilities: []string{"technical-writing"}, Agent: core.AgentRef{Name: "fabric"}},
	}}}
	r := NewAdapterResolver(builtins(), flow, nil)
	a, _ := resolve(t, r, makeStep("s", "", "technical-writing"))
	if a.Name() != "fabric" {
		t.Errorf("got %q, want fabric", a.Name())
	}
}

func TestResolverFlowMappingPartialIntersection(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Capabilities: []string{"planning"}, Agent: core.AgentRef{Name: "llm"}},
	}}}
	r := NewAdapterResolver(builtins(), flow, nil)
	a, _ := resolve(t, r, makeStep("s", "", "codebase-exploration", "planning", "architecture-analysis"))
	if a.Name() != "llm" {
		t.Errorf("got %q, want llm", a.Name())
	}
}

// --- Resolution step 3b: flow.Adapters.Mappings tools match ---

func TestResolverFlowMappingToolMatch(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Tools: []string{"bash"}, Agent: core.AgentRef{Name: "codex"}},
		{Tools: []string{"web-search"}, Agent: core.AgentRef{Name: "claude"}},
	}}}
	r := NewAdapterResolver(builtins(), flow, nil)
	a, _ := resolve(t, r, makeStepWithTools("s", "bash"))
	if a.Name() != "codex" {
		t.Errorf("got %q, want codex", a.Name())
	}
}

func TestResolverFlowMappingToolMatchSecondRule(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Tools: []string{"bash"}, Agent: core.AgentRef{Name: "codex"}},
		{Tools: []string{"web-search"}, Agent: core.AgentRef{Name: "claude"}},
	}}}
	r := NewAdapterResolver(builtins(), flow, nil)
	a, _ := resolve(t, r, makeStepWithTools("s", "web-search"))
	if a.Name() != "claude" {
		t.Errorf("got %q, want claude", a.Name())
	}
}

func TestResolverFlowMappingToolsAndCapsUnion(t *testing.T) {
	// mapping on tools; step has matching tool alongside unrelated cap
	flow := &core.Flow{Adapters: &core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Tools: []string{"bash"}, Agent: core.AgentRef{Name: "codex"}},
	}}}
	r := NewAdapterResolver(builtins(), flow, nil)
	step := makeStepWithCapsAndTools("s", []string{"planning"}, []string{"bash"})
	a, _ := resolve(t, r, step)
	if a.Name() != "codex" {
		t.Errorf("got %q, want codex", a.Name())
	}
}

func TestResolverFlowMappingCapsBeforeToolsInSameMapping(t *testing.T) {
	// both caps and tools in same mapping; cap matches first
	flow := &core.Flow{Adapters: &core.FlowAdapters{Mappings: []core.AdapterMapping{
		{
			Capabilities: []string{"planning"},
			Tools:        []string{"bash"},
			Agent:        core.AgentRef{Name: "llm"},
		},
	}}}
	r := NewAdapterResolver(builtins(), flow, nil)
	// step only has the tool
	a, _ := resolve(t, r, makeStepWithTools("s", "bash"))
	if a.Name() != "llm" {
		t.Errorf("got %q, want llm", a.Name())
	}
}

func TestResolverFlowMappingToolNoMatchFallsThrough(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{
		Default:  core.AgentRef{Name: "fabric"},
		Mappings: []core.AdapterMapping{{Tools: []string{"bash"}, Agent: core.AgentRef{Name: "codex"}}},
	}}
	r := NewAdapterResolver(builtins(), flow, nil)
	a, _ := resolve(t, r, makeStepWithTools("s", "other-tool"))
	if a.Name() != "fabric" {
		t.Errorf("got %q, want fabric (default)", a.Name())
	}
}

// --- Resolution step 4: global.Adapters.Mappings ---

func TestResolverGlobalMappingFallback(t *testing.T) {
	global := &GlobalAdapterConfig{Adapters: core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Capabilities: []string{"testing"}, Agent: core.AgentRef{Name: "codex"}},
	}}}
	r := NewAdapterResolver(builtins(), &core.Flow{}, global)
	a, _ := resolve(t, r, makeStep("s", "", "testing"))
	if a.Name() != "codex" {
		t.Errorf("got %q, want codex", a.Name())
	}
}

func TestResolverFlowMappingBeatsGlobal(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Capabilities: []string{"testing"}, Agent: core.AgentRef{Name: "llm"}},
	}}}
	global := &GlobalAdapterConfig{Adapters: core.FlowAdapters{Mappings: []core.AdapterMapping{
		{Capabilities: []string{"testing"}, Agent: core.AgentRef{Name: "codex"}},
	}}}
	r := NewAdapterResolver(builtins(), flow, global)
	a, _ := resolve(t, r, makeStep("s", "", "testing"))
	if a.Name() != "llm" {
		t.Errorf("got %q, want llm", a.Name())
	}
}

// --- Resolution step 5: flow.Adapters.Default ---

func TestResolverFlowAdaptersDefault(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{
		Default:  core.AgentRef{Name: "fabric"},
		Mappings: []core.AdapterMapping{{Capabilities: []string{"planning"}, Agent: core.AgentRef{Name: "llm"}}},
	}}
	r := NewAdapterResolver(builtins(), flow, nil)
	a, _ := resolve(t, r, makeStep("s", "", "something-else"))
	if a.Name() != "fabric" {
		t.Errorf("got %q, want fabric", a.Name())
	}
}

// --- Resolution step 6: global.Adapters.Default ---

func TestResolverGlobalAdaptersDefault(t *testing.T) {
	global := &GlobalAdapterConfig{Adapters: core.FlowAdapters{Default: core.AgentRef{Name: "opencode"}}}
	r := NewAdapterResolver(builtins(), &core.Flow{}, global)
	a, _ := resolve(t, r, makeStep("s", ""))
	if a.Name() != "opencode" {
		t.Errorf("got %q, want opencode", a.Name())
	}
}

// --- Resolution step 7: fatal error ---

func TestResolverFatalNoAdapter(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Flow{}, nil)
	_, _, err := r.Resolve(makeStep("my-step", ""))
	if err == nil {
		t.Fatal("Resolve() expected error, got nil")
	}
	if !contains(err.Error(), "my-step") {
		t.Errorf("error %q does not mention step id", err.Error())
	}
}

func TestResolverNoCapsNoMappingMatch(t *testing.T) {
	flow := &core.Flow{Adapters: &core.FlowAdapters{
		Default:  core.AgentRef{Name: "claude"},
		Mappings: []core.AdapterMapping{{Capabilities: []string{"planning"}, Agent: core.AgentRef{Name: "llm"}}},
	}}
	r := NewAdapterResolver(builtins(), flow, nil)
	a, _ := resolve(t, r, makeStep("s", ""))
	if a.Name() != "claude" {
		t.Errorf("got %q, want claude", a.Name())
	}
}

func TestResolverUnknownAdapterName(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Flow{}, nil)
	_, _, err := r.Resolve(makeStep("s", "unknown-agent"))
	if err == nil {
		t.Fatal("Resolve() expected error for unknown adapter, got nil")
	}
	if !contains(err.Error(), "unknown-agent") {
		t.Errorf("error %q does not mention adapter name", err.Error())
	}
}

// --- Config resolution chain ---

// Config step 1: step.Agent.Config wins over everything
func TestResolverConfigStepAgentConfig(t *testing.T) {
	flow := &core.Flow{
		Adapters: &core.FlowAdapters{
			Configs: map[string]map[string]any{
				"claude": {"dir": "/flow-level/.claude"},
			},
		},
	}
	step := makeStepWithConfig("s", "claude", map[string]any{"dir": "/step-level/.claude"})
	r := NewAdapterResolver(builtins(), flow, nil)
	_, cfg, err := r.Resolve(step)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg["dir"] != "/step-level/.claude" {
		t.Errorf("cfg[dir]=%q, want /step-level/.claude", cfg["dir"])
	}
}

// Config step 2: matched mapping Agent.Config (no step config)
func TestResolverConfigMappingAgentConfig(t *testing.T) {
	flow := &core.Flow{
		Adapters: &core.FlowAdapters{
			Configs: map[string]map[string]any{
				"llm": {"dir": "/flow-configs-level/llm"},
			},
			Mappings: []core.AdapterMapping{
				{
					Capabilities: []string{"planning"},
					Agent:        core.AgentRef{Name: "llm", Config: map[string]any{"dir": "/mapping-level/llm"}},
				},
			},
		},
	}
	r := NewAdapterResolver(builtins(), flow, nil)
	_, cfg, err := r.Resolve(makeStep("s", "", "planning"))
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg["dir"] != "/mapping-level/llm" {
		t.Errorf("cfg[dir]=%q, want /mapping-level/llm", cfg["dir"])
	}
}

// Config step 3: flow.Adapters.Configs[name] (no step or mapping config)
func TestResolverConfigFlowAdaptersConfigs(t *testing.T) {
	flow := &core.Flow{
		Agent: core.AgentRef{Name: "claude"},
		Adapters: &core.FlowAdapters{
			Configs: map[string]map[string]any{
				"claude": {"dir": "/flow-configs/.claude"},
			},
		},
	}
	global := &GlobalAdapterConfig{
		Adapters: core.FlowAdapters{
			Configs: map[string]map[string]any{
				"claude": {"dir": "/global-configs/.claude"},
			},
		},
	}
	r := NewAdapterResolver(builtins(), flow, global)
	_, cfg, err := r.Resolve(makeStep("s", ""))
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg["dir"] != "/flow-configs/.claude" {
		t.Errorf("cfg[dir]=%q, want /flow-configs/.claude", cfg["dir"])
	}
}

// Config step 4: global.Adapters.Configs[name] (no flow config)
func TestResolverConfigGlobalAdaptersConfigs(t *testing.T) {
	flow := &core.Flow{Agent: core.AgentRef{Name: "claude"}}
	global := &GlobalAdapterConfig{
		Adapters: core.FlowAdapters{
			Configs: map[string]map[string]any{
				"claude": {"dir": "/global-configs/.claude"},
			},
		},
	}
	r := NewAdapterResolver(builtins(), flow, global)
	_, cfg, err := r.Resolve(makeStep("s", ""))
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg["dir"] != "/global-configs/.claude" {
		t.Errorf("cfg[dir]=%q, want /global-configs/.claude", cfg["dir"])
	}
}

// Config step 5: nil when no config anywhere (adapter auto-detects)
func TestResolverConfigNilWhenNone(t *testing.T) {
	r := NewAdapterResolver(builtins(), &core.Flow{Agent: core.AgentRef{Name: "claude"}}, nil)
	_, cfg, err := r.Resolve(makeStep("s", ""))
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	if cfg != nil {
		t.Errorf("cfg=%v, want nil (auto-detect)", cfg)
	}
}

// --- helpers ---

func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
