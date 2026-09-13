package cli

import (
	"reflect"
	"testing"
)

func TestTokenizePromptBasic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantVerb string
		wantNoun string
		wantMods []string
	}{
		{
			name:     "simple list tasks",
			input:    "list tasks",
			wantVerb: "list",
			wantNoun: "tasks",
			wantMods: nil,
		},
		{
			name:     "count active tracks",
			input:    "count active tracks",
			wantVerb: "count",
			wantNoun: "tracks",
			wantMods: []string{"status:active"},
		},
		{
			name:     "word order: active tracks count",
			input:    "active tracks count",
			wantVerb: "count",
			wantNoun: "tracks",
			wantMods: []string{"status:active"},
		},
		{
			name:     "show blocked tasks",
			input:    "show blocked tasks",
			wantVerb: "show",
			wantNoun: "tasks",
			wantMods: []string{"blocked"},
		},
		{
			name:     "my stale tracks",
			input:    "my stale tracks",
			wantVerb: "",
			wantNoun: "tracks",
			wantMods: []string{"mine", "stale"},
		},
		{
			name:     "create task",
			input:    "create task",
			wantVerb: "create",
			wantNoun: "task",
			wantMods: nil,
		},
		{
			name:     "new track",
			input:    "new track",
			wantVerb: "new",
			wantNoun: "track",
			wantMods: nil,
		},
		{
			name:     "delete project",
			input:    "delete project",
			wantVerb: "delete",
			wantNoun: "project",
			wantMods: nil,
		},
		{
			name:     "list my done tasks",
			input:    "list my done tasks",
			wantVerb: "list",
			wantNoun: "tasks",
			wantMods: []string{"mine", "status:done"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TokenizePrompt(tc.input)
			if tc.wantNoun == "" {
				if got.Noun != "" {
					t.Errorf("expected empty result, got %+v", got)
				}
				return
			}
			if got.Noun == "" {
				t.Fatalf("TokenizePrompt(%q) returned empty Noun", tc.input)
			}
			if got.Verb != tc.wantVerb {
				t.Errorf("Verb = %q want %q", got.Verb, tc.wantVerb)
			}
			if got.Noun != tc.wantNoun {
				t.Errorf("Noun = %q want %q", got.Noun, tc.wantNoun)
			}
			// Normalize nil vs empty for comparison
			wantMods := tc.wantMods
			if len(wantMods) == 0 {
				wantMods = nil
			}
			gotMods := got.Modifiers
			if len(gotMods) == 0 {
				gotMods = nil
			}
			if !reflect.DeepEqual(gotMods, wantMods) {
				t.Errorf("Modifiers = %v want %v", gotMods, wantMods)
			}
		})
	}
}

func TestTokenizePromptEmptyOnNoMatch(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"random words here",
		"something something",
	}
	for _, c := range cases {
		got := TokenizePrompt(c)
		if got.Noun != "" {
			t.Errorf("TokenizePrompt(%q) = %+v, want empty result", c, got)
		}
	}
}

func TestTokenizePromptRest(t *testing.T) {
	got := TokenizePrompt("list tasks called auth")
	if got.Noun == "" {
		t.Fatal("expected non-empty result")
	}
	want := []string{"called", "auth"}
	if !reflect.DeepEqual(got.Rest, want) {
		t.Errorf("Rest = %v want %v", got.Rest, want)
	}
}
