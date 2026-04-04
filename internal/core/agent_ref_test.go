package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- AgentRef YAML unmarshal ---

func TestAgentRefUnmarshalString(t *testing.T) {
	const doc = `
flow_id: "flow:ref-test:1.0"
name: "Ref Test"
version: "1.0"
entry_step: "only"
agent: claude
steps:
  only:
    step_id: "only"
    type: task
    title: "Only"
`
	f, err := ParseFlow(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow: %v", err)
	}
	if f.Agent.Name != "claude" {
		t.Errorf("Flow.Agent.Name = %q, want %q", f.Agent.Name, "claude")
	}
	if f.Agent.Config != nil {
		t.Errorf("Flow.Agent.Config = %v, want nil", f.Agent.Config)
	}
}

func TestAgentRefUnmarshalStruct(t *testing.T) {
	const doc = `
flow_id: "flow:ref-test:1.0"
name: "Ref Test"
version: "1.0"
entry_step: "only"
agent:
  name: gemini
  config:
    dir: /custom/gemini
steps:
  only:
    step_id: "only"
    type: task
    title: "Only"
`
	f, err := ParseFlow(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow: %v", err)
	}
	if f.Agent.Name != "gemini" {
		t.Errorf("Flow.Agent.Name = %q, want %q", f.Agent.Name, "gemini")
	}
	if f.Agent.Config["dir"] != "/custom/gemini" {
		t.Errorf("Flow.Agent.Config[dir] = %q, want %q", f.Agent.Config["dir"], "/custom/gemini")
	}
}

func TestStepAgentRefString(t *testing.T) {
	const doc = `
flow_id: "flow:ref-test:1.0"
name: "Ref Test"
version: "1.0"
entry_step: "only"
steps:
  only:
    step_id: "only"
    type: task
    title: "Only"
    agent: fabric
`
	f, err := ParseFlow(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow: %v", err)
	}
	if f.Steps["only"].Agent.Name != "fabric" {
		t.Errorf("Step.Agent.Name = %q, want %q", f.Steps["only"].Agent.Name, "fabric")
	}
}

func TestStepAgentRefStruct(t *testing.T) {
	const doc = `
flow_id: "flow:ref-test:1.0"
name: "Ref Test"
version: "1.0"
entry_step: "only"
steps:
  only:
    step_id: "only"
    type: task
    title: "Only"
    agent:
      name: llm
      config:
        dir: /custom/llm
`
	f, err := ParseFlow(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow: %v", err)
	}
	step := f.Steps["only"]
	if step.Agent.Name != "llm" {
		t.Errorf("Step.Agent.Name = %q, want %q", step.Agent.Name, "llm")
	}
	if step.Agent.Config["dir"] != "/custom/llm" {
		t.Errorf("Step.Agent.Config[dir] = %q, want %q", step.Agent.Config["dir"], "/custom/llm")
	}
}

func TestAdapterMappingAgentRefStruct(t *testing.T) {
	const doc = `
flow_id: "flow:ref-test:1.0"
name: "Ref Test"
version: "1.0"
entry_step: "only"
adapters:
  configs:
    gemini:
      dir: ~/.gemini
  mappings:
    - capabilities: ["planning"]
      agent:
        name: gemini
        config:
          dir: /work/.gemini
steps:
  only:
    step_id: "only"
    type: task
    title: "Only"
`
	f, err := ParseFlow(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow: %v", err)
	}
	if f.Adapters == nil {
		t.Fatal("Adapters is nil")
	}
	if len(f.Adapters.Mappings) != 1 {
		t.Fatalf("len(Mappings) = %d, want 1", len(f.Adapters.Mappings))
	}
	m := f.Adapters.Mappings[0]
	if m.Agent.Name != "gemini" {
		t.Errorf("Mapping.Agent.Name = %q, want %q", m.Agent.Name, "gemini")
	}
	if m.Agent.Config["dir"] != "/work/.gemini" {
		t.Errorf("Mapping.Agent.Config[dir] = %q, want %q", m.Agent.Config["dir"], "/work/.gemini")
	}
	if f.Adapters.Configs["gemini"]["dir"] != "~/.gemini" {
		t.Errorf("Adapters.Configs[gemini][dir] = %q, want %q",
			f.Adapters.Configs["gemini"]["dir"], "~/.gemini")
	}
}

func TestAdapterMappingAgentRefString(t *testing.T) {
	const doc = `
flow_id: "flow:ref-test:1.0"
name: "Ref Test"
version: "1.0"
entry_step: "only"
adapters:
  mappings:
    - capabilities: ["planning"]
      agent: codex
steps:
  only:
    step_id: "only"
    type: task
    title: "Only"
`
	f, err := ParseFlow(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow: %v", err)
	}
	if f.Adapters.Mappings[0].Agent.Name != "codex" {
		t.Errorf("Mapping.Agent.Name = %q, want codex", f.Adapters.Mappings[0].Agent.Name)
	}
}

// --- AgentRef JSON round-trip ---

func TestAgentRefJSONString(t *testing.T) {
	ref := AgentRef{Name: "claude"}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got AgentRef
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Name != "claude" {
		t.Errorf("got Name=%q, want claude", got.Name)
	}
}

func TestAgentRefJSONStruct(t *testing.T) {
	ref := AgentRef{Name: "gemini", Config: map[string]any{"dir": "/custom"}}
	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got AgentRef
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Name != "gemini" || got.Config["dir"] != "/custom" {
		t.Errorf("got %+v, want {gemini /custom}", got)
	}
}

// --- Backward compat: existing flows without agent fields still parse ---

func TestExistingFlowNoAgentFieldsUnchanged(t *testing.T) {
	const doc = `
flow_id: "flow:compat:1.0"
name: "Compat"
version: "1.0"
entry_step: "only"
steps:
  only:
    step_id: "only"
    type: task
    title: "Only"
`
	f, err := ParseFlow(strings.NewReader(doc), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow: %v", err)
	}
	if f.Agent.Name != "" {
		t.Errorf("Flow.Agent.Name = %q, want empty", f.Agent.Name)
	}
	if f.Steps["only"].Agent.Name != "" {
		t.Errorf("Step.Agent.Name = %q, want empty", f.Steps["only"].Agent.Name)
	}
}
