package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var (
	taskExecAgentExpr      string
	taskExecAgentOverrides []string
	taskExecForce          bool
	taskExecWithPod        string
	taskExecTimeout        time.Duration
	taskExecDryRun         bool
	taskExecPrompt         string
	taskExecCtxtRefs       []string
	taskExecNoState        bool
	taskExecKeepPod        bool
	taskExecEnv            []string
	taskExecMounts         []string
	taskExecNetwork        string
	taskExecTrustProject   bool
)

// TaskExecCmd implements `tlc task execute <id>` (alias `exec`).
//
// The agent is resolved (in order) from --agent, the task's assignee, or
// the current user. Names are fuzzy-matched against the agent registry;
// ambiguity fails loudly with the candidate list. Reassigning a task
// requires --force.
//
// --agent accepts a composite expression: "name[:k=v[:k=v]...]". Inline
// k=v pairs override fields on the resolved AgentConfig. The same
// overrides can also be supplied repeatedly via -A/--agent-config; -A
// values are applied last, so they win on duplicate keys.
var TaskExecCmd = &cobra.Command{
	Use:     "execute <task-id>",
	Aliases: []string{"exec"},
	Short:   "Execute a task via an agent",
	Long: `Execute a task by dispatching it to an agent.

The agent is resolved from --agent, the task's assignee, or the current
user (in that order). Names fuzzy-match against the agent registry.
Reassigning a task to a different agent requires --force.

--agent supports a single inline config override via "name:key=value"
shorthand (the value can contain ':' — image refs work). For multiple
overrides, use the repeatable -A/--agent-config flag.

--with-pod controls execution mode: "false" runs locally, "true" runs in
the agent's default container, any other value is taken as a container
image reference.

With --recipe the task is the subject: the recipe's steps are created
in the task's track with the task blocked on the run's leaves, then run
through the task executor, which completes the task once every leaf is
done. --var binds the recipe's vars; --dry-run prints the steps that
would be created without writing or running anything.

Examples:
  tlc task execute T-0042
  tlc task execute T-0042 --recipe fix-flow --var branch=main
  tlc task execute T-0042 --agent claude
  tlc task execute T-0042 --agent claude:env.MODEL=opus
  tlc task execute T-0042 --agent claude:image=ghcr.io/me/agent:dev
  tlc task execute T-0042 --agent claude -A env.MODEL=opus -A default_timeout=10m
  tlc task execute T-0042 --agent claude --force
  tlc task execute T-0042 --with-pod false
  tlc task execute T-0042 --with-pod ghcr.io/me/agent:dev
  tlc task execute T-0042 --ctxt 'engineering?tag=runtime'
  tlc task execute T-0042 --dry-run`,
	Annotations: map[string]string{
		"kit/side-effect": "interactive",
		"kit/idempotent":  "no",
	},
	Args: cobra.ExactArgs(1),
	RunE: runTaskExec,
}

func init() {
	f := TaskExecCmd.Flags()
	f.StringVar(&taskExecAgentExpr, "agent", "",
		"Agent name with optional inline overrides (\"name[:k=v[:k=v]...]\"); defaults to task assignee or current user")
	f.StringArrayVarP(&taskExecAgentOverrides, "agent-config", "A", nil,
		"Agent config override (key=value, repeatable; same keys as inline --agent overrides)")
	f.BoolVar(&taskExecForce, "force", false,
		"Allow reassigning a task with an existing assignee")
	f.StringVar(&taskExecWithPod, "with-pod", "true",
		"Execution mode: \"true\" (default image), \"false\" (local), or a container image reference")
	f.DurationVar(&taskExecTimeout, "timeout", 30*time.Minute, "Timeout")
	f.BoolVar(&taskExecDryRun, "dry-run", false, "Print plan only")
	f.StringVar(&taskExecPrompt, "prompt", "", "Prompt override")
	f.StringSliceVar(&taskExecCtxtRefs, "ctxt", nil,
		"ctxt query handle (id[?filter], repeatable) the agent may query")
	f.BoolVar(&taskExecNoState, "no-state-update", false, "Skip state transitions")
	f.BoolVar(&taskExecKeepPod, "keep-pod", false, "Keep container after execution")
	f.StringSliceVar(&taskExecEnv, "env", nil, "Env var (KEY=VALUE)")
	f.StringSliceVar(&taskExecMounts, "mount", nil, "Bind mount")
	f.StringVar(&taskExecNetwork, "network", "", "Container network")
	f.BoolVar(&taskExecTrustProject, "trust-project", false,
		"Trust project-local agent config without prompting")
	registerRecipeRefFlags(TaskExecCmd)

	TaskCmd.AddCommand(TaskExecCmd)
}

