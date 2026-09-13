package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"hop.top/tlc/internal/core"
)

// ContainerAgentRunner satisfies core.AgentRunner for flow execution.
type ContainerAgentRunner struct {
	AgentName string
	Registry  *core.AgentRegistry
	Local     bool
	Timeout   time.Duration
	EnvExtra  map[string]string
	Mounts    []core.MountSpec
	Network   string
	KeepPod   bool
	NoState   bool
}

// CanHandle reports whether the step has an agent reference that this
// runner can dispatch.
func (r *ContainerAgentRunner) CanHandle(step core.Step) bool {
	return step.Agent.Name != "" || r.AgentName != ""
}

// Run executes the agent for a flow step through the shared exec path
// and returns the step outputs.
func (r *ContainerAgentRunner) Run(
	ctx context.Context, step core.Step, prompt string,
) (map[string]any, error) {
	agentName := step.Agent.Name
	if agentName == "" {
		agentName = r.AgentName
	}

	cfg, err := r.Registry.Get(agentName)
	if err != nil {
		return nil, fmt.Errorf("agent runner: %w", err)
	}

	p := newExecParams(agentName, r.Local)
	p.mounts = r.Mounts
	p.env = envPairs(r.EnvExtra)
	p.network = r.Network
	p.keepPod = r.KeepPod

	ac := &core.AgentContext{
		Version:    core.AgentContextVersion,
		FlowStepID: step.ID,
		StepType:   string(step.Type),
		StepTitle:  step.Title,
		RepoRoot:   p.repoRoot,
		Prompt:     prompt,
	}
	record := &core.AgentRunRecord{ID: uuid.New().String(), Agent: agentName}

	result, err := executeAgent(ctx, p, cfg, ac, record)
	if err != nil {
		return nil, err
	}
	if result.Status == core.AgentStatusFailed {
		return nil, fmt.Errorf("agent %s failed: %s", agentName, result.Summary)
	}

	outputs := result.Outputs
	if outputs == nil {
		outputs = make(map[string]any)
	}
	outputs["status"] = string(result.Status)
	outputs["summary"] = result.Summary
	return outputs, nil
}

// envPairs flattens a map into KEY=VALUE overlays in key order.
func envPairs(m map[string]string) []string {
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	return pairs
}
