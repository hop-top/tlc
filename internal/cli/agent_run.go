package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/output"
	"hop.top/tlc/internal/core"
)

// Compile-time check: ContainerAgentRunner satisfies core.AgentRunner.
var _ core.AgentRunner = (*ContainerAgentRunner)(nil)

var (
	agentRunAgent        string
	agentRunTasks        []string
	agentRunFlow         string
	agentRunTrack        string
	agentRunImage        string
	agentRunMounts       []string
	agentRunEnv          []string
	agentRunTimeout      time.Duration
	agentRunTotalTimeout time.Duration
	agentRunDryRun       bool
	agentRunPrompt       string
	agentRunContext      []string
	agentRunNoState      bool
	agentRunKeepPod      bool
	agentRunLocal        bool
	agentRunAsync        bool
	agentRunRetries      int
	agentRunNetwork      string
	agentRunJSON         bool
)

// AgentRunCmd implements `tlc agent run`.
var AgentRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute an agent against tasks, a flow, or a track",
	Long: `Execute an agent in a container (or locally with --local).

The orchestration flow is:
  validate → load agent config → build context →
  create container (or local exec) → claim task →
  create audit record → upload context → exec agent →
  collect results → update state → print summary

Examples:
  tlc agent run --agent claude --task T-0042
  tlc agent run --agent claude --task T-0042 --task T-0043
  tlc agent run --agent claude --flow flow:example:1.0
  tlc agent run --agent claude --track my-feature
  tlc agent run --agent claude --task T-0042 --local
  tlc agent run --agent claude --task T-0042 --dry-run`,
	RunE: runAgentRun,
}

func init() {
	f := AgentRunCmd.Flags()
	f.StringVar(&agentRunAgent, "agent", "", "Agent name (required)")
	f.StringSliceVar(&agentRunTasks, "task", nil, "Task ID(s) (repeatable)")
	f.StringVar(&agentRunFlow, "flow", "", "Flow reference")
	f.StringVar(&agentRunTrack, "track", "", "Track ID")
	f.StringVar(&agentRunImage, "image", "", "Override container image")
	f.StringSliceVar(&agentRunMounts, "mount", nil, "Bind mount (source:target[:mode])")
	f.StringSliceVar(&agentRunEnv, "env", nil, "Environment variable (KEY=VALUE)")
	f.DurationVar(&agentRunTimeout, "timeout", 30*time.Minute, "Per-task timeout")
	f.DurationVar(&agentRunTotalTimeout, "total-timeout", 0, "Total execution timeout")
	f.BoolVar(&agentRunDryRun, "dry-run", false, "Print plan without executing")
	f.StringVar(&agentRunPrompt, "prompt", "", "Prompt file or inline text")
	f.StringSliceVar(&agentRunContext, "context", nil, "Extra context files")
	f.BoolVar(&agentRunNoState, "no-state-update", false, "Skip tlc state transitions")
	f.BoolVar(&agentRunKeepPod, "keep-pod", false, "Keep container after execution")
	f.BoolVar(&agentRunLocal, "local", false, "Execute locally (no container)")
	f.BoolVar(&agentRunAsync, "async", false, "Dispatch asynchronously")
	f.IntVar(&agentRunRetries, "retries", 0, "Retry count on failure")
	f.StringVar(&agentRunNetwork, "network", "", "Container network")
	f.BoolVar(&agentRunJSON, "json", false, "Output as JSON")

	_ = AgentRunCmd.MarkFlagRequired("agent")
}