// resetTaskExecFlags clears the package-level flag state. Called by the
// shared resetTaskFlags() test helper so a previous test's --agent /
// --force / --with-pod selection doesn't leak into unrelated tests.
func resetTaskExecFlags() {
	taskExecAgentExpr = ""
	taskExecAgentOverrides = nil
	taskExecForce = false
	taskExecWithPod = "true"
	taskExecDryRun = false
	taskExecPrompt = ""
	taskExecCtxtRefs = nil
	taskExecNoState = false
	taskExecKeepPod = false
	taskExecEnv = nil
	taskExecMounts = nil
	taskExecNetwork = ""
	taskExecTrustProject = false
}

// loadAgentRegistry loads the agent registry defaults, trusting the
// project-local config when asked.
func loadAgentRegistry(trustProject bool) (*core.AgentRegistry, error) {
	registry := core.NewAgentRegistry()
	if err := registry.LoadDefaults(); err != nil {
		return nil, fmt.Errorf("load agent config: %w", err)
	}
	if trustProject {
		registry.TrustProject(registry.ProjectConfigPath())
	}
	return registry, nil
}

// withPodMode parses the --with-pod value into (local bool, imageOverride
// string). "false"/"" → local; "true" → container with default image; any
// other value → container with the value as image reference.
func withPodMode(v string) (local bool, imageOverride string) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "false", "":
		return true, ""
	case "true":
		return false, ""
	default:
		return false, v
	}
}

