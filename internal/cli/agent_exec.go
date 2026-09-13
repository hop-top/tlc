package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hop.top/tlc/internal/core"
)

// Where a container run's context is uploaded and its results collected.
// Both are exported to the agent as core.EnvContextPath and
// core.EnvResultsPath, the same names a local run uses.
const (
	containerContextPath = "/workspace/.tlc/context.json"
	containerResultsPath = "/workspace/.tlc/results.json"
)

// execParams is everything one agent run needs from the command line. A
// command's RunE builds it once from its own flag bindings and threads it
// down; nothing on the exec path reads the agentRun* / taskExec*
// variables, so concurrent runs cannot observe each other's settings.
type execParams struct {
	agent    string             // resolved agent name: audit label and result.Agent
	local    bool               // run the binary on the host instead of in a container
	image    string             // --image override; only warned about in local mode
	mounts   []core.MountSpec   // container bind mounts
	env      []string           // KEY=VALUE overlays applied over the agent config env
	network  string             // container network
	keepPod  bool               // leave the container running after the run
	repoRoot string             // agent working directory: host cwd or /workspace
	runsDir  string             // local mode: per-run directories are created under here
	timeout  time.Duration      // per-run deadline; 0 leaves the caller's ctx in charge
	runner   core.CommandRunner // pod CLI runner; nil selects os/exec
}

// newExecParams returns params for agent in the given mode with the
// mode's repo root and, for local runs, the per-run directory root.
func newExecParams(agent string, local bool) execParams {
	root := repoRootForMode(local)
	return execParams{
		agent:    agent,
		local:    local,
		repoRoot: root,
		runsDir:  filepath.Join(root, ".tlc", "runs"),
	}
}

// runDir is where one local run's context and results files live.
func (p execParams) runDir(runID string) string {
	return filepath.Join(p.runsDir, runID)
}

func (p execParams) commandRunner() core.CommandRunner {
	if p.runner != nil {
		return p.runner
	}
	return &execRunner{}
}

// executeAgent runs one context in the mode p selects and returns the
// collected result.
func executeAgent(
	ctx context.Context,
	p execParams,
	cfg *core.AgentConfig,
	ac *core.AgentContext,
	record *core.AgentRunRecord,
) (*core.AgentResult, error) {
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	contextJSON, err := json.Marshal(ac)
	if err != nil {
		return nil, fmt.Errorf("marshal agent context: %w", err)
	}

	collector := core.NewResultCollector()
	if p.local {
		return executeLocal(ctx, p, cfg, contextJSON, record, collector)
	}
	return executeContainer(ctx, p, cfg, contextJSON, record, collector)
}

// executeLocal runs the agent binary on the host. Each run gets its own
// directory under p.runsDir holding context.json and results.json, and
// the binary learns both paths from its environment. The directory is
// removed once results are collected and kept when collection fails so
// the raw files can be inspected.
func executeLocal(
	ctx context.Context,
	p execParams,
	cfg *core.AgentConfig,
	contextJSON []byte,
	record *core.AgentRunRecord,
	collector *core.ResultCollector,
) (*core.AgentResult, error) {
	core.WarnIgnoredFlags(map[string]bool{
		"image":    p.image != "",
		"mount":    len(p.mounts) > 0,
		"network":  p.network != "",
		"keep-pod": p.keepPod,
	})
	if cfg.Binary == "" {
		return nil, fmt.Errorf(
			"agent %q has no binary configured; set binary in agents.yaml "+
				"or use container mode (remove --local)",
			p.agent,
		)
	}

	runDir := p.runDir(record.ID)
	if err := os.MkdirAll(runDir, 0o750); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	keepRunDir := false
	defer func() {
		if !keepRunDir {
			_ = os.RemoveAll(runDir)
		}
	}()
	contextPath := filepath.Join(runDir, "context.json")
	if err := os.WriteFile(contextPath, contextJSON, 0o600); err != nil {
		return nil, fmt.Errorf("write context file: %w", err)
	}

	mgr := core.NewLocalExecManager(p.commandRunner(), filepath.Join(runDir, "results.json"))
	stdout, _, exitCode, err := mgr.Exec(ctx, core.LocalExecOpts{
		Binary:      cfg.Binary,
		EnvVars:     mergeEnvVars(cfg.Env, p.env),
		RepoRoot:    p.repoRoot,
		ContextPath: contextPath,
	})
	if err != nil {
		return nil, fmt.Errorf("agent exec: %w", err)
	}

	result, err := collector.CollectFromExec(stdout, mgr.ResultsFilePath(p.repoRoot))
	if err != nil {
		keepRunDir = true
		return failedResult(exitCode, fmt.Sprintf("result collection failed: %v", err)), nil
	}
	result.ExitCode = exitCode
	result.Agent = p.agent
	return result, nil
}

