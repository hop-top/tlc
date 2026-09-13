package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"hop.top/tlc/internal/core"
)

var (
	trackExecuteAgent        string
	trackExecuteWithPod      string
	trackExecuteConcurrency  int
	trackExecutePermissive   bool
	trackExecuteReclaim      time.Duration
	trackExecuteWait         bool
	trackExecutePoll         time.Duration
	trackExecuteDryRun       bool
	trackExecuteTrustProject bool
	trackExecuteCtxtRefs     []string
	trackExecuteTimeout      time.Duration
)

// trackExecuteCmd implements `tlc track execute <id>` (alias `exec`).
var trackExecuteCmd = &cobra.Command{
	Use:     "execute <track-id>",
	Aliases: []string{"exec"},
	Short:   "Execute a track's ready tasks",
	Long: `Run a track through the task executor: readiness is recomputed
from blocked_by each round, ready tasks are claimed and dispatched by
kind (agent tasks to the named agent, exec tasks as their argv), and the
outcome is applied — done, retried, or blocked with a reason.

Only tasks created by a recipe run are dispatched unless --permissive is
set. Human tasks are never dispatched: when only human tasks remain
ready the command reports them and exits 0 (or keeps polling with
--wait); resolve them with 'tlc task approve|reject'.

Exec tasks run on the host by default. Pass --with-pod (true, or an
image) to run each in a fresh pod from that image, else the --agent's:
the working tree is copied to /workspace, exec.env is set at creation
and exec.cwd resolves under /workspace. The pod protocol cannot signal
the remote command nor cap its output at the source, so exec.timeout
only ends the wait (the command dies with the pod) and exec.stdout_max
cuts the output after it was transferred.

Examples:
  tlc track execute my-feature --agent claude
  tlc track execute my-feature --agent claude --concurrency 4
  tlc track execute my-feature --dry-run
  tlc track execute my-feature --with-pod=ghcr.io/org/tools:1   # exec tasks in a pod too
  tlc track execute my-feature --reclaim 30m     # take over claims older than 30m
  tlc track execute my-feature --wait --poll 30s`,
	Annotations: map[string]string{
		"kit/side-effect": "interactive",
		"kit/idempotent":  "no",
	},
	Args: cobra.ExactArgs(1),
	RunE: runTrackExecute,
}

func init() {
	f := trackExecuteCmd.Flags()
	f.StringVar(&trackExecuteAgent, "agent", "", "Agent for agent-kind tasks that name none")
	f.StringVar(&trackExecuteWithPod, "with-pod", "true",
		"Run agents in a container (true), locally (false), or in the given image; "+
			"when passed, exec tasks run in a container too")
	f.IntVar(&trackExecuteConcurrency, "concurrency", 1, "In-flight dispatches (exec tasks; agent tasks run one at a time)")
	f.BoolVar(&trackExecutePermissive, "permissive", false, "Also dispatch tasks not created by a recipe run")
	f.DurationVar(&trackExecuteReclaim, "reclaim", 0, "Re-dispatch tasks whose claim is older than this")
	f.BoolVar(&trackExecuteWait, "wait", false, "Keep polling while only human tasks are ready")
	f.DurationVar(&trackExecutePoll, "poll", 5*time.Second, "Poll interval for --wait")
	f.BoolVar(&trackExecuteDryRun, "dry-run", false, "Print the batch plan and the ready set only")
	f.BoolVar(&trackExecuteTrustProject, "trust-project", false,
		"Trust project-local agent config without prompting")
	f.StringSliceVar(&trackExecuteCtxtRefs, "ctxt", nil,
		"ctxt query handle (id[?filter], repeatable) the agent may query")
	f.DurationVar(&trackExecuteTimeout, "timeout", 0, "Total timeout")

	TrackCmd.AddCommand(trackExecuteCmd)
}

// resetTrackExecuteFlags clears package-level flag state. Called by the
// shared resetTaskFlags() test helper.
func resetTrackExecuteFlags() {
	trackExecuteAgent = ""
	trackExecuteWithPod = "true"
	trackExecuteConcurrency = 1
	trackExecutePermissive = false
	trackExecuteReclaim = 0
	trackExecuteWait = false
	trackExecutePoll = 5 * time.Second
	trackExecuteDryRun = false
	trackExecuteTrustProject = false
	trackExecuteCtxtRefs = nil
	trackExecuteTimeout = 0
	trackExecuteCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
}

func runTrackExecute(cmd *cobra.Command, args []string) error {
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := cmdContext(cmd)
	if trackExecuteTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, trackExecuteTimeout)
		defer cancel()
	}

	// resolveTrackID already fails loudly for an unknown track; the
	// executor scopes tasks to the same project the resolver used.
	trackID, err := resolveTrackID(ctx, s, args[0])
	if err != nil {
		return err
	}
	projectID := currentProjectID()

	registry := core.NewAgentRegistry()
	if err := registry.LoadDefaults(); err != nil {
		return fmt.Errorf("load agent config: %w", err)
	}
	if trackExecuteTrustProject {
		registry.TrustProject(registry.ProjectConfigPath())
	}
	local, imageOverride := withPodMode(trackExecuteWithPod)

	wm := core.DefaultWorkflow()
	out := cmd.OutOrStdout()
	opts := core.ExecutorOpts{
		Actor:       core.GetCurrentUser(),
		Concurrency: trackExecuteConcurrency,
		Permissive:  trackExecutePermissive,
		Reclaim:     trackExecuteReclaim,
		Wait:        trackExecuteWait,
		Poll:        trackExecutePoll,
		EvaKey:      os.Getenv("EVA_KEY"),
		Out:         out,
	}
	trackDisplay := trackDisplayID(ctx, core.NewTrackService(s, s), trackID)

	if trackExecuteDryRun {
		return printTrackExecuteDryRun(ctx, cmd, s, wm, trackDisplay, trackID, projectID, opts)
	}

	executor := core.NewExecutor(s, s, s, wm, opts)
	executor.Register(core.TaskKindAgent, &agentDispatcher{
		s:             s,
		registry:      registry,
		defaultAgent:  trackExecuteAgent,
		ctxtRefs:      trackExecuteCtxtRefs,
		local:         local,
		imageOverride: imageOverride,
		updater:       core.NewStateUpdater(core.NewTaskService(s, s), GetEventBus()),
	})
	execDispatcher, err := execDispatcherForMode(cmd, registry, local, imageOverride)
	if err != nil {
		return err
	}
	executor.Register(core.TaskKindExec, execDispatcher)

	_, _ = fmt.Fprintf(out, "Executing track %s\n", trackDisplay)
	report, err := executor.RunTrack(ctx, trackID, projectID)
	if err != nil {
		return fmt.Errorf("execute track %s: %w", trackDisplay, err)
	}
	printExecReport(ctx, out, s, trackDisplay, report)
	return nil
}

