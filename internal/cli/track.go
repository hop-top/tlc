package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var (
	trackCreateType       string
	trackCreateID         string
	trackCreateAssignedTo string

	trackUpdateTitle      string
	trackUpdateStatus     string
	trackUpdateAssignedTo string
	trackUpdateType       string
	trackUpdateAddPlan    string

	trackShowOutputPath  string
	trackShowIncludeLogs bool
)

// TrackCmd is the parent command for track operations.
var TrackCmd = &cobra.Command{
	Use:   "track",
	Short: "Manage tracks",
}

var trackCreateCmd = &cobra.Command{
	Use:   "create [title]",
	Short: "Create a new track",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := trimMatchingQuotes(args[0])

		// User-supplied --id (or auto-derived from title) becomes the
		// human-facing Slug. Track.ID itself is the durable TypeID,
		// minted by TrackService.CreateTrack via core.NewTrackID().
		slug := trackCreateID
		if slug == "" {
			slug = core.SlugFromTitle(title)
		}

		if trackCreateType == "" {
			trackCreateType = getConfigDefaultTrackType()
		}
		cfgTypes := getConfigTrackTypes()
		if !core.ValidTrackType(trackCreateType, cfgTypes) {
			return fmt.Errorf(
				"track type %q invalid; valid types: %s",
				trackCreateType, core.TrackTypeList(cfgTypes),
			)
		}

		// Stage gate: refuse the create up-front when the active scope's
		// kit/core/stage forbids it (feature_freeze / maintenance / sunset
		// / archived per the default policy table). Scope is the project
		// detected for cwd; an empty scope skips the gate so stage-naive
		// adopters see no change. See core.GateTrackCreate for the table.
		if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
			if err := core.GateTrackCreate(proj.ProjectID, trackCreateType); err != nil {
				return err
			}
		}

		s, err := getStorageRaw()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		svc := core.NewTrackService(s, s)
		ctx := context.Background()

		var assignee *string
		if trackCreateAssignedTo != "" {
			trackCreateAssignedTo = normalizeAssignee(trackCreateAssignedTo)
			assignee = &trackCreateAssignedTo
		}

		track := &core.Track{
			Slug:       slug,
			Title:      title,
			Type:       trackCreateType,
			AssignedTo: assignee,
		}

		if err := svc.CreateTrack(ctx, track); err != nil {
			return err
		}

		w := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(w, "Created track %s: %s\n", formatTrackAlias(track), track.Title)
		if isShowTypeIDOutput() && track.ID != "" && track.ID != formatTrackAlias(track) {
			_, _ = fmt.Fprintf(w, "  ID:     %s\n", track.ID)
		}
		_, _ = fmt.Fprintf(w, "  Type:   %s\n", track.Type)
		_, _ = fmt.Fprintf(w, "  Status: %s\n", track.Status)
		if track.AssignedTo != nil {
			_, _ = fmt.Fprintf(w, "  Assigned: %s\n", *track.AssignedTo)
		}

		if configDir := resolveConfigDir(); configDir != "" {
			scaffoldTrackDir(w, track, configDir)
		}
		return nil
	},
}

// normalizeAssignee strips a leading "@" from an assignee string.
func normalizeAssignee(s string) string {
	if len(s) > 1 && s[0] == '@' {
		return s[1:]
	}
	return s
}

func init() {
	trackCreateCmd.Flags().StringVar(
		&trackCreateType, "type", "",
		"Track type (default: from config or fix)",
	)
	trackCreateCmd.Flags().StringVar(
		&trackCreateID, "id", "",
		"Custom track ID slug (derived from title if omitted)",
	)
	trackCreateCmd.Flags().StringVar(
		&trackCreateAssignedTo, "assigned-to", "",
		"Assignee (e.g. @me)",
	)

	TrackCmd.AddCommand(trackCreateCmd)
	TrackCmd.AddCommand(trackUpdateCmd)
	TrackCmd.AddCommand(trackArchiveCmd)
	TrackCmd.AddCommand(trackAbandonCmd)
	TrackCmd.AddCommand(trackDeleteCmd)
	RootCmd.AddCommand(TrackCmd)
}

// resetTrackFlags clears CLI flag state between tests.
func resetTrackFlags() {
	trackCreateType = ""
	trackCreateID = ""
	trackCreateAssignedTo = ""
	trackUpdateTitle = ""
	trackUpdateStatus = ""
	trackUpdateAssignedTo = ""
	trackUpdateType = ""
	trackUpdateAddPlan = ""
	trackShowOutputPath = ""
	trackShowIncludeLogs = false
	trackAbandonNoPrompt = false
	resetTrackListFlags()
}
