package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

var trackSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show project health and track overview",
	Long: `Render an at-a-glance project health overview across all tracks,
showing per-track progress, state flags, and last-update humanized time.

Read-only; pure projection over the local track and task tables.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: runTrackSummary,
}

type summaryRow struct {
	ID        string
	Progress  core.TrackProgress
	State     []core.TrackStateFlag
	UpdatedAt time.Time
}

func runTrackSummary(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	svc := core.NewTrackService(s, s)

	tracks, err := svc.ListTracks(ctx, core.TrackQuery{})
	if err != nil {
		return err
	}

	staleThreshold := viper.GetDuration("tracks.stale_threshold")
	if staleThreshold == 0 {
		staleThreshold = 48 * time.Hour
	}

	// Per-track computed data.
	type trackData struct {
		track    *core.Track
		progress core.TrackProgress
		state    []core.TrackStateFlag
	}
	items := make([]trackData, 0, len(tracks))
	for _, t := range tracks {
		_, flags, progress, err := svc.GetTrackWithState(ctx, t.ID, staleThreshold)
		if err != nil {
			return fmt.Errorf(
				"failed to compute state for track %q: %w", t.ID, err,
			)
		}
		items = append(items, trackData{
			track:    t,
			progress: *progress,
			state:    flags,
		})
	}

	// Count by status.
	counts := map[core.TrackStatus]int{}
	for _, d := range items {
		counts[d.track.Status]++
	}

	// Health thresholds from config.
	maxActive := viper.GetInt("tracks.health.max_active")
	if maxActive <= 0 {
		maxActive = 3
	}
	minProgress := viper.GetInt("tracks.health.min_progress_to_start")
	if !viper.IsSet("tracks.health.min_progress_to_start") && minProgress == 0 {
		minProgress = 50
	}

	w := cmd.OutOrStdout()
	projectID := viper.GetString("project.id")
	if projectID == "" {
		projectID = "(unknown)"
	}

	// Header.
	title := lipgloss.NewStyle().Bold(true).Render("Project: " + projectID)
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, title)
	_, _ = fmt.Fprintln(w)

	// Status counts.
	renderSummaryCounts(w, counts)

	// Health line.
	warning, err := svc.CheckOvercommitWarning(ctx, maxActive, minProgress)
	if err != nil {
		return err
	}
	renderSummaryHealth(w, warning)

	// Active track table.
	var rows []summaryRow
	for _, d := range items {
		if d.track.Status != core.TrackStatusActive {
			continue
		}
		rows = append(rows, summaryRow{
			ID:        formatTrackAlias(d.track),
			Progress:  d.progress,
			State:     d.state,
			UpdatedAt: d.track.UpdatedAt,
		})
	}
	renderSummaryTable(w, rows)

	return nil
}

func renderSummaryCounts(w io.Writer, counts map[core.TrackStatus]int) {
	bold := lipgloss.NewStyle().Bold(true)
	parts := []string{
		bold.Render(fmt.Sprintf("Active: %d", counts[core.TrackStatusActive])),
		bold.Render(fmt.Sprintf("Pending: %d", counts[core.TrackStatusPending])),
		bold.Render(fmt.Sprintf("Completed: %d",
			counts[core.TrackStatusCompleted])),
		bold.Render(fmt.Sprintf("Abandoned: %d",
			counts[core.TrackStatusAbandoned])),
	}
	_, _ = fmt.Fprintln(w, strings.Join(parts, "    "))
	_, _ = fmt.Fprintln(w)
}

func renderSummaryHealth(w io.Writer, warning string) {
	if warning != "" {
		line := lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Render("Health: \u26a0 overcommitted (" + warning + ")")
		_, _ = fmt.Fprintln(w, line)
	} else {
		line := lipgloss.NewStyle().
			Foreground(lipgloss.Color("34")).
			Render("Health: \u2714 ok")
		_, _ = fmt.Fprintln(w, line)
	}
	_, _ = fmt.Fprintln(w)
}

func renderSummaryTable(w io.Writer, rows []summaryRow) {
	if len(rows) == 0 {
		return
	}

	header := lipgloss.NewStyle().Bold(true)
	_, _ = fmt.Fprintf(w, " %-20s %-10s %-14s %s\n",
		header.Render("ID"),
		header.Render("Progress"),
		header.Render("State"),
		header.Render("Updated"),
	)
	_, _ = fmt.Fprintln(w, strings.Repeat("\u2500", 60))

	for _, r := range rows {
		stateStrs := make([]string, len(r.State))
		for i, f := range r.State {
			stateStrs[i] = string(f)
		}
		updated := humanize.Time(r.UpdatedAt)
		pct := summaryProgressPct(r.Progress)

		_, _ = fmt.Fprintf(w, " %-20s %-10s %-14s %s\n",
			r.ID,
			fmt.Sprintf("%d%%", pct),
			strings.Join(stateStrs, ","),
			updated,
		)
	}
}

func summaryProgressPct(p core.TrackProgress) int {
	if p.TotalTasks == 0 {
		return 0
	}
	return p.CompletedTasks * 100 / p.TotalTasks
}

func init() {
	TrackCmd.AddCommand(trackSummaryCmd)
}
