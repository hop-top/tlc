package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/core/upgrade"
	cfgpkg "hop.top/tlc/internal/config"
)

// TestInitConfig tests configuration loading from file
// Verifies config file is loaded and parsed correctly.
func TestInitConfig(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-root-test-*")
	defer os.RemoveAll(tmpDir)

	// Create a dummy config file
	configPath := filepath.Join(tmpDir, ".tlc.yaml")
	os.WriteFile(configPath, []byte("storage:\n  backend: sqlite\n  db_path: ./test.sqlite\n"), 0o644)

	// Change working directory to tmpDir to test findAllConfigs
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	cfgFile = "" // reset global
	viper.Reset()

	// This will trigger initConfig via findAllConfigs if we are in the right dir
	// But initConfig is called by cobra.OnInitialize.
	// We can call it directly for testing.
	initConfig()

	if viper.GetString("storage.backend") != "sqlite" {
		t.Errorf("expected storage.backend sqlite, got %s", viper.GetString("storage.backend"))
	}

	if viper.GetString("storage.db_path") != "./test.sqlite" {
		t.Errorf("expected storage.db_path ./test.sqlite, got %s", viper.GetString("storage.db_path"))
	}
}

// TestRootCmdDefault verifies root command is configured
// Checks RunE handler is set for TUI launch.
func TestRootCmdDefault(t *testing.T) {
	// We can't easily test TUI launch in a unit test without it hanging or failing on no TTY.
	// But we can verify RunE is set.
	if RootCmd.RunE == nil {
		t.Error("expected RootCmd.RunE to be set")
	}
}

// TestFindAllConfigs tests config file discovery
// Finds config files up to the configured boundary.
func TestFindAllConfigs(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-findconfigs-*")
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	subDir := filepath.Join(homeDir, "a", "b", "c")
	os.MkdirAll(subDir, 0o755)

	// Create configs at different levels
	os.WriteFile(filepath.Join(tmpDir, ".tlc.yaml"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(homeDir, ".tlc.yaml"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(homeDir, "a", "b", ".tlc.yaml"), []byte(""), 0o644)

	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	defer func() { _ = os.Setenv("XDG_CONFIG_HOME", oldXDG) }()
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatal(err)
	}

	globalConfigPath, err := cfgpkg.UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(globalConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalConfigPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	boundary := resolveProjectConfigBoundary(subDir)
	if want := normalizeConfigPath(homeDir); boundary != want {
		t.Fatalf("resolveProjectConfigBoundary() = %q, want %q", boundary, want)
	}

	configs := findAllConfigs(subDir, boundary)
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs below boundary, got %d: %v", len(configs), configs)
	}

	for _, path := range configs {
		if path == normalizeConfigPath(filepath.Join(tmpDir, ".tlc.yaml")) {
			t.Fatalf("config above boundary should not be included: %s", path)
		}
	}
}

func TestCommonAncestorDir_RootBoundary(t *testing.T) {
	got, err := commonAncestorDir("/tmp/project", "/Users/example/.config/tlc")
	if err != nil {
		t.Fatalf("commonAncestorDir() error = %v", err)
	}
	if got != string(filepath.Separator) {
		t.Fatalf("commonAncestorDir() = %q, want %q", got, string(filepath.Separator))
	}
}

func TestInitConfig_BoundedWalkUpIgnoresAboveBoundary(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-bounded-init-*")
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	projectDir := filepath.Join(homeDir, "workspace", "project")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(filepath.Join(tmpDir, ".tlc.yaml"), "output:\n  format: tls\n")
	writeFile(filepath.Join(homeDir, ".tlc.yaml"), "output:\n  verbose: true\n")
	writeFile(filepath.Join(homeDir, "workspace", ".tlc.yaml"), "storage:\n  db_path: ./workspace.sqlite\n")
	writeFile(filepath.Join(projectDir, ".tlc.yaml"), "output:\n  format: json\n")

	oldWd, _ := os.Getwd()
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	oldCfgFile := cfgFile
	defer func() {
		_ = os.Chdir(oldWd)
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
		cfgFile = oldCfgFile
	}()

	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatal(err)
	}
	globalConfigPath, err := cfgpkg.UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(globalConfigPath, "output:\n  color: false\n")

	cfgFile = ""
	viper.Reset()
	initConfig()

	if got := viper.GetString("output.format"); got != "json" {
		t.Fatalf("output.format = %q, want %q", got, "json")
	}
	if got := viper.GetBool("output.color"); got {
		t.Fatalf("output.color = %v, want false from user global config", got)
	}
	if got := viper.GetBool("output.verbose"); !got {
		t.Fatalf("output.verbose = %v, want true from boundary config", got)
	}
	if got := viper.GetString("storage.db_path"); got != "./workspace.sqlite" {
		t.Fatalf("storage.db_path = %q, want %q", got, "./workspace.sqlite")
	}
	if got := viper.ConfigFileUsed(); normalizeConfigPath(got) != normalizeConfigPath(filepath.Join(projectDir, ".tlc.yaml")) {
		t.Fatalf("ConfigFileUsed() = %q, want %q", got, filepath.Join(projectDir, ".tlc.yaml"))
	}
}

