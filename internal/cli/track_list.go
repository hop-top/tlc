package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"hop.top/tlc/internal/core"
)

// track list flag vars
var (
	trackListStatus      []string
	trackListState       string
	trackListType        string
	trackListLimit       int
	trackListOffset      int
	trackListAllProjects bool
)

var trackListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tracks with filters",
	RunE:  runTrackList,
}

func runTrackList(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	query := core.TrackQuery{
		Limit:       trackListLimit,
		Offset:      trackListOffset,
		AllProjects: trackListAllProjects,
	}

	if trackListType != "" {
		if !core.ValidTrackType(trackListType) {
			return fmt.Errorf(
				"track type %q invalid; valid types: feature, bug, refactor",
				trackListType,
			)
		}
		query.Type = trackListType
	}

	for _, st := range trackListStatus {
		ts := core.TrackStatus(strings.ToLower(st))
		if !core.ValidTrackStatus(ts) {
			return fmt.Errorf(
				"track status %q invalid; valid: pending, active, completed, "+
					"abandoned, archived",
				st,
			)
		}
		query.Status = append(query.Status, ts)
	}

	svc := core.NewTrackService(s, s)
	tracks, err := svc.ListTracks(ctx, query)
	if err != nil {
		return err
	}

	// Compute state and progress for each track.
	staleThreshold := viper.GetDuration("tracks.stale_threshold")
	if staleThreshold == 0 {
		staleThreshold = 48 * time.Hour
	}
	rows := make([]trackRowData, 0, len(tracks))
	for _, t := range tracks {
		_, flags, progress, err := svc.GetTrackWithState(ctx, t.ID, staleThreshold)
		if err != nil {
			return fmt.Errorf(
				"failed to compute state for track %q: %w", t.ID, err,
			)
		}
		rows = append(rows, trackRowData{
			Track:    t,
			State:    flags,
			Progress: *progress,
		})
	}

	// Post-query filter by computed state.
	if trackListState != "" {
		normalizedState := strings.ToLower(trackListState)
		switch normalizedState {
		case "stale", "blocked", "unlinked", "healthy":
		default:
			return fmt.Errorf(
				"track state %q invalid; valid: stale, blocked, unlinked, healthy",
				trackListState,
			)
		}
		wantState := core.TrackStateFlag(normalizedState)
		filtered := rows[:0]
		for _, r := range rows {
			for _, f := range r.State {
				if f == wantState {
					filtered = append(filtered, r)
					break
				}
			}
		}
		rows = filtered
	}

	format := viper.GetString("output.format")
	switch format {
	case formatJSON:
		return renderTrackListJSON(cmd.OutOrStdout(), rows)
	case formatYAML:
		return renderTrackListYAML(cmd.OutOrStdout(), rows)
	default:
		renderTrackListTable(cmd.OutOrStdout(), rows, trackListAllProjects)
	}
	return nil
}

type trackListOutput struct {
	ID       string   `json:"id" yaml:"id"`
	Project  string   `json:"project,omitempty" yaml:"project,omitempty"`
	Title    string   `json:"title" yaml:"title"`
	Type     string   `json:"type" yaml:"type"`
	Status   string   `json:"status" yaml:"status"`
	State    []string `json:"state" yaml:"state"`
	Progress string   `json:"progress" yaml:"progress"`
	Assignee string   `json:"assignee" yaml:"assignee"`
}

func toTrackListOutput(t *core.Track, state []core.TrackStateFlag, p core.TrackProgress) trackListOutput {
	flags := make([]string, len(state))
	for i, f := range state {
		flags[i] = string(f)
	}
	assignee := "-"
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		assignee = "@" + *t.AssignedTo
	}
	project := ""
	if t.ProjectID != nil && *t.ProjectID != "" {
		project = *t.ProjectID
	}
	return trackListOutput{
		ID:       t.ID,
		Project:  project,
		Title:    t.Title,
		Type:     t.Type,
		Status:   string(t.Status),
		State:    flags,
		Progress: core.FormatProgress(p),
		Assignee: assignee,
	}
}

