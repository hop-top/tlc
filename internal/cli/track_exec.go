package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var (
	trackExecAgent        string
	trackExecLocal        bool
	trackExecTimeout      time.Duration
	trackExecDryRun       bool
	trackExecNoState      bool
	trackExecTrustProject bool
)

// trackExecCmd implements `tlc track exec <id> --agent <name>`.
var trackExecCmd = &cobra.Command{
	Use:   "exec <track-id>",
	Short: "Execute track tasks via an agent",
	Long: `Resolve linked TODO tasks in dependency order and execute
each sequentially via the named agent.

Examples:
  tlc track exec my-feature --agent claude
  tlc track exec my-feature --agent claude --local
  tlc track exec my-feature --agent claude --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if trackExecAgent == "" {
			return fmt.Errorf(
				"--agent is required; specify which agent to dispatch tasks to",
			)
		}

		// Load agent config.
		registry := core.NewAgentRegistry()
		if err := registry.LoadDefaults(); err != nil {
			return fmt.Errorf("load agent config: %w", err)
		}
		if trackExecTrustProject {
			registry.TrustProject(registry.ProjectConfigPath())
		}

		agentCfg, err := registry.Get(trackExecAgent)
		if err != nil {
			if _, ok := err.(*core.ErrTrustRequired); ok {
				return fmt.Errorf(
					"%w; re-run with --trust-project to approve", err,
				)
			}
			return err
		}

		// Open storage.
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		if trackExecTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, trackExecTimeout)
			defer cancel()
		}

		// Resolve TypeID/slug input to the canonical track TypeID
		// before downstream services (ContextBuilder, DepGraph) consume it.
		trackID, err := resolveTrackID(ctx, s, args[0])
		if err != nil {
			return err
		}

		// Resolve linked TODO tasks in dependency order.
		builder := core.NewContextBuilder(s)
		taskContexts, err := builder.BuildForTrack(ctx, trackID, core.BuildOpts{
			RepoRoot: repoRootForMode(trackExecLocal),
		})
		if err != nil {
			return err
		}

		if trackExecDryRun {
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Dry run — track %s\n", trackID)
			_, _ = fmt.Fprintf(out, "  Agent: %s\n", trackExecAgent)
			_, _ = fmt.Fprintf(out, "  Tasks: %d\n\n", len(taskContexts))
			for i, ac := range taskContexts {
				_, _ = fmt.Fprintf(out, "  %d. %s: %s\n",
					i+1, ac.TaskID, ac.TaskTitle)
			}
			return nil
		}

		// Execute tasks sequentially.
		taskSvc := core.NewTaskService(s, s)
		updater := core.NewStateUpdater(taskSvc, GetEventBus())

		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Executing track %s (%d tasks) via agent %s\n\n",
			trackID, len(taskContexts), trackExecAgent)

		for i, ac := range taskContexts {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[%d/%d] %s: %s\n",
				i+1, len(taskContexts), ac.TaskID, ac.TaskTitle)

			if err := taskExecForTrack(
				ctx, cmd, trackExecAgent, ac.TaskID,
				agentCfg, s, updater,
				trackExecLocal, trackExecNoState,
			); err != nil {
				return err
			}
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"\nTrack %s execution complete.\n", trackID)
		return nil
	},
}

func init() {
	f := trackExecCmd.Flags()
	f.StringVar(&trackExecAgent, "agent", "", "Agent name (required)")
	f.BoolVar(&trackExecLocal, "local", false, "Execute locally")
	f.DurationVar(&trackExecTimeout, "timeout", 0, "Total timeout")
	f.BoolVar(&trackExecDryRun, "dry-run", false, "Print plan only")
	f.BoolVar(&trackExecNoState, "no-state-update", false, "Skip state transitions")
	f.BoolVar(&trackExecTrustProject, "trust-project", false,
		"Trust project-local agent config without prompting")

	TrackCmd.AddCommand(trackExecCmd)
}
