package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
)

var trackShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show track details",
	Args:  cobra.ExactArgs(1),
	RunE:  runTrackShow,
}

func runTrackShow(cmd *cobra.Command, args []string) error {
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

	svc := core.NewTrackService(s, s)
	staleThreshold := viper.GetDuration("tracks.stale_threshold")
	if staleThreshold == 0 {
		staleThreshold = 48 * time.Hour
	}
	track, flags, progress, err := svc.GetTrackWithState(ctx, id, staleThreshold)
	if err != nil {
		return err
	}

	// Fetch linked tasks for phase breakdown display.
	//
	// Track IDs are unique only within a (project_id, id) composite, so we
	// must scope the task lookup by the track's owning project. Otherwise a
	// task from a different project that happens to share the same track_id
	// string leaks into this track's display. AllProjects:true bypasses the
	// auto-scope by current cwd; the explicit project_id filter below is
	// what enforces the correct boundary.
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

	// Compute execution strategy when tasks have blocked-by deps.
	var strategy *core.ExecutionStrategy
	if graph, gErr := core.NewDepGraph(tasks); gErr == nil && graph.HasDeps() {
		strategy, err = graph.ComputeStrategy()
		if err != nil {
			return fmt.Errorf("failed to compute execution strategy: %w", err)
		}
	}

	format := viper.GetString("output.format")
	switch format {
	case formatJSON, formatYAML:
		out := buildTrackShowOutput(track, flags, progress, strategy)
		return output.Render(cmd.OutOrStdout(), format, out)
	case formatVtodo:
		return writeVtodo(cmd, tasks, []*core.Track{track}, trackShowOutputPath, trackShowIncludeLogs)
	default:
		renderTrackShowDetail(cmd.OutOrStdout(), track, flags, progress, tasks, strategy)
	}
	return nil
}

// trackShowOutput is the structured output for JSON/YAML rendering.
type trackShowOutput struct {
	ID                string                  `json:"id" yaml:"id"`
	Slug              string                  `json:"slug,omitempty" yaml:"slug,omitempty"`
	Title             string                  `json:"title" yaml:"title"`
	Type              string                  `json:"type" yaml:"type"`
	Status            string                  `json:"status" yaml:"status"`
	State             []string                `json:"state" yaml:"state"`
	Assignee          string                  `json:"assignee" yaml:"assignee"`
	CreatedAt         string                  `json:"created_at" yaml:"created_at"`
	UpdatedAt         string                  `json:"updated_at" yaml:"updated_at"`
	Progress          *core.TrackProgress     `json:"progress" yaml:"progress"`
	ExecutionStrategy *core.ExecutionStrategy `json:"execution_strategy,omitempty" yaml:"execution_strategy,omitempty"`
}

func buildTrackShowOutput(
	t *core.Track,
	flags []core.TrackStateFlag,
	progress *core.TrackProgress,
	strategy *core.ExecutionStrategy,
) trackShowOutput {
	stateStrs := make([]string, len(flags))
	for i, f := range flags {
		stateStrs[i] = string(f)
	}
	assignee := "-"
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		assignee = "@" + *t.AssignedTo
	}
	return trackShowOutput{
		ID:                t.ID,
		Slug:              t.Slug,
		Title:             t.Title,
		Type:              t.Type,
		Status:            string(t.Status),
		State:             stateStrs,
		Assignee:          assignee,
		CreatedAt:         DisplayTime(t.CreatedAt, LayoutDate),
		UpdatedAt:         DisplayTime(t.UpdatedAt, LayoutDate),
		Progress:          progress,
		ExecutionStrategy: strategy,
	}
}

