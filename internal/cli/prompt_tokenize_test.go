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
		wantVC   VerbClass
		wantNoun string
		wantDom  NounDomain
		wantMods []string
		wantRem  string
	}{
		{
			name:     "simple list tasks",
			input:    "list tasks",
			wantVerb: "list",
			wantVC:   VerbQuery,
			wantNoun: "tasks",
			wantDom:  DomainTask,
			wantMods: nil,
		},
		{
			name:     "count active tracks",
			input:    "count active tracks",
			wantVerb: "count",
			wantVC:   VerbQuery,
			wantNoun: "tracks",
			wantDom:  DomainTrack,
			wantMods: []string{"status:active"},
		},
		{
			name:     "word order: active tracks count",
			input:    "active tracks count",
			wantVerb: "count",
			wantVC:   VerbQuery,
			wantNoun: "tracks",
			wantDom:  DomainTrack,
			wantMods: []string{"status:active"},
		},
		{
			name:     "show blocked tasks",
			input:    "show blocked tasks",
			wantVerb: "show",
			wantVC:   VerbQuery,
			wantNoun: "tasks",
			wantDom:  DomainTask,
			wantMods: []string{"blocked"},
		},
		{
			name:     "my stale flows",
			input:    "my stale flows",
			wantVerb: "",
			wantVC:   "",
			wantNoun: "flows",
			wantDom:  DomainFlow,
			wantMods: []string{"mine", "stale"},
		},
		{
			name:     "create task",
			input:    "create task",
			wantVerb: "create",
			wantVC:   VerbCreate,
			wantNoun: "task",
			wantDom:  DomainTask,
			wantMods: nil,
		},
		{
			name:     "new flow",
			input:    "new flow",
			wantVerb: "new",
			wantVC:   VerbCreate,
			wantNoun: "flow",
			wantDom:  DomainFlow,
			wantMods: nil,
		},
		{
			name:     "delete project",
			input:    "delete project",
			wantVerb: "delete",
			wantVC:   VerbDestroy,
			wantNoun: "project",
			wantDom:  DomainProject,
			wantMods: nil,
		},
		{
			name:     "list my done tasks",
			input:    "list my done tasks",
			wantVerb: "list",
			wantVC:   VerbQuery,
			wantNoun: "tasks",
			wantDom:  DomainTask,
			wantMods: []string{"mine", "status:done"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TokenizePrompt(tc.input)
			if tc.wantVerb == "" && tc.wantNoun == "" {
				if got != nil {
					t.Errorf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("TokenizePrompt(%q) = nil", tc.input)
			}
			if got.Verb != tc.wantVerb {
				t.Errorf("Verb = %q want %q", got.Verb, tc.wantVerb)
			}
			if got.VerbClass != tc.wantVC {
				t.Errorf("VerbClass = %q want %q", got.VerbClass, tc.wantVC)
			}
			if got.Noun != tc.wantNoun {
				t.Errorf("Noun = %q want %q", got.Noun, tc.wantNoun)
			}
			if got.Domain != tc.wantDom {
				t.Errorf("Domain = %q want %q", got.Domain, tc.wantDom)
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

func TestTokenizePromptNilOnNoMatch(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"random words here",
		"something something",
	}
	for _, c := range cases {
		if got := TokenizePrompt(c); got != nil {
			t.Errorf("TokenizePrompt(%q) = %+v, want nil", c, got)
		}
	}
}

func TestTokenizePromptRemaining(t *testing.T) {
	got := TokenizePrompt("list tasks called auth")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Remaining != "called auth" {
		t.Errorf("Remaining = %q want %q", got.Remaining, "called auth")
	}
}
