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
		"hop-top/c12n#T-0018": {
			CrossProject: &CrossProjectRef{
				ProjectID: "hop-top/c12n",
				TaskID:    "T-0018",
			},
		},
	}
	for want, ref := range cases {
		if got := ref.Raw(); got != want {
			t.Errorf("Raw() = %q, want %q", got, want)
		}
	}
}

func TestParseBlockedByRef_CrossProject(t *testing.T) {
	tests := []struct {
		input    string
		wantProj string
		wantTask string
	}{
		{
			input:    "hop-top/c12n#T-0018",
			wantProj: "hop-top/c12n",
			wantTask: "T-0018",
		},
		{
			input:    "my_org/my_project#T-0001",
			wantProj: "my_org/my_project",
			wantTask: "T-0001",
		},
		{
			input:    "tlc://hop-top/c12n/T-0018",
			wantProj: "hop-top/c12n",
			wantTask: "T-0018",
		},
		{
			// Local URI: tlc:///T-0018 → bare task ID
			input:    "tlc:///T-0018",
			wantProj: "",
			wantTask: "T-0018",
		},
	}
	for _, tt := range tests {
		ref, err := parseBlockedByRef(tt.input)
		if err != nil {
			t.Errorf("parseBlockedByRef(%q): unexpected error: %v",
				tt.input, err)
			continue
		}

		if tt.wantProj == "" && ref.CrossProject != nil {
			t.Errorf("parseBlockedByRef(%q): want no CrossProject, "+
				"got %+v", tt.input, ref.CrossProject)
			continue
		}

		if tt.wantProj != "" {
			if ref.CrossProject == nil {
				t.Errorf("parseBlockedByRef(%q): want CrossProject "+
					"%q/%q, got nil", tt.input, tt.wantProj,
					tt.wantTask)
				continue
			}
			if ref.CrossProject.ProjectID != tt.wantProj {
				t.Errorf("parseBlockedByRef(%q): ProjectID = %q, "+
					"want %q", tt.input,
					ref.CrossProject.ProjectID, tt.wantProj)
			}
			if ref.CrossProject.TaskID != tt.wantTask {
				t.Errorf("parseBlockedByRef(%q): TaskID = %q, "+
					"want %q", tt.input,
					ref.CrossProject.TaskID, tt.wantTask)
			}
		} else {
			// Local ref — should be a bare TaskID.
			if ref.TaskID != tt.wantTask {
				t.Errorf("parseBlockedByRef(%q): TaskID = %q, "+
					"want %q", tt.input, ref.TaskID,
					tt.wantTask)
			}
		}
	}
}

func TestParseBlockedByRef_CrossProject_Invalid(t *testing.T) {
	invalids := []string{
		"hop-top/c12n#",     // missing task ID
		"tlc://",            // incomplete URI
		"tlc:///",           // no task ID
		"tlc://hop-top/c12n/not-a-task", // bad task ID format
	}
	for _, s := range invalids {
		if _, err := parseBlockedByRef(s); err == nil {
			t.Errorf("parseBlockedByRef(%q): expected error", s)
		}
	}
}

func TestParsePlanFrontmatter_BlockedByCrossProject(t *testing.T) {
	path := writeTempPlan(t, `---
title: Cross-project plan
tracks: [foo]
tasks:
  - title: "A"
  - title: "B"
    blocked-by:
      - "hop-top/c12n#T-0018"
      - "tlc://hop-top/c12n/T-0019"
---
# body
`)
	fm, err := ParsePlanFrontmatter(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	refs := fm.Tasks[1].BlockedBy
	if len(refs) != 2 {
		t.Fatalf("refs len = %d, want 2: %+v", len(refs), refs)
	}

	for i, want := range []struct {
		proj string
		task string
	}{
		{"hop-top/c12n", "T-0018"},
		{"hop-top/c12n", "T-0019"},
	} {
		if refs[i].CrossProject == nil {
			t.Errorf("refs[%d]: want CrossProject, got %+v",
				i, refs[i])
			continue
		}
		if refs[i].CrossProject.ProjectID != want.proj {
			t.Errorf("refs[%d].ProjectID = %q, want %q",
				i, refs[i].CrossProject.ProjectID, want.proj)
		}
		if refs[i].CrossProject.TaskID != want.task {
			t.Errorf("refs[%d].TaskID = %q, want %q",
				i, refs[i].CrossProject.TaskID, want.task)
		}
	}
}
