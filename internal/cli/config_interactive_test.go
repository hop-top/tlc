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
	if !sliceContains(keys, "output.format") {
		t.Errorf("expected output.format, got %v", keys)
	}
}

func TestResolveKeys_Substring(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	got := resolveKeys("editor", hints)
	if len(got) == 0 {
		t.Fatal("expected keys for substring 'editor'")
	}

	keys := hintKeys(got)
	if !sliceContains(keys, "ui.editor") {
		t.Errorf("expected ui.editor, got %v", keys)
	}
}

func TestResolveKeys_GroupWinsOverPrefix(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	// "git" is both a group and a prefix. Group should win, meaning
	// the result uses the curated group patterns (which include
	// git.track, git.branch.*, git.commit.*) rather than all git.* keys.
	got := resolveKeys("git", hints)
	if len(got) == 0 {
		t.Fatal("expected keys for 'git'")
	}

	keys := hintKeys(got)
	if !sliceContains(keys, "git.track") {
		t.Errorf("expected git.track in group result, got %v", keys)
	}
}

func TestResolveKeys_ComboDedup(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	// Resolve two tokens where "core" includes storage.backend
	// and "storage" as prefix also includes storage.backend.
	got := resolveMultipleTokens("core,storage", hints)

	seen := map[string]int{}
	for _, h := range got {
		seen[h.Key]++
	}
	if seen["storage.backend"] > 1 {
		t.Errorf("storage.backend duplicated %d times", seen["storage.backend"])
	}
}

func TestResolveKeys_Unknown(t *testing.T) {
	setTestViperDefaults(t)
	hints := defaultKeyHints()

	got := resolveKeys("nonexistent_xyz_999", hints)
	if len(got) != 0 {
		t.Errorf("expected empty result, got %d keys", len(got))
	}
}

// ---------------------------------------------------------------------------
// runWizard tests
// ---------------------------------------------------------------------------

func TestWizard_EnumValid(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("storage.backend", "sqlite")

	keys := []keyHint{
		{Key: "storage.backend", Description: "Storage engine",
			Enum: []string{"sqlite", "local", "postgres"}},
	}

	input := "postgres\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changes["storage.backend"] != "postgres" {
		t.Errorf("expected postgres, got %q", changes["storage.backend"])
	}
}

func TestWizard_EnumInvalidThenValid(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("storage.backend", "sqlite")

	keys := []keyHint{
		{Key: "storage.backend", Description: "Storage engine",
			Enum: []string{"sqlite", "local", "postgres"}},
	}

	// First line is invalid, second is valid.
	input := "badvalue\nlocal\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changes["storage.backend"] != "local" {
		t.Errorf("expected local, got %q", changes["storage.backend"])
	}
	if !strings.Contains(out.String(), "invalid") {
		t.Error("expected invalid message in output")
	}
}

func TestWizard_BoolYN(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("git.track", false)

	keys := []keyHint{
		{Key: "git.track", Description: "Git tracking", IsBool: true},
	}

	input := "y\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changes["git.track"] != "true" {
		t.Errorf("expected true, got %q", changes["git.track"])
	}
}

func TestWizard_BoolInvalid(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("git.track", false)

	keys := []keyHint{
		{Key: "git.track", Description: "Git tracking", IsBool: true},
	}

	input := "maybe\nn\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changes["git.track"] != "false" {
		// "false" same as current → not in changes
		if _, ok := changes["git.track"]; ok {
			t.Errorf("expected no change, got %q", changes["git.track"])
		}
	}
}

func TestWizard_EmptyInput_NoChanges(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("ui.editor", "vim")

	keys := []keyHint{
		{Key: "ui.editor", Description: "Editor"},
	}

	input := "\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %v", changes)
	}
}

func TestWizard_QuestionMark_ShowsDescription(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("ui.editor", "vim")

	keys := []keyHint{
		{Key: "ui.editor", Description: "Editor command for editing"},
	}

	// ? then empty (keep current)
	input := "?\n\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "Editor command for editing") {
		t.Error("expected description in output")
	}
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %v", changes)
	}
}

func TestWizard_MapKeySkipped(t *testing.T) {
	setTestViperDefaults(t)

	keys := []keyHint{
		{Key: "ui.tag_colors", Description: "Tag colors", IsMap: true},
	}

	// No input needed — map keys are auto-skipped.
	input := ""
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
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

func TestWizard_EOF_Aborts(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("ui.editor", "vim")

	keys := []keyHint{
		{Key: "ui.editor", Description: "Editor"},
	}

	// Empty reader = immediate EOF
	out := &bytes.Buffer{}
	_, err := runWizard(keys, strings.NewReader(""), out, true)
	if err == nil {
		t.Fatal("expected abort error on EOF")
	}
	if !strings.Contains(err.Error(), "aborted") {
		t.Errorf("expected 'aborted', got %q", err.Error())
	}
}

func TestWizard_FreeText(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("ui.editor", "vim")

	keys := []keyHint{
		{Key: "ui.editor", Description: "Editor"},
	}

	input := "nano\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changes["ui.editor"] != "nano" {
		t.Errorf("expected nano, got %q", changes["ui.editor"])
	}
}

func TestWizard_Duration(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("task.archive_threshold", "168h0m0s")

	keys := []keyHint{
		{Key: "task.archive_threshold", Description: "Archive threshold",
			IsDuration: true},
	}

	input := "72h\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changes["task.archive_threshold"] != "72h" {
		t.Errorf("expected 72h, got %q",
			changes["task.archive_threshold"])
	}
}

func TestWizard_Duration_InvalidThenValid(t *testing.T) {
	setTestViperDefaults(t)
	viper.Set("task.archive_threshold", "168h0m0s")

	keys := []keyHint{
		{Key: "task.archive_threshold", Description: "Archive threshold",
			IsDuration: true},
	}

	input := "notaduration\n72h\n"
	out := &bytes.Buffer{}
	changes, err := runWizard(keys, strings.NewReader(input), out, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changes["task.archive_threshold"] != "72h" {
		t.Errorf("expected 72h after retry, got %q",
			changes["task.archive_threshold"])
	}
	if !strings.Contains(strings.ToLower(out.String()), "invalid") {
		t.Errorf("expected invalid duration message in output, got %q",
			out.String())
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
// formatPrompt tests
// ---------------------------------------------------------------------------

func TestFormatPrompt_Enum(t *testing.T) {
	kh := keyHint{
		Key: "storage.backend", Enum: []string{"sqlite", "local"},
	}
	p := formatPrompt(kh, "sqlite", "", "", "")
	if !strings.Contains(p, "sqlite/local") {
		t.Errorf("expected enum choices in prompt, got %q", p)
	}
}

func TestFormatPrompt_Bool(t *testing.T) {
	kh := keyHint{Key: "git.track", IsBool: true}
	p := formatPrompt(kh, "false", "", "", "")
	if !strings.Contains(p, "y/n") {
		t.Errorf("expected y/n in prompt, got %q", p)
	}
}

func TestFormatPrompt_Duration(t *testing.T) {
	kh := keyHint{Key: "task.archive_threshold", IsDuration: true}
	p := formatPrompt(kh, "168h0m0s", "", "", "")
	if !strings.Contains(p, "72h") {
		t.Errorf("expected duration hint in prompt, got %q", p)
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
