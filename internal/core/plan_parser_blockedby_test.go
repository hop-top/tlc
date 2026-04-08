package core

// Unit tests for BlockedByRef parsing across YAML frontmatter
// forms. Covers story 076 scenario 5 (concrete T-NNNN accepted)
// plus mixed-entry sanity checks.

import (
	"testing"
)

func TestParsePlanFrontmatter_BlockedByMixedEntries(t *testing.T) {
	path := writeTempPlan(t, `---
title: Mixed plan
tracks: [foo]
tasks:
  - title: "A"
  - title: "B"
    blocked-by: [0, "T-0099", "bar#3"]
---
# body
`)
	fm, err := ParsePlanFrontmatter(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(fm.Tasks) != 2 {
		t.Fatalf("tasks len = %d, want 2", len(fm.Tasks))
	}
	refs := fm.Tasks[1].BlockedBy
	if len(refs) != 3 {
		t.Fatalf("refs len = %d, want 3: %+v", len(refs), refs)
	}

	if !refs[0].IsIndex() || refs[0].Index != 0 {
		t.Errorf("refs[0] = %+v, want index 0", refs[0])
	}
	if refs[1].TaskID != "T-0099" {
		t.Errorf("refs[1] = %+v, want TaskID T-0099", refs[1])
	}
	if refs[2].CrossTrack == nil ||
		refs[2].CrossTrack.TrackID != "bar" ||
		refs[2].CrossTrack.TaskNum != 3 {
		t.Errorf("refs[2] = %+v, want bar#3", refs[2])
	}
}

func TestParseBlockedByRef_Invalid(t *testing.T) {
	for _, s := range []string{"not-a-ref", "#5", "bar#0", "T-", ""} {
		if _, err := parseBlockedByRef(s); err == nil {
			t.Errorf("parseBlockedByRef(%q): expected error", s)
		}
	}
}

func TestBlockedByRef_UnmarshalJSON_RejectsNullAndEmpty(t *testing.T) {
	for _, input := range []string{"null", ""} {
		var r BlockedByRef
		err := r.UnmarshalJSON([]byte(input))
		if err == nil {
			t.Errorf("UnmarshalJSON(%q): expected error, got nil "+
				"(would silently create index-0 dep)", input)
		}
	}
}

func TestBlockedByRef_Raw(t *testing.T) {
	cases := map[string]BlockedByRef{
		"0":     {Index: 0},
		"T-007": {TaskID: "T-007"},
		"alpha#2": {
			CrossTrack: &CrossTrackRef{TrackID: "alpha", TaskNum: 2},
		},
	}
	for want, ref := range cases {
		if got := ref.Raw(); got != want {
			t.Errorf("Raw() = %q, want %q", got, want)
		}
	}
}
