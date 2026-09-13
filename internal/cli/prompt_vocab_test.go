package cli

import "testing"

func TestVocabVerbLookup(t *testing.T) {
	tests := []struct {
		word string
		want VerbCategory
		ok   bool
	}{
		{"list", VerbQuery, true},
		{"show", VerbQuery, true},
		{"count", VerbQuery, true},
		{"find", VerbQuery, true},
		{"create", VerbCreate, true},
		{"new", VerbCreate, true},
		{"add", VerbCreate, true},
		{"complete", VerbComplete, true},
		{"finish", VerbComplete, true},
		{"delete", VerbDestroy, true},
		{"remove", VerbDestroy, true},
		{"drop", VerbDestroy, true},
		{"unknown", "", false},
	}
	for _, tc := range tests {
		got, ok := LookupVerb(tc.word)
		if ok != tc.ok {
			t.Errorf("LookupVerb(%q) ok=%v want %v", tc.word, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("LookupVerb(%q) = %q want %q", tc.word, got, tc.want)
		}
	}
}

func TestVocabNounLookup(t *testing.T) {
	tests := []struct {
		word string
		want NounDomain
		ok   bool
	}{
		{"task", DomainTask, true},
		{"tasks", DomainTask, true},
		{"todo", DomainTask, true},
		{"track", DomainTrack, true},
		{"tracks", DomainTrack, true},
		{"feature", DomainTrack, true},
		{"epic", DomainTrack, true},
		{"project", DomainProject, true},
		{"projects", DomainProject, true},
		{"widget", "", false},
	}
	for _, tc := range tests {
		got, ok := LookupNoun(tc.word)
		if ok != tc.ok {
			t.Errorf("LookupNoun(%q) ok=%v want %v", tc.word, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("LookupNoun(%q) = %q want %q", tc.word, got, tc.want)
		}
	}
}

func TestVocabModifierLookup(t *testing.T) {
	tests := []struct {
		word string
		want ModifierFlag
		ok   bool
	}{
		{"active", "status:active", true},
		{"wip", "status:active", true},
		{"blocked", "blocked", true},
		{"stuck", "blocked", true},
		{"stale", "stale", true},
		{"old", "stale", true},
		{"my", "mine", true},
		{"mine", "mine", true},
		{"done", "status:done", true},
		{"completed", "status:done", true},
		// New modifier aliases for incomplete/open/pending/etc.
		{"incomplete", "status:active", true},
		{"incompleted", "status:active", true},
		{"open", "status:active", true},
		{"pending", "status:pending", true},
		{"remaining", "status:active", true},
		{"unfinished", "status:active", true},
		{"overdue", "stale", true},
		{"random", "", false},
	}
	for _, tc := range tests {
		got, ok := LookupModifier(tc.word)
		if ok != tc.ok {
			t.Errorf("LookupModifier(%q) ok=%v want %v", tc.word, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("LookupModifier(%q) = %q want %q", tc.word, got, tc.want)
		}
	}
}
