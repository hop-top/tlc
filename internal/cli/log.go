package cli

import (
	"context"
	"fmt"
	"image/color"
	"io"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/output"
	"hop.top/tlc/internal/core"
)

const (
	formatJSON     = "json"
	formatYAML     = "yaml"
	formatTable    = "table"
	formatSummary  = "summary"
	formatCounters = "counters"
	sortDesc       = "desc"
)

var (
	logTaskID        string
	logAction        string
	logBy            string
	logSince         string
	logUntil         string
	logLimit         int
	logOffset        int
	logSortDirection string
	logAll           bool
)

var logCmd = &cobra.Command{
	Use:   "log [task-id]",
	Short: "View task audit logs",
	Long: `View audit logs for tasks with filtering and querying capabilities.

Examples:
  # View logs for a specific task
  tlc log T-0001

  # View all logs across all tasks
  tlc log --all

  # Filter by action type
  tlc log --all --action CLAIMED

  # Filter by user
  tlc log --all --by engineer-1

  # View logs in different formats
  tlc log T-0001 --format json
  tlc log --all --format yaml

  # Limit and paginate results
  tlc log --all --limit 20 --offset 40
`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// If task ID provided as argument, use it
		if len(args) > 0 {
			logTaskID = args[0]
		}

		// Validate: either task-id or --all must be specified
		if logTaskID == "" && !logAll {
			return fmt.Errorf("either provide a task-id or use --all flag")
		}

		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()

		// Build log query
		query := core.LogQuery{
			TaskID:        logTaskID,
			Action:        logAction,
			By:            logBy,
			Limit:         logLimit,
			Offset:        logOffset,
			SortDirection: logSortDirection,
		}

		// Default sort direction
		if query.SortDirection == "" {
			query.SortDirection = sortDesc
		}

		// Get logs
		logs, err := s.ListLogs(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to query logs: %w", err)
		}

		// Format and output
		format := viper.GetString("output.format")
		formatLogs(cmd, logs, format)
		return nil
	},
}

func formatLogs(cmd *cobra.Command, logs []*core.LogEntry, format string) {
	out := cmd.OutOrStdout()
	switch format {
	case formatJSON, formatYAML:
		_ = output.Render(out, format, logs) //nolint:errcheck // best-effort output
	default: // table
		renderLogTable(out, logs)
	}
}

func renderLogTable(w io.Writer, logs []*core.LogEntry) {
	if len(logs) == 0 {
		_, _ = fmt.Fprintln(w, "No logs found")
		return
	}

	headers := []string{"Timestamp", "Task ID", "Action", "By", "Note"}

	rows := make([][]string, 0, len(logs))
	for _, l := range logs {
		timestamp := l.Timestamp.Format("2006-01-02 15:04:05")
		note := l.Note
		if len(note) > 47 {
			note = note[:47] + "..."
		}

		rows = append(rows, []string{
			timestamp,
			l.TaskID,
			formatLogAction(l.Action),
			l.By,
			note,
		})
	}

	renderTTYTable(w, headers, rows, termWidth())
	_, _ = fmt.Fprintf(w, "\nShowing %d log entries\n", len(logs))
}

func formatLogAction(action string) string {
	actionColors := map[string]color.Color{
		"CREATED":       lipgloss.Color("42"),  // green
		"CLAIMED":       lipgloss.Color("39"),  // blue
		"RELEASED":      lipgloss.Color("214"), // yellow
		"REASSIGNED":    lipgloss.Color("208"), // orange
		"UPDATED":       lipgloss.Color("45"),  // cyan
		"DONE":          lipgloss.Color("46"),  // bright green
		"SKIPPED":       lipgloss.Color("226"), // yellow
		"FAILURE":       lipgloss.Color("196"), // red
		"RETRY":         lipgloss.Color("214"), // orange
		"COMMENT":       lipgloss.Color("245"), // gray
		"SYNC_IMPORTED": lipgloss.Color("51"),  // cyan
		"SYNC_PULLED":   lipgloss.Color("51"),  // cyan
		"SYNC_PUSHED":   lipgloss.Color("51"),  // cyan
		"SYNC_CONFLICT": lipgloss.Color("196"), // red
		"SYNC_ERROR":    lipgloss.Color("196"), // red
		"FLOW_START":    lipgloss.Color("39"),  // blue
		"FLOW_END":      lipgloss.Color("42"),  // green
		"STEP_START":    lipgloss.Color("39"),  // blue
		"STEP_END":      lipgloss.Color("42"),  // green
	}

	color, exists := actionColors[action]
	if !exists {
		color = lipgloss.Color("255") // white/default
	}

	return lipgloss.NewStyle().Foreground(color).Render(action)
}

func init() {
	logCmd.Flags().StringVar(&logTaskID, "task-id", "", "Filter by task ID")
	logCmd.Flags().StringVar(&logAction, "action", "", "Filter by action type (e.g., CLAIMED, DONE, SYNC_PUSHED)")
	logCmd.Flags().StringVar(&logBy, "by", "", "Filter by actor/user")
	logCmd.Flags().StringVar(&logSince, "since", "", "Filter logs since timestamp (RFC3339)")
	logCmd.Flags().StringVar(&logUntil, "until", "", "Filter logs until timestamp (RFC3339)")
	logCmd.Flags().IntVarP(&logLimit, "limit", "n", 100, "Maximum number of logs to return")
	logCmd.Flags().IntVar(&logOffset, "offset", 0, "Skip first N logs (for pagination)")
	logCmd.Flags().StringVar(&logSortDirection, "sort", sortDesc, "Sort direction: asc or desc")
	logCmd.Flags().BoolVar(&logAll, "all", false, "Show logs from all tasks")

	RootCmd.AddCommand(logCmd)
}
