package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
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
	if rootCmd.RunE == nil {
		t.Error("expected rootCmd.RunE to be set")
	}
}

// TestFindAllConfigs tests config file discovery
// Finds config files up to the configured boundary.
func TestFindAllConfigs(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-findconfigs-*")
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	globalDir := filepath.Join(homeDir, ".config", "tlc")
	subDir := filepath.Join(homeDir, "a", "b", "c")
	os.MkdirAll(subDir, 0o755)
	os.MkdirAll(globalDir, 0o755)

	// Create configs at different levels
	os.WriteFile(filepath.Join(tmpDir, ".tlc.yaml"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(homeDir, ".tlc.yaml"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(homeDir, "a", "b", ".tlc.yaml"), []byte(""), 0o644)
	os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte(""), 0o644)

	oldHome := os.Getenv("HOME")
	defer func() { _ = os.Setenv("HOME", oldHome) }()
	if err := os.Setenv("HOME", homeDir); err != nil {
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
	globalDir := filepath.Join(homeDir, ".config", "tlc")

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
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

	writeFile(filepath.Join(globalDir, "config.yaml"), "output:\n  color: false\n")
	writeFile(filepath.Join(tmpDir, ".tlc.yaml"), "output:\n  format: tls\n")
	writeFile(filepath.Join(homeDir, ".tlc.yaml"), "output:\n  verbose: true\n")
	writeFile(filepath.Join(homeDir, "workspace", ".tlc.yaml"), "storage:\n  db_path: ./workspace.sqlite\n")
	writeFile(filepath.Join(projectDir, ".tlc.yaml"), "output:\n  format: json\n")

	oldWd, _ := os.Getwd()
	oldHome := os.Getenv("HOME")
	oldCfgFile := cfgFile
	defer func() {
		_ = os.Chdir(oldWd)
		_ = os.Setenv("HOME", oldHome)
		cfgFile = oldCfgFile
	}()

	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatal(err)
	}

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