type trackRowData struct {
	Track    *core.Track
	State    []core.TrackStateFlag
	Progress core.TrackProgress
}

func renderTrackListJSON(w io.Writer, rows []trackRowData) error {
	out := make([]trackListOutput, len(rows))
	for i, r := range rows {
		out[i] = toTrackListOutput(r.Track, r.State, r.Progress)
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal tracks to JSON: %w", err)
	}
	_, _ = fmt.Fprintln(w, string(data))
	return nil
}

func renderTrackListYAML(w io.Writer, rows []trackRowData) error {
	out := make([]trackListOutput, len(rows))
	for i, r := range rows {
		out[i] = toTrackListOutput(r.Track, r.State, r.Progress)
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("failed to marshal tracks to YAML: %w", err)
	}
	_, _ = fmt.Fprintln(w, string(data))
	return nil
}

func renderTrackListTable(w io.Writer, rows []trackRowData, showProject bool) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(w, "No tracks found")
		return
	}

	columns := []table.Column{
		{Title: "ID", Width: 22},
	}
	if showProject {
		columns = append(columns, table.Column{Title: "Project", Width: 18})
	}
	columns = append(columns,
		table.Column{Title: "Title", Width: 25},
		table.Column{Title: "Type", Width: 10},
		table.Column{Title: "Status", Width: 12},
		table.Column{Title: "State", Width: 12},
		table.Column{Title: "Progress", Width: 12},
		table.Column{Title: "Assignee", Width: 12},
	)

	tableRows := make([]table.Row, 0, len(rows))
	for _, r := range rows {
		assignee := "-"
		if r.Track.AssignedTo != nil && *r.Track.AssignedTo != "" {
			assignee = "@" + *r.Track.AssignedTo
		}

		stateStrs := make([]string, len(r.State))
		for i, f := range r.State {
			stateStrs[i] = string(f)
		}

		row := table.Row{r.Track.ID}
		if showProject {
			proj := "-"
			if r.Track.ProjectID != nil && *r.Track.ProjectID != "" {
				proj = *r.Track.ProjectID
			}
			row = append(row, proj)
		}
		row = append(row,
			r.Track.Title,
			r.Track.Type,
			string(r.Track.Status),
			strings.Join(stateStrs, ","),
			core.FormatProgress(r.Progress),
			assignee,
		)
		tableRows = append(tableRows, row)
	}

	tbl := table.New(
		table.WithColumns(columns),
		table.WithRows(tableRows),
		table.WithFocused(false),
		table.WithHeight(len(tableRows)+1),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	s.Selected = lipgloss.NewStyle()
	tbl.SetStyles(s)

	_, _ = fmt.Fprintln(w, tbl.View())
}

// resetTrackListFlags clears track list flag state between tests.
func resetTrackListFlags() {
	trackListStatus = []string{}
	trackListState = ""
	trackListType = ""
	trackListLimit = 100
	trackListOffset = 0
	trackListAllProjects = false
}

func init() {
	TrackCmd.AddCommand(trackListCmd)

	trackListCmd.Flags().StringSliceVar(
		&trackListStatus, "status", []string{}, "Filter by status (repeatable)",
	)
	trackListCmd.Flags().StringVar(
		&trackListState, "state", "", "Filter by computed state (stale, blocked, unlinked, healthy)",
	)
	trackListCmd.Flags().StringVar(
		&trackListType, "type", "", "Filter by type (feature, bug, refactor)",
	)
	trackListCmd.Flags().IntVarP(
		&trackListLimit, "limit", "n", 100, "Limit results",
	)
	trackListCmd.Flags().IntVar(
		&trackListOffset, "offset", 0, "Skip results",
	)
	trackListCmd.Flags().BoolVar(
		&trackListAllProjects, "all-projects", false,
		"Show tracks from all projects",
	)
}
