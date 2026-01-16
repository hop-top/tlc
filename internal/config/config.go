package config

import (
	"fmt"
	"time"
)

// Config represents the TLC configuration
type Config struct {
	Version string        `yaml:"version"`
	Output  OutputConfig  `yaml:"output"`
	Task    TaskConfig    `yaml:"task"`
	Git     GitConfig     `yaml:"git"`
	Sync    SyncConfig    `yaml:"sync"`
	Storage StorageConfig `yaml:"storage"`
	UI      UIConfig      `yaml:"ui"`
}

func (c *Config) Validate() error {
	if err := c.Output.Validate(); err != nil {
		return err
	}
	if err := c.Task.Validate(); err != nil {
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

type OutputConfig struct {
	Format  string `yaml:"format"`
	Color   bool   `yaml:"color"`
	Verbose bool   `yaml:"verbose"`
	Quiet   bool   `yaml:"quiet"`
	LogFile string `yaml:"log_file"`
}

func (o *OutputConfig) Validate() error {
	switch o.Format {
	case "table", "json", "yaml", "tls", "":
		return nil
	default:
		return fmt.Errorf("invalid output format: %s", o.Format)
	}
}

type TaskConfig struct {
	DefaultStatus    string `yaml:"default_status"`
	IDFormat         string `yaml:"id_format"`
	AutoAssign       bool   `yaml:"auto_assign"`
	RequireReference bool   `yaml:"require_reference"`
	TodoFile         string `yaml:"todo_file"`
}

func (t *TaskConfig) Validate() error {
	// Status validation logic could be shared with core
	return nil
}

type GitConfig struct {
	Worktree GitWorktreeConfig `yaml:"worktree"`
	Branch   GitBranchConfig   `yaml:"branch"`
	Commit   GitCommitConfig   `yaml:"commit"`
}

type GitWorktreeConfig struct {
	Directory  string `yaml:"directory"`
	AutoCreate bool   `yaml:"auto_create"`
	AutoRemove bool   `yaml:"auto_remove"`
}

type GitBranchConfig struct {
	PrefixFromType bool   `yaml:"prefix_from_type"`
	ZeroPadIssue   int    `yaml:"zero_pad_issue"`
	Separator      string `yaml:"separator"`
}

type GitCommitConfig struct {
	AutoGenerate bool   `yaml:"auto_generate"`
	Template     string `yaml:"template"`
	CoAuthor     string `yaml:"co_author"`
}

type SyncConfig struct {
	Enabled          bool                 `yaml:"enabled"`
	Interval         time.Duration        `yaml:"interval"`
	ConflictStrategy string               `yaml:"conflict_strategy"`
	BatchSize        int                  `yaml:"batch_size"`
	GitHub           GitHubSyncConfig     `yaml:"github"`
	Jira             JiraSyncConfig       `yaml:"jira"`
	Linear           LinearSyncConfig     `yaml:"linear"`
}

func (s *SyncConfig) Validate() error {
	if s.GitHub.Enabled && s.GitHub.Repo == "" {
		return fmt.Errorf("sync.github.repo is required when GitHub sync is enabled")
	}
	return nil
}

type GitHubSyncConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Repo           string `yaml:"repo"`
	SyncDirection  string `yaml:"sync_direction"`
	ImportLabels   bool   `yaml:"import_labels"`
	ImportMilestones bool `yaml:"import_milestones"`
}

type JiraSyncConfig struct {
	Enabled       bool   `yaml:"enabled"`
	URL           string `yaml:"url"`
	Project       string `yaml:"project"`
	SyncDirection string `yaml:"sync_direction"`
}

type LinearSyncConfig struct {
	Enabled       bool   `yaml:"enabled"`
	TeamID        string `yaml:"team_id"`
	SyncDirection string `yaml:"sync_direction"`
}

type StorageConfig struct {
	Backend          string `yaml:"backend"`
	DBPath           string `yaml:"db_path"`
	ConnectionString string `yaml:"connection_string"`
}

func (s *StorageConfig) Validate() error {
	switch s.Backend {
	case "sqlite", "local", "postgres", "":
		return nil
	default:
		return fmt.Errorf("invalid storage backend: %s", s.Backend)
	}
}

type UIConfig struct {
	Pager      string            `yaml:"pager"`
	Editor     string            `yaml:"editor"`
	DateFormat string            `yaml:"date_format"`
	Timezone   string            `yaml:"timezone"`
	TableStyle string            `yaml:"table_style"`
	TagColors  map[string]string `yaml:"tag_colors"`
	LogSortDirection   string            `yaml:"log_sort_direction"`
	Theme      string            `yaml:"theme"`
}
