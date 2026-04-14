package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/output"
	"hop.top/tlc/internal/core"
)

var (
	agentListStatus string
	agentListLimit  int
	agentListJSON   bool
)

// AgentListCmd implements `tlc agent list`.
var AgentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agent execution runs",
	Long: `List agent run audit records with optional filters.

Examples:
  tlc agent list
  tlc agent list --status succeeded
  tlc agent list --limit 20 --json`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		runs, err := s.ListAgentRuns(
			context.Background(), agentListStatus, agentListLimit,
		)
		if err != nil {
			return fmt.Errorf("list agent runs: %w", err)
		}

		format := viper.GetString("output.format")
		if agentListJSON || format == formatJSON {
			return output.Render(cmd.OutOrStdout(), formatJSON, runs)
		}

		out := cmd.OutOrStdout()
		if len(runs) == 0 {
			_, _ = fmt.Fprintln(out, "No agent runs found")
			return nil
		}

		renderAgentRunsTable(out, runs)
		return nil
	},
}

func renderAgentRunsTable(out io.Writer, runs []*core.AgentRunRecord) {
	headers := []string{"ID", "Agent", "Target", "Status", "Exit", "Started", "Duration"}

	rows := make([][]string, 0, len(runs))
	for _, r := range runs {
		targetStr := fmt.Sprintf("%s:%s", r.TargetType, r.TargetID)
		duration := "-"
		if r.EndedAt != nil {
			d := r.EndedAt.Sub(r.StartedAt)
			duration = d.Truncate(time.Second).String()
		}

		// Truncate ID for display.
		id := r.ID
		if len(id) > 8 {
			id = id[:8]
		}

		rows = append(rows, []string{
			id,
			r.Agent,
			targetStr,
			r.Status,
			fmt.Sprintf("%d", r.ExitCode),
			r.StartedAt.Format("2006-01-02 15:04"),
			duration,
		})
	}

	renderTTYTable(out, headers, rows, termWidth())
	_, _ = fmt.Fprintf(out, "\nShowing %d agent runs\n", len(runs))
}

func init() {
	f := AgentListCmd.Flags()
	f.StringVar(&agentListStatus, "status", "", "Filter by status")
	f.IntVar(&agentListLimit, "limit", 50, "Max results")
	f.BoolVar(&agentListJSON, "json", false, "Output as JSON")

	AgentCmd.AddCommand(AgentListCmd)
}
