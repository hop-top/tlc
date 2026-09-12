package core

import (
	"testing"
)

func TestValidateTrackSlug(t *testing.T) {
	tests := []struct {
		slug    string
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
		err := ValidateTrackSlug(tc.slug)
		if tc.wantErr && err == nil {
			t.Errorf("ValidateTrackSlug(%q) expected error, got nil", tc.slug)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("ValidateTrackSlug(%q) unexpected error: %v", tc.slug, err)
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
		got := SlugFromTitle(tc.title, MaxTrackSlugLen)
		if got != tc.want {
			t.Errorf("SlugFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestSlugFromTitle_Clamp64(t *testing.T) {
	long := "this is a very long title that should be clamped to sixty four characters maximum"
	slug := SlugFromTitle(long, MaxTrackSlugLen)
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

// TestSlugFromTitle_WordBoundary pins that clamping ends on a whole word
// rather than mid-token.
func TestSlugFromTitle_WordBoundary(t *testing.T) {
	tests := []struct {
		title string
		limit int
		want  string
	}{
		{
			title: "Config-driven label templates and workflow wiring",
			limit: 24,
			want:  "config-driven-label",
		},
		// Already inside the budget: untouched.
		{title: "Track Registry Design", limit: 24, want: "track-registry-design"},
		// No hyphen fits in the budget: hard cut.
		{title: "Supercalifragilisticexpialidocious rules", limit: 10, want: "supercalif"},
		// First word exactly fills the budget.
		{title: "abcdefghij klmno", limit: 10, want: "abcdefghij"},
		{title: "", limit: 24, want: ""},
	}
	for _, tc := range tests {
		got := SlugFromTitle(tc.title, tc.limit)
		if got != tc.want {
			t.Errorf("SlugFromTitle(%q, %d) = %q, want %q", tc.title, tc.limit, got, tc.want)
		}
	}
}

// TestSlugFromTitle_ClampInvariants pins the structural rules that hold for
// any title/limit pair: within budget, no trailing hyphen, and never a
// sub-minimum slug produced from a non-empty first word.
func TestSlugFromTitle_ClampInvariants(t *testing.T) {
	titles := []string{
		"this is a very long title that should be clamped to the configured maximum length",
		"Config-driven label templates and workflow wiring",
		"ab cdefghijklmnopqrstuvwxyz",
		"a-b-c-d-e-f-g-h-i-j-k-l-m-n-o-p",
	}
	for _, limit := range []int{3, 8, 24, 64} {
		for _, title := range titles {
			slug := SlugFromTitle(title, limit)
			// The budget holds, except where honoring it would emit a
			// slug below MinTrackSlugLen: the validity floor wins, and
			// the overrun is the shortest valid prefix.
			if len(slug) > limit {
				want := minLenPrefix(SlugFromTitle(title, MaxTrackSlugLen))
				if slug != want {
					t.Errorf("SlugFromTitle(%q, %d) = %q (%d chars), want <= %d or floor %q",
						title, limit, slug, len(slug), limit, want)
				}
			}
			if slug == "" {
				continue
			}
			if slug[len(slug)-1] == '-' {
				t.Errorf("SlugFromTitle(%q, %d) = %q ends with hyphen", title, limit, slug)
			}
			if err := ValidateTrackSlug(slug); err != nil {
				t.Errorf("SlugFromTitle(%q, %d) = %q fails read validation: %v",
					title, limit, slug, err)
			}
		}
	}
}

// TestValidateNewTrackSlug pins the write-path ceiling: new slugs are capped
// at the configured limit, while the read-path validator keeps the legacy
// 64-char ceiling so existing tracks stay resolvable.
func TestValidateNewTrackSlug(t *testing.T) {
	// 25 chars — over the default write limit, under the read ceiling.
	newSlug25 := "abcdefghijklmnopqrstuvwxy"
	if len(newSlug25) != 25 {
		t.Fatalf("fixture length drift: %d", len(newSlug25))
	}
	if err := ValidateNewTrackSlug(newSlug25, 24); err == nil {
		t.Errorf("ValidateNewTrackSlug(%q, 24) expected error, got nil", newSlug25)
	}
	if err := ValidateTrackSlug(newSlug25); err != nil {
		t.Errorf("ValidateTrackSlug(%q) unexpected error on read path: %v", newSlug25, err)
	}

	// 24 chars — exactly at the limit, accepted.
	at24 := newSlug25[:24]
	if err := ValidateNewTrackSlug(at24, 24); err != nil {
		t.Errorf("ValidateNewTrackSlug(%q, 24) unexpected error: %v", at24, err)
	}

	// Zero limit means "use the default".
	if err := ValidateNewTrackSlug(newSlug25, 0); err == nil {
		t.Errorf("ValidateNewTrackSlug(%q, 0) expected default limit to reject", newSlug25)
	}

	// Shape rules still apply on the write path.
	for _, bad := range []string{"ab", "-abc", "abc-", "My-Track", "my_track"} {
		if err := ValidateNewTrackSlug(bad, 24); err == nil {
			t.Errorf("ValidateNewTrackSlug(%q, 24) expected error, got nil", bad)
		}
	}
}

// TestTrackSlugGrandfathering is the crux of the short-id decision: a legacy
// 53-char slug must still resolve on the read path forever (no migration, no
// re-slugging) while a 25-char explicit new slug is refused on the write path.
func TestTrackSlugGrandfathering(t *testing.T) {
	legacy := "config-driven-label-templates-and-workflow-wiring-etc"
	if len(legacy) != 53 {
		t.Fatalf("legacy fixture length drift: %d", len(legacy))
	}

	// Read path: resolves.
	if err := ValidateTrackSlug(legacy); err != nil {
		t.Errorf("legacy slug %q (%d chars) must stay valid on read path: %v",
			legacy, len(legacy), err)
	}

	// Write path: refused for a brand-new track.
	if err := ValidateNewTrackSlug(legacy, 24); err == nil {
		t.Errorf("legacy-length slug %q must be refused as a NEW slug", legacy)
	}

	newOverLimit := "abcdefghijklmnopqrstuvwxy" // 25 chars
	if err := ValidateNewTrackSlug(newOverLimit, 24); err == nil {
		t.Errorf("25-char slug %q must be refused as a NEW slug", newOverLimit)
	}
}
