package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
)

var (
	trackGraphFormat       string
	trackGraphBatchesOnly  bool
	trackGraphCriticalPath bool
)

var trackGraphCmd = &cobra.Command{
	Use:   "graph <id>",
	Short: "Display dependency graph and execution strategy for a track",
	Args:  cobra.ExactArgs(1),
	RunE:  runTrackGraph,
}

func runTrackGraph(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	id, err := resolveTrackID(ctx, s, args[0])
	if err != nil {
		return err
	}

	// Load track to scope tasks by project.
	svc := core.NewTrackService(s, s)
	track, err := svc.GetTrack(ctx, id)
	if err != nil {
		return err
	}

	var trackProjectID string
	if track.ProjectID != nil {
		trackProjectID = *track.ProjectID
	}

	tasks, err := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "track_id", Operator: core.OpEq, Value: id},
			{Field: "project_id", Operator: core.OpEq, Value: trackProjectID},
		},
		AllProjects: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list linked tasks: %w", err)
	}

	if len(tasks) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No tasks linked to track "+id)
		return nil
	}

	graph, err := core.NewDepGraph(tasks)
	if err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	strategy, err := graph.ComputeStrategy()
	if err != nil {
		return err
	}

	// Resolve format: flag > viper > "table".
	format := trackGraphFormat
	if format == "" {
		format = viper.GetString("output.format")
	}
	if format == "" {
		format = "table"
	}

	w := cmd.OutOrStdout()

	switch format {
	case formatJSON, formatYAML:
		return output.Render(w, format, strategy)

	case "mermaid":
		_, _ = fmt.Fprint(w, core.RenderMermaid(strategy, tasks))
		return nil

	default: // table
		if !trackGraphBatchesOnly {
			tree := core.RenderDepTree(strategy, tasks)
			if tree != "" {
				_, _ = fmt.Fprintln(w, "Dependency Tree:")
				_, _ = fmt.Fprint(w, tree)
				_, _ = fmt.Fprintln(w)
			}
		}

		summary := core.RenderBatchSummary(strategy)
		if summary != "" {
			_, _ = fmt.Fprint(w, summary)
		}

		if trackGraphCriticalPath && len(strategy.CriticalPath) > 0 {
			_, _ = fmt.Fprintf(w, "\nCritical Path: %s\n",
				formatCriticalPath(strategy.CriticalPath))
		}
	}

	return nil
}

// formatCriticalPath renders a critical path as "A -> B -> C".
func formatCriticalPath(ids []string) string {
	if len(ids) == 0 {
		return "(none)"
	}
	result := ids[0]
	for _, id := range ids[1:] {
		result += " \u2192 " + id
	}
	return result
}

func init() {
	trackGraphCmd.Flags().StringVar(
		&trackGraphFormat, "format", "",
		"Output format: table, json, yaml, mermaid (default: table)",
	)
	trackGraphCmd.Flags().BoolVar(
		&trackGraphBatchesOnly, "batches-only", false,
		"Show only batch strategy, not dependency tree",
	)
	trackGraphCmd.Flags().BoolVar(
		&trackGraphCriticalPath, "critical-path", false,
		"Highlight critical path in output",
	)

	TrackCmd.AddCommand(trackGraphCmd)
}

// resetTrackGraphFlags clears track graph flag state between tests.
func resetTrackGraphFlags() {
	trackGraphFormat = ""
	trackGraphBatchesOnly = false
	trackGraphCriticalPath = false
}
