package config

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"
)

// ProjectConfig contains project-specific configuration.
type ProjectConfig struct {
	ID                  string `yaml:"id"`
	FallbackMode        string `yaml:"fallback_mode"`
	DuplicateIDStrategy string `yaml:"duplicate_id_strategy"`
	Workspace           string `yaml:"workspace,omitempty"`
}

// WorkspaceConfig contains workspace configuration.
type WorkspaceConfig struct {
	Name    string        `yaml:"name"`
	WsmID   string        `yaml:"wsm_id,omitempty"`
	Spaces  []SpaceConfig `yaml:"spaces"`
	Default bool          `yaml:"default,omitempty"`
}

// SpaceConfig contains configuration for a single space within a workspace.
type SpaceConfig struct {
	URI     string            `yaml:"uri"`
	Adapter string            `yaml:"adapter,omitempty"`
	Label   string            `yaml:"label,omitempty"`
	Options map[string]string `yaml:"options,omitempty"`
}

// TrackHealthConfig holds thresholds for project-level health checks.
type TrackHealthConfig struct {
	MaxActive          int `yaml:"max_active"`
	MinProgressToStart int `yaml:"min_progress_to_start"`
}

// TrackConfig holds track-related configuration.
type TrackConfig struct {
	StaleThreshold time.Duration    `yaml:"stale_threshold"`
	Health         TrackHealthConfig `yaml:"health"`
	PlanExtractor  string           `yaml:"plan_extractor,omitempty"`
}

// Validate validates the track configuration.
func (tc *TrackConfig) Validate() error {
	if tc.StaleThreshold < 0 {
		return fmt.Errorf(
			"tracks.stale_threshold must be >= 0, got %s",
			tc.StaleThreshold,
		)
	}
	if tc.StaleThreshold == 0 {
		tc.StaleThreshold = 48 * time.Hour
	}
	if tc.Health.MaxActive == 0 {
		tc.Health.MaxActive = 3
	}
	if tc.Health.MinProgressToStart == 0 {
		tc.Health.MinProgressToStart = 50
	}
	if tc.Health.MaxActive < 0 {
		return fmt.Errorf(
			"tracks.health.max_active must be > 0, got %d",
			tc.Health.MaxActive,
		)
	}
	if tc.Health.MinProgressToStart < 0 || tc.Health.MinProgressToStart > 100 {
		return fmt.Errorf(
			"tracks.health.min_progress_to_start must be 0-100, got %d",
			tc.Health.MinProgressToStart,
		)
	}
	return nil
}

type Config struct {
	Version    string            `yaml:"version"`
	Project    ProjectConfig     `yaml:"project"`
	Output     OutputConfig      `yaml:"output"`
	Task       TaskConfig        `yaml:"task"`
	Tracks     TrackConfig       `yaml:"tracks"`
	Git        GitConfig         `yaml:"git"`
	Sync       SyncConfig        `yaml:"sync"`
	Storage    StorageConfig     `yaml:"storage"`
	UI         UIConfig          `yaml:"ui"`
	Workspaces []WorkspaceConfig `yaml:"workspaces,omitempty"`
	Validation ValidationConfig  `yaml:"validation,omitempty"`
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
	if err := c.Tracks.Validate(); err != nil {
		return err
	}
	if err := c.Sync.Validate(); err != nil {
		return err
	}
	if err := c.Storage.Validate(); err != nil {
		return err
	}
	if err := c.ValidateWorkspaces(); err != nil {
		return err
	}
	if err := c.Validation.Validate(); err != nil {
		return err
	}
	return nil
}

// knownAdapters lists recognized space adapter names.
var knownAdapters = map[string]bool{
	"filesystem": true,
}

