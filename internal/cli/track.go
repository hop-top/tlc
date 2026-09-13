package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	kitcli "hop.top/kit/go/console/cli"
	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var (
	trackCreateType       string
	trackCreateID         string
	trackCreateAssignedTo string
	trackCreateDue        string

	trackUpdateTitle      string
	trackUpdateStatus     string
	trackUpdateAssignedTo string
	trackUpdateType       string
	trackUpdateAddPlan    string
	trackUpdateDue        string

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
	Long: `Create a new track with the given title.

Derives a slug from the title (or accepts --id) and mints a durable
TypeID. Honours the stage gate (feature_freeze / maintenance / sunset
/ archived) for the active project scope. Each invocation creates a
fresh track, so the operation is not idempotent.

With --recipe the track is built from a recipe: the title comes from the
argument or the recipe's track.title, the type from --type or
track.type, plan.md from track.plan, and the recipe's steps are
materialized into the new track as tasks. --var binds the recipe's
vars, --task keeps a subset of the steps, --for names the task or track
the recipe applies to, --assign picks assignees for steps that name
none. --dry-run prints what would be created without writing.

Examples:
  tlc track create "Browser rendering" --type feature
  tlc track create --recipe release --var version=1.4.0
  tlc track create "Review 42" --recipe code-review --var pr=42 --task 1-3 --with-deps`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "no",
	},
	Args: cobra.MaximumNArgs(1),
	RunE: runTrackCreate,
}

func runTrackCreate(cmd *cobra.Command, args []string) error {
	if recipeFlagRecipe != "" {
		return runTrackCreateRecipe(cmd, args)
	}
	if len(args) == 0 {
		return fmt.Errorf(
			"track create requires a title; re-run with: tlc track create \"<title>\" (or pass --recipe <recipe>)",
		)
	}
	track, err := newTrackFromFlags(trimMatchingQuotes(args[0]), trackCreateType)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	if kitcli.IsDryRun(cmd) {
		printTrackCreateDryRun(w, track)
		return nil
	}

	s, err := getStorageRaw()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	if err := newTrackService(s).CreateTrack(context.Background(), track); err != nil {
		return err //nolint:wrapcheck // the service names the slug or type that was refused
	}
	printTrackCreated(w, track)
	if configDir := resolveConfigDir(); configDir != "" {
		scaffoldTrackDir(w, track, configDir)
	}
	return nil
}

// newTrackFromFlags builds the track `track create` would write: slug
// from --id or the title, the given type (config default when empty)
// validated against the configured types, assignee and due from the
// flags. The stage gate runs here so a dry run reports it too.
func newTrackFromFlags(title, trackType string) (*core.Track, error) {
	// User-supplied --id (or auto-derived from title) becomes the
	// human-facing Slug. Track.ID itself is the durable TypeID,
	// minted by TrackService.CreateTrack via core.NewTrackID().
	slug := trackCreateID
	if slug == "" {
		slug = core.SlugFromTitle(title, getConfigSlugMaxLen())
	}

	if trackType == "" {
		trackType = getConfigDefaultTrackType()
	}
	cfgTypes := getConfigTrackTypes()
	if !core.ValidTrackType(trackType, cfgTypes) {
		return nil, fmt.Errorf(
			"track type %q invalid; valid types: %s",
			trackType, core.TrackTypeList(cfgTypes),
		)
	}

	// Stage gate: refuse the create up-front when the active scope's
	// kit/core/stage forbids it (feature_freeze / maintenance / sunset
	// / archived per the default policy table). Scope is the project
	// detected for cwd; an empty scope skips the gate so stage-naive
	// adopters see no change. See core.GateTrackCreate for the table.
	if proj := core.DetectProject(); proj != nil && proj.ProjectID != "" {
		if err := core.GateTrackCreate(proj.ProjectID, trackType); err != nil {
			return nil, err //nolint:wrapcheck // the gate names the stage and the fix
		}
	}

	track := &core.Track{Slug: slug, Title: title, Type: trackType}
	if trackCreateAssignedTo != "" {
		assignee := normalizeAssignee(trackCreateAssignedTo)
		track.AssignedTo = &assignee
	}
	if trackCreateDue != "" {
		t, perr := util.ParseUntil(trackCreateDue)
		if perr != nil {
			return nil, fmt.Errorf("invalid --due %q: %w", trackCreateDue, perr)
		}
		track.DueAt = &t
	}
	return track, nil
}

func newTrackService(s *storage.SQLiteStorage) *core.TrackService {
	return core.NewTrackService(s, s, core.WithSlugMaxLen(getConfigSlugMaxLen()))
}

func printTrackCreated(w io.Writer, track *core.Track) {
	_, _ = fmt.Fprintf(w, "Created track %s: %s\n", formatTrackAlias(track), track.Title)
	if isShowTypeIDOutput() && track.ID != "" && track.ID != formatTrackAlias(track) {
		_, _ = fmt.Fprintf(w, "  ID:     %s\n", track.ID)
	}
	_, _ = fmt.Fprintf(w, "  Type:   %s\n", track.Type)
	_, _ = fmt.Fprintf(w, "  Status: %s\n", track.Status)
	if track.AssignedTo != nil {
		_, _ = fmt.Fprintf(w, "  Assigned: %s\n", *track.AssignedTo)
	}
}

func printTrackCreateDryRun(w io.Writer, track *core.Track) {
	_, _ = fmt.Fprintf(w, "Dry run — would create track %s: %s\n", formatTrackAlias(track), track.Title)
	_, _ = fmt.Fprintf(w, "  Type:   %s\n", track.Type)
	if track.AssignedTo != nil {
		_, _ = fmt.Fprintf(w, "  Assigned: %s\n", *track.AssignedTo)
	}
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
	trackCreateCmd.Flags().StringVar(
		&trackCreateDue, "due", "",
		"Due date (tomorrow, in 3d, 2025-05-01, RFC3339)",
	)
	registerRecipeFlags(trackCreateCmd)

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
	trackCreateDue = ""
	trackUpdateTitle = ""
	trackUpdateStatus = ""
	trackUpdateAssignedTo = ""
	trackUpdateType = ""
	trackUpdateAddPlan = ""
	trackUpdateDue = ""
	trackShowOutputPath = ""
	trackShowIncludeLogs = false
	trackAbandonNoPrompt = false
	resetTrackListFlags()
}
