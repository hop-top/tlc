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
	agentListSource string
)

// AgentListCmd implements `tlc agent list`.
var AgentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List agent execution runs or configured agents",
	Long: `List agent run audit records (default) or registered agents.

Use --source to switch between:
  runs    Audit records of agent executions (default)
  config  Agents declared in agents.yaml (global + project-local)

Examples:
  tlc agent list
  tlc agent list --status succeeded
  tlc agent list --limit 20 --json
  tlc agent list --source config`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		switch agentListSource {
		case "", "runs":
			return runAgentListRuns(cmd)
		case "config":
			return runAgentListConfig(cmd)
		default:
			return fmt.Errorf(
				"agent list: invalid --source %q; valid: runs, config",
				agentListSource,
			)
		}
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

func runAgentListRuns(cmd *cobra.Command) error {
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

	renderAgentRunsTable(out, runs, s)
	return nil
}

// taskAliasLookup is the minimal storage surface renderAgentRunsTable
// needs to resolve task typeids → display aliases. Lets tests fake it
// without depending on the full *storage.SQLiteStorage surface.
type taskAliasLookup interface {
	GetTask(ctx context.Context, id string) (*core.Task, error)
}

func renderAgentRunsTable(out io.Writer, runs []*core.AgentRunRecord, s taskAliasLookup) {
	ctx := context.Background()
	// Cache typeid → alias lookups so a single renderAgentRunsTable call
	// fetches each unique task at most once (avoids N+1 when --limit is
	// high or several runs target the same task).
	aliasCache := make(map[string]string)
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
		target := r.TargetID
		if r.TargetType == "task" && core.IsTaskID(r.TargetID) && s != nil {
			if cached, ok := aliasCache[r.TargetID]; ok {
				if cached != "" {
					target = cached
				}
			} else {
				resolved := ""
				if t, err := s.GetTask(ctx, r.TargetID); err == nil && t != nil {
					resolved = core.FormatTaskAlias(t)
				}
				aliasCache[r.TargetID] = resolved // negative cache too
				if resolved != "" {
					target = resolved
				}
			}
		}
		rows[i] = agentRunRow{
			ID:       id,
			Agent:    r.Agent,
			Target:   fmt.Sprintf("%s:%s", r.TargetType, target),
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
	f.StringVar(&agentListStatus, "status", "", "Filter by status (source=runs)")
	f.IntVar(&agentListLimit, "limit", 50, "Max results (source=runs)")
	f.BoolVar(&agentListJSON, "json", false, "Output as JSON (source=runs)")
	f.StringVar(&agentListSource, "source", "runs",
		"Listing source: 'runs' (audit records) or 'config' (agents.yaml)")

	AgentCmd.AddCommand(AgentListCmd)
}