func runAgentRun(cmd *cobra.Command, _ []string) error {
	if err := validateAgentRunFlags(); err != nil {
		return err
	}

	// Load agent config.
	registry := core.NewAgentRegistry()
	if err := registry.LoadDefaults(); err != nil {
		return fmt.Errorf("load agent config: %w", err)
	}

	agentCfg, err := registry.Get(agentRunAgent)
	if err != nil {
		// Auto-trust project config for interactive use.
		if _, ok := err.(*core.ErrTrustRequired); ok {
			registry.TrustProject(".tlc/agents.yaml")
			agentCfg, err = registry.Get(agentRunAgent)
		}
		if err != nil {
			return err
		}
	}

	// Apply image override.
	if agentRunImage != "" {
		agentCfg.Image = agentRunImage
	}

	// Dry-run: print plan and exit.
	if agentRunDryRun {
		return printDryRun(cmd, agentCfg)
	}

	// Async not yet supported (kit/job not available).
	if agentRunAsync {
		return fmt.Errorf(
			"--async requires kit/job which is not available; " +
				"run without --async for synchronous execution",
		)
	}

	// Open storage.
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if agentRunTotalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, agentRunTotalTimeout)
		defer cancel()
	}

	// Build context for each target.
	builder := core.NewContextBuilder(s)
	var contexts []*core.AgentContext
	var targetType, targetID string

	switch {
	case len(agentRunTasks) > 0:
		targetType = "task"
		for _, tid := range agentRunTasks {
			ac, err := builder.BuildForTask(ctx, tid, buildOpts())
			if err != nil {
				return err
			}
			contexts = append(contexts, ac)
		}
		targetID = agentRunTasks[0]

	case agentRunFlow != "":
		targetType = "flow"
		targetID = agentRunFlow
		// For flow, we build a single context with flow metadata.
		ac := &core.AgentContext{
			Version:  core.AgentContextVersion,
			FlowID:   agentRunFlow,
			RepoRoot: repoRoot(),
			Prompt:   agentRunPrompt,
		}
		if ac.Prompt == "" {
			ac.Prompt = fmt.Sprintf("Execute flow %s", agentRunFlow)
		}
		contexts = append(contexts, ac)

	case agentRunTrack != "":
		targetType = "track"
		targetID = agentRunTrack
		trackContexts, err := builder.BuildForTrack(ctx, agentRunTrack, buildOpts())
		if err != nil {
			return err
		}
		contexts = trackContexts
	}

	// Execute each context.
	taskSvc := core.NewTaskService(s, s)
	updater := core.NewStateUpdater(taskSvc, GetEventBus())

	for i, ac := range contexts {
		runID := uuid.New().String()
		record := &core.AgentRunRecord{
			ID:         runID,
			Agent:      agentRunAgent,
			TargetType: targetType,
			TargetID:   ac.TaskID,
			StartedAt:  time.Now().UTC(),
			Status:     "running",
		}
		if record.TargetID == "" {
			record.TargetID = targetID
		}

		if err := updater.CreateRun(ctx, record); err != nil {
			return fmt.Errorf("create audit record: %w", err)
		}

		result, err := executeAgent(ctx, agentCfg, ac, record)
		if err != nil {
			// Record failure.
			failResult := &core.AgentResult{
				Version:  core.AgentResultVersion,
				Status:   core.AgentStatusFailed,
				ExitCode: 1,
				Summary:  err.Error(),
			}
			_ = updater.UpdateRun(ctx, runID, failResult, record)
			return fmt.Errorf("agent execution failed: %w", err)
		}

		if err := updater.UpdateRun(ctx, runID, result, record); err != nil {
			return fmt.Errorf("update audit record: %w", err)
		}

		// Update task state.
		tid := ac.TaskID
		if tid == "" {
			tid = targetID
		}
		if err := updater.Update(ctx, result, targetType, tid, core.UpdateOpts{
			NoStateUpdate: agentRunNoState,
		}); err != nil {
			return fmt.Errorf("state update: %w", err)
		}

		printRunSummary(cmd, result, i+1, len(contexts))
	}

	return nil
}

func executeAgent(
	ctx context.Context,
	cfg *core.AgentConfig,
	ac *core.AgentContext,
	record *core.AgentRunRecord,
) (*core.AgentResult, error) {
	if agentRunTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, agentRunTimeout)
		defer cancel()
	}

	// Serialize context to temp file.
	contextJSON, err := json.Marshal(ac)
	if err != nil {
		return nil, fmt.Errorf("marshal agent context: %w", err)
	}

	collector := core.NewResultCollector()

	if agentRunLocal {
		return executeLocal(ctx, cfg, ac, contextJSON, collector)
	}
	return executeContainer(ctx, cfg, ac, contextJSON, record, collector)
}

