package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
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
