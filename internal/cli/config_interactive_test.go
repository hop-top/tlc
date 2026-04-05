package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// resolveKeys tests
// ---------------------------------------------------------------------------

func TestResolveKeys_Group(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	got := resolveKeys("core", hints)
	if len(got) == 0 {
		t.Fatal("expected keys for group 'core'")
	}

	keys := hintKeys(got)
	if !sliceContains(keys, "project.id") {
		t.Errorf("expected project.id in core group, got %v", keys)
	}
	if !sliceContains(keys, "storage.backend") {
		t.Errorf("expected storage.backend in core group, got %v", keys)
	}
}

func TestResolveKeys_Prefix(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	got := resolveKeys("output", hints)
	if len(got) == 0 {
		t.Fatal("expected keys for prefix 'output'")
	}

	keys := hintKeys(got)
	for _, k := range keys {
		if !strings.HasPrefix(k, "output.") {
			t.Errorf("expected output.* key, got %q", k)
		}
	}
}

func TestResolveKeys_Substring(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	got := resolveKeys("editor", hints)
	if len(got) == 0 {
		t.Fatal("expected keys containing 'editor'")
	}

	for _, h := range got {
		if !strings.Contains(h.Key, "editor") {
			t.Errorf("expected key containing 'editor', got %q", h.Key)
		}
	}
}

func TestResolveKeys_GroupWinsOverPrefix(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	// "ai" is both a group name and could be a substring.
	// Group should win and return the curated list.
	got := resolveKeys("ai", hints)
	if len(got) == 0 {
		t.Fatal("expected keys for group 'ai'")
	}

	keys := hintKeys(got)
	if !sliceContains(keys, "prompt.llm_provider") {
		t.Errorf("expected prompt.llm_provider in ai group, got %v", keys)
	}
}

func TestResolveKeys_Unknown(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	got := resolveKeys("zzz_no_match_zzz", hints)
	if len(got) != 0 {
		t.Errorf("expected no keys for unknown token, got %v", hintKeys(got))
	}
}

func TestResolveMultipleTokens(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	got := resolveMultipleTokens("core,ui", hints)
	keys := hintKeys(got)

	if !sliceContains(keys, "project.id") {
		t.Error("expected project.id from core group")
	}
	if !sliceContains(keys, "ui.editor") {
		t.Error("expected ui.editor from ui group")
	}

	// Check dedup — no key should appear twice.
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate key: %s", k)
		}
		seen[k] = true
	}
}

// ---------------------------------------------------------------------------
// Wizard: map key skip (no huh interaction needed)
// ---------------------------------------------------------------------------

func TestWizard_MapKeySkipped(t *testing.T) {
	setTestViperDefaults(t)

	keys := []keyHint{
		{Key: "ui.tag_colors", Description: "Tag colors", IsMap: true},
	}

	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(""), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes for map key, got %v", changes)
	}
	if !strings.Contains(out.String(), "tlc config set") {
		t.Error("expected hint about tlc config set")
	}
}

// ---------------------------------------------------------------------------
// ExpandAliases seeded alias test
// ---------------------------------------------------------------------------

func TestExpandAliases_SeededSetup(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	args := []string{"tlc", "setup", "llm"}
	got, ok := ExpandAliases(args)
	if !ok {
		t.Fatal("expected seeded alias expansion for 'setup'")
	}
	want := []string{"tlc", "config", "interactive", "llm"}
	if !equalSlices(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandAliases_SeededSetupNoArgs(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	args := []string{"tlc", "setup"}
	got, ok := ExpandAliases(args)
	if !ok {
		t.Fatal("expected seeded alias expansion for 'setup'")
	}
	want := []string{"tlc", "config", "interactive"}
	if !equalSlices(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// defaultKeyHints coverage
// ---------------------------------------------------------------------------

func TestDefaultKeyHints_KnownEnums(t *testing.T) {
	hints := defaultKeyHints()

	tests := []struct {
		key  string
		enum []string
	}{
		{"storage.backend", []string{"sqlite", "local", "postgres"}},
		{"output.format", []string{"table", "json", "yaml", "tls", "summary"}},
		{"project.fallback_mode", []string{"auto", "detected", "prompt"}},
	}

	for _, tt := range tests {
		h, ok := hints[tt.key]
		if !ok {
			t.Errorf("hint for %q not found", tt.key)
			continue
		}
		if !equalSlices(h.Enum, tt.enum) {
			t.Errorf("%s enum = %v, want %v", tt.key, h.Enum, tt.enum)
		}
	}
}

func TestDefaultKeyHints_LLMProviderHasSuggestions(t *testing.T) {
	hints := defaultKeyHints()

	h, ok := hints["prompt.llm_provider"]
	if !ok {
		t.Fatal("prompt.llm_provider hint not found")
	}
	if len(h.Suggestions) == 0 {
		t.Error("expected suggestions for prompt.llm_provider")
	}

	// Check that at least one free option exists.
	hasFree := false
	for _, s := range h.Suggestions {
		if strings.Contains(strings.ToLower(s.Label), "free") {
			hasFree = true
			break
		}
	}
	if !hasFree {
		t.Error("expected at least one free model suggestion")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func setTestViperDefaults(t *testing.T) {
	t.Helper()
	viper.Reset()
	setDefaults()
}

func hintKeys(hints []keyHint) []string {
	keys := make([]string, len(hints))
	for i, h := range hints {
		keys[i] = h.Key
	}
	return keys
}

func sliceContains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// equalSlices is defined in alias_test.go
