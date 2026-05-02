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

// ValidTrackStatus returns true if s is a recognised track status (or empty).
func ValidTrackStatus(s TrackStatus) bool {
	switch s {
	case "", TrackStatusPending, TrackStatusActive, TrackStatusCompleted,
		TrackStatusAbandoned, TrackStatusArchived:
		return true
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
}

// trackSlugRe matches lowercase alphanumeric characters and hyphens, 3-64 chars.
var trackSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// ValidateTrackSlug checks that slug is lowercase alphanumeric + hyphens, 3-64 chars.
// Slugs are the human-typeable display alias for tracks; the durable identity
// is Track.ID (a TypeID like track_<26char>).
func ValidateTrackSlug(slug string) error {
	if len(slug) < 3 {
		return fmt.Errorf(
			"track slug %q too short (min 3 chars); use lowercase alphanum + hyphens",
			slug,
		)
	}
	if len(slug) > 64 {
		return fmt.Errorf(
			"track slug %q too long (max 64 chars); use lowercase alphanum + hyphens",
			slug,
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
// to 64 chars.
func SlugFromTitle(title string) string {
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
	if len(slug) > 64 {
		slug = strings.TrimRight(slug[:64], "-")
	}
	return slug
}