// executeContainer runs the agent in a pod of its own: create, upload
// the context, exec agent-run, download results, destroy unless
// p.keepPod.
func executeContainer(
	ctx context.Context,
	p execParams,
	cfg *core.AgentConfig,
	contextJSON []byte,
	record *core.AgentRunRecord,
	collector *core.ResultCollector,
) (*core.AgentResult, error) {
	pod := core.NewPodShell(p.commandRunner())
	if err := pod.CheckAvailable(ctx); err != nil {
		return nil, fmt.Errorf("container runtime: %w", err)
	}
	if cfg.Image == "" {
		return nil, fmt.Errorf(
			"agent %q has no image configured; set image in agents.yaml "+
				"or use --local for local execution",
			p.agent,
		)
	}

	envVars := mergeEnvVars(cfg.Env, p.env)
	envVars[core.EnvContextPath] = containerContextPath
	envVars[core.EnvResultsPath] = containerResultsPath

	podInfo, err := pod.Create(ctx, core.PodCreateOpts{
		Image:   cfg.Image,
		Mounts:  p.mounts,
		EnvVars: envVars,
		Network: p.network,
		Labels: map[string]string{
			"tlc.agent":  p.agent,
			"tlc.run-id": record.ID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	record.ContainerID = podInfo.ID
	if !p.keepPod {
		// Best-effort teardown: the run outcome is decided by then and a
		// failed destroy must not replace it.
		defer func() { _ = pod.Destroy(ctx, podInfo.Name) }() //nolint:errcheck // best-effort teardown, outcome already decided
	}

	if err := uploadContext(ctx, pod, podInfo.Name, contextJSON); err != nil {
		return nil, err
	}

	stdout, _, exitCode, err := pod.Exec(ctx, podInfo.Name, "agent-run")
	if err != nil {
		return nil, fmt.Errorf("agent exec: %w", err)
	}

	// Download the results file if the agent wrote one; a missing file
	// is not an error, stdout alone can carry the status.
	localResults := filepath.Join(os.TempDir(), fmt.Sprintf("tlc-results-%s.json", record.ID))
	defer func() { _ = os.Remove(localResults) }()
	_ = pod.CopyFrom(ctx, podInfo.Name, containerResultsPath, localResults) //nolint:errcheck // no results file is legitimate; stdout carries the status

	result, err := collector.CollectFromExec(stdout, localResults)
	if err != nil {
		return failedResult(exitCode, fmt.Sprintf("result collection failed: %v", err)), nil
	}
	result.ExitCode = exitCode
	result.Agent = p.agent
	return result, nil
}

// uploadContext writes contextJSON to a temp file, copies it into the pod
// and verifies it landed.
func uploadContext(ctx context.Context, pod *core.PodShell, podName string, contextJSON []byte) error {
	tmp, err := os.CreateTemp("", "tlc-context-*.json")
	if err != nil {
		return fmt.Errorf("create temp context file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(contextJSON); err != nil {
		return fmt.Errorf("write temp context: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp context: %w", err)
	}
	if err := pod.CopyTo(ctx, podName, tmp.Name(), containerContextPath); err != nil {
		return fmt.Errorf("upload context: %w", err)
	}
	if err := pod.VerifyFile(ctx, podName, containerContextPath); err != nil {
		return fmt.Errorf("verify context upload: %w", err)
	}
	return nil
}

// failedResult is the result recorded when a run produced no usable
// result of its own.
func failedResult(exitCode int, summary string) *core.AgentResult {
	return &core.AgentResult{
		Version:  core.AgentResultVersion,
		Status:   core.AgentStatusFailed,
		ExitCode: exitCode,
		Summary:  summary,
	}
}
