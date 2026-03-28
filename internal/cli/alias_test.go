package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// ExpandAliases unit tests
// ---------------------------------------------------------------------------

func TestExpandAliases_NoMatch(t *testing.T) {
	dir := t.TempDir()
	writeAliasConfig(t, dir, "tl", "task list")
	chdir(t, dir)

	args := []string{"tlc", "task", "show", "T-0001"}
	got, ok := ExpandAliases(args)
	if ok {
		t.Error("expected no expansion")
	}
	if len(got) != len(args) {
		t.Errorf("args mutated unexpectedly: %v", got)
	}
}

func TestExpandAliases_Match(t *testing.T) {
	dir := t.TempDir()
	writeAliasConfig(t, dir, "tl", "task list")
	chdir(t, dir)

	args := []string{"tlc", "tl"}
	got, ok := ExpandAliases(args)
	if !ok {
		t.Fatal("expected expansion")
	}
	want := []string{"tlc", "task", "list"}
	if !equalSlices(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandAliases_MatchWithExtraArgs(t *testing.T) {
	dir := t.TempDir()
	writeAliasConfig(t, dir, "tl", "task list")
	chdir(t, dir)

	args := []string{"tlc", "tl", "--status", "TODO"}
	got, ok := ExpandAliases(args)
	if !ok {
		t.Fatal("expected expansion")
	}
	want := []string{"tlc", "task", "list", "--status", "TODO"}
	if !equalSlices(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandAliases_FlagBeforeAlias(t *testing.T) {
	dir := t.TempDir()
	writeAliasConfig(t, dir, "tl", "task list")
	chdir(t, dir)

	// -c flag before alias name should be preserved.
	args := []string{"tlc", "-c", "/tmp/config.yaml", "tl"}
	got, ok := ExpandAliases(args)
	if !ok {
		t.Fatal("expected expansion")
	}
	want := []string{"tlc", "-c", "/tmp/config.yaml", "task", "list"}
	if !equalSlices(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandAliases_Empty(t *testing.T) {
	args := []string{"tlc"}
	got, ok := ExpandAliases(args)
	if ok {
		t.Error("expected no expansion for empty args")
	}
	if !equalSlices(got, args) {
		t.Errorf("args mutated: %v", got)
	}
}

// ---------------------------------------------------------------------------
// alias add / list / remove command tests
// ---------------------------------------------------------------------------

func TestAliasAdd_Local(t *testing.T) {
	resetAliasFlags(t)
	dir := t.TempDir()
	chdir(t, dir)
	setXDGConfig(t, t.TempDir()) // isolate global config

	cmd := newTestCmd()
	cmd.AddCommand(AliasCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"alias", "add", "tl", "task list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("alias add failed: %v", err)
	}
	if !contains(buf.String(), "Added local alias") {
		t.Errorf("unexpected output: %s", buf.String())
	}

	aliases, err := loadAliasesFrom(localAliasPath())
	if err != nil {
		t.Fatalf("load aliases: %v", err)
	}
	if aliases["tl"] != "task list" {
		t.Errorf("alias not saved: %v", aliases)
	}
}

func TestAliasAdd_Global(t *testing.T) {
	resetAliasFlags(t)
	globalBaseDir := t.TempDir()
	setXDGConfig(t, globalBaseDir)
	chdir(t, t.TempDir()) // no local config

	cmd := newTestCmd()
	cmd.AddCommand(AliasCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"alias", "add", "ts", "task show", "--global"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("alias add --global failed: %v", err)
	}
	if !contains(buf.String(), "Added global alias") {
		t.Errorf("unexpected output: %s", buf.String())
	}

	// globalAliasPath() = $XDG_CONFIG_HOME/tlc/config.yaml
	globalPath := filepath.Join(globalBaseDir, "tlc", "config.yaml")
	aliases, err := loadAliasesFrom(globalPath)
	if err != nil {
		t.Fatalf("load global aliases: %v", err)
	}
	if aliases["ts"] != "task show" {
		t.Errorf("global alias not saved: %v", aliases)
	}
}

func TestAliasList(t *testing.T) {
	resetAliasFlags(t)
	// Set up local config dir.
	localDir := t.TempDir()
	writeAliasConfig(t, localDir, "tl", "task list")
	chdir(t, localDir)

	// Set up global config dir.
	globalBaseDir := t.TempDir()
	setXDGConfig(t, globalBaseDir)
	globalPath := filepath.Join(globalBaseDir, "tlc", "config.yaml")
	if err := saveAliasesTo(globalPath, aliasMap{"ts": "task show"}); err != nil {
		t.Fatal(err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(AliasCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"alias", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("alias list failed: %v", err)
	}

	out := buf.String()
	if !contains(out, "tl") || !contains(out, "task list") {
		t.Errorf("local alias missing from list output: %s", out)
	}
	if !contains(out, "ts") || !contains(out, "task show") {
		t.Errorf("global alias missing from list output: %s", out)
	}
}

func TestAliasRemove(t *testing.T) {
	resetAliasFlags(t)
	dir := t.TempDir()
	writeAliasConfig(t, dir, "tl", "task list")
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())

	cmd := newTestCmd()
	cmd.AddCommand(AliasCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"alias", "remove", "tl"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("alias remove failed: %v", err)
	}
	if !contains(buf.String(), "Removed local alias") {
		t.Errorf("unexpected output: %s", buf.String())
	}

	aliases, _ := loadAliasesFrom(localAliasPath())
	if _, ok := aliases["tl"]; ok {
		t.Error("alias was not removed")
	}
}

func TestAliasRemove_NotFound(t *testing.T) {
	resetAliasFlags(t)
	dir := t.TempDir()
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())

	cmd := newTestCmd()
	cmd.AddCommand(AliasCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"alias", "remove", "nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error removing non-existent alias")
	}
}

func TestAliasAdd_BuiltinConflict(t *testing.T) {
	resetAliasFlags(t)
	dir := t.TempDir()
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())

	cmd := newTestCmd()
	cmd.AddCommand(AliasCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"alias", "add", "task", "task list"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when alias conflicts with built-in command")
	}
}

func TestAliasList_Empty(t *testing.T) {
	resetAliasFlags(t)
	dir := t.TempDir()
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())

	cmd := newTestCmd()
	cmd.AddCommand(AliasCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"alias", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("alias list failed: %v", err)
	}
	if !contains(buf.String(), "No aliases") {
		t.Errorf("expected empty message, got: %s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeAliasConfig writes a minimal YAML config with one alias to
// <dir>/.tlc/config.yaml.
func writeAliasConfig(t *testing.T, dir, name, expansion string) {
	t.Helper()
	cfgDir := filepath.Join(dir, ".tlc")
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "aliases:\n  " + name + ": " + expansion + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// chdir changes the working directory for the test and restores it after.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// setXDGConfig overrides XDG_CONFIG_HOME so that globalAliasPath() returns
// <base>/tlc/config.yaml. Restores original env after test.
func setXDGConfig(t *testing.T, base string) {
	t.Helper()
	prev := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", base)
	t.Cleanup(func() { os.Setenv("XDG_CONFIG_HOME", prev) })
}

// resetAliasFlags resets package-level alias flag state between tests.
func resetAliasFlags(t *testing.T) {
	t.Helper()
	prev := aliasGlobal
	aliasGlobal = false
	t.Cleanup(func() { aliasGlobal = prev })
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