// ValidateWorkspaces validates all workspace configurations.
func (c *Config) ValidateWorkspaces() error {
	names := make(map[string]bool, len(c.Workspaces))
	defaultCount := 0

	for i, ws := range c.Workspaces {
		if err := ws.Validate(); err != nil {
			return fmt.Errorf("workspaces[%d]: %w", i, err)
		}
		if names[ws.Name] {
			return fmt.Errorf("duplicate workspace name: %s", ws.Name)
		}
		names[ws.Name] = true
		if ws.Default {
			defaultCount++
		}
	}

	if defaultCount > 1 {
		return fmt.Errorf("at most one workspace can be default, found %d", defaultCount)
	}

	return nil
}

// Validate validates a single workspace configuration.
func (w *WorkspaceConfig) Validate() error {
	if strings.TrimSpace(w.Name) == "" {
		return fmt.Errorf("workspace name must not be empty")
	}
	for j, sp := range w.Spaces {
		if strings.TrimSpace(sp.URI) == "" {
			return fmt.Errorf("space[%d]: URI must not be empty", j)
		}
		if sp.Adapter != "" && !knownAdapters[sp.Adapter] {
			log.Printf("WARNING: workspace %q space[%d]: unknown adapter %q", w.Name, j, sp.Adapter)
		}
	}
	return nil
}

// DefaultWorkspace returns the workspace marked as default, or the first one,
// or nil if no workspaces are configured.
func (c *Config) DefaultWorkspace() *WorkspaceConfig {
	if len(c.Workspaces) == 0 {
		return nil
	}
	for i := range c.Workspaces {
		if c.Workspaces[i].Default {
			return &c.Workspaces[i]
		}
	}
	return &c.Workspaces[0]
}

// FindWorkspace returns the workspace with the given name, or nil.
func (c *Config) FindWorkspace(name string) *WorkspaceConfig {
	for i := range c.Workspaces {
		if c.Workspaces[i].Name == name {
			return &c.Workspaces[i]
		}
	}
	return nil
}

// InferAdapterFromURI returns the adapter name inferred from a URI.
// Returns "filesystem" for bare paths and file:// URIs, empty string for
// unknown schemes.
func InferAdapterFromURI(uri string) string {
	if uri == "" {
		return ""
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	switch parsed.Scheme {
	case "", "file":
		return "filesystem"
	default:
		return ""
	}
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
	case "table", "json", "yaml", "tls", "summary", "":
		return nil
	default:
		return fmt.Errorf("invalid output format: %s", o.Format)
	}
}

// StatusDefinition defines a single task status.
type StatusDefinition struct {
	Name        string `yaml:"name"`
	Label       string `yaml:"label"`
	Description string `yaml:"description,omitempty"`
	IsTerminal  bool   `yaml:"is_terminal"`
	Color       string `yaml:"color,omitempty"`
	Role        string `yaml:"role,omitempty"`       // "initial", "active", "completed"
	TLSMarker   string `yaml:"tls_marker,omitempty"` // Single char for TLS format
}

// WorkflowDefinition defines allowed state transitions.
type WorkflowDefinition struct {
	Rules map[string][]string `yaml:"rules"` // map[from][]to
}

// WorkflowOverride allows per-tag workflow customization.
type WorkflowOverride struct {
	StateMachine *WorkflowDefinition `yaml:"state_machine,omitempty"`
}

// StaleHook is a shell command run when a task crosses the stale threshold.
type StaleHook struct {
	Command string `yaml:"command"`
}

// StaleConfig holds staleness detection and hook configuration.
type StaleConfig struct {
	DefaultTimeout time.Duration `yaml:"default_timeout"`
	Hooks          []StaleHook   `yaml:"hooks,omitempty"`
}

// TaskConfig contains task-related configuration.
type TaskConfig struct {
	DefaultStatus    string                      `yaml:"default_status"`
	IDFormat         string                      `yaml:"id_format"`
	AutoAssign       bool                        `yaml:"auto_assign"`
	RequireReference bool                        `yaml:"require_reference"`
	TodoFile         string                      `yaml:"todo_file"`
	ArchiveThreshold time.Duration               `yaml:"archive_threshold"`
	Statuses         []StatusDefinition          `yaml:"statuses,omitempty"`
	StateMachine     *WorkflowDefinition         `yaml:"state_machine,omitempty"`
	Workflows        map[string]WorkflowOverride `yaml:"workflows,omitempty"`
	Stale            StaleConfig                 `yaml:"stale,omitempty"`
}

