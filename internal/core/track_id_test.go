package core

import (
	"testing"
)

func TestParseQualifiedTrackID_Local(t *testing.T) {
	q := ParseQualifiedTrackID("browser-rendering")
	if q.ProjectSlug != "" {
		t.Errorf("expected empty ProjectSlug, got %q", q.ProjectSlug)
	}
	if q.TrackID != "browser-rendering" {
		t.Errorf("expected TrackID %q, got %q", "browser-rendering", q.TrackID)
	}
	if !q.IsLocal() {
		t.Error("expected IsLocal() = true")
	}
}

func TestParseQualifiedTrackID_Qualified(t *testing.T) {
	q := ParseQualifiedTrackID("hop-top_tlc--browser-rendering")
	if q.ProjectSlug != "hop-top_tlc" {
		t.Errorf("expected ProjectSlug %q, got %q", "hop-top_tlc", q.ProjectSlug)
	}
	if q.TrackID != "browser-rendering" {
		t.Errorf("expected TrackID %q, got %q", "browser-rendering", q.TrackID)
	}
	if q.IsLocal() {
		t.Error("expected IsLocal() = false")
	}
	if q.ProjectID() != "hop-top/tlc" {
		t.Errorf("expected ProjectID %q, got %q", "hop-top/tlc", q.ProjectID())
	}
}

func TestParseQualifiedTrackID_Empty(t *testing.T) {
	q := ParseQualifiedTrackID("")
	if q.ProjectSlug != "" || q.TrackID != "" {
		t.Errorf("expected empty QualifiedTrackID, got %+v", q)
	}
}

func TestParseQualifiedTrackID_MultipleSeparators(t *testing.T) {
	// Split on first "--" only: "a--b--c" -> slug="a", trackID="b--c"
	q := ParseQualifiedTrackID("org_repo--track--extra")
	if q.ProjectSlug != "org_repo" {
		t.Errorf("expected ProjectSlug %q, got %q", "org_repo", q.ProjectSlug)
	}
	if q.TrackID != "track--extra" {
		t.Errorf("expected TrackID %q, got %q", "track--extra", q.TrackID)
	}
}

func TestParseQualifiedTrackID_NoSeparator(t *testing.T) {
	q := ParseQualifiedTrackID("simple-track")
	if q.ProjectSlug != "" {
		t.Errorf("expected empty ProjectSlug, got %q", q.ProjectSlug)
	}
	if q.TrackID != "simple-track" {
		t.Errorf("expected TrackID %q, got %q", "simple-track", q.TrackID)
	}
}

func TestParseMultiTrackIDs(t *testing.T) {
	ids := ParseMultiTrackIDs(
		"hop-top_tlc--browser-rendering,hop-top_aps--browser-rendering",
	)
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if ids[0].ProjectSlug != "hop-top_tlc" || ids[0].TrackID != "browser-rendering" {
		t.Errorf("unexpected first ID: %+v", ids[0])
	}
	if ids[1].ProjectSlug != "hop-top_aps" || ids[1].TrackID != "browser-rendering" {
		t.Errorf("unexpected second ID: %+v", ids[1])
	}
}

func TestParseMultiTrackIDs_Mixed(t *testing.T) {
	ids := ParseMultiTrackIDs("local-track,org_repo--remote-track")
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if !ids[0].IsLocal() || ids[0].TrackID != "local-track" {
		t.Errorf("unexpected first ID: %+v", ids[0])
	}
	if ids[1].IsLocal() || ids[1].TrackID != "remote-track" {
		t.Errorf("unexpected second ID: %+v", ids[1])
	}
}

func TestParseMultiTrackIDs_Empty(t *testing.T) {
	ids := ParseMultiTrackIDs("")
	if ids != nil {
		t.Errorf("expected nil, got %v", ids)
	}
}

func TestParseMultiTrackIDs_Whitespace(t *testing.T) {
	ids := ParseMultiTrackIDs("  a , b , ")
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if ids[0].TrackID != "a" {
		t.Errorf("expected TrackID %q, got %q", "a", ids[0].TrackID)
	}
	if ids[1].TrackID != "b" {
		t.Errorf("expected TrackID %q, got %q", "b", ids[1].TrackID)
	}
}

func TestProjectSlugToID(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hop-top_tlc", "hop-top/tlc"},
		{"simple", "simple"},
		{"a_b_c", "a/b_c"}, // only first underscore
		{"", ""},
	}
	for _, tt := range tests {
		got := ProjectSlugToID(tt.input)
		if got != tt.want {
			t.Errorf("ProjectSlugToID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProjectIDToSlug(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hop-top/tlc", "hop-top_tlc"},
		{"simple", "simple"},
		{"a/b/c", "a_b/c"}, // only first slash
		{"", ""},
	}
	for _, tt := range tests {
		got := ProjectIDToSlug(tt.input)
		if got != tt.want {
			t.Errorf("ProjectIDToSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProjectSlugID_Roundtrip(t *testing.T) {
	original := "hop-top/tlc"
	slug := ProjectIDToSlug(original)
	back := ProjectSlugToID(slug)
	if back != original {
		t.Errorf("roundtrip failed: %q -> %q -> %q", original, slug, back)
	}
}

func TestFormatQualifiedTrackID(t *testing.T) {
	tests := []struct {
		q    QualifiedTrackID
		want string
	}{
		{QualifiedTrackID{TrackID: "browser-rendering"}, "browser-rendering"},
		{
			QualifiedTrackID{ProjectSlug: "hop-top_tlc", TrackID: "browser-rendering"},
			"hop-top_tlc--browser-rendering",
		},
		{QualifiedTrackID{}, ""},
	}
	for _, tt := range tests {
		got := FormatQualifiedTrackID(tt.q)
		if got != tt.want {
			t.Errorf("FormatQualifiedTrackID(%+v) = %q, want %q", tt.q, got, tt.want)
		}
	}
}

func TestFormatParse_Roundtrip(t *testing.T) {
	raw := "hop-top_tlc--browser-rendering"
	q := ParseQualifiedTrackID(raw)
	back := FormatQualifiedTrackID(q)
	if back != raw {
		t.Errorf("roundtrip failed: %q -> %+v -> %q", raw, q, back)
	}
}
