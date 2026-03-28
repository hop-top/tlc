package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/tlc/internal/config"
)

func TestStaleConfig_DefaultTimeout(t *testing.T) {
	cfg := &config.TaskConfig{}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
	if cfg.Stale.DefaultTimeout != 6*time.Hour {
		t.Fatalf("expected 6h default, got %v", cfg.Stale.DefaultTimeout)
	}
}

func TestStaleConfig_HookCommands(t *testing.T) {
	cfg := &config.TaskConfig{
		Stale: config.StaleConfig{
			DefaultTimeout: time.Hour,
			Hooks: []config.StaleHook{
				{Command: `echo "stale: {{.ID}}"`},
			},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
	if len(cfg.Stale.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(cfg.Stale.Hooks))
	}
	if cfg.Stale.Hooks[0].Command != `echo "stale: {{.ID}}"` {
		t.Fatalf("hook command mismatch: %v", cfg.Stale.Hooks[0].Command)
	}
	// Explicit timeout should not be overwritten
	if cfg.Stale.DefaultTimeout != time.Hour {
		t.Fatalf("expected 1h, got %v", cfg.Stale.DefaultTimeout)
	}
}

// TestStaleConfig_LoadFromYAML is an integration test that loads a real config
// YAML containing a task.stale section and verifies defaults apply correctly.
func TestStaleConfig_LoadFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".tlc"), 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Write config with explicit stale section
	configData := `
task:
  stale:
    default_timeout: 2h
    hooks:
      - command: 'echo "stale: {{.ID}}"'
      - command: 'notify-send "Task stale"'
`
	if err := os.WriteFile(
		filepath.Join(projectRoot, ".tlc", "config.yaml"),
		[]byte(configData),
		0o644,
	); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := config.LoadConfig(projectRoot)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	if cfg.Task.Stale.DefaultTimeout != 2*time.Hour {
		t.Fatalf("expected 2h, got %v", cfg.Task.Stale.DefaultTimeout)
	}
	if len(cfg.Task.Stale.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(cfg.Task.Stale.Hooks))
	}
	if cfg.Task.Stale.Hooks[0].Command != `echo "stale: {{.ID}}"` {
		t.Fatalf("hook[0] command mismatch: %v", cfg.Task.Stale.Hooks[0].Command)
	}
}

// TestStaleConfig_LoadFromYAML_DefaultApplied verifies that when the stale
// section is absent from config, the 6h default is applied after load.
func TestStaleConfig_LoadFromYAML_DefaultApplied(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(filepath.Join(projectRoot, ".tlc"), 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	// Minimal config with no stale section
	configData := `
task:
  default_status: TODO
`
	if err := os.WriteFile(
		filepath.Join(projectRoot, ".tlc", "config.yaml"),
		[]byte(configData),
		0o644,
	); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := config.LoadConfig(projectRoot)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	if cfg.Task.Stale.DefaultTimeout != 6*time.Hour {
		t.Fatalf("expected 6h default, got %v", cfg.Task.Stale.DefaultTimeout)
	}
	if len(cfg.Task.Stale.Hooks) != 0 {
		t.Fatalf("expected no hooks, got %d", len(cfg.Task.Stale.Hooks))
	}
}
