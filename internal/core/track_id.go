package core

import "strings"

// QualifiedTrackID represents a track ID optionally scoped to a project.
// The qualified form is "project-slug--track-id"; the local form is just
// "track-id" (ProjectSlug is empty).
type QualifiedTrackID struct {
	ProjectSlug string // e.g. "hop-top_tlc" (empty = local)
	TrackID     string // e.g. "browser-rendering"
}

// qualifiedSep is the separator between project slug and track ID.
const qualifiedSep = "--"

// ParseQualifiedTrackID splits raw on the first "--" occurrence.
//
//	"hop-top_tlc--browser-rendering" -> {ProjectSlug: "hop-top_tlc", TrackID: "browser-rendering"}
//	"browser-rendering"              -> {ProjectSlug: "", TrackID: "browser-rendering"}
//	""                               -> {ProjectSlug: "", TrackID: ""}
func ParseQualifiedTrackID(raw string) QualifiedTrackID {
	idx := strings.Index(raw, qualifiedSep)
	if idx < 0 {
		return QualifiedTrackID{TrackID: raw}
	}
	return QualifiedTrackID{
		ProjectSlug: raw[:idx],
		TrackID:     raw[idx+len(qualifiedSep):],
	}
}

// ParseMultiTrackIDs splits a comma-separated string of qualified IDs.
// Empty string returns nil.
func ParseMultiTrackIDs(raw string) []QualifiedTrackID {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]QualifiedTrackID, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, ParseQualifiedTrackID(p))
	}
	return out
}

// ProjectSlugToID converts "hop-top_tlc" to "hop-top/tlc".
// Only the first underscore is replaced.
func ProjectSlugToID(slug string) string {
	idx := strings.Index(slug, "_")
	if idx < 0 {
		return slug
	}
	return slug[:idx] + "/" + slug[idx+1:]
}

// ProjectIDToSlug converts "hop-top/tlc" to "hop-top_tlc".
// Only the first slash is replaced.
func ProjectIDToSlug(id string) string {
	idx := strings.Index(id, "/")
	if idx < 0 {
		return id
	}
	return id[:idx] + "_" + id[idx+1:]
}

// FormatQualifiedTrackID returns the string form of a qualified track ID.
// If ProjectSlug is empty, returns just the TrackID.
func FormatQualifiedTrackID(q QualifiedTrackID) string {
	if q.ProjectSlug == "" {
		return q.TrackID
	}
	return q.ProjectSlug + qualifiedSep + q.TrackID
}

// IsLocal returns true if the qualified ID has no project scope.
func (q QualifiedTrackID) IsLocal() bool {
	return q.ProjectSlug == ""
}

// ProjectID returns the project ID (slug converted to org/repo form).
// Returns empty string for local IDs.
func (q QualifiedTrackID) ProjectID() string {
	if q.ProjectSlug == "" {
		return ""
	}
	return ProjectSlugToID(q.ProjectSlug)
}
