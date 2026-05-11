package cli

import (
	"context"
	"fmt"
	"image/color"
	"io"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
)

const (
	formatJSON     = "json"
	formatYAML     = "yaml"
	formatTable    = "table"
	formatSummary  = "summary"
	formatCounters = "counters"
	formatVtodo    = "vtodo"
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

		// Temporal filters (T-1381). Both --since and --until are
		// past-leaning boundaries by convention (`tlc log --until '2
		// hours ago'`), so they share util.ParseSince for parsing.
		// util.ParseSince also accepts forward-looking inputs like
		// "tomorrow"/"in 3d", so this does not lose expressiveness.
		if logSince != "" {
			t, err := util.ParseSince(logSince)
			if err != nil {
				return fmt.Errorf("invalid --since %q: %w; use formats like '2 days ago', '7d', or RFC3339", logSince, err)
			}
			query.Since = &t
		}
		if logUntil != "" {
			t, err := util.ParseSince(logUntil)
			if err != nil {
				return fmt.Errorf("invalid --until %q: %w; use formats like 'yesterday', '1h', or RFC3339", logUntil, err)
			}
			query.Until = &t
		}
		if query.Since != nil && query.Until != nil && query.Since.After(*query.Until) {
			return fmt.Errorf("--since %q is after --until %q; swap the values or widen the window", logSince, logUntil)
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

// logTableRow is the row schema for `tlc log` table output.
type logTableRow struct {
	Timestamp string `table:"Timestamp"`
	TaskID    string `table:"Task ID"`
	Action    string `table:"Action"`
	By        string `table:"By"`
	Note      string `table:"Note"`
}

func renderLogTable(w io.Writer, logs []*core.LogEntry) {
	if len(logs) == 0 {
		_, _ = fmt.Fprintln(w, "No logs found")
		return
	}

	// Resolve display aliases for the task IDs we render. Looking up
	// every log's task individually would be O(N) round-trips; instead
	// we open storage once and cache by task ID.
	aliasByTaskID := resolveLogTaskAliases(logs)

	rows := make([]logTableRow, len(logs))
	for i, l := range logs {
		note := l.Note
		if len(note) > 47 {
			note = note[:47] + "..."
		}
		taskCell := l.TaskID
		if alias, ok := aliasByTaskID[l.TaskID]; ok && alias != "" {
			taskCell = alias
		}
		rows[i] = logTableRow{
			Timestamp: DisplayTime(l.Timestamp, LayoutDateTime),
			TaskID:    taskCell,
			// Plain action — kit/output's tabwriter (non-TTY) passes
			// cell values through verbatim, so pre-styled lipgloss
			// escapes from formatLogAction would leak into pipes
			// (breaks `tlc log --all | grep CLAIMED`).
			Action: l.Action,
			By:     l.By,
			Note:   note,
		}
	}

	_ = renderStyledList(w, formatTable, rows, nil) //nolint:errcheck // best-effort output
	_, _ = fmt.Fprintf(w, "\nShowing %d log entries\n", len(logs))
}

// resolveLogTaskAliases returns a map of task TypeID → display alias for
// the unique task IDs referenced by the given log entries. Lookup errors
// (or non-typeid keys) silently degrade — callers fall back to the raw
// TaskID stored on the log row.
func resolveLogTaskAliases(logs []*core.LogEntry) map[string]string {
	if len(logs) == 0 {
		return nil
	}
	uniq := make(map[string]struct{}, len(logs))
	for _, l := range logs {
		if l == nil || l.TaskID == "" {
			continue
		}
		if !core.IsTaskID(l.TaskID) {
			continue
		}
		uniq[l.TaskID] = struct{}{}
	}
	if len(uniq) == 0 {
		return nil
	}
	s, err := getStorage()
	if err != nil {
		return nil
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	out := make(map[string]string, len(uniq))
	for id := range uniq {
		t, gErr := s.GetTask(ctx, id)
		if gErr != nil || t == nil {
			continue
		}
		if alias := core.FormatTaskAlias(t); alias != "" {
			out[id] = alias
		}
	}
	return out
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
	logCmd.Flags().StringVar(&logSince, "since", "", "Show logs at-or-after the given time (e.g. '2 days ago', '7d', 2026-04-01)")
	logCmd.Flags().StringVar(&logUntil, "until", "", "Show logs at-or-before the given time (e.g. 'yesterday', '1h', 2026-04-30)")
	logCmd.Flags().IntVarP(&logLimit, "limit", "n", 100, "Maximum number of logs to return")
	logCmd.Flags().IntVar(&logOffset, "offset", 0, "Skip first N logs (for pagination)")
	logCmd.Flags().StringVar(&logSortDirection, "sort", sortDesc, "Sort direction: asc or desc")
	logCmd.Flags().BoolVar(&logAll, "all", false, "Show logs from all tasks")

	RootCmd.AddCommand(logCmd)
}
