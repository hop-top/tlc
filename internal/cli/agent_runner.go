package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Run executes the agent for a flow step and returns the step outputs.
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

	ac := &core.AgentContext{
		Version:    core.AgentContextVersion,
		FlowStepID: step.ID,
		StepType:   string(step.Type),
		StepTitle:  step.Title,
		RepoRoot:   "/workspace",
		Prompt:     prompt,
	}

	contextJSON, err := json.Marshal(ac)
	if err != nil {
		return nil, fmt.Errorf("marshal context: %w", err)
	}

	collector := core.NewResultCollector()

	var result *core.AgentResult
	if r.Local {
		result, err = executeLocalForRunner(ctx, cfg, contextJSON, collector)
	} else {
		record := &core.AgentRunRecord{
			ID:    uuid.New().String(),
			Agent: agentName,
		}
		result, err = executeContainerForRunner(
			ctx, cfg, contextJSON, record, collector,
			r.Mounts, r.EnvExtra, r.Network, r.KeepPod,
		)
	}
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

func executeLocalForRunner(
	ctx context.Context,
	cfg *core.AgentConfig,
	contextJSON []byte,
	collector *core.ResultCollector,
) (*core.AgentResult, error) {
	runner := &execRunner{}
	mgr := core.NewLocalExecManager(runner, "")

	cwd, _ := os.Getwd()
	contextPath := filepath.Join(cwd, ".tlc", "context.json")
	if err := os.MkdirAll(filepath.Dir(contextPath), 0o750); err != nil {
		return nil, fmt.Errorf("create context dir: %w", err)
	}
	if err := os.WriteFile(contextPath, contextJSON, 0o600); err != nil {
		return nil, fmt.Errorf("write context: %w", err)
	}

	stdout, _, exitCode, err := mgr.Exec(ctx, core.LocalExecOpts{
		Binary:   cfg.Binary,
		EnvVars:  cfg.Env,
		RepoRoot: cwd,
	})
	if err != nil {
		return nil, err
	}

	result, err := collector.CollectFromExec(stdout, mgr.ResultsFilePath(cwd))
	if err != nil {
		return &core.AgentResult{
			Version:  core.AgentResultVersion,
			Status:   core.AgentStatusFailed,
			ExitCode: exitCode,
			Summary:  err.Error(),
		}, nil
	}
	result.ExitCode = exitCode
	return result, nil
}

func executeContainerForRunner(
	ctx context.Context,
	cfg *core.AgentConfig,
	contextJSON []byte,
	record *core.AgentRunRecord,
	collector *core.ResultCollector,
	mounts []core.MountSpec,
	envExtra map[string]string,
	network string,
	keepPod bool,
) (*core.AgentResult, error) {
	runner := &execRunner{}
	pod := core.NewPodShell(runner)

	if err := pod.CheckAvailable(ctx); err != nil {
		return nil, err
	}

	env := make(map[string]string, len(cfg.Env)+len(envExtra))
	for k, v := range cfg.Env {
		env[k] = v
	}
	for k, v := range envExtra {
		env[k] = v
	}

	podInfo, err := pod.Create(ctx, core.PodCreateOpts{
		Image:   cfg.Image,
		Mounts:  mounts,
		EnvVars: env,
		Network: network,
		Labels: map[string]string{
			"tlc.agent":  record.Agent,
			"tlc.run-id": record.ID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	if !keepPod {
		defer func() { _ = pod.Destroy(ctx, podInfo.Name) }()
	}

	tmpContext, err := os.CreateTemp("", "tlc-context-*.json")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpContext.Name()) }()
	if _, err := tmpContext.Write(contextJSON); err != nil {
		return nil, fmt.Errorf("write context file: %w", err)
	}
	_ = tmpContext.Close()

	if err := pod.CopyTo(ctx, podInfo.Name, tmpContext.Name(), "/workspace/.tlc/context.json"); err != nil {
		return nil, err
	}

	stdout, _, exitCode, err := pod.Exec(ctx, podInfo.Name, "agent-run")
	if err != nil {
		return nil, err
	}

	localResults := filepath.Join(
		os.TempDir(),
		fmt.Sprintf("tlc-results-%s.json", record.ID),
	)
	defer func() { _ = os.Remove(localResults) }()
	_ = pod.CopyFrom(ctx, podInfo.Name, "/workspace/.tlc/results.json", localResults)

	result, err := collector.CollectFromExec(stdout, localResults)
	if err != nil {
		return &core.AgentResult{
			Version:  core.AgentResultVersion,
			Status:   core.AgentStatusFailed,
			ExitCode: exitCode,
			Summary:  err.Error(),
		}, nil
	}
	result.ExitCode = exitCode
	return result, nil
}
