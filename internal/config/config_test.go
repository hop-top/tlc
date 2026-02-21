package config

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	testInvalid = "invalid"
	testPrompt  = "prompt"
)

func TestLoadConfig_Merging(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projectRoot := filepath.Join(tmpDir, "project")
	os.MkdirAll(filepath.Join(projectRoot, ".tlc"), 0o755)

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

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			setup:   func(_ *Config) {},
			wantErr: false,
		},
		{
			name: testInvalid + " output format",
			setup: func(c *Config) {
				c.Output.Format = testInvalid
			},
			wantErr: true,
		},
		{
			name: "missing github repo when enabled",
			setup: func(c *Config) {
				c.Sync.GitHub.Enabled = true
				c.Sync.GitHub.Repo = ""
			},
			wantErr: true,
		},
		{
			name: testInvalid + " storage backend",
			setup: func(c *Config) {
				c.Storage.Backend = testInvalid
			},
			wantErr: true,
		},
		{
			name: testInvalid + " fallback_mode",
			setup: func(c *Config) {
				c.Project.FallbackMode = testInvalid
			},
			wantErr: true,
		},
		{
			name: testInvalid + " duplicate_id_strategy",
			setup: func(c *Config) {
				c.Project.DuplicateIDStrategy = testInvalid
			},
			wantErr: true,
		},
		{
			name: "valid fallback_mode auto",
			setup: func(c *Config) {
				c.Project.FallbackMode = "auto"
			},
			wantErr: false,
		},
		{
			name: "valid fallback_mode detected",
			setup: func(c *Config) {
				c.Project.FallbackMode = "detected"
			},
			wantErr: false,
		},
		{
			name: "valid fallback_mode " + testPrompt,
			setup: func(c *Config) {
				c.Project.FallbackMode = testPrompt
			},
			wantErr: false,
		},
		{
			name: "valid duplicate_id_strategy share",
			setup: func(c *Config) {
				c.Project.DuplicateIDStrategy = "share"
			},
			wantErr: false,
		},
		{
			name: "valid duplicate_id_strategy unique",
			setup: func(c *Config) {
				c.Project.DuplicateIDStrategy = "unique"
			},
			wantErr: false,
		},
		{
			name: "valid duplicate_id_strategy " + testPrompt,
			setup: func(c *Config) {
				c.Project.DuplicateIDStrategy = testPrompt
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setup(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
