package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var (
	taskExecAgent   string
	taskExecImage   string
	taskExecLocal   bool
	taskExecTimeout time.Duration
	taskExecDryRun  bool
	taskExecPrompt  string
	taskExecContext []string
	taskExecNoState bool
	taskExecKeepPod bool
	taskExecEnv     []string
	taskExecMounts  []string
	taskExecNetwork string
	taskExecJSON    bool
)

// TaskExecCmd implements `tlc task exec <id> --agent <name>`.
var TaskExecCmd = &cobra.Command{
	Use:   "exec <task-id>",
	Short: "Execute a task via an agent",
	Long: `Execute a task by dispatching it to a named agent.

Delegates to the agent run orchestration with --task <id>.

Examples:
  tlc task exec T-0042 --agent claude
  tlc task exec T-0042 --agent claude --local
  tlc task exec T-0042 --agent claude --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskID := args[0]
		if taskExecAgent == "" {
			return fmt.Errorf(
				"--agent is required; run 'tlc agent run --help' to see available options",
			)
		}

		// Wire up the agent run flags.
		agentRunAgent = taskExecAgent
		agentRunTasks = []string{taskID}
		agentRunFlow = ""
		agentRunTrack = ""
		agentRunImage = taskExecImage
		agentRunLocal = taskExecLocal
		agentRunTimeout = taskExecTimeout
		agentRunDryRun = taskExecDryRun
		agentRunPrompt = taskExecPrompt
		agentRunContext = taskExecContext
		agentRunNoState = taskExecNoState
		agentRunKeepPod = taskExecKeepPod
		agentRunEnv = taskExecEnv
		agentRunMounts = taskExecMounts
		agentRunNetwork = taskExecNetwork
		agentRunJSON = taskExecJSON
		agentRunAsync = false

		return runAgentRun(cmd, nil)
	},
}

func init() {
	f := TaskExecCmd.Flags()
	f.StringVar(&taskExecAgent, "agent", "", "Agent name (required)")
	f.StringVar(&taskExecImage, "image", "", "Override container image")
	f.BoolVar(&taskExecLocal, "local", false, "Execute locally")
	f.DurationVar(&taskExecTimeout, "timeout", 30*time.Minute, "Timeout")
	f.BoolVar(&taskExecDryRun, "dry-run", false, "Print plan only")
	f.StringVar(&taskExecPrompt, "prompt", "", "Prompt override")
	f.StringSliceVar(&taskExecContext, "context", nil, "Extra context files")
	f.BoolVar(&taskExecNoState, "no-state-update", false, "Skip state transitions")
	f.BoolVar(&taskExecKeepPod, "keep-pod", false, "Keep container")
	f.StringSliceVar(&taskExecEnv, "env", nil, "Env var (KEY=VALUE)")
	f.StringSliceVar(&taskExecMounts, "mount", nil, "Bind mount")
	f.StringVar(&taskExecNetwork, "network", "", "Container network")
	f.BoolVar(&taskExecJSON, "json", false, "Output as JSON")

	TaskCmd.AddCommand(TaskExecCmd)
}

// taskExecForTrack runs a single task through agent execution. Used by
// track exec to process tasks sequentially.
func taskExecForTrack(
	ctx context.Context,
	cmd *cobra.Command,
	agentName string,
	taskID string,
	cfg *core.AgentConfig,
	s interface {
		core.Repository
		core.LogRepository
	},
	updater *core.StateUpdater,
	local bool,
	noState bool,
) error {
	builder := core.NewContextBuilder(s)
	ac, err := builder.BuildForTask(ctx, taskID, core.BuildOpts{
		RepoRoot: repoRootForMode(local),
	})
	if err != nil {
		return err
	}

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
		return fmt.Errorf("create audit record: %w", err)
	}

	contextJSON, err := json.Marshal(ac)
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
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
		return fmt.Errorf("agent execution failed for %s: %w", taskID, err)
	}

	if err := updater.UpdateRun(ctx, runID, result, record); err != nil {
		return fmt.Errorf("update audit: %w", err)
	}
	if err := updater.Update(ctx, result, "task", taskID, core.UpdateOpts{
		NoStateUpdate: noState,
	}); err != nil {
		return fmt.Errorf("state update: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s (%s)\n",
		taskID, result.Status, result.Summary)
	_ = os.Stdout.Sync()
	return nil
}
