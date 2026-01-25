package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// TestInitConfig tests configuration loading from file
// Verifies config file is loaded and parsed correctly
func TestInitConfig(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-root-test-*")
	defer os.RemoveAll(tmpDir)

	// Create a dummy config file
	configPath := filepath.Join(tmpDir, ".tlc.yaml")
	os.WriteFile(configPath, []byte("storage:\n  backend: sqlite\n  db_path: ./test.sqlite\n"), 0644)

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
// Checks RunE handler is set for TUI launch
func TestRootCmdDefault(t *testing.T) {
	// We can't easily test TUI launch in a unit test without it hanging or failing on no TTY.
	// But we can verify RunE is set.
	if rootCmd.RunE == nil {
		t.Error("expected rootCmd.RunE to be set")
	}
}

// TestFindAllConfigs tests config file discovery
// Finds all .tlc.yaml files in parent directories
func TestFindAllConfigs(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "tlc-findconfigs-*")
	defer os.RemoveAll(tmpDir)

	subDir := filepath.Join(tmpDir, "a", "b", "c")
	os.MkdirAll(subDir, 0755)

	// Create configs at different levels
	os.WriteFile(filepath.Join(tmpDir, ".tlc.yaml"), []byte(""), 0644)
	os.WriteFile(filepath.Join(tmpDir, "a", "b", ".tlc.yaml"), []byte(""), 0644)

	configs := findAllConfigs(subDir)
	if len(configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(configs))
	}
}
