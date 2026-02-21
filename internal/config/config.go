package config

import (
	"fmt"
	"time"
)

// ProjectConfig contains project-specific configuration.
type ProjectConfig struct {
	ID                  string `yaml:"id"`
	FallbackMode        string `yaml:"fallback_mode"`
	DuplicateIDStrategy string `yaml:"duplicate_id_strategy"`
}

type Config struct {
	Version string        `yaml:"version"`
	Project ProjectConfig `yaml:"project"`
	Output  OutputConfig  `yaml:"output"`
	Task    TaskConfig    `yaml:"task"`
	Git     GitConfig     `yaml:"git"`
	Sync    SyncConfig    `yaml:"sync"`
	Storage StorageConfig `yaml:"storage"`
	UI      UIConfig      `yaml:"ui"`
}

// Validate validates the project configuration.
func (p *ProjectConfig) Validate() error {
	switch p.FallbackMode {
	case "", "auto", "detected", "prompt":
	default:
		return fmt.Errorf("invalid fallback_mode: %s (must be auto, detected, or prompt)", p.FallbackMode)
	}
	switch p.DuplicateIDStrategy {
	case "", "share", "unique", "prompt":
	default:
		return fmt.Errorf("invalid duplicate_id_strategy: %s (must be share, unique, or prompt)", p.DuplicateIDStrategy)
	}
	return nil
}

// Validate validates the complete configuration.
func (c *Config) Validate() error {
	if err := c.Output.Validate(); err != nil {
		return err
	}
	if err := c.Task.Validate(); err != nil {
		return err
	}
	if err := c.Project.Validate(); err != nil {
		return err
	}
	if err := c.Sync.Validate(); err != nil {
		return err
	}
	if err := c.Storage.Validate(); err != nil {
		return err
	}
	return nil
}

// OutputConfig contains output-related configuration.
type OutputConfig struct {
	Format  string `yaml:"format"`
	Color   bool   `yaml:"color"`
	Verbose bool   `yaml:"verbose"`
	Quiet   bool   `yaml:"quiet"`
	LogFile string `yaml:"log_file"`
}

// Validate validates the output configuration.
func (o *OutputConfig) Validate() error {
	switch o.Format {
	case "table", "json", "yaml", "tls", "":
		return nil
	default:
		return fmt.Errorf("invalid output format: %s", o.Format)
	}
}

// TaskConfig contains task-related configuration.
type TaskConfig struct {
	DefaultStatus    string        `yaml:"default_status"`
	IDFormat         string        `yaml:"id_format"`
	AutoAssign       bool          `yaml:"auto_assign"`
	RequireReference bool          `yaml:"require_reference"`
	TodoFile         string        `yaml:"todo_file"`
	ArchiveThreshold time.Duration `yaml:"archive_threshold"`
}

// Validate validates the task configuration.
func (t *TaskConfig) Validate() error {
	// Status validation logic could be shared with core
	return nil
}

// GitConfig contains git-related configuration.
type GitConfig struct {
	Track  bool            `yaml:"track"`
	Branch GitBranchConfig `yaml:"branch"`
	Commit GitCommitConfig `yaml:"commit"`
}

// GitBranchConfig contains git branch configuration.
type GitBranchConfig struct {
	PrefixFromType bool   `yaml:"prefix_from_type"`
	ZeroPadIssue   int    `yaml:"zero_pad_issue"`
	Separator      string `yaml:"separator"`
}

// GitCommitConfig contains git commit configuration.
type GitCommitConfig struct {
	AutoGenerate bool   `yaml:"auto_generate"`
	Template     string `yaml:"template"`
	CoAuthor     string `yaml:"co_author"`
}

// SyncConfig contains synchronization configuration.
type SyncConfig struct {
	Enabled          bool             `yaml:"enabled"`
	AutoPush         bool             `yaml:"auto_push"`
	Interval         time.Duration    `yaml:"interval"`
	ConflictStrategy string           `yaml:"conflict_strategy"`
	BatchSize        int              `yaml:"batch_size"`
	GitHub           GitHubSyncConfig `yaml:"github"`
	Jira             JiraSyncConfig   `yaml:"jira"`
	Linear           LinearSyncConfig `yaml:"linear"`
}

// Validate validates the sync configuration.
func (s *SyncConfig) Validate() error {
	if s.GitHub.Enabled && s.GitHub.Repo == "" {
		return fmt.Errorf("sync.github.repo is required when GitHub sync is enabled")
	}
	return nil
}

// GitHubSyncConfig contains GitHub sync configuration.
type GitHubSyncConfig struct {
	Enabled          bool   `yaml:"enabled"`
	Repo             string `yaml:"repo"`
	SyncDirection    string `yaml:"sync_direction"`
	ImportLabels     bool   `yaml:"import_labels"`
	ImportMilestones bool   `yaml:"import_milestones"`
}

// JiraSyncConfig contains Jira sync configuration.
type JiraSyncConfig struct {
	Enabled       bool   `yaml:"enabled"`
	URL           string `yaml:"url"`
	Project       string `yaml:"project"`
	SyncDirection string `yaml:"sync_direction"`
}

// LinearSyncConfig contains Linear sync configuration.
type LinearSyncConfig struct {
	Enabled       bool   `yaml:"enabled"`
	TeamID        string `yaml:"team_id"`
	SyncDirection string `yaml:"sync_direction"`
}

// StorageConfig contains storage configuration.
type StorageConfig struct {
	Backend          string `yaml:"backend"`
	DBPath           string `yaml:"db_path"`
	ConnectionString string `yaml:"connection_string"`
}

// Validate validates the storage configuration.
func (s *StorageConfig) Validate() error {
	switch s.Backend {
	case "sqlite", "local", "postgres", "":
		return nil
	default:
		return fmt.Errorf("invalid storage backend: %s", s.Backend)
	}
}

// UIConfig contains UI-related configuration.
type UIConfig struct {
	Pager            string            `yaml:"pager"`
	Editor           string            `yaml:"editor"`
	DateFormat       string            `yaml:"date_format"`
	Timezone         string            `yaml:"timezone"`
	TableStyle       string            `yaml:"table_style"`
	TagColors        map[string]string `yaml:"tag_colors"`
	LogSortDirection string            `yaml:"log_sort_direction"`
	Theme            string            `yaml:"theme"`
}