func executeLocal(
	ctx context.Context,
	cfg *core.AgentConfig,
	ac *core.AgentContext,
	contextJSON []byte,
	collector *core.ResultCollector,
) (*core.AgentResult, error) {
	core.WarnIgnoredFlags(map[string]bool{
		"image":    agentRunImage != "",
		"mount":    len(agentRunMounts) > 0,
		"network":  agentRunNetwork != "",
		"keep-pod": agentRunKeepPod,
	})

	runner := &execRunner{}
	mgr := core.NewLocalExecManager(runner, "")

	// Write context file to repo root.
	contextPath := filepath.Join(repoRoot(), ".tlc", "context.json")
	if err := os.MkdirAll(filepath.Dir(contextPath), 0o750); err != nil {
		return nil, fmt.Errorf("create context dir: %w", err)
	}
	if err := os.WriteFile(contextPath, contextJSON, 0o600); err != nil {
		return nil, fmt.Errorf("write context file: %w", err)
	}

	binary := cfg.Binary
	if binary == "" {
		return nil, fmt.Errorf(
			"agent %q has no binary configured; set binary in agents.yaml "+
				"or use container mode (remove --local)",
			agentRunAgent,
		)
	}

	envVars := mergeEnvVars(cfg.Env)

	stdout, _, exitCode, err := mgr.Exec(ctx, core.LocalExecOpts{
		Binary:   binary,
		EnvVars:  envVars,
		RepoRoot: repoRoot(),
	})
	if err != nil {
		return nil, err
	}

	resultsPath := mgr.ResultsFilePath(repoRoot())
	result, err := collector.CollectFromExec(stdout, resultsPath)
	if err != nil {
		return &core.AgentResult{
			Version:  core.AgentResultVersion,
			Status:   core.AgentStatusFailed,
			ExitCode: exitCode,
			Summary:  fmt.Sprintf("result collection failed: %v", err),
		}, nil
	}
	result.ExitCode = exitCode
	result.Agent = agentRunAgent
	return result, nil
}