func runTaskExec(cmd *cobra.Command, args []string) error {
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	taskID, err := parseTaskRefForCLI(ctx, s, args[0])
	if err != nil {
		return err
	}

	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task %s: %w", args[0], err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", args[0])
	}
	if recipeFlagRecipe != "" {
		return runTaskExecRecipe(ctx, cmd, s, task)
	}

	// 1) Resolve agent name + inline overrides from --agent, with
	// fall-throughs to the task's assignee, then current user.
	exprName, inlineOverrides := parseAgentExpr(taskExecAgentExpr)
	queryName := exprName
	if queryName == "" {
		if task.AssignedTo != nil && *task.AssignedTo != "" {
			queryName = *task.AssignedTo
		}
	}
	if queryName == "" {
		queryName = core.GetCurrentUser()
	}
	if queryName == "" {
		return fmt.Errorf(
			"cannot resolve agent: --agent not set, task has no assignee, and current user is unknown",
		)
	}

	// 2) Load registry + fuzzy-resolve the canonical agent name.
	registry, err := loadAgentRegistry(taskExecTrustProject)
	if err != nil {
		return err
	}

	resolvedAgent, err := resolveAgentName(registry, queryName)
	if err != nil {
		return err
	}

	// 3) Reassign guard. Fires only when an explicit --agent (or its
	// fall-through to current user) would *change* the existing
	// assignee. The default-agent chain's middle rung — task already
	// assigned to X, --agent unset → resolves to X — does NOT trip
	// the guard because in that path queryName == AssignedTo by
	// construction. That's intentional: the existing assignment is
	// treated as the user's prior consent for this agent. If you
	// don't trust pre-set assignments (e.g. from a track plan
	// generated upstream), audit them before calling exec — the
	// guard is not the place to enforce that.
	if task.AssignedTo != nil && *task.AssignedTo != "" && *task.AssignedTo != resolvedAgent && !taskExecForce {
		return fmt.Errorf(
			"task %s is assigned to %q; pass --force to reassign to %q",
			formatTaskAlias(task), *task.AssignedTo, resolvedAgent,
		)
	}

	// 4) Resolve & apply the AgentConfig overrides.
	agentCfg, err := registry.Get(resolvedAgent)
	if err != nil {
		if _, ok := err.(*core.ErrTrustRequired); ok {
			return fmt.Errorf(
				"%w; re-run with --trust-project to approve", err,
			)
		}
		return err
	}
	if err := applyAgentOverrides(agentCfg, inlineOverrides, taskExecAgentOverrides); err != nil {
		return err
	}

	// 5) Decode --with-pod into the existing local/image axes used by
	// the shared runner.
	local, imageOverride := withPodMode(taskExecWithPod)
	if imageOverride != "" {
		agentCfg.Image = imageOverride
	}

	if taskExecTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, taskExecTimeout)
		defer cancel()
	}

	// 6) Build context (with ctxt refs threaded through).
	builder := core.NewContextBuilder(s)
	ac, err := builder.BuildForTask(ctx, taskID, core.BuildOpts{
		PromptFile: taskExecPrompt,
		CtxtRefs:   taskExecCtxtRefs,
		RepoRoot:   repoRootForMode(local),
	})
	if err != nil {
		return err
	}

	if taskExecDryRun {
		return printTaskExecDryRun(cmd, task, resolvedAgent, agentCfg, ac, local, imageOverride)
	}

	// 7) Execute via the same runner used by `tlc track exec`. The
	// runner takes its own env/mount globals from agentRun*; thread
	// task-level values into them so the runner's mergeEnvVars and
	// parseMounts pick them up. agentRunAgent/agentRunLocal are
	// stashed inside taskExecForTrack itself.
	prevEnv, prevMounts, prevNetwork, prevKeep := agentRunEnv, agentRunMounts, agentRunNetwork, agentRunKeepPod
	agentRunEnv = taskExecEnv
	agentRunMounts = taskExecMounts
	agentRunNetwork = taskExecNetwork
	agentRunKeepPod = taskExecKeepPod
	defer func() {
		agentRunEnv, agentRunMounts, agentRunNetwork, agentRunKeepPod = prevEnv, prevMounts, prevNetwork, prevKeep
	}()

	taskSvc := core.NewTaskService(s, s)
	updater := core.NewStateUpdater(taskSvc, GetEventBus())

	return taskExecForTrack(
		ctx, cmd, resolvedAgent, taskID, formatTaskAlias(task), ac,
		agentCfg, s, updater,
		local, taskExecNoState,
	)
}

func printTaskExecDryRun(
	cmd *cobra.Command,
	task *core.Task,
	agentName string,
	cfg *core.AgentConfig,
	ac *core.AgentContext,
	local bool,
	imageOverride string,
) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "Dry run — task %s\n", formatTaskAlias(task))
	_, _ = fmt.Fprintf(out, "  Agent:    %s\n", agentName)
	if local {
		_, _ = fmt.Fprintf(out, "  Mode:     local\n")
	} else {
		image := cfg.Image
		if imageOverride != "" {
			image = imageOverride
		}
		_, _ = fmt.Fprintf(out, "  Mode:     container (%s)\n", image)
	}
	if cfg.DefaultTimeout > 0 {
		_, _ = fmt.Fprintf(out, "  Timeout:  %s\n", cfg.DefaultTimeout)
	}
	if len(cfg.Env) > 0 {
		_, _ = fmt.Fprintf(out, "  Env:      %v\n", cfg.Env)
	}
	if len(ac.CtxtRefs) > 0 {
		_, _ = fmt.Fprintf(out, "  Ctxt:     %v\n", ac.CtxtRefs)
	}
	if len(ac.Files) > 0 {
		_, _ = fmt.Fprintf(out, "  Files:    %v\n", ac.Files)
	}
	return nil
}

