package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	kitconfig "hop.top/kit/go/core/config"
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

// LoadConfig loads configuration by merging system, user, and project
// config files (in that order) via kit/config.Load, then applies env
// overrides and validates the result.
func LoadConfig(projectRoot string) (*Config, error) {
	cfg := DefaultConfig()

	userConfigPath, err := UserConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user config path: %w", err)
	}

	var projectConfigPath string
	if projectRoot != "" {
		projectConfigPath = filepath.Join(projectRoot, ".tlc", "config.yaml")
	}

	opts := kitconfig.Options{
		SystemConfigPath:  SystemConfigPath(),
		UserConfigPath:    userConfigPath,
		ProjectConfigPath: projectConfigPath,
		EnvOverride: func(dst any) {
			if c, ok := dst.(*Config); ok {
				applyEnvOverrides(c)
			}
		},
	}

	if err := kitconfig.Load(cfg, opts); err != nil {
		return nil, fmt.Errorf("config load failed: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
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
