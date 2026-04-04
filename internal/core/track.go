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
	TrackTypeBug      = "bug"
	TrackTypeRefactor = "refactor"
)

// ValidTrackType returns true if t is a recognised track type (or empty).
func ValidTrackType(t string) bool {
	switch t {
	case "", TrackTypeFeature, TrackTypeBug, TrackTypeRefactor:
		return true
	}
	return false
}

// Track represents a grouping of related tasks toward a deliverable.
type Track struct {
	ID         string         `json:"id" yaml:"id"`
	Title      string         `json:"title" yaml:"title"`
	Type       string         `json:"type" yaml:"type"`
	Status     TrackStatus    `json:"status" yaml:"status"`
	AssignedTo *string        `json:"assigned_to,omitempty" yaml:"assigned_to,omitempty"`
	CreatedAt  time.Time      `json:"created_at" yaml:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at" yaml:"updated_at"`
	ProjectID  *string        `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	Meta       map[string]any `json:"meta,omitempty" yaml:"meta,omitempty"`
}

// trackIDRe matches lowercase alphanumeric characters and hyphens, 3-64 chars.
var trackIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// ValidateTrackID checks that id is lowercase alphanumeric + hyphens, 3-64 chars.
func ValidateTrackID(id string) error {
	if len(id) < 3 {
		return fmt.Errorf(
			"track ID %q too short (min 3 chars); use lowercase alphanum + hyphens",
			id,
		)
	}
	if len(id) > 64 {
		return fmt.Errorf(
			"track ID %q too long (max 64 chars); use lowercase alphanum + hyphens",
			id,
		)
	}
	if !trackIDRe.MatchString(id) {
		return fmt.Errorf(
			"track ID %q invalid; must be lowercase alphanum + hyphens, "+
				"start/end with alphanum",
			id,
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