// printExecReport summarizes a run and names the human tasks that are
// waiting on a decision.
func printExecReport(ctx context.Context, out io.Writer, s core.Repository, trackDisplay string, report *core.ExecReport) {
	_, _ = fmt.Fprintf(out, "\nTrack %s: %d done, %d skipped, %d retried, %d blocked, %d reclaimed\n",
		trackDisplay, len(report.Done), len(report.Skipped), len(report.Failed), len(report.Blocked), len(report.Reclaimed))
	for _, id := range report.WaitingHuman {
		display := id
		if t, err := s.GetTask(ctx, id); err == nil && t != nil {
			display = formatTaskAlias(t) + " " + t.Title
		}
		_, _ = fmt.Fprintf(out, "⏸ waiting on %s (human); resolve with 'tlc task approve|reject %s'\n", display, id)
	}
}

// printTrackExecuteDryRun prints the batch plan and what the first round
// would dispatch, without claiming or running anything.
func printTrackExecuteDryRun(
	ctx context.Context, cmd *cobra.Command, s core.Repository, wm *core.WorkflowManager,
	trackDisplay, trackID, projectID string, opts core.ExecutorOpts,
) error {
	out := cmd.OutOrStdout()
	tasks, err := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "track_id", Operator: core.OpEq, Value: trackID},
			{Field: "project_id", Operator: core.OpEq, Value: projectID},
		},
		AllProjects:     true,
		IncludeArchived: true,
	})
	if err != nil {
		return fmt.Errorf("list track %s tasks: %w", trackDisplay, err)
	}
	graph, err := core.NewDepGraph(tasks)
	if err != nil {
		return fmt.Errorf("dependency graph of track %s: %w", trackDisplay, err)
	}
	strategy, err := graph.ComputeStrategy()
	if err != nil {
		return fmt.Errorf("execution strategy of track %s: %w", trackDisplay, err)
	}

	_, _ = fmt.Fprintf(out, "Dry run — track %s\n", trackDisplay)
	if trackExecuteAgent != "" {
		_, _ = fmt.Fprintf(out, "  Agent:       %s\n", trackExecuteAgent)
	}
	_, _ = fmt.Fprintf(out, "  Concurrency: %d\n", opts.Concurrency)
	if opts.Permissive {
		_, _ = fmt.Fprintf(out, "  Mode:        permissive (tasks without recipe provenance included)\n")
	}
	if len(trackExecuteCtxtRefs) > 0 {
		_, _ = fmt.Fprintf(out, "  Ctxt:        %v\n", trackExecuteCtxtRefs)
	}
	_, _ = fmt.Fprintf(out, "\nBatches (%d, max parallelism %d):\n", len(strategy.Batches), strategy.MaxParallelism)
	for _, batch := range strategy.Batches {
		marker := "  "
		if batch.Parallel {
			marker = "║ "
		}
		for _, t := range batch.Tasks {
			_, _ = fmt.Fprintf(out, "  batch %d %s%s %s [%s] %s\n",
				batch.Index+1, marker, formatTaskAlias(t), t.Title, t.Status, t.EffectiveKind())
		}
	}

	_, _ = fmt.Fprintln(out, "\nReady now:")
	ready := 0
	for _, batch := range strategy.Batches {
		for _, t := range batch.Tasks {
			if !dryRunReady(t, tasks, wm, opts.Permissive) {
				continue
			}
			ready++
			_, _ = fmt.Fprintf(out, "  %s %s (%s)\n", formatTaskAlias(t), t.Title, t.EffectiveKind())
		}
	}
	if ready == 0 {
		_, _ = fmt.Fprintln(out, "  (none)")
	}
	return nil
}

// dryRunReady mirrors the executor's readiness for display: initial
// status, no blocked reason, provenance in strict mode, blockers terminal.
func dryRunReady(t *core.Task, tasks []*core.Task, wm *core.WorkflowManager, permissive bool) bool {
	initial, err := wm.StatusForRole("initial")
	if err != nil || t.Status != initial {
		return false
	}
	if t.BlockedReason != nil && *t.BlockedReason != "" {
		return false
	}
	if !permissive && t.RunID == "" {
		return false
	}
	byID := make(map[string]*core.Task, len(tasks))
	for _, other := range tasks {
		byID[other.ID] = other
	}
	for _, dep := range t.BlockedBy() {
		if blocker, ok := byID[dep]; ok && !wm.IsTerminal(blocker.Status) {
			return false
		}
	}
	return true
}
