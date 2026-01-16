package config

import (
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
		},
		Git: GitConfig{
			Worktree: GitWorktreeConfig{
				Directory:  ".worktrees",
				AutoCreate: true,
			},
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

	// 1. User config
	home, _ := os.UserHomeDir()
	userConfigPath := filepath.Join(home, ".config", "tlc", "config.yaml")
	if err := mergeFile(cfg, userConfigPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load user config: %w", err)
	}

	// 2. Project config
	if projectRoot != "" {
		projectConfigPath := filepath.Join(projectRoot, ".tlc", "config.yaml")
		if err := mergeFile(cfg, projectConfigPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load project config: %w", err)
		}
	}

	// 3. Environment variables (simplistic implementation for now)
	applyEnvOverrides(cfg)

	// 4. Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, cfg)
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
