package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/uri"
)

var (
	flowRunBy     string
	flowStatusAll bool
	flowRunVars   []string
)

var FlowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Flow execution and management",
	Long: `Execute and manage task flows.

Flows are declarative workflows that orchestrate task execution with
control flow semantics like parallel, sequential, branching, and retry.`,
}

var FlowRunCmd = &cobra.Command{
	Use:   "run <flow-file>",
	Short: "Execute a flow definition",
	Long: `Execute a flow definition from a YAML or JSON file.

The flow file defines a workflow with steps that reference tasks.
Steps can be executed sequentially, in parallel, with branching logic,
retries, and other control flow patterns.

Use --dry-run to preview the execution plan without dispatching agents.

Example flow file (flow.yaml):
  flow_id: "flow:example:1.0"
  name: "Example Flow"
  version: "1.0"
  entry_step: "step1"
  steps:
    step1:
      step_id: "step1"
      type: "task"
      title: "First task"
      task_ref: "T-0001"
    step2:
      step_id: "step2"
      type: "task"
      title: "Second task"
      task_ref: "T-0002"
      depends_on: ["step1"]

Usage:
  tlc flow run flow.yaml
  tlc flow run flow.yaml --dry-run
  tlc flow run tlc://hop-top/tlc/flow:example:1.0
  tlc flow run tests/fixtures/parallel-flow.yaml`,
	Annotations: map[string]string{
		"kit/side-effect": "interactive",
		"kit/idempotent":  "no",
	},
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		flowRef := args[0]

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		resolver := uri.NewResolver(s)
		resolver.FlowsDir = flowsDirFromConfig()
		res, err := resolver.ResolveFlow(ctx, flowRef)
		if err != nil {
			return err
		}
		flow := res.Flow

		if flowDryRun {
			return dryRunFlow(cmd.OutOrStdout(), flow)
		}

		providedVars, err := core.ParseFlowVarFlags(flowRunVars)
		if err != nil {
			return err
		}
		inputs, err := core.ResolveFlowInputs(flow, providedVars)
		if err != nil {
			return err
		}

		executor := core.NewFlowExecutor(s, s).
			WithEvaKey(os.Getenv("EVA_KEY")).
			WithInputs(inputs).
			WithApprovalStore(approvalStore())
		if runner := buildFlowAgentRunner(); runner != nil {
			executor = executor.WithAgentRunner(runner)
		}

		by := flowRunBy
		if by == "" {
			by = core.GetCurrentUser()
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Starting flow: %s (ID: %s)\n", flow.Name, flow.ID)
		run, _, err := executor.Execute(ctx, flow, by)
		if err != nil {
			return fmt.Errorf("flow execution failed: %w", err)
		}

		// Post-execution recap (table-style detail). Humanise
		// Started/Ended ("2m ago"). T-1384.
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nFlow execution completed!\n")
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Run ID: %s\n", run.ID)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Status: %s\n", run.Status)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Started: %s\n", DisplayTimeRelative(run.StartedAt))
		if run.EndedAt != nil {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Ended: %s\n", DisplayTimePtrRelative(run.EndedAt))
			duration := run.EndedAt.Sub(run.StartedAt)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Duration: %s\n", duration)
		}

		return nil
	},
}

var FlowStatusCmd = &cobra.Command{
	Use:   "status [run-id]",
	Short: "View flow run status",
	Long: `View the status of a flow run or list all flow runs.

View specific run:
  tlc flow status run:abc123

List all runs:
  tlc flow status --all

Output formats:
  tlc flow status run:abc123 --format json
  tlc flow status --all --format table`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()

		if len(args) > 0 {
			runID := args[0]
			run, err := s.GetFlowRun(ctx, runID)
			if err != nil {
				return fmt.Errorf("failed to get flow run: %w", err)
			}
			if run == nil {
				return fmt.Errorf("flow run not found: %s", runID)
			}

			format := viper.GetString("output.format")
			printFlowRun(cmd, run, format)
			return nil
		}

		if !flowStatusAll {
			return fmt.Errorf("either provide a run-id or use --all flag")
		}

		runs, err := s.ListFlowRuns(ctx, core.Query{})
		if err != nil {
			return fmt.Errorf("failed to list flow runs: %w", err)
		}

		format := viper.GetString("output.format")
		formatFlowRuns(cmd, runs, format)
		return nil
	},
}

var FlowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all flow runs",
	Long:  `List all flow execution runs with their status.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		runs, err := s.ListFlowRuns(ctx, core.Query{})
		if err != nil {
			return fmt.Errorf("failed to list flow runs: %w", err)
		}

		format := viper.GetString("output.format")
		formatFlowRuns(cmd, runs, format)
		return nil
	},
}

func printFlowRun(cmd *cobra.Command, run *core.FlowRun, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON, formatYAML:
		_ = output.Render(out, format, run) //nolint:errcheck // best-effort output
	default:
		// printFlowRun detail (table-style). Humanise Started/Ended.
		// JSON/YAML branch above keeps RFC3339. T-1384.
		_, _ = fmt.Fprintf(out, "Flow Run: %s\n", run.ID)
		_, _ = fmt.Fprintf(out, "  Flow ID: %s\n", run.FlowID)
		_, _ = fmt.Fprintf(out, "  Status: %s\n", formatFlowStatus(run.Status))
		_, _ = fmt.Fprintf(out, "  Started: %s\n", DisplayTimeRelative(run.StartedAt))
		if run.EndedAt != nil {
			_, _ = fmt.Fprintf(out, "  Ended: %s\n", DisplayTimePtrRelative(run.EndedAt))
			duration := run.EndedAt.Sub(run.StartedAt)
			_, _ = fmt.Fprintf(out, "  Duration: %s\n", duration)
		}

		if len(run.Results) > 0 {
			_, _ = fmt.Fprintln(out, "\n  Results:")
			for k, v := range run.Results {
				_, _ = fmt.Fprintf(out, "    %s: %v\n", k, v)
			}
		}
	}
}

func formatFlowRuns(cmd *cobra.Command, runs []*core.FlowRun, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON, formatYAML:
		_ = output.Render(out, format, normalizeEmptySlices(runs)) //nolint:errcheck // best-effort output
	default:
		if len(runs) == 0 {
			_, _ = fmt.Fprintln(out, "No flow runs found")
			return
		}
		renderFlowRunsTable(out, runs)
	}
}

// flowRunRow is the row schema for `tlc flow runs` table output.
type flowRunRow struct {
	RunID    string `table:"Run ID"`
	FlowID   string `table:"Flow ID"`
	Status   string `table:"Status"`
	Started  string `table:"Started"`
	Duration string `table:"Duration"`
}

func renderFlowRunsTable(out io.Writer, runs []*core.FlowRun) {
	rows := make([]flowRunRow, len(runs))
	for i, r := range runs {
		duration := "-"
		if r.EndedAt != nil {
			duration = r.EndedAt.Sub(r.StartedAt).String()
		}
		rows[i] = flowRunRow{
			RunID:  r.ID,
			FlowID: r.FlowID,
			// Plain status — kit/output's tabwriter (non-TTY) passes
			// cell values through verbatim, so pre-styled lipgloss
			// escapes from formatFlowStatus would leak into pipes.
			Status: string(r.Status),
			// Table column: humanise StartedAt ("5m ago"). T-1384.
			Started:  DisplayTimeRelative(r.StartedAt),
			Duration: duration,
		}
	}

	_ = renderStyledList(out, formatTable, rows, nil) //nolint:errcheck // best-effort output
	_, _ = fmt.Fprintf(out, "\nShowing %d flow runs\n", len(runs))
}

func formatFlowStatus(status core.FlowStatus) string {
	switch status {
	case core.FlowStatusQueued:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render("QUEUED")
	case core.FlowStatusRunning:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render("RUNNING")
	case core.FlowStatusSucceeded:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true).Render("SUCCEEDED")
	case core.FlowStatusFailed:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("FAILED")
	case core.FlowStatusCanceled:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("CANCELED")
	default:
		return string(status)
	}
}

var FlowInvokeCmd = &cobra.Command{
	Use:   "invoke <flow-file>",
	Short: "Invoke a flow to generate and assign tasks",
	Long: `Invoke a flow definition that uses task templates to generate tasks.

