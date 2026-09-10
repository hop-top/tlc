package config_test

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"

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

// decodeConfigYAML decodes config YAML the way the runtime does: struct
// tags first, then Validate() to apply post-decode defaults.
func decodeConfigYAML(t *testing.T, data string) *config.Config {
	t.Helper()
	var cfg config.Config
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("yaml.Unmarshal() error: %v", err)
	}
	if err := cfg.Task.Validate(); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
	return &cfg
}

// TestStaleConfig_LoadFromYAML decodes a real config YAML containing a
// task.stale section and verifies values survive decode + validate.
func TestStaleConfig_LoadFromYAML(t *testing.T) {
	cfg := decodeConfigYAML(t, `
task:
  stale:
    default_timeout: 2h
    hooks:
      - command: 'echo "stale: {{.ID}}"'
      - command: 'notify-send "Task stale"'
`)

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
// section is absent from config, the 6h default is applied after decode.
func TestStaleConfig_LoadFromYAML_DefaultApplied(t *testing.T) {
	cfg := decodeConfigYAML(t, `
task:
  default_status: TODO
`)

	if cfg.Task.Stale.DefaultTimeout != 6*time.Hour {
		t.Fatalf("expected 6h default, got %v", cfg.Task.Stale.DefaultTimeout)
	}
	if len(cfg.Task.Stale.Hooks) != 0 {
		t.Fatalf("expected no hooks, got %d", len(cfg.Task.Stale.Hooks))
	}
}
