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

func TestNormalizeEffort(t *testing.T) {
	tests := []struct {
		input  string
		want   string
		wantOK bool
	}{
		// exact canonical
		{"XS", "XS", true},
		{"S", "S", true},
		{"M", "M", true},
		{"L", "L", true},
		{"XL", "XL", true},
		// case-insensitive
		{"xs", "XS", true},
		{"s", "S", true},
		{"m", "M", true},
		{"l", "L", true},
		{"xl", "XL", true},
		{"Xs", "XS", true},
		{"xL", "XL", true},
		// descriptive aliases
		{"extra-small", "XS", true},
		{"extrasmall", "XS", true},
		{"xsmall", "XS", true},
		{"tiny", "XS", true},
		{"small", "S", true},
		{"medium", "M", true},
		{"med", "M", true},
		{"large", "L", true},
		{"extra-large", "XL", true},
		{"extralarge", "XL", true},
		{"xlarge", "XL", true},
		{"huge", "XL", true},
		// mixed-case alias
		{"Medium", "M", true},
		{"LARGE", "L", true},
		{"Extra-Small", "XS", true},
		// whitespace trimming
		{"  m  ", "M", true},
		{" small ", "S", true},
		// no match
		{"xyz", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := NormalizeEffort(tc.input)
			if ok != tc.wantOK {
				t.Errorf("NormalizeEffort(%q) ok=%v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("NormalizeEffort(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestUnescapeMarkdown(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"no backticks", "no backticks"},
		{"inline \\`code\\`", "inline `code`"},
		{"\\`\\`\\`go\nfmt.Println()\n\\`\\`\\`", "```go\nfmt.Println()\n```"},
		{"already `clean`", "already `clean`"},
		{"", ""},
	}

	for _, tc := range tests {
		got := unescapeMarkdown(tc.input)
		if got != tc.want {
			t.Errorf("unescapeMarkdown(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestNormalizeEffort_FuzzyTieDeterministic guards against the
// non-deterministic fuzzy fallback in normalizeEnum. The effort
// alias table has multiple keys that score-tie for ambiguous inputs
// like "extra" (which fuzzy-matches both "extralarge" and
// "extrasmall" with score 205). Pre-fix, the winner depended on Go
// map iteration order — different between runs. After fix, the
// resolver picks a stable winner regardless of map traversal.
//
// Calling NormalizeEffort("extra") 100 times in a row must produce
// the same result every time. Without sorted alias keys this test
// can pass once and fail on the next invocation because Go
// randomises map iteration per process.
func TestNormalizeEffort_FuzzyTieDeterministic(t *testing.T) {
	first, ok := NormalizeEffort("extra")
	if !ok {
		t.Fatalf("expected 'extra' to resolve via fuzzy, got !ok")
	}
	for i := 0; i < 100; i++ {
		got, ok := NormalizeEffort("extra")
		if !ok {
			t.Fatalf("iteration %d: !ok", i)
		}
		if got != first {
			t.Fatalf("non-deterministic resolution: first=%q got=%q at iteration %d", first, got, i)
		}
	}
}