func renderTrackShowDetail(
	w io.Writer,
	t *core.Track,
	flags []core.TrackStateFlag,
	progress *core.TrackProgress,
	tasks []*core.Task,
	strategy *core.ExecutionStrategy,
) {
	// Header
	_, _ = fmt.Fprintln(w, titleStyle.Render(fmt.Sprintf("Track: %s", formatTrackAlias(t))))
	if isShowTypeIDOutput() && t.ID != "" && t.ID != formatTrackAlias(t) {
		_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("ID:"), t.ID)
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", labelStyle.Render("Title:"), t.Title)

	stateStrs := make([]string, len(flags))
	for i, f := range flags {
		stateStrs[i] = string(f)
	}
	assignee := "-"
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		assignee = "@" + *t.AssignedTo
	}

	_, _ = fmt.Fprintf(w, "%s %s    %s %s    %s %s    %s %s\n",
		labelStyle.Render("Type:"), t.Type,
		lipgloss.NewStyle().Foreground(mutedColor).Render("Status:"),
		string(t.Status),
		lipgloss.NewStyle().Foreground(mutedColor).Render("State:"),
		strings.Join(stateStrs, ","),
		lipgloss.NewStyle().Foreground(mutedColor).Render("Assignee:"),
		assignee,
	)

	// Detail view: humanise CreatedAt/UpdatedAt ("2d ago"). The
	// JSON/YAML projection in buildTrackShowOutput keeps absolute
	// LayoutDate for tooling. T-1384.
	_, _ = fmt.Fprintf(w, "%s %s    %s %s\n",
		labelStyle.Render("Created:"), DisplayTimeRelative(t.CreatedAt),
		lipgloss.NewStyle().Foreground(mutedColor).Render("Updated:"),
		DisplayTimeRelative(t.UpdatedAt),
	)

	// Progress summary
	if progress.TotalTasks > 0 {
		pct := 0
		if progress.TotalTasks > 0 {
			pct = progress.CompletedTasks * 100 / progress.TotalTasks
		}
		_, _ = fmt.Fprintf(w, "\nProgress: %d/%d tasks (%d%%)\n",
			progress.CompletedTasks, progress.TotalTasks, pct,
		)
	} else {
		_, _ = fmt.Fprintln(w, "\nProgress: no tasks linked")
	}

	// Phase breakdown
	if len(progress.Phases) > 0 {
		// Build phase -> tasks map
		phaseTasks := buildPhaseTasks(tasks)

		for _, phase := range progress.Phases {
			checkmark := ""
			if phase.Total > 0 && phase.Completed == phase.Total {
				checkmark = " " + doneStyle.Render("\u2713")
			}
			label := phase.Label
			if label == "" {
				label = fmt.Sprintf("Phase %d", phase.Phase)
			}
			_, _ = fmt.Fprintf(w, "\n%s %d \u2014 %-18s %d/%d%s\n",
				lipgloss.NewStyle().Bold(true).Render("Phase"),
				phase.Phase, label,
				phase.Completed, phase.Total, checkmark,
			)

			// Show tasks in this phase.
			for _, task := range phaseTasks[phase.Phase] {
				renderPhaseTask(w, task)
			}
		}
	}

	// Unphased tasks
	_, _ = fmt.Fprintln(w, "\nUnphased")
	if len(progress.Unphased) == 0 {
		_, _ = fmt.Fprintln(w, "  (none)")
	} else {
		for _, task := range progress.Unphased {
			renderPhaseTask(w, task)
		}
	}

	// Execution strategy — only shown when deps exist.
	if strategy != nil && len(strategy.Batches) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprint(w, core.RenderBatchSummary(strategy))

		if len(strategy.CriticalPath) > 0 {
			_, _ = fmt.Fprintf(w, "\nCritical Path (%d tasks): %s\n",
				len(strategy.CriticalPath),
				core.FormatCriticalPath(strategy.CriticalPath, tasks))
		}
	}
}

// renderPhaseTask renders a single task line within a phase/unphased section.
func renderPhaseTask(w io.Writer, task *core.Task) {
	statusStr := string(task.Status)
	styled := statusStr
	switch task.Status {
	case core.StatusDone:
		styled = doneStyle.Render("[DONE]")
	case core.StatusInProgress:
		styled = inProgressStyle.Render("[IN_PROGRESS]")
	case core.StatusSkipped:
		styled = skippedStyle.Render("[SKIPPED]")
	default:
		styled = fmt.Sprintf("[%s]", statusStr)
	}
	_, _ = fmt.Fprintf(w, "  %-10s %-15s %s\n", formatTaskAlias(task), styled, task.Title)
}

// buildPhaseTasks groups tasks by their phase tag number.
func buildPhaseTasks(tasks []*core.Task) map[int][]*core.Task {
	result := make(map[int][]*core.Task)
	for _, t := range tasks {
		phase, ok := extractPhaseNum(t.Tags)
		if !ok {
			continue
		}
		result[phase] = append(result[phase], t)
	}
	return result
}

// extractPhaseNum extracts the phase number from a task's tags.
func extractPhaseNum(tags []string) (int, bool) {
	for _, tag := range tags {
		if !strings.HasPrefix(tag, "phase:") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(tag, "phase:%d", &n); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

func init() {
	trackShowCmd.Flags().StringVar(&trackShowOutputPath, "output", "", "Write output to a file instead of stdout")
	trackShowCmd.Flags().BoolVar(&trackShowIncludeLogs, "include-logs", false, "Include audit log entries (vtodo: emit VJOURNAL components)")
	TrackCmd.AddCommand(trackShowCmd)
}
