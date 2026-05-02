package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"hop.top/kit/go/console/alias"
)

// ---------------------------------------------------------------------------
// ExpandAliases unit tests
// ---------------------------------------------------------------------------

func TestExpandAliases_NoMatch(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())
	writeLocalAlias(t, dir, "tl", "task list")

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
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())
	writeLocalAlias(t, dir, "tl", "task list")

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
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())
	writeLocalAlias(t, dir, "tl", "task list")

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
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())
	writeLocalAlias(t, dir, "tl", "task list")

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

func TestExpandAliases_LocalOverridesGlobal(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	xdgBase := t.TempDir()
	setXDGConfig(t, xdgBase)

	// Global says tl → flow list; local says tl → task list (project wins).
	writeGlobalAlias(t, xdgBase, "tl", "flow list")
	writeLocalAlias(t, dir, "tl", "task list")

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

// ---------------------------------------------------------------------------
// alias add / list / remove command tests
// ---------------------------------------------------------------------------

func TestAliasAdd_Local(t *testing.T) {
	resetAliasFlags(t)
	dir := t.TempDir()
	mkLocalConfigDir(t, dir)
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())

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

	store := alias.NewStore(localAliasPath())
	if err := store.Load(); err != nil {
		t.Fatalf("load aliases: %v", err)
	}
	if v, ok := store.Get("tl"); !ok || v != "task list" {
		t.Errorf("alias not saved: %v", store.All())
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

	gp, err := globalAliasPath()
	if err != nil {
		t.Fatalf("global path: %v", err)
	}
	store := alias.NewStore(gp)
	if err := store.Load(); err != nil {
		t.Fatalf("load global aliases: %v", err)
	}
	if v, ok := store.Get("ts"); !ok || v != "task show" {
		t.Errorf("global alias not saved: %v", store.All())
	}
}

func TestAliasList(t *testing.T) {
	resetAliasFlags(t)
	localDir := t.TempDir()
	chdir(t, localDir)
	globalBaseDir := t.TempDir()
	setXDGConfig(t, globalBaseDir)

	writeLocalAlias(t, localDir, "tl", "task list")
	writeGlobalAlias(t, globalBaseDir, "ts", "task show")

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
	mkLocalConfigDir(t, dir)
	chdir(t, dir)
	setXDGConfig(t, t.TempDir())
	writeLocalAlias(t, dir, "tl", "task list")

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

	store := alias.NewStore(localAliasPath())
	_ = store.Load()
	if _, ok := store.Get("tl"); ok {
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

// mkLocalConfigDir creates <dir>/.tlc so localAliasPath() returns a
// project-local store path. Mirrors what `tlc init` would have done.
func mkLocalConfigDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".tlc"), 0o750); err != nil {
		t.Fatal(err)
	}
}

// writeLocalAlias persists name→expansion to <dir>/.tlc/aliases.yaml using
// the kit alias.Store directly (mirrors the path resolved by
// localAliasPath()).
func writeLocalAlias(t *testing.T, dir, name, expansion string) {
	t.Helper()
	path := filepath.Join(dir, ".tlc", "aliases.yaml")
	store := alias.NewStore(path)
	_ = store.Load()
	if err := store.Set(name, expansion); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
}

// writeGlobalAlias persists name→expansion to <xdgBase>/tlc/aliases.yaml.
func writeGlobalAlias(t *testing.T, xdgBase, name, expansion string) {
	t.Helper()
	path := filepath.Join(xdgBase, "tlc", "aliases.yaml")
	store := alias.NewStore(path)
	_ = store.Load()
	if err := store.Set(name, expansion); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
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
// <base>/tlc/aliases.yaml. Restores original env after test.
func setXDGConfig(t *testing.T, base string) {
	t.Helper()
	prev, had := os.LookupEnv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", base)
	t.Cleanup(func() {
		if had {
			os.Setenv("XDG_CONFIG_HOME", prev)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	})
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