The flow extracts tasks from step templates and auto-assigns them to
assignees based on capability matching.

Example:
  tlc flow invoke examples/flows/brainstorming.yaml
  tlc flow invoke tlc://hop-top/tlc/flow:brainstorming:1.0`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "no",
	},
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		flowRef := args[0]

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		resolver := uri.NewResolver(s)
		resolver.FlowsDir = flowsDirFromConfig()
		res, err := resolver.ResolveFlow(ctx, flowRef)
		if err != nil {
			return err
		}
		flow := res.Flow

		executor := core.NewFlowExecutor(s, s).WithEvaKey(os.Getenv("EVA_KEY"))

		runID := fmt.Sprintf("run:%s", generateID())

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Invoking flow: %s (ID: %s)\n", flow.Name, flow.ID)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Run ID: %s\n\n", runID)

		providedVars, err := core.ParseFlowVarFlags(flowRunVars)
		if err != nil {
			return err
		}
		inputs, err := core.ResolveFlowInputs(flow, providedVars)
		if err != nil {
			return err
		}

		tasks, err := executor.ExtractTasksFromFlow(ctx, flow, runID, inputs)
		if err != nil {
			return fmt.Errorf("failed to extract tasks from flow: %w", err)
		}

		if len(tasks) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No tasks to generate. Flow has no task templates.\n")
			return nil
		}

		assigneesDir := "examples/assignees"
		assigneeLoader := core.NewAssigneeLoader(assigneesDir)
		assignees, err := assigneeLoader.LoadAll()
		if err != nil {
			return fmt.Errorf("failed to load assignees: %w", err)
		}

		if len(assignees) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: No assignees available. Tasks will not be auto-assigned.\n")
		}

		engine := core.NewAssignmentEngine(assignees)

		taskService := core.NewTaskService(s, s)

		by := flowRunBy
		if by == "" {
			by = core.GetCurrentUser()
		}

		for _, task := range tasks {
			if err := taskService.CreateTaskWithAssignment(ctx, task, engine, by, fmt.Sprintf("Created by flow %s", flow.ID)); err != nil {
				return fmt.Errorf("failed to create task %s: %w", task.ID, err)
			}

			assigneeName := "unassigned"
			if task.AssignedTo != nil {
				assigneeName = *task.AssignedTo
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created task %s: %s (assigned to %s)\n",
				formatTaskAlias(task), task.Title, assigneeName)
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nFlow %s invoked successfully. Created %d tasks.\n",
			flow.ID, len(tasks))

		return nil
	},
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}

func init() {
	FlowRunCmd.Flags().StringVar(&flowRunBy, "by", "", "Actor executing the flow (default: current user)")
	FlowRunCmd.Flags().StringSliceVar(&flowRunVars, "var", nil, "Flow input variable as key=value (repeatable)")
	FlowInvokeCmd.Flags().StringVar(&flowRunBy, "by", "", "Actor invoking the flow (default: current user)")
	FlowInvokeCmd.Flags().StringSliceVar(&flowRunVars, "var", nil, "Flow input variable as key=value (repeatable)")

	FlowStatusCmd.Flags().BoolVar(&flowStatusAll, "all", false, "List all flow runs")

	FlowCmd.AddCommand(FlowRunCmd)
	FlowCmd.AddCommand(FlowInvokeCmd)
	FlowCmd.AddCommand(FlowStatusCmd)
	FlowCmd.AddCommand(FlowListCmd)
	RootCmd.AddCommand(FlowCmd)
}
