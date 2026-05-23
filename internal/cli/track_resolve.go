package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sahilm/fuzzy"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// ErrTrackNotFound is returned when no track matches and zero fuzzy
// candidates exist — the caller may offer to auto-create.
var ErrTrackNotFound = errors.New("track not found")

// trackNotFoundError carries the user-facing message and routes through
// kit's cli middleware as a NOT_FOUND envelope (exit 3). Wraps the
// ErrTrackNotFound sentinel so existing errors.Is callers keep working.
type trackNotFoundError struct {
	msg string
}

func (e *trackNotFoundError) Error() string { return e.msg }
func (e *trackNotFoundError) Unwrap() error { return ErrTrackNotFound }
func (e *trackNotFoundError) AsCLIError() *output.Error {
	return output.NotFoundError(e.msg)
}

// newTrackNotFoundError constructs a typed not-found error from the
// existing message format.
func newTrackNotFoundError(format string, args ...any) error {
	return &trackNotFoundError{msg: fmt.Sprintf(format, args...)}
}

// currentProjectID returns the active project ID for the current cwd,
// or "" if no project context is detected.
func currentProjectID() string {
	if proj := core.DetectProject(); proj != nil && proj.InProject {
		return proj.ProjectID
	}
	return ""
}

// resolveTrackID resolves a track reference to its canonical TypeID.
//
// Accepted user-facing forms:
//   - "track_<26char>"   TypeID — strict resolution, no fuzzy fallback
//   - "<slug>"           exact slug under the active project
//   - partial / prefix / fuzzy — fallback for slug-shaped input
//
// TypeID inputs are routed through core.ParseTrackRef so callers
// downstream always see the canonical TypeID. A typeid that doesn't
// resolve fails fast (no fuzzy fallback). Slug inputs first try the
// strict (project_id, slug) lookup; if that misses, the resolver falls
// through to prefix and then fuzzy matching for legacy convenience.
func resolveTrackID(
	ctx context.Context, s *storage.SQLiteStorage, input string,
) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf(
			"track ID is required; run 'tlc track list'",
		)
	}

	projectID := currentProjectID()

	// 1a. Strict TypeID path — never fall through to fuzzy.
	if core.IsTrackID(input) {
		id, err := core.ParseTrackRef(ctx, s, projectID, input)
		if err != nil {
			return "", newTrackNotFoundError(
				"track %q not found; run 'tlc track list' to see available tracks",
				input,
			)
		}
		return id, nil
	}

	// 1b. Strict slug path — exact (project_id, slug) match.
	if id, err := core.ParseTrackRef(ctx, s, projectID, input); err == nil {
		return id, nil
	}

	// Fallback: slug fallback inside SQLiteStorage.GetTrack also covers
	// pre-typeid rows where Track.ID holds the slug (used by some legacy
	// tests and unmigrated data).
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
		return "", newTrackNotFoundError(
			"track %q not found; no tracks exist; run 'tlc track create' first",
			input,
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
		return "", newTrackNotFoundError(
			"track %q not found; run 'tlc track list' to see available tracks",
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