func executeContainer(
	ctx context.Context,
	cfg *core.AgentConfig,
	ac *core.AgentContext,
	contextJSON []byte,
	record *core.AgentRunRecord,
	collector *core.ResultCollector,
) (*core.AgentResult, error) {
	runner := &execRunner{}
	pod := core.NewPodShell(runner)

	if err := pod.CheckAvailable(ctx); err != nil {
		return nil, err
	}

	image := cfg.Image
	if image == "" {
		return nil, fmt.Errorf(
			"agent %q has no image configured; set image in agents.yaml "+
				"or use --local for local execution",
			agentRunAgent,
		)
	}

	mounts := parseMounts(agentRunMounts)
	envVars := mergeEnvVars(cfg.Env)

	podInfo, err := pod.Create(ctx, core.PodCreateOpts{
		Image:   image,
		Mounts:  mounts,
		EnvVars: envVars,
		Network: agentRunNetwork,
		Labels: map[string]string{
			"tlc.agent":  agentRunAgent,
			"tlc.run-id": record.ID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	record.ContainerID = podInfo.ID

	if !agentRunKeepPod {
		defer func() { _ = pod.Destroy(ctx, podInfo.Name) }()
	}

	// Write context to temp file and upload.
	tmpContext, err := os.CreateTemp("", "tlc-context-*.json")
	if err != nil {
		return nil, fmt.Errorf("create temp context file: %w", err)
	}
	defer func() { _ = os.Remove(tmpContext.Name()) }()

	if _, err := tmpContext.Write(contextJSON); err != nil {
		return nil, fmt.Errorf("write temp context: %w", err)
	}
	if err := tmpContext.Close(); err != nil {
		return nil, fmt.Errorf("close temp context: %w", err)
	}

	remotePath := "/workspace/.tlc/context.json"
	if err := pod.CopyTo(ctx, podInfo.Name, tmpContext.Name(), remotePath); err != nil {
		return nil, fmt.Errorf("upload context: %w", err)
	}
	if err := pod.VerifyFile(ctx, podInfo.Name, remotePath); err != nil {
		return nil, err
	}

	// Execute agent inside container.
	stdout, _, exitCode, err := pod.Exec(ctx, podInfo.Name, "agent-run")
	if err != nil {
		return nil, fmt.Errorf("agent exec: %w", err)
	}

	// Download results file if it exists.
	localResults := filepath.Join(os.TempDir(), fmt.Sprintf("tlc-results-%s.json", record.ID))
	defer func() { _ = os.Remove(localResults) }()
	_ = pod.CopyFrom(ctx, podInfo.Name, "/workspace/.tlc/results.json", localResults)

	result, err := collector.CollectFromExec(stdout, localResults)
	if err != nil {
		return &core.AgentResult{
			Version:  core.AgentResultVersion,
			Status:   core.AgentStatusFailed,
			ExitCode: exitCode,
			Summary:  fmt.Sprintf("result collection failed: %v", err),
		}, nil
	}
	result.ExitCode = exitCode
	result.Agent = agentRunAgent
	return result, nil
}

func validateAgentRunFlags() error {
	targets := 0
	if len(agentRunTasks) > 0 {
		targets++
	}
	if agentRunFlow != "" {
		targets++
	}
	if agentRunTrack != "" {
		targets++
	}

	if targets == 0 {
		return fmt.Errorf(
			"at least one target required; use --task, --flow, or --track",
		)
	}
	if targets > 1 && (agentRunFlow != "" || agentRunTrack != "") {
		return fmt.Errorf(
			"--flow and --track are mutually exclusive and cannot combine " +
				"with --task; use only one target type",
		)
	}
	return nil
}

func printDryRun(cmd *cobra.Command, cfg *core.AgentConfig) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "Dry run — no execution will occur\n\n")
	_, _ = fmt.Fprintf(out, "  Agent:   %s\n", agentRunAgent)
	_, _ = fmt.Fprintf(out, "  Image:   %s\n", cfg.Image)
	_, _ = fmt.Fprintf(out, "  Binary:  %s\n", cfg.Binary)
	_, _ = fmt.Fprintf(out, "  Local:   %v\n", agentRunLocal)
	_, _ = fmt.Fprintf(out, "  Timeout: %s\n", agentRunTimeout)

	if len(agentRunTasks) > 0 {
		_, _ = fmt.Fprintf(out, "  Tasks:   %v\n", agentRunTasks)
	}
	if agentRunFlow != "" {
		_, _ = fmt.Fprintf(out, "  Flow:    %s\n", agentRunFlow)
	}
	if agentRunTrack != "" {
		_, _ = fmt.Fprintf(out, "  Track:   %s\n", agentRunTrack)
	}
	if len(agentRunMounts) > 0 {
		_, _ = fmt.Fprintf(out, "  Mounts:  %v\n", agentRunMounts)
	}
	if len(agentRunEnv) > 0 {
		_, _ = fmt.Fprintf(out, "  Env:     %v\n", agentRunEnv)
	}
	return nil
}

func printRunSummary(cmd *cobra.Command, result *core.AgentResult, idx, total int) {
	out := cmd.OutOrStdout()
	format := viper.GetString("output.format")

	if agentRunJSON || format == formatJSON {
		_ = output.Render(out, formatJSON, result) //nolint:errcheck
		return
	}

	if total > 1 {
		_, _ = fmt.Fprintf(out, "\n[%d/%d] ", idx, total)
	} else {
		_, _ = fmt.Fprintln(out)
	}
	_, _ = fmt.Fprintf(out, "Agent: %s  Status: %s\n", result.Agent, result.Status)
	_, _ = fmt.Fprintf(out, "  Exit code: %d\n", result.ExitCode)
	_, _ = fmt.Fprintf(out, "  Summary:   %s\n", result.Summary)
	if len(result.Artifacts) > 0 {
		_, _ = fmt.Fprintf(out, "  Artifacts: %d files\n", len(result.Artifacts))
	}
}
