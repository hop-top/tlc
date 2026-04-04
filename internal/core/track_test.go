package core

import (
	"testing"
)

func TestValidateTrackID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"abc", false},
		{"my-track", false},
		{"feature-login-v2", false},
		{"a1b2c3", false},
		{"aaa-bbb-ccc-ddd", false},
		// exactly 64 chars
		{"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01", false},
		// too short
		{"ab", true},
		{"a", true},
		{"", true},
		// too long (65 chars)
		{"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz012", true},
		// uppercase
		{"My-Track", true},
		// starts with hyphen
		{"-abc", true},
		// ends with hyphen
		{"abc-", true},
		// spaces
		{"my track", true},
		// underscore
		{"my_track", true},
		// special chars
		{"my@track", true},
	}

	for _, tc := range tests {
		err := ValidateTrackID(tc.id)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateTrackID(%q) expected error, got nil", tc.id)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ValidateTrackID(%q) unexpected error: %v", tc.id, err)
		}
	}
}

func TestSlugFromTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Track Registry Design", "track-registry-design"},
		{"  Hello World  ", "hello-world"},
		{"feature/login-v2", "feature-login-v2"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Special @#$ chars!", "special-chars"},
		{"Already-slugged", "already-slugged"},
		{"123 numeric start", "123-numeric-start"},
		{"", ""},
		{"  ", ""},
		{"trailing dash --", "trailing-dash"},
	}

	for _, tc := range tests {
		got := SlugFromTitle(tc.title)
		if got != tc.want {
			t.Errorf("SlugFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestSlugFromTitle_Clamp64(t *testing.T) {
	long := "this is a very long title that should be clamped to sixty four characters maximum"
	slug := SlugFromTitle(long)
	if len(slug) > 64 {
		t.Errorf("SlugFromTitle produced slug of %d chars, want <= 64", len(slug))
	}
	// must not end with hyphen after truncation
	if slug[len(slug)-1] == '-' {
		t.Errorf("SlugFromTitle slug ends with hyphen after clamp: %q", slug)
	}
}

func TestValidTrackType(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{"", true},
		{TrackTypeFeature, true},
		{TrackTypeBug, true},
		{TrackTypeRefactor, true},
		{"epic", false},
		{"Feature", false},
		{"unknown", false},
	}

	for _, tc := range tests {
		got := ValidTrackType(tc.typ)
		if got != tc.want {
			t.Errorf("ValidTrackType(%q) = %v, want %v", tc.typ, got, tc.want)
		}
	}
}

func TestValidTrackStatus(t *testing.T) {
	tests := []struct {
		status TrackStatus
		want   bool
	}{
		{"", true},
		{TrackStatusPending, true},
		{TrackStatusActive, true},
		{TrackStatusCompleted, true},
		{TrackStatusAbandoned, true},
		{TrackStatusArchived, true},
		{"invalid", false},
		{"ACTIVE", false},
	}

	for _, tc := range tests {
		got := ValidTrackStatus(tc.status)
		if got != tc.want {
			t.Errorf("ValidTrackStatus(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
