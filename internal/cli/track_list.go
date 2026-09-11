package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
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
	Long: `List tracks in the active project (or all projects with
--all-projects), filtered by status, lifecycle state, or type.

Supports --status (repeatable, validated against TrackStatus values)
and --state (single state flag); both can be combined.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
	},
	RunE: runTrackList,
}

func runTrackList(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()

	fromConfig, err := applyConfigDefaults(cmd)
	if err != nil {
		return err
	}

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
		cfgTypes := getConfigTrackTypes()
		if !core.ValidTrackType(trackListType, cfgTypes) {
			return fmt.Errorf(
				"track type %q invalid; valid types: %s",
				trackListType, core.TrackTypeList(cfgTypes),
			)
		}
		query.Type = trackListType
	}

	statusProvided := len(trackListStatus) > 0 || fromConfig["status"]
	if statusProvided {
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
	} else {
		// Default: show pending + active (in-progress) tracks.
		query.Status = []core.TrackStatus{
			core.TrackStatusPending,
			core.TrackStatusActive,
		}
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
		// Use the already-fetched track to avoid GetTrack auto-scoping
		// to the current project — required for --all-projects from
		// inside a project context.
		flags, progress, err := svc.ComputeStateForTrack(ctx, t, staleThreshold)
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
	case formatJSON, formatYAML:
		out := make([]trackListOutput, len(rows))
		for i, r := range rows {
			out[i] = toTrackListOutput(r.Track, r.State, r.Progress)
		}
		return output.Render(cmd.OutOrStdout(), format, out) //nolint:wrapcheck // pass-through helper; kit's typed errors surface verbatim
	default:
		cols := effectiveTrackColumns(cmd, statusProvided, trackListAllProjects)
		renderTrackListTable(cmd.OutOrStdout(), rows, trackListAllProjects, cols)
	}
	return nil
}

type trackListOutput struct {
	ID       string   `json:"id" yaml:"id"`
	Slug     string   `json:"slug,omitempty" yaml:"slug,omitempty"`
	Project  string   `json:"project,omitempty" yaml:"project,omitempty"`
	Title    string   `json:"title" yaml:"title"`
	Type     string   `json:"type" yaml:"type"`
	Status   string   `json:"status" yaml:"status"`
	State    []string `json:"state" yaml:"state"`
	Progress string   `json:"progress" yaml:"progress"`
	Assignee string   `json:"assignee" yaml:"assignee"`
}

// trackTableRow is the table-only schema (no Project column).
type trackTableRow struct {
	ID       string `table:"ID"`
	Title    string `table:"Title"`
	Type     string `table:"Type"`
	Status   string `table:"Status"`
	State    string `table:"State"`
	Progress string `table:"Progress"`
	Assignee string `table:"Assignee"`
}

// trackTableRowWithProject mirrors trackTableRow with a Project column,
// rendered when --all-projects is set.
type trackTableRowWithProject struct {
	ID       string `table:"ID"`
	Project  string `table:"Project"`
	Title    string `table:"Title"`
	Type     string `table:"Type"`
	Status   string `table:"Status"`
	State    string `table:"State"`
	Progress string `table:"Progress"`
	Assignee string `table:"Assignee"`
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
		Slug:     t.Slug,
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

// effectiveTrackColumns resolves the table header list for `track list`.
// See resolveEffectiveColumns for the full ladder and pruning logic.
func effectiveTrackColumns(cmd *cobra.Command, statusProvided, showProject bool) []string {
	return resolveEffectiveColumns(cmd, trackListDefaultColumns, trackColumnHeaders, statusProvided,
		func(keys []string) ([]string, bool) {
			if showProject && !containsKey(keys, "project") {
				return injectAfter(keys, "id", "project"), true
			}
			return keys, false
		})
}

func renderTrackListTable(w io.Writer, rows []trackRowData, showProject bool, cols []string) {
	if len(rows) == 0 {
		_, _ = fmt.Fprintln(w, "No tracks found")
		return
	}

	// Compute row emphasis from track status + state flags.
	// Primary: active + healthy. Secondary: active + stale/blocked.
	// Muted: abandoned. None: everything else.
	emphasis := make(map[int]output.EmphasisKind)
	for i, r := range rows {
		switch r.Track.Status {
		case core.TrackStatusActive:
			hasFlag := false
			for _, f := range r.State {
				if f == core.TrackStateStale || f == core.TrackStateBlocked {
					hasFlag = true
					break
				}
			}
			if hasFlag {
				emphasis[i] = output.EmphasisSecondary
			} else {
				emphasis[i] = output.EmphasisPrimary
			}
		case core.TrackStatusAbandoned:
			emphasis[i] = output.EmphasisMuted
		}
	}

	if showProject {
		out := make([]trackTableRowWithProject, len(rows))
		for i, r := range rows {
			out[i] = trackTableRowWithProject{
				ID:       formatTrackAlias(r.Track),
				Project:  trackProject(r.Track),
				Title:    r.Track.Title,
				Type:     r.Track.Type,
				Status:   string(r.Track.Status),
				State:    joinTrackState(r.State),
				Progress: core.FormatProgress(r.Progress),
				Assignee: trackAssignee(r.Track),
			}
		}
		_ = renderStyledListCols(w, formatTable, out, emphasis, cols) //nolint:errcheck // best-effort output
		return
	}

	out := make([]trackTableRow, len(rows))
	for i, r := range rows {
		out[i] = trackTableRow{
			ID:       formatTrackAlias(r.Track),
			Title:    r.Track.Title,
			Type:     r.Track.Type,
			Status:   string(r.Track.Status),
			State:    joinTrackState(r.State),
			Progress: core.FormatProgress(r.Progress),
			Assignee: trackAssignee(r.Track),
		}
	}
	_ = renderStyledListCols(w, formatTable, out, emphasis, cols) //nolint:errcheck // best-effort output
}

func trackProject(t *core.Track) string {
	if t.ProjectID != nil && *t.ProjectID != "" {
		return *t.ProjectID
	}
	return "-"
}

func trackAssignee(t *core.Track) string {
	if t.AssignedTo != nil && *t.AssignedTo != "" {
		return "@" + *t.AssignedTo
	}
	return "-"
}

func joinTrackState(state []core.TrackStateFlag) string {
	parts := make([]string, len(state))
	for i, f := range state {
		parts[i] = string(f)
	}
	return strings.Join(parts, ",")
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
		&trackListType, "type", "", "Filter by track type",
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
