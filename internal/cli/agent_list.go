package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
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

// agentRunRow is the row schema for `tlc agent list` table output.
type agentRunRow struct {
	ID       string `table:"ID"`
	Agent    string `table:"Agent"`
	Target   string `table:"Target"`
	Status   string `table:"Status"`
	Exit     string `table:"Exit"`
	Started  string `table:"Started"`
	Duration string `table:"Duration"`
}

func renderAgentRunsTable(out io.Writer, runs []*core.AgentRunRecord) {
	rows := make([]agentRunRow, len(runs))
	for i, r := range runs {
		duration := "-"
		if r.EndedAt != nil {
			duration = r.EndedAt.Sub(r.StartedAt).Truncate(time.Second).String()
		}
		id := r.ID
		if len(id) > 8 {
			id = id[:8]
		}
		rows[i] = agentRunRow{
			ID:       id,
			Agent:    r.Agent,
			Target:   fmt.Sprintf("%s:%s", r.TargetType, r.TargetID),
			Status:   r.Status,
			Exit:     fmt.Sprintf("%d", r.ExitCode),
			Started:  r.StartedAt.Format("2006-01-02 15:04"),
			Duration: duration,
		}
	}

	_ = renderStyledList(out, formatTable, rows, nil) //nolint:errcheck // best-effort output
	_, _ = fmt.Fprintf(out, "\nShowing %d agent runs\n", len(runs))
}

func init() {
	f := AgentListCmd.Flags()
	f.StringVar(&agentListStatus, "status", "", "Filter by status")
	f.IntVar(&agentListLimit, "limit", 50, "Max results")
	f.BoolVar(&agentListJSON, "json", false, "Output as JSON")

	AgentCmd.AddCommand(AgentListCmd)
}
