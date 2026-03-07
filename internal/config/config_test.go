package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
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

func TestTaskConfig_TerminalDefaultStatusRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.DefaultStatus = "DONE"
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for terminal default_status, got nil")
	}
}

func TestTaskConfig_MissingInitialRoleRejected(t *testing.T) {
	cfg := DefaultConfig()
	// Remove the "initial" role from all statuses
	for i := range cfg.Task.Statuses {
		if cfg.Task.Statuses[i].Role == "initial" {
			cfg.Task.Statuses[i].Role = "completed"
		}
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing initial role, got nil")
	}
}

func TestTaskConfig_MissingActiveRoleRejected(t *testing.T) {
	cfg := DefaultConfig()
	// Remove the "active" role from all statuses
	for i := range cfg.Task.Statuses {
		if cfg.Task.Statuses[i].Role == "active" {
			cfg.Task.Statuses[i].Role = "completed"
		}
	}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for missing active role, got nil")
	}
}

func TestTaskConfig_DuplicateTLSMarkerRejected(t *testing.T) {
	cfg := DefaultConfig()
	// Set two statuses to the same TLS marker
	cfg.Task.Statuses[0].TLSMarker = "x"
	cfg.Task.Statuses[2].TLSMarker = "x"
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for duplicate TLS marker, got nil")
	}
}

func TestTaskConfig_DuplicateStatusNameRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.Statuses = append(cfg.Task.Statuses, StatusDefinition{
		Name:       "TODO",
		Label:      "Duplicate Todo",
		IsTerminal: false,
		Role:       "initial",
		TLSMarker:  "?",
	})
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for duplicate status name, got nil")
	}
}

func TestTaskConfig_StateMachineUnknownStatusRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.StateMachine.Rules["NONEXISTENT"] = []string{"TODO"}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for state machine rule referencing unknown status, got nil")
	}
}

func TestTaskConfig_StateMachineUnknownTargetStatusRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.StateMachine.Rules["TODO"] = []string{"NONEXISTENT"}
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for state machine rule referencing unknown target status, got nil")
	}
}

func TestTaskConfig_EmptyStatusesPopulatedWithDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.Statuses = nil
	cfg.Task.StateMachine = nil
	err := cfg.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Task.Statuses) != 4 {
		t.Errorf("expected 4 default statuses, got %d", len(cfg.Task.Statuses))
	}
	if cfg.Task.StateMachine == nil {
		t.Error("expected state machine to be populated with defaults")
	}
}