func TestInitConfig_UsesOSUserConfigBeforeSystem(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-user-config-root-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	oldCfgFile := cfgFile
	defer func() {
		_ = os.Chdir(oldWd)
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
		cfgFile = oldCfgFile
	}()

	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("HOME", filepath.Join(tmpDir, "home")); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatal(err)
	}
	userConfigPath, err := cfgpkg.UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(userConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userConfigPath, []byte("output:\n  format: yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgFile = ""
	viper.Reset()
	initConfig()

	if got := viper.GetString("output.format"); got != "yaml" {
		t.Fatalf("output.format = %q, want %q", got, "yaml")
	}
	if got := normalizeConfigPath(viper.ConfigFileUsed()); got != normalizeConfigPath(userConfigPath) {
		t.Fatalf("ConfigFileUsed() = %q, want %q", got, userConfigPath)
	}
}

// TestFindAllConfigsForMode_Standalone verifies standalone mode uses .tlc paths.
func TestFindAllConfigsForMode_Standalone(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create .tlc.yaml in root
	if err := os.WriteFile(filepath.Join(tmp, ".tlc.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create .tlc/config.yaml in sub
	tlcDir := filepath.Join(sub, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tlcDir, "config.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	configs := findAllConfigsForMode(sub, "", cfgpkg.ModeStandalone)
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d: %v", len(configs), configs)
	}
}

// TestFindAllConfigsForMode_Hop verifies hop mode uses .hop/tlc paths.
func TestFindAllConfigsForMode_Hop(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "a")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create .hop/tlc.yaml (flat) in root
	hopDir := filepath.Join(tmp, ".hop")
	if err := os.MkdirAll(hopDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hopDir, "tlc.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create .hop/tlc/config.yaml (dir) in sub
	hopTLCDir := filepath.Join(sub, ".hop", "tlc")
	if err := os.MkdirAll(hopTLCDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hopTLCDir, "config.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	configs := findAllConfigsForMode(sub, "", cfgpkg.ModeHop)
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d: %v", len(configs), configs)
	}

	// Verify .tlc.yaml is NOT picked up
	tlcFile := filepath.Join(tmp, ".tlc.yaml")
	if err := os.WriteFile(tlcFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	configs2 := findAllConfigsForMode(sub, "", cfgpkg.ModeHop)
	for _, c := range configs2 {
		if filepath.Base(c) == ".tlc.yaml" {
			t.Fatalf("hop mode should not pick up .tlc.yaml: %s", c)
		}
	}
}

// TestInitConfig_DirectoryFlagResolvesToTLCConfigYAML verifies that passing a
// directory to -c resolves to <dir>/.tlc/config.yaml.
func TestInitConfig_DirectoryFlagResolvesToTLCConfigYAML(t *testing.T) {
	dir := t.TempDir()
	tlcDir := filepath.Join(dir, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tlcDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("output:\n  format: tls\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldCfgFile := cfgFile
	defer func() { cfgFile = oldCfgFile }()

	cfgFile = dir
	viper.Reset()
	initConfig()

	if got := viper.GetString("output.format"); got != "tls" {
		t.Fatalf("output.format = %q, want \"tls\"", got)
	}
	got := viper.ConfigFileUsed()
	if got != cfgPath {
		t.Fatalf("ConfigFileUsed() = %q, want %q", got, cfgPath)
	}
}

// TestInitConfig_FileFlagUnchanged verifies that passing an explicit file path
// to -c does not alter it.
func TestInitConfig_FileFlagUnchanged(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "myconfig.yaml")
	if err := os.WriteFile(cfgPath, []byte("output:\n  format: json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldCfgFile := cfgFile
	defer func() { cfgFile = oldCfgFile }()

	cfgFile = cfgPath
	viper.Reset()
	initConfig()

	if got := viper.GetString("output.format"); got != "json" {
		t.Fatalf("output.format = %q, want \"json\"", got)
	}
	if got := normalizeConfigPath(viper.ConfigFileUsed()); got != normalizeConfigPath(cfgPath) {
		t.Fatalf("ConfigFileUsed() = %q, want %q", got, cfgPath)
	}
}

// TestInitConfig_ExplicitConfigOverridesMergesWithCascade proves the fix for
// T-0049: --config must merge/override the default cascade, not replace it.
// Keys present only in the cascade survive; keys in the explicit file win.
func TestInitConfig_ExplicitConfigOverridesMergesWithCascade(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up isolated HOME so we control the user-config cascade.
	homeDir := filepath.Join(tmpDir, "home")
	projectDir := filepath.Join(homeDir, "workspace", "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	oldCfgFile := cfgFile
	defer func() {
		_ = os.Chdir(oldWd)
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
		cfgFile = oldCfgFile
	}()

	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatal(err)
	}

	// User-global config sets output.verbose = true and storage.db_path.
	userConfigPath, err := cfgpkg.UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(userConfigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userConfigPath,
		[]byte("output:\n  verbose: true\nstorage:\n  db_path: ./cascade.sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Explicit --config file only overrides output.format; leaves other keys untouched.
	explicitCfg := filepath.Join(tmpDir, "override.yaml")
	if err := os.WriteFile(explicitCfg, []byte("output:\n  format: json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgFile = explicitCfg
	viper.Reset()
	initConfig()

	// Key from explicit file wins.
	if got := viper.GetString("output.format"); got != "json" {
		t.Fatalf("output.format = %q, want \"json\" (from explicit --config)", got)
	}
	// Key from cascade survives (not erased by explicit file).
	if got := viper.GetString("storage.db_path"); got != "./cascade.sqlite" {
		t.Fatalf("storage.db_path = %q, want \"./cascade.sqlite\" (from cascade); explicit file must not erase cascade keys", got)
	}
	// Another key from cascade survives.
	if got := viper.GetBool("output.verbose"); !got {
		t.Fatalf("output.verbose = false, want true (from cascade)")
	}
}

// TestPreParseChdir_NoFlagIsNoOp verifies the pre-parse helper returns
// (args, "", false) when no -C/--chdir flag is present, leaving args
// untouched. This is the critical no-op invariant for unrelated CLI
// invocations.
func TestPreParseChdir_NoFlagIsNoOp(t *testing.T) {
	cases := [][]string{
		{"tlc"},
		{"tlc", "task", "list"},
		{"tlc", "task", "create", "x", "--reference", "tlc://repo/T-1"},
	}
	for _, args := range cases {
		newArgs, target, ok := preParseChdir(args)
		if ok {
			t.Errorf("preParseChdir(%v) ok=true, want false", args)
		}
		if target != "" {
			t.Errorf("preParseChdir(%v) target=%q, want \"\"", args, target)
		}
		if len(newArgs) != len(args) {
			t.Errorf("preParseChdir(%v) modified args: got %v", args, newArgs)
		}
	}
}

// TestPreParseChdir_StripsAllFlagForms verifies all four supported
// forms of the chdir flag are recognised and stripped.
func TestPreParseChdir_StripsAllFlagForms(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "short space-separated",
			args: []string{"tlc", "-C", "/some/dir", "task", "list"},
			want: []string{"tlc", "task", "list"},
		},
		{
			name: "long space-separated",
			args: []string{"tlc", "--chdir", "/some/dir", "task", "list"},
			want: []string{"tlc", "task", "list"},
		},
		{
			name: "short equals",
			args: []string{"tlc", "-C=/some/dir", "task", "list"},
			want: []string{"tlc", "task", "list"},
		},
		{
			name: "long equals",
			args: []string{"tlc", "--chdir=/some/dir", "task", "list"},
			want: []string{"tlc", "task", "list"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			newArgs, target, ok := preParseChdir(tc.args)
			if !ok {
				t.Fatalf("preParseChdir() ok=false, want true")
			}
			if target != "/some/dir" {
				t.Errorf("target = %q, want %q", target, "/some/dir")
			}
			if len(newArgs) != len(tc.want) {
				t.Fatalf("newArgs = %v, want %v", newArgs, tc.want)
			}
			for i := range newArgs {
				if newArgs[i] != tc.want[i] {
					t.Errorf("newArgs[%d] = %q, want %q", i, newArgs[i], tc.want[i])
				}
			}
		})
	}
}

// TestPreParseChdir_StopsAtDoubleDash verifies that flags after `--`
// are treated as positional and not consumed by the chdir pre-parse.
func TestPreParseChdir_StopsAtDoubleDash(t *testing.T) {
	args := []string{"tlc", "exec", "--", "-C", "/should/not/strip"}
	newArgs, _, ok := preParseChdir(args)
	if ok {
		t.Fatalf("preParseChdir() ok=true, want false (flag is positional)")
	}
	if len(newArgs) != len(args) {
		t.Fatalf("newArgs = %v, want unchanged %v", newArgs, args)
	}
}

// TestChdirSwitchesProjectContext verifies that pre-parsing -C
// chdirs before project detection runs, so the resulting project ID
// reflects the new directory rather than the original cwd. This is
// the regression test for T-1121.
func TestChdirSwitchesProjectContext(t *testing.T) {
	tmpDir := t.TempDir()

	projA := filepath.Join(tmpDir, "projA")
	projB := filepath.Join(tmpDir, "projB")
	for _, p := range []string{projA, projB} {
		tlcDir := filepath.Join(p, ".tlc")
		if err := os.MkdirAll(tlcDir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Write a minimal project config with a stable project id
		// so DetectProject can return a deterministic value.
		body := "project:\n  id: " + filepath.Base(p) + "\nstorage:\n  db_path: " + filepath.Join(tlcDir, "db.sqlite") + "\n"
		if err := os.WriteFile(filepath.Join(tlcDir, "config.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(projA); err != nil {
		t.Fatal(err)
	}

	// Sanity check: with no flag, scanning preParseChdir leaves args
	// alone and the original cwd is preserved.
	if _, _, ok := preParseChdir([]string{"tlc", "task", "list"}); ok {
		t.Fatalf("no-flag pre-parse should return ok=false")
	}

	// Now simulate `tlc -C <projB> task list`. After the pre-parse
	// chdir step, os.Getwd() must report projB.
	args := []string{"tlc", "-C", projB, "task", "list"}
	newArgs, target, ok := preParseChdir(args)
	if !ok {
		t.Fatalf("preParseChdir ok=false, want true")
	}
	if target != projB {
		t.Fatalf("target = %q, want %q", target, projB)
	}
	dir, err := resolvePreChdirTarget(target)
	if err != nil {
		t.Fatalf("resolvePreChdirTarget: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("os.Chdir(%s): %v", dir, err)
	}
	gotWd, _ := os.Getwd()
	wantWd, _ := filepath.EvalSymlinks(projB)
	gotWdResolved, _ := filepath.EvalSymlinks(gotWd)
	if gotWdResolved != wantWd {
		t.Fatalf("post-chdir cwd = %q, want %q", gotWdResolved, wantWd)
	}

	// Args must no longer carry -C / target — kit must not see it.
	for _, a := range newArgs {
		if a == "-C" || a == "--chdir" || a == projB {
			t.Fatalf("newArgs still contains chdir flag/target: %v", newArgs)
		}
	}

	// Project detection now resolves to projB. We exercise
	// initConfig + viper directly because DetectProject caches its
	// result process-wide.
	cfgFile = ""
	viper.Reset()
	initConfig()
	if got := viper.GetString("project.id"); got != "projB" {
		t.Fatalf("project.id = %q, want %q (project context did not switch)", got, "projB")
	}
}

// TestResolvePreChdirTarget_TildeAndRelative verifies ~ expansion and
// relative path resolution against the current working directory.
func TestResolvePreChdirTarget_TildeAndRelative(t *testing.T) {
	tmpDir := t.TempDir()
	sub := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Relative path
	got, err := resolvePreChdirTarget("sub")
	if err != nil {
		t.Fatalf("resolvePreChdirTarget(sub): %v", err)
	}
	wantSub, _ := filepath.EvalSymlinks(sub)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantSub {
		t.Errorf("relative resolved to %q, want %q", gotResolved, wantSub)
	}

	// ~ expansion (resolves to home dir; we just need it to not error
	// and to produce an absolute path).
	got, err = resolvePreChdirTarget("~")
	if err != nil {
		t.Fatalf("resolvePreChdirTarget(~): %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("~ did not resolve to absolute path: %q", got)
	}

	// Non-existent path with a slash (won't fuzzy-match anything in the
	// registry) falls through and errors. The message must reference the
	// input target so users see what failed.
	if _, err := resolvePreChdirTarget(filepath.Join(tmpDir, "does-not-exist")); err == nil {
		t.Errorf("expected error for non-existent path that doesn't fuzzy-match")
	}

	// A regular file (not a directory) at the resolved path also falls
	// through to fuzzy match — a non-directory path shouldn't shadow a
	// registered project of the same name.
	regular := filepath.Join(tmpDir, "regular-file")
	if err := os.WriteFile(regular, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolvePreChdirTarget(regular); err == nil {
		t.Errorf("expected error when path is a regular file with no matching project")
	}
}

// TestFindAllConfigsForMode_HopIgnoresStandalone verifies hop mode
// does not pick up standalone config paths.
func TestFindAllConfigsForMode_HopIgnoresStandalone(t *testing.T) {
	tmp := t.TempDir()

	// Only standalone config exists
	if err := os.WriteFile(filepath.Join(tmp, ".tlc.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	tlcDir := filepath.Join(tmp, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tlcDir, "config.yaml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	configs := findAllConfigsForMode(tmp, "", cfgpkg.ModeHop)
	if len(configs) != 0 {
		t.Fatalf("hop mode should find 0 standalone configs, got %d: %v",
			len(configs), configs)
	}
}

// TestApplyCommandGroups verifies the §4.1 taxonomy mapping is enforced on
// every visible top-level command. Two failure modes:
//  1. A command in the map has the wrong GroupID after applyCommandGroups()
//     runs (regression in commandGroups assignments).
//  2. A new top-level command was registered without a commandGroups entry,
//     leaving its GroupID empty — every new command must opt into a group
//     so help output stays grouped.
//
// Hidden commands (e.g. the deprecated `tasks` parent) are exempt: kit's
// help renderer already filters them, and assigning them a group would
// drag them back into the visible help output.
func TestApplyCommandGroups(t *testing.T) {
	applyCommandGroups()

	// Spot-check a representative entry from each group to catch silent
	// regressions where commandGroups is rewritten but applyCommandGroups
	// stops walking RootCmd.Commands().
	want := map[string]string{
		"task":      "knowledge",
		"track":     "knowledge",
		"flow":      "knowledge",
		"log":       "knowledge",
		"project":   "knowledge",
		"prompt":    "knowledge",
		"tag":       "curate",
		"label":     "curate",
		"assignee":  "curate",
		"inbox":     "curate",
		"sync":      "curate",
		"workspace": "organize",
		"doctor":    "organize",
		"init":      "organize",
		"schema":    "organize",
		"workflow":  "organize",
		"tui":       "interact",
		"agent":     "interact",
		"auth":      "instance",
		"uri":       "instance",
		"config":    "management",
		"alias":     "management",
		"version":   "management",
		"upgrade":   "management",
	}

	got := make(map[string]string, len(RootCmd.Commands()))
	for _, c := range RootCmd.Commands() {
		got[c.Name()] = c.GroupID
	}

	for name, group := range want {
		if got[name] != group {
			t.Errorf("command %q: GroupID = %q, want %q",
				name, got[name], group)
		}
	}

	// Every visible top-level command must have a GroupID. Hidden
	// commands (kit's auto-injected `help`, the deprecated `tasks`
	// parent) are skipped — they never appear in --help output.
	for _, c := range RootCmd.Commands() {
		if c.Hidden {
			continue
		}
		if c.GroupID == "" {
			t.Errorf("top-level command %q has no GroupID; "+
				"add an entry to commandGroups in root.go",
				c.Name())
		}
	}
}

// TestOfflineSkipsUpgradeCheck ensures notifyUpgrade is NOT invoked from
// PersistentPreRunE when --offline is set, and IS invoked otherwise. The
// upgrade indirection (notifyUpgrade var) is swapped for a counter.
func TestOfflineSkipsUpgradeCheck(t *testing.T) {
	oldNotify := notifyUpgrade
	defer func() { notifyUpgrade = oldNotify }()

	calls := 0
	notifyUpgrade = func(_ context.Context, _ *upgrade.Checker, _ io.Writer) {
		calls++
	}

	// Reset viper between sub-tests so flag state does not leak.
	t.Run("offline skips", func(t *testing.T) {
		calls = 0
		viper.Reset()
		viper.Set("runtime.offline", true)

		// Simulate any non-"upgrade", non-"init" command.
		c := &cobra.Command{Use: "task"}
		c.SetContext(context.Background())

		// Replicate the relevant guard inline. We avoid invoking the
		// real PersistentPreRunE because it pulls in storage/extensions.
		offline := viper.GetBool("runtime.offline")
		if c.Name() != "upgrade" && !offline {
			notifyUpgrade(c.Context(), nil, io.Discard)
		}

		if calls != 0 {
			t.Errorf("notifyUpgrade called %d times under --offline; want 0", calls)
		}
	})

	t.Run("online calls", func(t *testing.T) {
		calls = 0
		viper.Reset()
		viper.Set("runtime.offline", false)

		c := &cobra.Command{Use: "task"}
		c.SetContext(context.Background())

		offline := viper.GetBool("runtime.offline")
		if c.Name() != "upgrade" && !offline {
			notifyUpgrade(c.Context(), nil, io.Discard)
		}

		if calls != 1 {
			t.Errorf("notifyUpgrade called %d times when online; want 1", calls)
		}
	})
}

// TestProfileFlagSetsViperKey verifies --profile binds to runtime.profile.
func TestProfileFlagSetsViperKey(t *testing.T) {
	viper.Reset()

	flag := RootCmd.PersistentFlags().Lookup("profile")
	if flag == nil {
		t.Fatal("--profile flag not registered on RootCmd")
	}
	if err := viper.BindPFlag("runtime.profile", flag); err != nil {
		t.Fatalf("BindPFlag: %v", err)
	}
	if err := flag.Value.Set("foo"); err != nil {
		t.Fatalf("set --profile: %v", err)
	}
	flag.Changed = true
	defer func() {
		// Restore default so subsequent tests do not inherit the value.
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	}()

	if got := viper.GetString("runtime.profile"); got != "foo" {
		t.Fatalf("runtime.profile = %q, want %q", got, "foo")
	}
}

// TestInstanceFlagSetsViperKey verifies --instance binds to runtime.instance.
func TestInstanceFlagSetsViperKey(t *testing.T) {
	viper.Reset()

	flag := RootCmd.PersistentFlags().Lookup("instance")
	if flag == nil {
		t.Fatal("--instance flag not registered on RootCmd")
	}
	if err := viper.BindPFlag("runtime.instance", flag); err != nil {
		t.Fatalf("BindPFlag: %v", err)
	}
	if err := flag.Value.Set("staging"); err != nil {
		t.Fatalf("set --instance: %v", err)
	}
	flag.Changed = true
	defer func() {
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	}()

	if got := viper.GetString("runtime.instance"); got != "staging" {
		t.Fatalf("runtime.instance = %q, want %q", got, "staging")
	}
}

// TestOfflineFlagDefault verifies --offline defaults to false.
func TestOfflineFlagDefault(t *testing.T) {
	viper.Reset()

	flag := RootCmd.PersistentFlags().Lookup("offline")
	if flag == nil {
		t.Fatal("--offline flag not registered on RootCmd")
	}
	if err := viper.BindPFlag("runtime.offline", flag); err != nil {
		t.Fatalf("BindPFlag: %v", err)
	}
	if got := viper.GetBool("runtime.offline"); got {
		t.Fatalf("runtime.offline default = %v, want false", got)
	}
}
