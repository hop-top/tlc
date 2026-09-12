package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// getConfigTrackTypes returns the allowed track types from config,
// or nil to use core.DefaultTrackTypes.
func getConfigTrackTypes() []string {
	types := viper.GetStringSlice("tracks.types")
	if len(types) == 0 {
		return nil
	}
	return types
}

// getConfigDefaultTrackType returns the configured default track type,
// falling back to "fix".
func getConfigDefaultTrackType() string {
	if dt := viper.GetString("tracks.default_type"); dt != "" {
		return dt
	}
	return core.TrackTypeFix
}

// getConfigSlugMaxLen returns the configured write-path limit for new
// track slugs, falling back to the package default.
func getConfigSlugMaxLen() int {
	return core.ClampSlugMaxLen(viper.GetInt("tracks.slug_max_len"))
}

// titleFromID derives a title from a track ID by replacing hyphens
// with spaces and title-casing each word.
func titleFromID(id string) string {
	words := strings.Split(id, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// autoCreateTrack creates a track after confirmation (unless
// --no-prompt). Writes status to w. Returns the new track ID.
func autoCreateTrack(
	ctx context.Context, w io.Writer,
	s *storage.SQLiteStorage, input string,
) (string, error) {
	// Write path: the typed reference becomes a brand-new slug.
	if err := core.ValidateNewTrackSlug(input, getConfigSlugMaxLen()); err != nil {
		return "", fmt.Errorf(
			"cannot auto-create track: %w; "+
				"run 'tlc track create' manually",
			err,
		)
	}

	if !taskNoPrompt {
		if !writerInteractive(w) {
			return "", fmt.Errorf(
				"track %q does not exist; "+
					"use --no-prompt to auto-create in non-interactive mode",
				input,
			)
		}
		var confirm bool
		err := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf(
						"Track %q does not exist. Create it?", input,
					)).
					Value(&confirm),
			),
		).WithOutput(w).Run()
		if err != nil || !confirm {
			return "", fmt.Errorf(
				"track %q does not exist; "+
					"run 'tlc track create %q' or use --no-prompt to auto-create",
				input, input,
			)
		}
	}

	title := titleFromID(input)
	trackType := getConfigDefaultTrackType()

	track := &core.Track{
		ID:    input,
		Title: title,
		Type:  trackType,
	}

	svc := core.NewTrackService(s, s, core.WithSlugMaxLen(getConfigSlugMaxLen()))
	if err := svc.CreateTrack(ctx, track); err != nil {
		return "", fmt.Errorf("auto-create track failed: %w", err)
	}

	_, _ = fmt.Fprintf(
		w,
		"Created track %s: %s (type: %s)\n",
		track.ID, track.Title, track.Type,
	)

	return track.ID, nil
}

// maybeAutoCreateTrack wraps autoCreateTrack for cobra commands.
func maybeAutoCreateTrack(
	ctx context.Context, cmd *cobra.Command,
	s *storage.SQLiteStorage, input string,
) (string, error) {
	return autoCreateTrack(ctx, cmd.OutOrStdout(), s, input)
}

// maybeAutoCreateTrackFromWriter wraps autoCreateTrack for io.Writer callers.
func maybeAutoCreateTrackFromWriter(
	ctx context.Context, w io.Writer,
	s *storage.SQLiteStorage, input string,
) (string, error) {
	return autoCreateTrack(ctx, w, s, input)
}
