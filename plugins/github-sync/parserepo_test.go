package main

import (
	"strings"
	"testing"
)

func TestParseRepo(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantErr   string // substring match; empty = no error
	}{
		{name: "hyphenated owner", input: "hop-top/tlc", wantOwner: "hop-top", wantRepo: "tlc"},
		{name: "hyphenated owner+dot in repo", input: "Idea-Crafters/tlc.git", wantOwner: "Idea-Crafters", wantRepo: "tlc.git"},
		{name: "shortest valid", input: "a/b", wantOwner: "a", wantRepo: "b"},
		{name: "underscore in repo", input: "owner/some_repo", wantOwner: "owner", wantRepo: "some_repo"},
		{name: "no slash", input: "plain", wantErr: "invalid repo format"},
		{name: "leading slash", input: "/leading-slash", wantErr: "invalid repo format"},
		{name: "trailing slash", input: "trailing/", wantErr: "invalid repo format"},
		{name: "empty string", input: "", wantErr: "invalid repo format"},
		{name: "double slash", input: "owner//repo", wantErr: "invalid repo format"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := parseRepo(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("parseRepo(%q): expected error %q, got nil", tc.input, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseRepo(%q): expected error containing %q, got %v", tc.input, tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRepo(%q): unexpected error: %v", tc.input, err)
			}
			if owner != tc.wantOwner || repo != tc.wantRepo {
				t.Fatalf("parseRepo(%q) = (%q,%q); want (%q,%q)", tc.input, owner, repo, tc.wantOwner, tc.wantRepo)
			}
		})
	}
}
