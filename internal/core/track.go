package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// TrackStatus represents the lifecycle state of a track.
type TrackStatus string

const (
	TrackStatusPending   TrackStatus = "pending"
	TrackStatusActive    TrackStatus = "active"
	TrackStatusCompleted TrackStatus = "completed"
	TrackStatusAbandoned TrackStatus = "abandoned"
	TrackStatusArchived  TrackStatus = "archived"
)

// trackStatuses is the closed set of track statuses in lifecycle order.
// Distinct from taskStatuses: a track's status set shares the `--status`
// flag NAME with a task's but not its values, which is why the CLI scopes
// its flag-enum registrations per command rather than tree-wide.
var trackStatuses = []TrackStatus{
	TrackStatusPending,
	TrackStatusActive,
	TrackStatusCompleted,
	TrackStatusAbandoned,
	TrackStatusArchived,
}

// TrackStatuses returns the closed set of track statuses in lifecycle
// order. The returned slice is a copy.
func TrackStatuses() []TrackStatus {
	return append([]TrackStatus(nil), trackStatuses...)
}

// TrackStatusStrings returns TrackStatuses as plain strings.
func TrackStatusStrings() []string {
	return enumStrings(trackStatuses)
}

// ValidTrackStatus returns true if s is a recognised track status (or empty).
func ValidTrackStatus(s TrackStatus) bool {
	if s == "" {
		return true
	}
	for _, v := range trackStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// TrackType constants for categorising tracks.
const (
	TrackTypeFeature  = "feature"
	TrackTypeFix      = "fix"
	TrackTypeBug      = "bug"
	TrackTypeRefactor = "refactor"
	TrackTypeChore    = "chore"
	TrackTypeCI       = "ci"
	TrackTypeDocs     = "docs"
	TrackTypeStyle    = "style"
	TrackTypePerf     = "perf"
	TrackTypeTest     = "test"
	TrackTypeBuild    = "build"
)

// DefaultTrackTypes is the built-in set of allowed track types.
var DefaultTrackTypes = []string{
	TrackTypeFeature, TrackTypeFix, TrackTypeBug, TrackTypeRefactor,
	TrackTypeChore, TrackTypeCI, TrackTypeDocs, TrackTypeStyle,
	TrackTypePerf, TrackTypeTest, TrackTypeBuild,
}

// ValidTrackType returns true if t is a recognised track type (or empty).
// When configTypes is non-empty, it overrides the default set.
func ValidTrackType(t string, configTypes ...[]string) bool {
	if t == "" {
		return true
	}
	types := DefaultTrackTypes
	if len(configTypes) > 0 && len(configTypes[0]) > 0 {
		types = configTypes[0]
	}
	for _, allowed := range types {
		if t == allowed {
			return true
		}
	}
	return false
}

// TrackTypeList returns a comma-separated string of allowed types.
func TrackTypeList(configTypes ...[]string) string {
	types := DefaultTrackTypes
	if len(configTypes) > 0 && len(configTypes[0]) > 0 {
		types = configTypes[0]
	}
	return strings.Join(types, ", ")
}

// Track represents a grouping of related tasks toward a deliverable.
type Track struct {
	ID          string         `json:"id" yaml:"id" table:"ID"`
	Slug        string         `json:"slug" yaml:"slug"`
	Title       string         `json:"title" yaml:"title" table:"Title"`
	Type        string         `json:"type" yaml:"type" table:"Type"`
	Status      TrackStatus    `json:"status" yaml:"status" table:"Status"`
	AssignedTo  *string        `json:"assigned_to,omitempty" yaml:"assigned_to,omitempty" table:"Assigned"`
	CreatedAt   time.Time      `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" yaml:"updated_at"`
	ProjectID   *string        `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Meta        map[string]any `json:"meta,omitempty" yaml:"meta,omitempty"`
	PlanMapping map[int]string `json:"plan_mapping,omitempty" yaml:"plan_mapping,omitempty"`
	DueAt       *time.Time     `json:"due_at,omitempty" yaml:"due_at,omitempty"`
}

// trackSlugRe matches lowercase alphanumeric characters and hyphens, 3-64 chars.
var trackSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// MinTrackSlugLen is the shortest acceptable track slug.
const MinTrackSlugLen = 3

// MaxTrackSlugLen is the READ-path ceiling. Slugs up to this length stay
// resolvable forever: tracks created before the shorter write-path limit
// keep their names, and there is no migration or re-slugging.
const MaxTrackSlugLen = 64

// DefaultNewTrackSlugMaxLen is the write-path ceiling applied to newly
// created slugs when no limit is configured.
const DefaultNewTrackSlugMaxLen = 24

// ValidateTrackSlug is the READ-path validator: it checks that slug is
// lowercase alphanumeric + hyphens, MinTrackSlugLen..MaxTrackSlugLen chars.
// Slugs are the human-typeable display alias for tracks; the durable identity
// is Track.ID (a TypeID like track_<26char>).
//
// Keep this ceiling at MaxTrackSlugLen. Lookups must continue to resolve
// long slugs minted before the write-path limit existed. To validate a slug
// that is about to be written, use ValidateNewTrackSlug instead.
func ValidateTrackSlug(slug string) error {
	return validateTrackSlug(slug, MaxTrackSlugLen)
}

// ValidateNewTrackSlug is the WRITE-path validator: it applies every
// ValidateTrackSlug rule plus the configured length limit for slugs being
// minted now. A maxLen of 0 means DefaultNewTrackSlugMaxLen; values above
// MaxTrackSlugLen are capped there so the write limit can never exceed what
// lookups accept.
//
// This is a distinct entry point rather than a mode flag on
// ValidateTrackSlug: read and write call sites must be classified at the
// call, not toggled by an argument that is easy to pass wrong.
func ValidateNewTrackSlug(slug string, maxLen int) error {
	return validateTrackSlug(slug, ClampSlugMaxLen(maxLen))
}

// ClampSlugMaxLen normalises a configured new-slug limit: 0 (or less) means
// the default, and anything above the read ceiling is capped there.
func ClampSlugMaxLen(maxLen int) int {
	if maxLen <= 0 {
		return DefaultNewTrackSlugMaxLen
	}
	if maxLen > MaxTrackSlugLen {
		return MaxTrackSlugLen
	}
	return maxLen
}

func validateTrackSlug(slug string, maxLen int) error {
	if len(slug) < MinTrackSlugLen {
		return fmt.Errorf(
			"track slug %q too short (min %d chars); use lowercase alphanum + hyphens",
			slug, MinTrackSlugLen,
		)
	}
	if len(slug) > maxLen {
		return fmt.Errorf(
			"track slug %q too long (max %d chars); use lowercase alphanum + hyphens",
			slug, maxLen,
		)
	}
	if !trackSlugRe.MatchString(slug) {
		return fmt.Errorf(
			"track slug %q invalid; must be lowercase alphanum + hyphens, "+
				"start/end with alphanum",
			slug,
		)
	}
	return nil
}

// SlugFromTitle derives a track ID slug from a human-readable title.
// Lowercases, replaces non-alphanum runs with hyphens, trims, and clamps
// to maxLen chars (0 means DefaultNewTrackSlugMaxLen).
//
// Clamping prefers a word boundary: the slug is cut at the last hyphen
// inside the budget so it ends on a whole word. When no hyphen fits (the
// first word alone overruns the budget) it falls back to a hard cut.
func SlugFromTitle(title string, maxLen int) string {
	lower := strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	prevHyphen := false
	for _, r := range lower {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	slug := strings.TrimRight(b.String(), "-")
	return clampSlug(slug, ClampSlugMaxLen(maxLen))
}

// clampSlug trims slug to at most maxLen chars, preferring the last hyphen
// inside the budget so the result ends on a whole word. Falls back to a hard
// cut when no hyphen fits. Never leaves a trailing hyphen.
//
// The budget yields to the MinTrackSlugLen floor: when every prefix inside
// the budget would be shorter than the minimum (a leading word shorter than
// the minimum followed by a hyphen), the cut extends to the first prefix
// that is long enough and ends on an alphanum.
func clampSlug(slug string, maxLen int) string {
	if len(slug) <= maxLen {
		return slug
	}
	cut := strings.TrimRight(slug[:maxLen], "-")
	if i := strings.LastIndexByte(cut, '-'); i >= MinTrackSlugLen {
		cut = strings.TrimRight(cut[:i], "-")
	}
	if len(cut) >= MinTrackSlugLen {
		return cut
	}
	return minLenPrefix(slug)
}

// minLenPrefix returns the shortest prefix of slug that is at least
// MinTrackSlugLen chars and ends on an alphanum, or slug itself when no
// such prefix exists.
func minLenPrefix(slug string) string {
	for n := MinTrackSlugLen; n <= len(slug); n++ {
		if slug[n-1] != '-' {
			return slug[:n]
		}
	}
	return strings.TrimRight(slug, "-")
}
