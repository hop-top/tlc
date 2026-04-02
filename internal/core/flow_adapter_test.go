package core

import (
	"strings"
	"testing"
)

const flowWithAdapters = `
flow_id: "flow:adapter-test:1.0"
name: "Adapter Test Flow"
version: "1.0"
entry_step: "step-a"
agent: claude

adapters:
  default: claude
  mappings:
    - capabilities: ["requirements-gathering"]
      agent: gemini
    - capabilities: ["technical-writing"]
      agent: fabric
    - tools: ["bash"]
      agent: codex

steps:
  step-a:
    step_id: "step-a"
    type: task
    title: "Step A"
    agent: gemini
    task_template:
      title: "Do A"
      requirements:
        capabilities: ["requirements-gathering"]
  step-b:
    step_id: "step-b"
    type: task
    title: "Step B"
    depends_on: ["step-a"]
    task_template:
      title: "Do B"
      requirements:
        capabilities: ["technical-writing"]
  step-c:
    step_id: "step-c"
    type: task
    title: "Step C"
    depends_on: ["step-b"]
    task_template:
      title: "Do C"
      requirements:
        tools: ["bash"]
`

func TestFlowAgentFieldParsed(t *testing.T) {
	f, err := ParseFlow(strings.NewReader(flowWithAdapters), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow() error: %v", err)
	}
	if f.Agent.Name != "claude" {
		t.Errorf("Flow.Agent.Name = %q, want %q", f.Agent.Name, "claude")
	}
}

func TestFlowAdaptersParsed(t *testing.T) {
	f, err := ParseFlow(strings.NewReader(flowWithAdapters), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow() error: %v", err)
	}
	if f.Adapters == nil {
		t.Fatal("Flow.Adapters is nil, want non-nil")
	}
	if f.Adapters.Default.Name != "claude" {
		t.Errorf("Adapters.Default.Name = %q, want %q", f.Adapters.Default.Name, "claude")
	}
	if len(f.Adapters.Mappings) != 3 {
		t.Fatalf("len(Adapters.Mappings) = %d, want 3", len(f.Adapters.Mappings))
	}
	if f.Adapters.Mappings[0].Agent.Name != "gemini" {
		t.Errorf("Mappings[0].Agent.Name = %q, want %q", f.Adapters.Mappings[0].Agent.Name, "gemini")
	}
	if len(f.Adapters.Mappings[0].Capabilities) != 1 ||
		f.Adapters.Mappings[0].Capabilities[0] != "requirements-gathering" {
		t.Errorf("Mappings[0].Capabilities = %v, want [requirements-gathering]",
			f.Adapters.Mappings[0].Capabilities)
	}
	if f.Adapters.Mappings[1].Agent.Name != "fabric" {
		t.Errorf("Mappings[1].Agent.Name = %q, want %q", f.Adapters.Mappings[1].Agent.Name, "fabric")
	}
}

func TestFlowAdaptersMappingToolsParsed(t *testing.T) {
	f, err := ParseFlow(strings.NewReader(flowWithAdapters), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow() error: %v", err)
	}
	m := f.Adapters.Mappings[2]
	if m.Agent.Name != "codex" {
		t.Errorf("Mappings[2].Agent.Name = %q, want codex", m.Agent.Name)
	}
	if len(m.Tools) != 1 || m.Tools[0] != "bash" {
		t.Errorf("Mappings[2].Tools = %v, want [bash]", m.Tools)
	}
	if len(m.Capabilities) != 0 {
		t.Errorf("Mappings[2].Capabilities = %v, want empty", m.Capabilities)
	}
}

func TestStepToolsParsed(t *testing.T) {
	f, err := ParseFlow(strings.NewReader(flowWithAdapters), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow() error: %v", err)
	}
	stepC := f.Steps["step-c"]
	if stepC.TaskTemplate == nil || stepC.TaskTemplate.Requirements == nil {
		t.Fatal("step-c TaskTemplate.Requirements is nil")
	}
	if len(stepC.TaskTemplate.Requirements.Tools) != 1 ||
		stepC.TaskTemplate.Requirements.Tools[0] != "bash" {
		t.Errorf("step-c Requirements.Tools = %v, want [bash]",
			stepC.TaskTemplate.Requirements.Tools)
	}
}

func TestStepAgentFieldParsed(t *testing.T) {
	f, err := ParseFlow(strings.NewReader(flowWithAdapters), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow() error: %v", err)
	}
	stepA := f.Steps["step-a"]
	if stepA.Agent.Name != "gemini" {
		t.Errorf("step-a Agent.Name = %q, want %q", stepA.Agent.Name, "gemini")
	}
	stepB := f.Steps["step-b"]
	if stepB.Agent.Name != "" {
		t.Errorf("step-b Agent.Name = %q, want empty (inherit from flow)", stepB.Agent.Name)
	}
}

func TestFlowWithoutAdaptersStillParses(t *testing.T) {
	const minimal = `
flow_id: "flow:minimal:1.0"
name: "Minimal"
version: "1.0"
entry_step: "only"
steps:
  only:
    step_id: "only"
    type: task
    title: "Only step"
`
	f, err := ParseFlow(strings.NewReader(minimal), "test.yaml")
	if err != nil {
		t.Fatalf("ParseFlow() error: %v", err)
	}
	if f.Agent.Name != "" {
		t.Errorf("Flow.Agent.Name = %q, want empty", f.Agent.Name)
	}
	if f.Adapters != nil {
		t.Errorf("Flow.Adapters = %v, want nil", f.Adapters)
	}
}
