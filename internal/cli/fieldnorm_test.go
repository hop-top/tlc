package cli

import (
	"testing"
)

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		input    string
		want     string
		wantOK   bool
	}{
		// exact canonical
		{"TODO", "TODO", true},
		{"IN_PROGRESS", "IN_PROGRESS", true},
		{"DONE", "DONE", true},
		{"SKIPPED", "SKIPPED", true},
		// case-insensitive
		{"todo", "TODO", true},
		{"Todo", "TODO", true},
		{"done", "DONE", true},
		{"Done", "DONE", true},
		{"in_progress", "IN_PROGRESS", true},
		{"In_Progress", "IN_PROGRESS", true},
		{"skipped", "SKIPPED", true},
		// aliases
		{"complete", "DONE", true},
		{"completed", "DONE", true},
		{"finish", "DONE", true},
		{"finished", "DONE", true},
		{"open", "TODO", true},
		{"to-do", "TODO", true},
		{"wip", "IN_PROGRESS", true},
		{"inprogress", "IN_PROGRESS", true},
		{"in-progress", "IN_PROGRESS", true},
		{"skip", "SKIPPED", true},
		// fuzzy
		{"comp", "DONE", true},   // comp → complete (alias) not matched, fuzzy hits skipped or done via lower
		{"tod", "TODO", true},    // fuzzy prefix of "todo"
		{"done", "DONE", true},   // exact via alias
		// no match
		{"xyz", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := NormalizeStatus(tc.input)
			if ok != tc.wantOK {
				t.Errorf("NormalizeStatus(%q) ok=%v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("NormalizeStatus(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizePriority(t *testing.T) {
	tests := []struct {
		input  string
		want   string
		wantOK bool
	}{
		// exact canonical
		{"P0", "P0", true},
		{"P1", "P1", true},
		{"P2", "P2", true},
		{"P3", "P3", true},
		// case-insensitive
		{"p0", "P0", true},
		{"p1", "P1", true},
		{"p2", "P2", true},
		{"p3", "P3", true},
		// numeric aliases
		{"0", "P0", true},
		{"1", "P1", true},
		{"2", "P2", true},
		{"3", "P3", true},
		// descriptive aliases
		{"critical", "P0", true},
		{"high", "P1", true},
		{"medium", "P2", true},
		{"med", "P2", true},
		{"low", "P3", true},
		// fuzzy
		{"crit", "P0", true},   // fuzzy: "crit" → "critical" alias not in canonical; but alias table doesn't cover fuzzy — let's see
		{"hig", "P1", true},    // fuzzy over lowercased canonical ["p0","p1","p2","p3"]; "hig" won't match these directly
		// no match
		{"xyz", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := NormalizePriority(tc.input)
			if ok != tc.wantOK {
				t.Errorf("NormalizePriority(%q) ok=%v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("NormalizePriority(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
