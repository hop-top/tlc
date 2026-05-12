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
	trackExecCtxtRefs     []string
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
	Annotations: map[string]string{
		"kit/side-effect": "interactive",
		"kit/idempotent":  "no",
	},
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

		resolvedAgent, err := resolveAgentName(registry, trackExecAgent)
		if err != nil {
			return err
		}

		agentCfg, err := registry.Get(resolvedAgent)
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
			CtxtRefs: trackExecCtxtRefs,
			RepoRoot: repoRootForMode(trackExecLocal),
		})
		if err != nil {
			return err
		}

		// Build an alias lookup once so the loops below can show
		// T-NNNN instead of the durable typeid.
		taskSvc := core.NewTaskService(s, s)
		trackSvc := core.NewTrackService(s, s)
		trackDisplay := trackDisplayID(ctx, trackSvc, trackID)
		taskAlias := make(map[string]string, len(taskContexts))
		for _, ac := range taskContexts {
			if t, err := s.GetTask(ctx, ac.TaskID); err == nil && t != nil {
				taskAlias[ac.TaskID] = formatTaskAlias(t)
			} else {
				taskAlias[ac.TaskID] = ac.TaskID
			}
		}

		if trackExecDryRun {
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Dry run — track %s\n", trackDisplay)
			_, _ = fmt.Fprintf(out, "  Agent: %s\n", resolvedAgent)
			_, _ = fmt.Fprintf(out, "  Tasks: %d\n", len(taskContexts))
			if len(trackExecCtxtRefs) > 0 {
				_, _ = fmt.Fprintf(out, "  Ctxt:  %v\n", trackExecCtxtRefs)
			}
			_, _ = fmt.Fprintln(out)
			for i, ac := range taskContexts {
				_, _ = fmt.Fprintf(out, "  %d. %s: %s\n",
					i+1, taskAlias[ac.TaskID], ac.TaskTitle)
			}
			return nil
		}

		// Execute tasks sequentially.
		updater := core.NewStateUpdater(taskSvc, GetEventBus())

		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Executing track %s (%d tasks) via agent %s\n\n",
			trackDisplay, len(taskContexts), resolvedAgent)

		for i, ac := range taskContexts {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[%d/%d] %s: %s\n",
				i+1, len(taskContexts), taskAlias[ac.TaskID], ac.TaskTitle)

			if err := taskExecForTrack(
				ctx, cmd, resolvedAgent, ac.TaskID, taskAlias[ac.TaskID], ac,
				agentCfg, s, updater,
				trackExecLocal, trackExecNoState,
			); err != nil {
				return err
			}
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"\nTrack %s execution complete.\n", trackDisplay)
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
	f.StringSliceVar(&trackExecCtxtRefs, "ctxt", nil,
		"ctxt query handle (id[?filter], repeatable) the agent may query")

	TrackCmd.AddCommand(trackExecCmd)
}

// resetTrackExecFlags clears package-level flag state. Called by the
// shared resetTaskFlags() test helper.
func resetTrackExecFlags() {
	trackExecAgent = ""
	trackExecLocal = false
	trackExecTimeout = 0
	trackExecDryRun = false
	trackExecNoState = false
	trackExecTrustProject = false
	trackExecCtxtRefs = nil
}
