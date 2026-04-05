package cli

import "testing"

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		// transposition: standard levenshtein = 2
		{"compleet", "complete", 2},
		// deletion+insertion
		{"trakcs", "tracks", 2},
		{"lst", "list", 1},
		{"crete", "create", 1},
		{"listt", "list", 1},
		{"kitten", "sitting", 3},
	}
	for _, tc := range tests {
		got := LevenshteinDistance(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("LevenshteinDistance(%q, %q) = %d want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestFuzzyMatchVerb(t *testing.T) {
	tests := []struct {
		word        string
		wantClass   VerbCategory
		wantConf    float64
		wantNoMatch bool
	}{
		// Exact matches → confidence 1.0
		{"list", VerbQuery, 1.0, false},
		{"create", VerbCreate, 1.0, false},
		{"complete", VerbComplete, 1.0, false},
		// Distance 1 → confidence 0.9
		{"crete", VerbCreate, 0.9, false},
		{"lst", VerbQuery, 0.9, false},
		{"listt", VerbQuery, 0.9, false},
		// Distance 2 → confidence 0.8
		// "compleet" vs "complete" = transposition = dist 2
		{"compleet", VerbComplete, 0.8, false},
		// Distance >2 → no match
		{"xyzabc", "", 0, true},
		{"", "", 0, true},
	}
	for _, tc := range tests {
		vc, conf := FuzzyMatchVerb(tc.word)
		if tc.wantNoMatch {
			if conf != 0 {
				t.Errorf("FuzzyMatchVerb(%q) = (%q, %.1f), want no match", tc.word, vc, conf)
			}
			continue
		}
		if vc != tc.wantClass {
			t.Errorf("FuzzyMatchVerb(%q) class=%q want %q", tc.word, vc, tc.wantClass)
		}
		if conf != tc.wantConf {
			t.Errorf("FuzzyMatchVerb(%q) conf=%.1f want %.1f", tc.word, conf, tc.wantConf)
		}
	}
}

func TestFuzzyMatchNoun(t *testing.T) {
	tests := []struct {
		word        string
		wantDomain  NounDomain
		wantConf    float64
		wantNoMatch bool
	}{
		// Exact
		{"task", DomainTask, 1.0, false},
		{"flow", DomainFlow, 1.0, false},
		{"tracks", DomainTrack, 1.0, false},
		// Distance 1 → confidence 0.9
		// "taks" is closest to "tasks" (dist 1) not "task" (dist 2)
		{"taks", DomainTask, 0.9, false},
		// "flws" is closest to "flows" (dist 1) not "flow" (dist 2)
		{"flws", DomainFlow, 0.9, false},
		// Distance 2 → confidence 0.8
		// "trakcs" vs "tracks" = dist 2, vs "track" = dist 2
		{"trakcs", DomainTrack, 0.8, false},
		// Distance >2 → no match
		{"zzzzz", "", 0, true},
	}
	for _, tc := range tests {
		nd, conf := FuzzyMatchNoun(tc.word)
		if tc.wantNoMatch {
			if conf != 0 {
				t.Errorf("FuzzyMatchNoun(%q) = (%q, %.1f), want no match", tc.word, nd, conf)
			}
			continue
		}
		if nd != tc.wantDomain {
			t.Errorf("FuzzyMatchNoun(%q) domain=%q want %q", tc.word, nd, tc.wantDomain)
		}
		if conf != tc.wantConf {
			t.Errorf("FuzzyMatchNoun(%q) conf=%.1f want %.1f", tc.word, conf, tc.wantConf)
		}
	}
}