// taskExecForTrack runs a single task through agent execution. Shared
// by `tlc task exec` and `tlc track exec`.
//
// Callers MUST pass:
//   - ac: the prebuilt AgentContext (so per-call --ctxt/--prompt are
//     honored without leaking through globals; avoids a second
//     storage round-trip).
//   - taskDisplay: the human-readable alias to surface in user-facing
//     strings (callers already loaded the task to format it).
//   - agentName / local: forwarded both to the run record AND to
//     execute{Local,Container} via stashed agentRun* globals (those
//     helpers read agentRunAgent/agentRunLocal/repoRoot() and we
//     don't want to refactor agent_run.go in this PR).
func taskExecForTrack(
	ctx context.Context,
	cmd *cobra.Command,
	agentName string,
	taskID string,
	taskDisplay string,
	ac *core.AgentContext,
	cfg *core.AgentConfig,
	s interface {
		core.Repository
		core.LogRepository
	},
	updater *core.StateUpdater,
	local bool,
	noState bool,
) error {
	result, err := execTaskWithAgent(ctx, agentName, taskID, ac, cfg, updater, local)
	if err != nil {
		return fmt.Errorf("agent execution failed for %s: %w", taskDisplay, err)
	}
	if err := updater.Update(ctx, result, "task", taskID, core.UpdateOpts{
		NoStateUpdate: noState,
	}); err != nil {
		return fmt.Errorf("state update: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s (%s)\n",
		taskDisplay, result.Status, result.Summary)
	_ = os.Stdout.Sync()
	return nil
}

// execTaskWithAgent runs one task through the agent runtime and records
// the audit run, returning the agent's result without touching task
// state. taskExecForTrack applies the state update for `task execute`;
// the track executor applies its own.
func execTaskWithAgent(
	ctx context.Context,
	agentName string,
	taskID string,
	ac *core.AgentContext,
	cfg *core.AgentConfig,
	updater *core.StateUpdater,
	local bool,
) (*core.AgentResult, error) {
	// Stash/restore the agentRun* globals that execute{Local,Container}
	// + repoRoot() read. Without this, result.Agent ends up empty and
	// repoRoot() falls back to whatever the last `tlc agent run`
	// invocation left in the process state (or its zero value, which
	// for agentRunLocal=false routes through "/workspace" but never
	// gets the per-call mode flip). Restored on return so re-entrancy
	// from sequential calls is safe.
	prevAgent, prevLocal := agentRunAgent, agentRunLocal
	agentRunAgent = agentName
	agentRunLocal = local
	defer func() { agentRunAgent, agentRunLocal = prevAgent, prevLocal }()

	runID := uuid.New().String()
	record := &core.AgentRunRecord{
		ID:         runID,
		Agent:      agentName,
		TargetType: "task",
		TargetID:   taskID,
		StartedAt:  time.Now().UTC(),
		Status:     "running",
	}

	if err := updater.CreateRun(ctx, record); err != nil {
		return nil, fmt.Errorf("create audit record: %w", err)
	}

	contextJSON, err := json.Marshal(ac)
	if err != nil {
		return nil, fmt.Errorf("marshal context: %w", err)
	}

	collector := core.NewResultCollector()
	var result *core.AgentResult

	if local {
		result, err = executeLocal(ctx, cfg, ac, contextJSON, collector)
	} else {
		result, err = executeContainer(ctx, cfg, ac, contextJSON, record, collector)
	}
	if err != nil {
		failResult := &core.AgentResult{
			Version:  core.AgentResultVersion,
			Status:   core.AgentStatusFailed,
			ExitCode: 1,
			Summary:  err.Error(),
		}
		_ = updater.UpdateRun(ctx, runID, failResult, record)
		return nil, err
	}

	if err := updater.UpdateRun(ctx, runID, result, record); err != nil {
		return nil, fmt.Errorf("update audit: %w", err)
	}
	return result, nil
}
