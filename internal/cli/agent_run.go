package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
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
	agentRunTrustProject bool
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

Container runs upload the context to /workspace/.tlc/context.json and
collect /workspace/.tlc/results.json. Local runs (--local) give each run
its own .tlc/runs/<run-id>/ directory under the repo root for the same
two files. In both modes the agent receives the paths as TLC_CONTEXT_PATH
and TLC_RESULTS_PATH.

Examples:
  tlc agent run --agent claude --task T-0042
  tlc agent run --agent claude --task T-0042 --task T-0043
  tlc agent run --agent claude --flow flow:example:1.0
  tlc agent run --agent claude --track my-feature
  tlc agent run --agent claude --task T-0042 --local
  tlc agent run --agent claude --task T-0042 --dry-run`,
	Annotations: map[string]string{
		"kit/side-effect": "interactive",
		"kit/idempotent":  "no",
	},
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
	f.StringVar(&agentRunPrompt, "prompt", "", "Inline prompt text")
	f.StringSliceVar(&agentRunContext, "context", nil, "Extra context files")
	f.BoolVar(&agentRunNoState, "no-state-update", false, "Skip tlc state transitions")
	f.BoolVar(&agentRunKeepPod, "keep-pod", false, "Keep container after execution")
	f.BoolVar(&agentRunLocal, "local", false, "Execute locally (no container)")
	f.BoolVar(&agentRunAsync, "async", false, "Dispatch asynchronously")
	// TODO: implement retry logic (currently unused)
	// f.IntVar(&agentRunRetries, "retries", 0, "Retry count on failure")
	f.StringVar(&agentRunNetwork, "network", "", "Container network")
	f.BoolVar(&agentRunJSON, "json", false, "Output as JSON")
	f.BoolVar(&agentRunTrustProject, "trust-project", false,
		"Trust project-local agent config without prompting")

	_ = AgentRunCmd.MarkFlagRequired("agent")
}

// agentRunParams builds the exec-path parameters from this command's
// flag bindings. RunE calls it once; the exec path never reads the
// agentRun* variables itself.
func agentRunParams() execParams {
	p := newExecParams(agentRunAgent, agentRunLocal)
	p.image = agentRunImage
	p.mounts = parseMounts(agentRunMounts)
	p.env = agentRunEnv
	p.network = agentRunNetwork
	p.keepPod = agentRunKeepPod
	p.timeout = agentRunTimeout
	return p
}

// resetAgentRunFlags restores the package-level flag state to the
// defaults declared in init. Tests that poke the bindings directly call
// it on the way out.
func resetAgentRunFlags() {
	agentRunAgent = ""
	agentRunTasks = nil
	agentRunFlow = ""
	agentRunTrack = ""
	agentRunImage = ""
	agentRunMounts = nil
	agentRunEnv = nil
	agentRunTimeout = 30 * time.Minute
	agentRunTotalTimeout = 0
	agentRunDryRun = false
	agentRunPrompt = ""
	agentRunContext = nil
	agentRunNoState = false
	agentRunKeepPod = false
	agentRunLocal = false
	agentRunAsync = false
	agentRunRetries = 0
	agentRunNetwork = ""
	agentRunJSON = false
	agentRunTrustProject = false
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

	if agentRunTrustProject {
		registry.TrustProject(registry.ProjectConfigPath())
	}

	agentCfg, err := registry.Get(agentRunAgent)
	if err != nil {
		if _, ok := err.(*core.ErrTrustRequired); ok {
			return fmt.Errorf(
				"%w; re-run with --trust-project to approve", err,
			)
		}
		return err
	}

	// Apply image override.
	if agentRunImage != "" {
		agentCfg.Image = agentRunImage
	}

	// Dry-run: print plan and exit.
	if agentRunDryRun {
		return printDryRun(cmd, agentCfg)
	}

	// Async: enqueue to local job queue and exit.
	if agentRunAsync {
		_, err := enqueueAgentJob(cmd)
		return err
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

	p := agentRunParams()

	// Build context for each target.
	builder := core.NewContextBuilder(s)
	var contexts []*core.AgentContext
	var targetType, targetID string

	switch {
	case len(agentRunTasks) > 0:
		targetType = "task"
		for _, tid := range agentRunTasks {
			ac, err := builder.BuildForTask(ctx, tid, buildOpts(p.repoRoot))
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
			RepoRoot: p.repoRoot,
			Prompt:   agentRunPrompt,
		}
		if ac.Prompt == "" {
			ac.Prompt = fmt.Sprintf("Execute flow %s", agentRunFlow)
		}
		contexts = append(contexts, ac)

	case agentRunTrack != "":
		targetType = "track"
		targetID = agentRunTrack
		trackContexts, err := builder.BuildForTrack(ctx, agentRunTrack, buildOpts(p.repoRoot))
		if err != nil {
			return err
		}
		contexts = trackContexts
	}

	// Execute each context.
	taskSvc := core.NewTaskService(s, s)
	updater := core.NewStateUpdater(taskSvc, GetEventBus()).WithRunStore(s)

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

		result, err := executeAgent(ctx, p, agentCfg, ac, record)
		if err != nil {
			_ = updater.UpdateRun(ctx, runID, failedResult(1, err.Error()), record) //nolint:errcheck // the exec error is returned; a failed audit write must not mask it
			return fmt.Errorf("agent execution failed: %w", err)
		}

		if err := updater.UpdateRun(ctx, runID, result, record); err != nil {
			return fmt.Errorf("update audit record: %w", err)
		}

		// Update task state. When running in --track mode, each
		// individual task uses targetType="task" so StateUpdater
		// applies the transition (it no-ops on "track").
		tid := ac.TaskID
		updateType := targetType
		if tid == "" {
			tid = targetID
		}
		if targetType == "track" && ac.TaskID != "" {
			updateType = "task"
		}
		if err := updater.Update(ctx, result, updateType, tid, core.UpdateOpts{
			NoStateUpdate: agentRunNoState,
		}); err != nil {
			return fmt.Errorf("state update: %w", err)
		}

		printRunSummary(cmd, result, i+1, len(contexts))
	}

	return nil
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
