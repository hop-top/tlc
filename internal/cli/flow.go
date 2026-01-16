package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var (
	flowRunFile   string
	flowRunBy     string
	flowStatusAll bool
)

var flowCmd = &cobra.Command{
	Use:   "flow",
	Short: "Flow execution and management",
	Long: `Execute and manage task flows.

Flows are declarative workflows that orchestrate task execution with
control flow semantics like parallel, sequential, branching, and retry.`,
}

var flowRunCmd = &cobra.Command{
	Use:   "run <flow-file>",
	Short: "Execute a flow definition",
	Long: `Execute a flow definition from a YAML or JSON file.

The flow file defines a workflow with steps that reference tasks.
Steps can be executed sequentially, in parallel, with branching logic,
retries, and other control flow patterns.

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
  tlc flow run tests/fixtures/parallel-flow.yaml
  tlc flow run deployment-flow.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		flowFile := args[0]

		// Read and parse flow file
		f, err := os.Open(flowFile)
		if err != nil {
			return fmt.Errorf("failed to open flow file: %w", err)
		}
		defer f.Close()

		flow, err := core.ParseFlow(f, flowFile)
		if err != nil {
			return fmt.Errorf("failed to parse flow: %w", err)
		}

		// Get storage
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()

		// Create flow executor
		executor := core.NewFlowExecutor(s, s)

		// Determine actor
		by := flowRunBy
		if by == "" {
			by = core.GetCurrentUser()
		}

		// Execute flow
		fmt.Fprintf(cmd.OutOrStdout(), "Starting flow: %s (ID: %s)\n", flow.Name, flow.ID)
		run, err := executor.Execute(ctx, flow, by)
		if err != nil {
			return fmt.Errorf("flow execution failed: %w", err)
		}

		// Output result
		fmt.Fprintf(cmd.OutOrStdout(), "\nFlow execution completed!\n")
		fmt.Fprintf(cmd.OutOrStdout(), "  Run ID: %s\n", run.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "  Status: %s\n", run.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "  Started: %s\n", run.StartedAt.Format("2006-01-02 15:04:05"))
		if run.EndedAt != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "  Ended: %s\n", run.EndedAt.Format("2006-01-02 15:04:05"))
			duration := run.EndedAt.Sub(run.StartedAt)
			fmt.Fprintf(cmd.OutOrStdout(), "  Duration: %s\n", duration)
		}

		return nil
	},
}

var flowStatusCmd = &cobra.Command{
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
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

		ctx := context.Background()

		// If specific run ID provided
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

		// Otherwise list all runs
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

var flowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all flow runs",
	Long:  `List all flow execution runs with their status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer s.Close()

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
	case "json":
		data, _ := json.MarshalIndent(run, "", "  ")
		fmt.Fprintln(out, string(data))
	case "yaml":
		data, _ := yaml.Marshal(run)
		fmt.Fprintln(out, string(data))
	default:
		// Pretty print
		fmt.Fprintf(out, "Flow Run: %s\n", run.ID)
		fmt.Fprintf(out, "  Flow ID: %s\n", run.FlowID)
		fmt.Fprintf(out, "  Status: %s\n", formatFlowStatus(run.Status))
		fmt.Fprintf(out, "  Started: %s\n", run.StartedAt.Format("2006-01-02 15:04:05"))
		if run.EndedAt != nil {
			fmt.Fprintf(out, "  Ended: %s\n", run.EndedAt.Format("2006-01-02 15:04:05"))
			duration := run.EndedAt.Sub(run.StartedAt)
			fmt.Fprintf(out, "  Duration: %s\n", duration)
		}

		if len(run.Results) > 0 {
			fmt.Fprintln(out, "\n  Results:")
			for k, v := range run.Results {
				fmt.Fprintf(out, "    %s: %v\n", k, v)
			}
		}
	}
}

func formatFlowRuns(cmd *cobra.Command, runs []*core.FlowRun, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case "json":
		data, _ := json.MarshalIndent(runs, "", "  ")
		fmt.Fprintln(out, string(data))
	case "yaml":
		data, _ := yaml.Marshal(runs)
		fmt.Fprintln(out, string(data))
	default:
		if len(runs) == 0 {
			fmt.Fprintln(out, "No flow runs found")
			return
		}
		renderFlowRunsTable(out, runs)
	}
}

func renderFlowRunsTable(out io.Writer, runs []*core.FlowRun) {
	columns := []table.Column{
		{Title: "Run ID", Width: 30},
		{Title: "Flow ID", Width: 25},
		{Title: "Status", Width: 15},
		{Title: "Started", Width: 20},
		{Title: "Duration", Width: 12},
	}

	rows := []table.Row{}
	for _, r := range runs {
		duration := "-"
		if r.EndedAt != nil {
			d := r.EndedAt.Sub(r.StartedAt)
			duration = d.String()
		}

		rows = append(rows, table.Row{
			r.ID,
			r.FlowID,
			formatFlowStatus(r.Status),
			r.StartedAt.Format("2006-01-02 15:04:05"),
			duration,
		})
	}

	tbl := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)+1),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	tbl.SetStyles(s)

	fmt.Fprintln(out, tbl.View())
	fmt.Fprintf(out, "\nShowing %d flow runs\n", len(runs))
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

func init() {
	flowRunCmd.Flags().StringVar(&flowRunBy, "by", "", "Actor executing the flow (default: current user)")

	flowStatusCmd.Flags().BoolVar(&flowStatusAll, "all", false, "List all flow runs")

	flowCmd.AddCommand(flowRunCmd)
	flowCmd.AddCommand(flowStatusCmd)
	flowCmd.AddCommand(flowListCmd)
	rootCmd.AddCommand(flowCmd)
}
