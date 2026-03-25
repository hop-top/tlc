package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfig_Merging(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectRoot := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".tlc"), 0o755); err != nil {
		t.Fatalf("failed to create project config dir: %v", err)
	}

	projectConfigPath := filepath.Join(projectRoot, ".tlc", "config.yaml")
	projectConfigData := `
output:
  format: json
task:
  default_status: IN_PROGRESS
`
	if err := os.WriteFile(projectConfigPath, []byte(projectConfigData), 0o644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg, err := LoadConfig(projectRoot)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Output.Format != "json" {
		t.Errorf("expected output format json, got %s", cfg.Output.Format)
	}
	if cfg.Task.DefaultStatus != "IN_PROGRESS" {
		t.Errorf("expected default status IN_PROGRESS, got %s", cfg.Task.DefaultStatus)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	cfg := DefaultConfig()
	os.Setenv("TLC_OUTPUT_FORMAT", "yaml")
	defer os.Unsetenv("TLC_OUTPUT_FORMAT")

	applyEnvOverrides(cfg)

	if cfg.Output.Format != "yaml" {
		t.Errorf("expected output format yaml from env, got %s", cfg.Output.Format)
	}
}

func TestLoadConfig_UsesOSUserConfigDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-userdir-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}()
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatalf("failed to unset XDG_CONFIG_HOME: %v", err)
	}

	configPath, err := UserConfigPath()
	if err != nil {
		t.Fatalf("failed to resolve user config path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configData := `
output:
  format: yaml
`
	if err := os.WriteFile(configPath, []byte(configData), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Output.Format != "yaml" {
		t.Fatalf("expected output format yaml, got %q", cfg.Output.Format)
	}
}

func TestPrepareViperForWrite_FallsBackToUserConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-config-write-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	homeDir := filepath.Join(tmpDir, "home")
	oldHome := os.Getenv("HOME")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}()
	if err := os.Setenv("HOME", homeDir); err != nil {
		t.Fatalf("failed to set HOME: %v", err)
	}
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatalf("failed to unset XDG_CONFIG_HOME: %v", err)
	}

	v := viper.New()
	v.SetConfigFile(SystemConfigPath())

	target, err := PrepareViperForWrite(v)
	if err != nil {
		t.Fatalf("PrepareViperForWrite() error = %v", err)
	}

	want, err := UserConfigPath()
	if err != nil {
		t.Fatalf("failed to resolve user config path: %v", err)
	}
	if target != want {
		t.Fatalf("PrepareViperForWrite() = %q, want %q", target, want)
	}

	if _, err := os.Stat(filepath.Dir(target)); err != nil {
		t.Fatalf("expected config dir to exist: %v", err)
	}
}
