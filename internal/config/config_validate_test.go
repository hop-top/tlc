package config

import (
	"testing"
	"time"
)

const (
	testInvalid = "invalid"
	testPrompt  = "prompt"
)

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

func TestTrackConfig_DefaultsApplied(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tracks.StaleThreshold != 48*time.Hour {
		t.Errorf("expected 48h stale threshold, got %v", cfg.Tracks.StaleThreshold)
	}
	if cfg.Tracks.Health.MaxActive != 3 {
		t.Errorf("expected max_active=3, got %d", cfg.Tracks.Health.MaxActive)
	}
	if cfg.Tracks.Health.MinProgressToStart != 50 {
		t.Errorf("expected min_progress_to_start=50, got %d",
			cfg.Tracks.Health.MinProgressToStart)
	}
}

func TestTrackConfig_NegativeMaxActiveRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tracks.Health.MaxActive = -1
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for negative max_active, got nil")
	}
}

func TestTrackConfig_MinProgressOutOfRange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tracks.Health.MinProgressToStart = 101
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for min_progress_to_start > 100, got nil")
	}
}

func TestTrackConfig_MinProgressNegativeRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tracks.Health.MinProgressToStart = -5
	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for negative min_progress_to_start, got nil")
	}
}

func TestTrackConfig_CustomValuesPreserved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tracks.StaleThreshold = 72 * time.Hour
	cfg.Tracks.Health.MaxActive = 5
	cfg.Tracks.Health.MinProgressToStart = 80
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tracks.StaleThreshold != 72*time.Hour {
		t.Errorf("expected 72h, got %v", cfg.Tracks.StaleThreshold)
	}
	if cfg.Tracks.Health.MaxActive != 5 {
		t.Errorf("expected 5, got %d", cfg.Tracks.Health.MaxActive)
	}
	if cfg.Tracks.Health.MinProgressToStart != 80 {
		t.Errorf("expected 80, got %d", cfg.Tracks.Health.MinProgressToStart)
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
