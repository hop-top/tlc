package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// ErrTrackNotFound is returned when no track matches and zero fuzzy
// candidates exist — the caller may offer to auto-create.
var ErrTrackNotFound = errors.New("track not found")

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

	// Load all track IDs for prefix + fuzzy (no limit).
	all, err := s.ListTracks(ctx, core.TrackQuery{})
	if err != nil {
		return "", fmt.Errorf(
			"track list failed: %w", err,
		)
	}
	if len(all) == 0 {
		return "", fmt.Errorf(
			"track %q: %w; no tracks exist; "+
				"run 'tlc track create' first",
			input, ErrTrackNotFound,
		)
	}

	// Deduplicate IDs (tracks may appear per-project).
	seen := make(map[string]struct{}, len(all))
	var ids []string
	for _, t := range all {
		if _, ok := seen[t.ID]; ok {
			continue
		}
		seen[t.ID] = struct{}{}
		ids = append(ids, t.ID)
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
			"track %q: %w; run 'tlc track list' "+
				"to see available tracks",
			input, ErrTrackNotFound,
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
