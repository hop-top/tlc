package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// resolveTrackID resolves a possibly-partial track ID to an exact
// match. Resolution order: exact → prefix → fuzzy.
// Returns an actionable error on no match or ambiguous match.
func resolveTrackID(
	ctx context.Context, s *storage.SQLiteStorage, input string,
) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf(
			"track ID is required; run 'tlc track list'",
		)
	}

	// 1. Exact match.
	track, err := s.GetTrack(ctx, input)
	if err != nil {
		return "", fmt.Errorf(
			"track lookup failed: %w", err,
		)
	}
	if track != nil {
		return track.ID, nil
	}

	// Load all track IDs for prefix + fuzzy.
	all, err := s.ListTracks(ctx, core.TrackQuery{Limit: 500})
	if err != nil {
		return "", fmt.Errorf(
			"track list failed: %w", err,
		)
	}
	if len(all) == 0 {
		return "", fmt.Errorf(
			"track %q not found; no tracks exist; "+
				"run 'tlc track create' first",
			input,
		)
	}

	ids := make([]string, len(all))
	for i, t := range all {
		ids[i] = t.ID
	}

	// 2. Prefix match.
	lower := strings.ToLower(input)
	var prefixMatches []string
	for _, id := range ids {
		if strings.HasPrefix(strings.ToLower(id), lower) {
			prefixMatches = append(prefixMatches, id)
		}
	}
	if len(prefixMatches) == 1 {
		return prefixMatches[0], nil
	}
	if len(prefixMatches) > 1 {
		return "", fmt.Errorf(
			"track %q is ambiguous; matches: %s",
			input, strings.Join(prefixMatches, ", "),
		)
	}

	// 3. Fuzzy match.
	results := fuzzy.Find(lower, ids)
	if len(results) == 0 {
		return "", fmt.Errorf(
			"track %q not found; run 'tlc track list' "+
				"to see available tracks",
			input,
		)
	}
	if len(results) == 1 || results[0].Score > results[1].Score {
		return ids[results[0].Index], nil
	}

	// Multiple equally-scored fuzzy matches.
	var candidates []string
	topScore := results[0].Score
	for _, r := range results {
		if r.Score < topScore {
			break
		}
		candidates = append(candidates, ids[r.Index])
	}
	return "", fmt.Errorf(
		"track %q is ambiguous; matches: %s",
		input, strings.Join(candidates, ", "),
	)
}
