package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

func DefaultConfig() *Config {
	return &Config{
		Version: "0.1",
		Output: OutputConfig{
			Format:  "table",
			Color:   true,
			Verbose: false,
		},
		Task: TaskConfig{
			DefaultStatus:    "TODO",
			IDFormat:         "T-{seq:04d}",
			AutoAssign:       false,
			RequireReference: true,
			Statuses:         GetDefaultStatuses(),
			StateMachine:     GetDefaultStateMachine(),
		},
		Git: GitConfig{
			Branch: GitBranchConfig{
				PrefixFromType: true,
				ZeroPadIssue:   4,
				Separator:      "/",
			},
			Commit: GitCommitConfig{
				AutoGenerate: true,
				Template:     "{type}: {description} (closes #{issue})",
			},
		},
		Sync: SyncConfig{
			Enabled:          true,
			Interval:         5 * time.Minute,
			ConflictStrategy: "prompt",
			BatchSize:        50,
		},
		Storage: StorageConfig{
			Backend: "sqlite",
			DBPath:  ".tlc/db.sqlite",
		},
		UI: UIConfig{
			Pager:      "auto",
			DateFormat: "2006-01-02 15:04:05",
			Timezone:   "local",
			TableStyle: "unicode",
		},
	}
}

func LoadConfig(projectRoot string) (*Config, error) {
	cfg := DefaultConfig()

	// 1. System config
	if err := mergeFile(cfg, SystemConfigPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load system config: %w", err)
	}

	// 2. User config
	userConfigPath, err := UserConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user config path: %w", err)
	}
	if err := mergeFile(cfg, userConfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load user config: %w", err)
	}

	// 3. Project config
	if projectRoot != "" {
		projectConfigPath := filepath.Join(projectRoot, ".tlc", "config.yaml")
		if err := mergeFile(cfg, projectConfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to load project config: %w", err)
		}
	}

	// 4. Environment variables (simplistic implementation for now)
	applyEnvOverrides(cfg)

	// 5. Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return nil
}

func applyEnvOverrides(cfg *Config) {
	if val := os.Getenv("TLC_OUTPUT_FORMAT"); val != "" {
		cfg.Output.Format = val
	}
	if val := os.Getenv("TLC_STORAGE_BACKEND"); val != "" {
		cfg.Storage.Backend = val
	}
	// Add more as needed
}
