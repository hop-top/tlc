package config

import (
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

type OutputConfig struct {
	Format  string `yaml:"format"`
	Color   bool   `yaml:"color"`
	Verbose bool   `yaml:"verbose"`
	Quiet   bool   `yaml:"quiet"`
	LogFile string `yaml:"log_file"`
}

type TaskConfig struct {
	DefaultStatus    string `yaml:"default_status"`
	IDFormat         string `yaml:"id_format"`
	AutoAssign       bool   `yaml:"auto_assign"`
	RequireReference bool   `yaml:"require_reference"`
	TodoFile         string `yaml:"todo_file"`
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

type UIConfig struct {
	Pager      string `yaml:"pager"`
	Editor     string `yaml:"editor"`
	DateFormat string `yaml:"date_format"`
	Timezone   string `yaml:"timezone"`
	TableStyle string `yaml:"table_style"`
}
