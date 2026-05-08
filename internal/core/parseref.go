package core

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// taskAliasRe matches the T-NNNN+ display alias form (≥1 digits).
// Case-insensitive so "t-0034" resolves identically to "T-0034".
var taskAliasRe = regexp.MustCompile(`(?i)^T-(\d+)$`)

// bareDigitsRe matches a bare numeric task reference like "42".
var bareDigitsRe = regexp.MustCompile(`^\d+$`)

// ParseTaskRef accepts the user-facing forms of a task reference and
// returns the durable TypeID after lookup against the repository.
//
// Accepted forms:
//   - "task_<26char>"   typeid (returned as-is after validation)
//   - "T-NNNN" / "T-1234567"  display alias resolved via (project_id, seq)
//   - "1234"            bare digits, treated as a seq
//
// projectID is the scope used for alias-based lookups (slugs/seq are
// per-project). Empty projectID matches the global bucket.
//
// Returns ErrNotFound (sentinel via fmt.Errorf) for inputs that look
// well-formed but don't resolve, and a different error for malformed
// inputs.
func ParseTaskRef(ctx context.Context, repo Repository, projectID, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty task reference")
	}

	// 1. typeid path — verify the row exists, but the input is already
	// the durable identity.
	if IsTaskID(input) {
		task, err := repo.GetTask(ctx, input)
		if err != nil {
			return "", fmt.Errorf("lookup task %q: %w", input, err)
		}
		if task == nil {
			return "", fmt.Errorf("task %q not found", input)
		}
		return task.ID, nil
	}

	// 2. T-NNNN alias.
	if m := taskAliasRe.FindStringSubmatch(input); m != nil {
		seq, parseErr := strconv.ParseInt(m[1], 10, 64)
		if parseErr != nil {
			return "", fmt.Errorf("malformed task alias %q: %w", input, parseErr)
		}
		return resolveTaskBySeq(ctx, repo, projectID, seq, input)
	}

	// 3. Bare digits.
	if bareDigitsRe.MatchString(input) {
		seq, _ := strconv.ParseInt(input, 10, 64)
		return resolveTaskBySeq(ctx, repo, projectID, seq, input)
	}

	return "", fmt.Errorf("task reference %q must be task_<typeid>, T-NNNN, or a number", input)
}

func resolveTaskBySeq(ctx context.Context, repo Repository, projectID string, seq int64, input string) (string, error) {
	task, err := repo.GetTaskBySeq(ctx, projectID, seq)
	if err != nil {
		return "", fmt.Errorf("lookup task seq %d: %w", seq, err)
	}
	if task == nil {
		return "", fmt.Errorf("task %q not found in project %q", input, projectID)
	}
	return task.ID, nil
}

// ParseTrackRef accepts the user-facing forms of a track reference and
// returns the durable TypeID after lookup.
//
// Accepted forms:
//   - "track_<26char>"   typeid
//   - "<slug>"           user-supplied alias resolved via (project_id, slug)
func ParseTrackRef(ctx context.Context, repo TrackRepository, projectID, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty track reference")
	}

	if IsTrackID(input) {
		track, err := repo.GetTrack(ctx, input)
		if err != nil {
			return "", fmt.Errorf("lookup track %q: %w", input, err)
		}
		if track == nil {
			return "", fmt.Errorf("track %q not found", input)
		}
		return track.ID, nil
	}

	// Slug path. Validate only after we know it's not a typeid so we
	// give a useful error for malformed input.
	if err := ValidateTrackSlug(input); err != nil {
		return "", fmt.Errorf("track reference %q invalid: %w", input, err)
	}

	track, err := repo.GetTrackBySlug(ctx, projectID, input)
	if err != nil {
		return "", fmt.Errorf("lookup track slug %q: %w", input, err)
	}
	if track == nil {
		return "", fmt.Errorf("track %q not found in project %q", input, projectID)
	}
	return track.ID, nil
}

// FormatTaskSeq renders a sequence number as the "T-NNNN" display alias.
// The minimum width of 4 digits preserves the legacy zero-padding for
// sub-9999 sequences while letting larger values grow naturally
// (e.g. seq 12345 → "T-12345"). Returns "" for non-positive seq.
func FormatTaskSeq(seq int64) string {
	if seq <= 0 {
		return ""
	}
	return fmt.Sprintf("T-%04d", seq)
}

// FormatTaskAlias renders a Task as its display alias (e.g. "T-0042" or
// "T-1234567"). Delegates to FormatTaskSeq so width policy lives in one
// place.
func FormatTaskAlias(t *Task) string {
	if t == nil {
		return ""
	}
	return FormatTaskSeq(t.Seq)
}

// FormatTaskDisplay returns the display alias when available, otherwise
// the task ID. Used by renderers that must always show *something* and
// can't gracefully degrade when seq is unset (e.g. dep tree, batch
// summary, critical path). Empty input yields empty output.
func FormatTaskDisplay(t *Task) string {
	if t == nil {
		return ""
	}
	if alias := FormatTaskSeq(t.Seq); alias != "" {
		return alias
	}
	return t.ID
}