// GetDefaultStatuses returns the four default task statuses.
func GetDefaultStatuses() []StatusDefinition {
	return []StatusDefinition{
		{
			Name:       "TODO",
			Label:      "To Do",
			IsTerminal: false,
			Color:      "yellow",
			Role:       "initial",
			TLSMarker:  " ",
		},
		{
			Name:       "IN_PROGRESS",
			Label:      "In Progress",
			IsTerminal: false,
			Color:      "blue",
			Role:       "active",
			TLSMarker:  ">",
		},
		{
			Name:       "DONE",
			Label:      "Done",
			IsTerminal: true,
			Color:      "green",
			Role:       "completed",
			TLSMarker:  "x",
		},
		{
			Name:       "SKIPPED",
			Label:      "Skipped",
			IsTerminal: true,
			Color:      "gray",
			Role:       "completed",
			TLSMarker:  "-",
		},
	}
}

// GetDefaultStateMachine returns the default workflow transition rules.
func GetDefaultStateMachine() *WorkflowDefinition {
	return &WorkflowDefinition{
		Rules: map[string][]string{
			"TODO":        {"IN_PROGRESS", "SKIPPED"},
			"IN_PROGRESS": {"DONE", "TODO", "SKIPPED"},
		},
	}
}

// Validate validates the task configuration.
func (t *TaskConfig) Validate() error {
	// Apply default stale timeout if not set
	if t.Stale.DefaultTimeout == 0 {
		t.Stale.DefaultTimeout = 6 * time.Hour
	}

	// If no statuses defined, populate with defaults
	if len(t.Statuses) == 0 {
		t.Statuses = GetDefaultStatuses()
	}
	if t.StateMachine == nil {
		t.StateMachine = GetDefaultStateMachine()
	}

	statusSet := make(map[string]bool, len(t.Statuses))
	markerSet := make(map[string]bool, len(t.Statuses))
	hasInitial := false
	hasActive := false

	for _, s := range t.Statuses {
		// Check duplicate status names
		if statusSet[s.Name] {
			return fmt.Errorf("duplicate status name: %s", s.Name)
		}
		statusSet[s.Name] = true

		// Check duplicate TLS markers
		if s.TLSMarker != "" {
			if markerSet[s.TLSMarker] {
				return fmt.Errorf("duplicate TLS marker %q on status %s", s.TLSMarker, s.Name)
			}
			markerSet[s.TLSMarker] = true
		}

		// Track roles
		switch s.Role {
		case "initial":
			hasInitial = true
		case "active":
			hasActive = true
		}
	}

	if !hasInitial {
		return fmt.Errorf("at least one status must have role \"initial\"")
	}
	if !hasActive {
		return fmt.Errorf("at least one status must have role \"active\"")
	}

	// Validate default_status exists and is non-terminal
	if t.DefaultStatus != "" {
		if !statusSet[t.DefaultStatus] {
			return fmt.Errorf("default_status %q does not match any defined status", t.DefaultStatus)
		}
		for _, s := range t.Statuses {
			if s.Name == t.DefaultStatus && s.IsTerminal {
				return fmt.Errorf("default_status %q must not be a terminal status", t.DefaultStatus)
			}
		}
	}

	// Validate state machine rules reference defined statuses
	if t.StateMachine != nil {
		for from, toList := range t.StateMachine.Rules {
			if !statusSet[from] {
				return fmt.Errorf("state machine rule references unknown status: %s", from)
			}
			for _, to := range toList {
				if !statusSet[to] {
					return fmt.Errorf("state machine rule references unknown target status: %s", to)
				}
			}
		}
	}

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
