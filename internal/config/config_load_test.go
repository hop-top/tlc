package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func TestListDefaults_RoundTrip(t *testing.T) {
	input := `
version: "0.1"
task:
  list:
    columns: [id, title]
    status: [TODO]
tracks:
  list:
    columns: [id, progress]
defaults:
  list:
    columns: [id, title, status]
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(input), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Task.List assertions
	if cfg.Task.List == nil {
		t.Fatal("cfg.Task.List is nil")
	}
	if want := []string{"id", "title"}; !slicesEqual(cfg.Task.List.Columns, want) {
		t.Errorf("Task.List.Columns = %v, want %v", cfg.Task.List.Columns, want)
	}
	if want := []string{"TODO"}; !slicesEqual(cfg.Task.List.Status, want) {
		t.Errorf("Task.List.Status = %v, want %v", cfg.Task.List.Status, want)
	}

	// Tracks.List assertions
	if cfg.Tracks.List == nil {
		t.Fatal("cfg.Tracks.List is nil")
	}
	if want := []string{"id", "progress"}; !slicesEqual(cfg.Tracks.List.Columns, want) {
		t.Errorf("Tracks.List.Columns = %v, want %v", cfg.Tracks.List.Columns, want)
	}

	// Defaults.List assertions
	if cfg.Defaults == nil {
		t.Fatal("cfg.Defaults is nil")
	}
	if cfg.Defaults.List == nil {
		t.Fatal("cfg.Defaults.List is nil")
	}
	if want := []string{"id", "title", "status"}; !slicesEqual(cfg.Defaults.List.Columns, want) {
		t.Errorf("Defaults.List.Columns = %v, want %v", cfg.Defaults.List.Columns, want)
	}

	// Round-trip: marshal back and re-unmarshal
	out, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cfg2 Config
	if err := yaml.Unmarshal(out, &cfg2); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if cfg2.Task.List == nil || !slicesEqual(cfg2.Task.List.Columns, []string{"id", "title"}) {
		t.Errorf("round-trip Task.List.Columns = %v, want [id title]", cfg2.Task.List)
	}
	if cfg2.Tracks.List == nil || !slicesEqual(cfg2.Tracks.List.Columns, []string{"id", "progress"}) {
		t.Errorf("round-trip Tracks.List.Columns = %v, want [id progress]", cfg2.Tracks.List)
	}
	if cfg2.Defaults == nil || cfg2.Defaults.List == nil || !slicesEqual(cfg2.Defaults.List.Columns, []string{"id", "title", "status"}) {
		t.Errorf("round-trip Defaults.List.Columns = %v, want [id title status]", cfg2.Defaults)
	}

	// Validate: empty-fields config must not emit list: {} or defaults: {} blocks
	empty := Config{}
	emptyOut, err := yaml.Marshal(&empty)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	emptyStr := string(emptyOut)
	if contains(emptyStr, "list:") {
		t.Errorf("empty Config marshaled with 'list:' block: %s", emptyStr)
	}
	if contains(emptyStr, "defaults:") {
		t.Errorf("empty Config marshaled with 'defaults:' block: %s", emptyStr)
	}

	// cfg.Validate() on this config must return nil
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

func slicesEqual(a, b []string) bool {
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

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
