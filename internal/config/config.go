package config

import (
	"fmt"
	"log"
	"maps"
	"net/url"
	"path/filepath"
	"slices"
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

// ListDefaults holds per-command default flag values for `list` commands
// (task list, track list). All fields optional; absence means "use the
// built-in default". Consumed by the runtime via viper keys
// <domain>.list.<field> and defaults.list.<field>.
type ListDefaults struct {
	Columns []string `yaml:"columns,omitempty"`
	Status  []string `yaml:"status,omitempty"`
	Limit   int      `yaml:"limit,omitempty"`
	SortBy  string   `yaml:"sort-by,omitempty"`
}

// DefaultsConfig holds cross-command fallback defaults (the `defaults`
// config namespace). `list` applies to any `* list` command not overridden
// by its own domain (task.list.* / tracks.list.*).
type DefaultsConfig struct {
	List *ListDefaults `yaml:"list,omitempty"`
}

// TrackHealthConfig holds thresholds for project-level health checks.
type TrackHealthConfig struct {
	MaxActive          int `yaml:"max_active"`
	MinProgressToStart int `yaml:"min_progress_to_start"`
}

// TrackConfig holds track-related configuration.
type TrackConfig struct {
	Dir            string            `yaml:"dir,omitempty"`
	StaleThreshold time.Duration     `yaml:"stale_threshold"`
	Health         TrackHealthConfig `yaml:"health"`
	PlanExtractor  string            `yaml:"plan_extractor,omitempty"`
	Types          []string          `yaml:"types,omitempty"`
	DefaultType    string            `yaml:"default_type,omitempty"`
	List           *ListDefaults     `yaml:"list,omitempty"`
}

// TracksDir returns the configured tracks directory or the default "tracks".
func (tc *TrackConfig) TracksDir() string {
	if tc.Dir != "" {
		return tc.Dir
	}
	return "tracks"
}

// validateRelativePath checks that a path is relative (not absolute) when set.
func validateRelativePath(field, value string) error {
	if value != "" && filepath.IsAbs(value) {
		return fmt.Errorf(
			"%s must be a relative path, got %q",
			field, value,
		)
	}
	return nil
}

// Validate validates the track configuration.
func (tc *TrackConfig) Validate() error {
	if err := validateRelativePath("tracks.dir", tc.Dir); err != nil {
		return err
	}
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
	if tc.DefaultType != "" {
		if len(tc.Types) > 0 {
			found := false
			for _, t := range tc.Types {
				if t == tc.DefaultType {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf(
					"tracks.default_type %q not in tracks.types",
					tc.DefaultType,
				)
			}
		}
	}
	return nil
}

// FlowConfig holds flow-related configuration.
type FlowConfig struct {
	Dir          string `yaml:"dir,omitempty"`
	AssigneesDir string `yaml:"assignees_dir,omitempty"`
}

// Validate validates the flow configuration.
func (fc *FlowConfig) Validate() error {
	if err := validateRelativePath("flow.dir", fc.Dir); err != nil {
		return err
	}
	if err := validateRelativePath("flow.assignees_dir", fc.AssigneesDir); err != nil {
		return err
	}
	return nil
}

// FlowsDir returns the configured flows directory or the default.
func (fc *FlowConfig) FlowsDir() string {
	if fc.Dir != "" {
		return fc.Dir
	}
	return filepath.Join("examples", "flows")
}

// AssigneesDirectory returns the configured assignees directory or the default.
func (fc *FlowConfig) AssigneesDirectory() string {
	if fc.AssigneesDir != "" {
		return fc.AssigneesDir
	}
	return filepath.Join("examples", "assignees")
}

type Config struct {
	Version    string            `yaml:"version"`
	Project    ProjectConfig     `yaml:"project"`
	Output     OutputConfig      `yaml:"output"`
	Task       TaskConfig        `yaml:"task"`
	Tracks     TrackConfig       `yaml:"tracks"`
	Defaults   *DefaultsConfig   `yaml:"defaults,omitempty"`
	Flow       FlowConfig        `yaml:"flow,omitempty"`
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
	if err := c.Flow.Validate(); err != nil {
		return err
	}
	if err := c.Storage.Inbox.Validate(); err != nil {
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
	case "table", "json", "yaml", "tls", "summary", "counters", "vtodo", "":
		return nil
	default:
		return fmt.Errorf("invalid output format: %s", o.Format)
	}
}

// StatusDefinition defines a single task status.
// Semantic status roles. A role names what a status MEANS to the
// lifecycle commands, so those commands never have to spell a status
// name: `claim` targets RoleActive, `complete` targets RoleCompleted,
// and so on. This is what lets a user rename their whole vocabulary
// without any command losing its target.
//
// Roles are advisory, not an enum: `role` may hold any string, and an
// unrecognised one simply indexes a role nothing asks for. Only
// RoleInitial and RoleActive are required (see TaskConfig.Validate).
//
// RoleSkipped distinguishes "finished, not done" from "finished, done".
// Both are terminal, so IsTerminal cannot tell them apart, and both
// historically carried RoleCompleted — which made the skip target
// unreachable by role, because the role index keeps the FIRST status
// declaring a role and DONE is declared first. Declaring the skip
// status separately is additive: configs that omit it keep working
// through the documented fallback in WorkflowManager.SkippedStatus.
const (
	RoleInitial   = "initial"
	RoleActive    = "active"
	RoleCompleted = "completed"
	RoleSkipped   = "skipped"
)

type StatusDefinition struct {
	Name        string `yaml:"name"`
	Label       string `yaml:"label"`
	Description string `yaml:"description,omitempty"`
	IsTerminal  bool   `yaml:"is_terminal"`
	Color       string `yaml:"color,omitempty"`
	Role        string `yaml:"role,omitempty"`       // see Role* constants
	TLSMarker   string `yaml:"tls_marker,omitempty"` // Single char for TLS format
}

// PriorityDefinition defines a single task priority.
//
// Declared as a LIST, and declaration order IS rank order, most urgent
// first — the same shape `task.statuses` uses, for the same reasons.
// There is deliberately no explicit `rank` field:
//
//   - A map keyed by name cannot express order at all. The decoder lands
//     YAML mappings in a Go map, whose iteration order is randomised, so
//     a map schema would have no order to read.
//   - A `rank` field alongside a list would be a SECOND source of truth
//     for the same fact. Two sources drift: a list whose declaration
//     order disagrees with its ranks has no defensible reading, and
//     validating them into agreement only re-derives the list order.
//
// Unlike StatusDefinition there are no roles. A role exists so a
// lifecycle command (`claim`, `complete`) can name what it MEANS rather
// than spell a status; no command targets "the urgent one", so a
// priority's only semantics are its name and its rank.
type PriorityDefinition struct {
	Name        string `yaml:"name"`
	Label       string `yaml:"label,omitempty"`
	Description string `yaml:"description,omitempty"`
	Color       string `yaml:"color,omitempty"`
}

// EffortDefinition defines a single task effort size.
//
// Same shape and same reasoning as PriorityDefinition: a LIST whose
// declaration order IS rank order, smallest first, with no separate
// `rank` field. A map cannot express order (the decoder randomises it),
// and a rank field beside a list is a second source of truth for one
// fact.
//
// The only axis difference is which end of the list is "first". Priority
// declares most urgent first; effort declares smallest first, matching
// the built-in XS, S, M, L, XL and the ascending reading of "sort by
// size". Both are simply "index 0 is rank 0"; what rank 0 MEANS is the
// axis's own business.
//
// Effort ranks matter more than priority's did. With P0..P3 rank order
// and alphabetical order coincide by accident, which is why nothing
// noticed the text sort; XS, S, M, L, XL does not — it sorts to L, M, S,
// XL, XS as text, wrong even for the built-in vocabulary.
type EffortDefinition struct {
	Name        string `yaml:"name"`
	Label       string `yaml:"label,omitempty"`
	Description string `yaml:"description,omitempty"`
	Color       string `yaml:"color,omitempty"`
}

// TagPolicy names how `task.tags.allowed` is enforced.
//
// Two values only, and the pair is deliberately not three: there is no
// "warn" mode. A tag that is warned about is still written, so the next
// reader of the store sees a vocabulary that the config says does not
// exist — which is the drift a policy exists to prevent, arriving one
// warning later.
type TagPolicy string

const (
	// TagPolicyOpen accepts any tag. The default, and the whole reason
	// this key can be added without breaking a single existing project:
	// a config that says nothing about tags behaves exactly as it did
	// before the key existed.
	TagPolicyOpen TagPolicy = "open"

	// TagPolicyClosed accepts only tags the vocabulary admits.
	TagPolicyClosed TagPolicy = "closed"
)

// TagsConfig is the `task.tags` section: the tag vocabulary and how
// strictly it is enforced.
//
// Allowed is ADDITIVE to the axes tlc already generates — see
// core.TagVocabulary. A closed policy that made the user restate
// `type:feat`, every `priority:*` and every `status:*` before they could
// use them would be restating vocabularies that already exist in this
// same config file, and would silently rot the moment they renamed a
// priority. So the generated axes are admitted by construction and
// `allowed` says only what is project-specific.
type TagsConfig struct {
	// Policy is "open" (default) or "closed". Empty means open.
	Policy TagPolicy `yaml:"policy,omitempty"`

	// Allowed lists the project-specific tags a closed policy admits, on
	// top of the generated axes.
	//
	// An entry ending in `:*` admits the whole prefix — `domain:*` admits
	// `domain:storage` and any other `domain:` tag. The wildcard is
	// deliberately limited to that one shape: it is anchored to a
	// dimension prefix, so it can widen a namespace but can never widen
	// to everything, which a free `*` or a regex could. An open-ended
	// axis like `domain:` is the case that makes a literal-only list
	// unusable in practice — nobody can enumerate their domains up front
	// — while an axis the project genuinely wants closed is still closed
	// by listing its members literally. Both guarantees are available;
	// which one applies is per-prefix and visible in the config.
	Allowed []string `yaml:"allowed,omitempty"`
}

// Effective returns the policy this section implies: the declared value,
// or open when none is declared.
func (t TagsConfig) Effective() TagPolicy {
	if t.Policy == "" {
		return TagPolicyOpen
	}
	return t.Policy
}

// Validate rejects a policy value outside the known set.
//
// On the FATAL ValidateWorkflow path rather than the advisory one, for
// the same reason ValidateEfforts is: this value decides what the write
// gate accepts. A typo'd `policy: colsed` that only warned would fall
// back to open and silently accept every tag the user believed they had
// closed off — a policy that reports itself as configured while
// enforcing nothing is worse than no policy at all.
func (t TagsConfig) Validate() error {
	switch t.Policy {
	case "", TagPolicyOpen, TagPolicyClosed:
	default:
		return fmt.Errorf(
			"task.tags.policy %q is not a known policy: must be one of %s, %s",
			t.Policy, TagPolicyOpen, TagPolicyClosed,
		)
	}
	for i, a := range t.Allowed {
		if strings.TrimSpace(a) == "" {
			return fmt.Errorf("task.tags.allowed[%d]: tag must not be empty", i)
		}
	}
	return nil
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

// PriorityScheduleRule defines auto-due and auto-remind for a priority.
type PriorityScheduleRule struct {
	Due   time.Duration `yaml:"due"`
	RRule string        `yaml:"rrule"`
}

// AgeNudgeRule defines age-based reminders for tasks without explicit due.
type AgeNudgeRule struct {
	Status    string        `yaml:"status"`
	Threshold time.Duration `yaml:"threshold"`
	Action    string        `yaml:"action"` // "remind" or "escalate"
}

// SchedulingConfig holds auto-scheduling rules by priority and age.
type SchedulingConfig struct {
	ByPriority map[string]PriorityScheduleRule `yaml:"by_priority,omitempty"`
	AgeNudges  []AgeNudgeRule                  `yaml:"age_nudges,omitempty"`
}

// PriorityDerivationRule is one "if this, then that priority" rule.
//
// Declared as a LIST, and declaration order IS precedence order, first
// match wins — the same shape and the same reasoning as
// `task.priorities` and `task.statuses`. A map keyed by rule name cannot
// express order at all: the decoder lands YAML mappings in a Go map with
// randomised iteration, which is exactly the trap the per-tag workflow
// override hit. Order here is the user's own, readable in their config,
// and identical on every run.
//
// Every rule MUST be named. The name is not decoration: it is what a
// `--explain` line cites, what a duplicate-name error can point at, and
// what makes a rule set reviewable. An unnamed rule is unciteable.
//
// Conditions are ANDed. A rule declaring both `due_within` and
// `min_dependents` fires only when both hold. There is deliberately no
// OR: two conditions that should each suffice are two rules, which reads
// the same and keeps precedence explicit.
//
// A rule with no condition at all is rejected rather than treated as a
// catch-all. A catch-all is spellable — `due_within: 87600h` or
// `min_age: 0s` — and an accidentally empty rule (a typo'd key name that
// decoded to nothing) would otherwise silently claim every task.
type PriorityDerivationRule struct {
	// Name identifies the rule. Required and unique within the set.
	Name string `yaml:"name"`

	// Then is the priority the rule assigns. Must be in the effective
	// priority vocabulary.
	Then string `yaml:"then"`

	// DueWithin fires when the task has a due date no further out than
	// this. An overdue task (due in the past) satisfies every DueWithin.
	// A task with no due date satisfies none.
	DueWithin time.Duration `yaml:"due_within,omitempty"`

	// MinAge fires when the task was created at least this long ago.
	MinAge time.Duration `yaml:"min_age,omitempty"`

	// MinDependents fires when at least this many other tasks declare
	// this one in their `blocked_by`. A task blocking many others is
	// worth more than one blocking none.
	MinDependents int `yaml:"min_dependents,omitempty"`

	// Tag fires when the task carries this tag.
	Tag string `yaml:"tag,omitempty"`

	// Always makes the rule a catch-all, matching every task.
	//
	// A catch-all is a legitimate last rule ("everything else is P3"),
	// but it must be SAID. `min_age: 0s` cannot say it: a zero duration
	// is what an unset field decodes to, so a rule whose only condition
	// key was mistyped would be indistinguishable from a deliberate
	// catch-all and would silently claim every task. This field is the
	// difference between the two.
	//
	// Combining Always with another condition is rejected: the other
	// condition would be dead text.
	Always bool `yaml:"always,omitempty"`
}

// PriorityDerivationConfig holds config-driven priority derivation.
//
// Off unless `rules` is non-empty: with no rules declared, every
// derivation entry point is a no-op and behaviour is byte-identical to a
// build without this feature.
type PriorityDerivationConfig struct {
	// Rules are evaluated in declaration order; the first whose
	// conditions all hold supplies the priority.
	Rules []PriorityDerivationRule `yaml:"rules,omitempty"`

	// OnCreate derives a priority for a task created without one.
	//
	// Default false. Deriving at create time is the least surprising
	// moment there is — the task has no priority for the derivation to
	// contradict, and the user sees the result in the create output — but
	// it is still a value they did not type, so they opt in.
	OnCreate bool `yaml:"on_create,omitempty"`

	// IncludeActive lets derivation touch tasks sitting in an
	// `active`-role status.
	//
	// Default false, which is the mid-flight guard: re-prioritising the
	// task someone is working on right now moves it in every list they
	// have open, for a reason they did not act on. Excluded tasks are
	// reported as skipped rather than silently passed over.
	IncludeActive bool `yaml:"include_active,omitempty"`
}

// TaskConfig contains task-related configuration.
type TaskConfig struct {
	DefaultStatus    string                      `yaml:"default_status"`
	AutoAssign       bool                        `yaml:"auto_assign"`
	RequireReference bool                        `yaml:"require_reference"`
	TodoFile         string                      `yaml:"todo_file"`
	ProjectionDir    string                      `yaml:"projection_dir,omitempty"`
	ArchiveThreshold time.Duration               `yaml:"archive_threshold"`
	Statuses         []StatusDefinition          `yaml:"statuses,omitempty"`
	Priorities       []PriorityDefinition        `yaml:"priorities,omitempty"`
	Efforts          []EffortDefinition          `yaml:"efforts,omitempty"`
	Tags             TagsConfig                  `yaml:"tags,omitempty"`
	StateMachine     *WorkflowDefinition         `yaml:"state_machine,omitempty"`
	Workflows        map[string]WorkflowOverride `yaml:"workflows,omitempty"`
	Stale            StaleConfig                 `yaml:"stale,omitempty"`
	Scheduling       SchedulingConfig            `yaml:"scheduling,omitempty"`
	PriorityDeriv    PriorityDerivationConfig    `yaml:"priority_derivation,omitempty"`
	List             *ListDefaults               `yaml:"list,omitempty"`
}

// ProjectionDirectory returns the configured projection directory or "tasks".
func (t *TaskConfig) ProjectionDirectory() string {
	if t.ProjectionDir != "" {
		return t.ProjectionDir
	}
	return "tasks"
}

// TodoFilePath returns the configured todo file path or the default "todo.txt".
func (t *TaskConfig) TodoFilePath() string {
	if t.TodoFile != "" {
		return t.TodoFile
	}
	return "todo.txt"
}

// GetDefaultStatuses returns the four default task statuses.
func GetDefaultStatuses() []StatusDefinition {
	return []StatusDefinition{
		{
			Name:       "TODO",
			Label:      "To Do",
			IsTerminal: false,
			Color:      "yellow",
			Role:       RoleInitial,
			TLSMarker:  " ",
		},
		{
			Name:       "IN_PROGRESS",
			Label:      "In Progress",
			IsTerminal: false,
			Color:      "blue",
			Role:       RoleActive,
			TLSMarker:  ">",
		},
		{
			Name:       "DONE",
			Label:      "Done",
			IsTerminal: true,
			Color:      "green",
			Role:       RoleCompleted,
			TLSMarker:  "x",
		},
		{
			Name:       "SKIPPED",
			Label:      "Skipped",
			IsTerminal: true,
			Color:      "gray",
			Role:       RoleSkipped,
			TLSMarker:  "-",
		},
	}
}

// GetDefaultPriorities returns the four built-in priorities in
// descending urgency order — the fallback when config declares none.
//
// Unlike GetDefaultStatuses this is NOT written back into TaskConfig by
// Validate. Statuses are mandatory (the workflow engine needs a
// vocabulary to enforce), so an empty list there is filled in. Priority
// is optional end to end, and materialising the built-ins into the
// struct would make "declared no priorities" indistinguishable from
// "declared exactly the built-in four" — which is precisely the
// distinction the by_priority check and the enum restamp read.
func GetDefaultPriorities() []PriorityDefinition {
	return []PriorityDefinition{
		{Name: "P0", Label: "Critical", Color: "red"},
		{Name: "P1", Label: "High", Color: "yellow"},
		{Name: "P2", Label: "Medium", Color: "blue"},
		{Name: "P3", Label: "Low", Color: "gray"},
	}
}

// GetDefaultEfforts returns the five built-in effort sizes in ascending
// order — the fallback when config declares none.
//
// Not materialised into TaskConfig by Validate, for the same reason
// GetDefaultPriorities is not: effort is optional end to end, and
// filling the struct in would make "declared no efforts" and "declared
// exactly the built-in five" indistinguishable — the distinction the
// enum restamp's built-in short-circuit reads.
//
// The colours are the swatches the label axis has always emitted for
// these five, moved here so the vocabulary and its presentation are
// declared in one place rather than re-listed beside the axis.
func GetDefaultEfforts() []EffortDefinition {
	return []EffortDefinition{
		{Name: "XS", Label: "Extra small", Description: "XS — extra small", Color: "C2E0C6"},
		{Name: "S", Label: "Small", Description: "S — small", Color: "9EDAB0"},
		{Name: "M", Label: "Medium", Description: "M — medium", Color: "7BC99B"},
		{Name: "L", Label: "Large", Description: "L — large", Color: "4FA97F"},
		{Name: "XL", Label: "Extra large", Description: "XL — extra large", Color: "2E8B62"},
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

// Validate validates the task configuration: everything ValidateWorkflow
// covers, plus the checks that are advisory rather than structural.
//
// Split from ValidateWorkflow because the two have different
// consequences. This one runs on the CLI's warning-only config path;
// ValidateWorkflow additionally runs on DefaultWorkflow's FATAL path, so
// anything added here that does not genuinely make the workflow
// unbuildable would turn a cosmetic config mistake into a tool that
// refuses to start.
func (t *TaskConfig) Validate() error {
	if err := t.ValidateWorkflow(); err != nil {
		return err
	}
	if err := t.validateSchedulingKeys(); err != nil {
		return err
	}
	// Derivation rules ride the advisory path, not the fatal one: a
	// broken rule set must not stop every command in the tool. The
	// derivation entry points re-run this check and refuse outright, so
	// a rule set reported here is never half-applied there.
	return t.ValidatePriorityDerivation()
}

// ValidateWorkflow validates the parts of the task configuration the
// workflow engine and the CLI vocabularies are built from: the statuses,
// their roles and markers, the declared priorities, and the transition
// rules. A failure here means no coherent workflow can be constructed.
func (t *TaskConfig) ValidateWorkflow() error {
	if err := validateRelativePath("task.projection_dir", t.ProjectionDir); err != nil {
		return err
	}

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
		case RoleInitial:
			hasInitial = true
		case RoleActive:
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

	if err := t.ValidatePriorities(); err != nil {
		return err
	}

	if err := t.ValidateEfforts(); err != nil {
		return err
	}

	if err := t.Tags.Validate(); err != nil {
		return err
	}

	// Validate state machine rules reference defined statuses
	if err := validateRules(t.StateMachine, statusSet, ""); err != nil {
		return err
	}

	// Per-tag overrides get the same check. They share the base
	// vocabulary, so a rule naming an undeclared status is the same
	// error here — and one the workflow engine would otherwise accept
	// silently, then refuse every transition for that tag.
	for _, tag := range slices.Sorted(maps.Keys(t.Workflows)) {
		if err := validateRules(t.Workflows[tag].StateMachine, statusSet, tag); err != nil {
			return err
		}
	}

	return nil
}

// EffectivePriorities returns the priority vocabulary this config
// implies: the declared list when non-empty, else the built-in four.
// One helper so the CLI, the validator and the scheduling lookup cannot
// disagree about what "the vocabulary" is.
func (t *TaskConfig) EffectivePriorities() []PriorityDefinition {
	if len(t.Priorities) == 0 {
		return GetDefaultPriorities()
	}
	return t.Priorities
}

// EffectiveEfforts returns the effort vocabulary this config implies:
// the declared list when non-empty, else the built-in five.
// EffectivePriorities for the effort axis.
func (t *TaskConfig) EffectiveEfforts() []EffortDefinition {
	if len(t.Efforts) == 0 {
		return GetDefaultEfforts()
	}
	return t.Efforts
}

// ValidatePriorities checks the declared priority vocabulary itself:
// every definition names something, and no name is declared twice.
//
// Exported and separate from validateSchedulingKeys because the two have
// different blast radii. This one gates the vocabulary the workflow and
// the CLI both read, so it belongs on the fatal DefaultWorkflow path
// alongside the status checks. The scheduling-key check does not.
func (t *TaskConfig) ValidatePriorities() error {
	seen := make(map[string]bool, len(t.Priorities))
	for _, p := range t.Priorities {
		if p.Name == "" {
			return fmt.Errorf("priority definition must have a name")
		}
		if seen[p.Name] {
			return fmt.Errorf("duplicate priority name: %s", p.Name)
		}
		seen[p.Name] = true
	}
	return nil
}

// ValidateEfforts checks the declared effort vocabulary itself: every
// definition names something, and no name is declared twice.
//
// ValidatePriorities for the effort axis, and on the same fatal path for
// the same reason: this vocabulary is what the CLI's write gate accepts,
// so a config the CLI cannot derive a coherent vocabulary from is a
// config that would silently reject every effort the user writes.
func (t *TaskConfig) ValidateEfforts() error {
	seen := make(map[string]bool, len(t.Efforts))
	for _, e := range t.Efforts {
		if e.Name == "" {
			return fmt.Errorf("effort definition must have a name")
		}
		if seen[e.Name] {
			return fmt.Errorf("duplicate effort name: %s", e.Name)
		}
		seen[e.Name] = true
	}
	return nil
}

// ValidatePriorityDerivation checks the declared derivation rule set:
// every rule is named, named once, states a priority in the effective
// vocabulary, and declares at least one condition. It also refuses two
// rules whose conditions are IDENTICAL, since which of them "wins" would
// then be an artefact of the order they happened to be typed in rather
// than a decision.
//
// Deliberately NOT called from DefaultWorkflowE. A broken derivation rule
// set must not stop `task list` from running — the vocabulary gates every
// write, derivation gates only the command that asks for it. The
// derivation entry points call this themselves and refuse to run, and the
// CLI's warning-only validation path reports it everywhere else. That is
// the "reported, not silently resolved" contract: the rule set never
// half-applies.
//
// Returns nil when no rules are declared: derivation off is not an error.
func (t *TaskConfig) ValidatePriorityDerivation() error {
	rules := t.PriorityDeriv.Rules
	if len(rules) == 0 {
		return nil
	}

	valid := make(map[string]bool)
	names := make([]string, 0, len(t.EffectivePriorities()))
	for _, p := range t.EffectivePriorities() {
		valid[p.Name] = true
		names = append(names, p.Name)
	}

	seenName := make(map[string]int, len(rules))
	seenCond := make(map[string]string, len(rules))
	for i, r := range rules {
		if r.Name == "" {
			return fmt.Errorf(
				"task.priority_derivation.rules[%d]: rule must have a name; "+
					"the name is what an explain line and a duplicate-rule "+
					"error cite", i)
		}
		if prev, dup := seenName[r.Name]; dup {
			return fmt.Errorf(
				"task.priority_derivation.rules: duplicate rule name %q "+
					"(indices %d and %d); rule names must be unique so a "+
					"rule can be cited unambiguously", r.Name, prev, i)
		}
		seenName[r.Name] = i

		if r.Then == "" {
			return fmt.Errorf(
				"task.priority_derivation.rules[%q]: `then` must name the "+
					"priority to assign (one of: %s)",
				r.Name, strings.Join(names, ", "))
		}
		if !valid[r.Then] {
			return fmt.Errorf(
				"task.priority_derivation.rules[%q]: `then: %s` is not in "+
					"the priority vocabulary (%s)",
				r.Name, r.Then, strings.Join(names, ", "))
		}

		cond := r.conditionKey()
		if cond == "" {
			return fmt.Errorf(
				"task.priority_derivation.rules[%q]: rule declares no "+
					"condition; a catch-all must be spelled explicitly as "+
					"`always: true` so a mistyped condition key cannot "+
					"silently claim every task", r.Name)
		}
		if r.Always && cond != "always" {
			return fmt.Errorf(
				"task.priority_derivation.rules[%q]: `always: true` cannot "+
					"be combined with another condition (%s); the other "+
					"condition would never be read", r.Name, cond)
		}
		if prev, clash := seenCond[cond]; clash {
			return fmt.Errorf(
				"task.priority_derivation.rules: rules %q and %q declare "+
					"identical conditions; which one applies would depend on "+
					"the order they were typed rather than on a decision — "+
					"merge them, or make their conditions differ",
				prev, r.Name)
		}
		seenCond[cond] = r.Name
	}

	return nil
}

// conditionKey renders a rule's conditions as a canonical string, so two
// rules that test exactly the same thing compare equal.
//
// Field-by-field rather than by reflection or by hashing the struct:
// Name and Then are deliberately excluded (two rules may legitimately
// assign the same priority), and a condition field added later should
// force a deliberate decision here rather than silently defaulting into
// or out of the comparison.
func (r PriorityDerivationRule) conditionKey() string {
	var parts []string
	if r.Always {
		parts = append(parts, "always")
	}
	if r.DueWithin != 0 {
		parts = append(parts, "due_within="+r.DueWithin.String())
	}
	if r.MinAge != 0 {
		parts = append(parts, "min_age="+r.MinAge.String())
	}
	if r.MinDependents != 0 {
		parts = append(parts, fmt.Sprintf("min_dependents=%d", r.MinDependents))
	}
	if r.Tag != "" {
		parts = append(parts, "tag="+r.Tag)
	}
	return strings.Join(parts, ",")
}

// validateSchedulingKeys rejects a `task.scheduling.by_priority` entry
// naming a priority outside the vocabulary.
//
// It exists because such an entry is otherwise SILENT.
// SchedulingConfig.ByPriority is a map[string]... — typed open, but
// effectively closed, because the lookup happens with a priority that has
// already been validated against the vocabulary. An entry naming anything
// else is unreachable: it never matches, never errors, and never fires. A
// user who renames their vocabulary and forgets to rename the scheduling
// keys gets silence rather than a diagnosis.
//
// Deliberately NOT called from DefaultWorkflowE, which treats a Validate
// failure as fatal. Auto-scheduling defaults are an optional convenience;
// a stale key in them is a reason to warn, not a reason to refuse to run
// every command in the tool. It runs on the warning-only config
// validation path in the CLI instead — which is where the user sees it,
// and where the status vocabulary's own soft checks already live.
func (t *TaskConfig) validateSchedulingKeys() error {
	effective := t.EffectivePriorities()
	prioritySet := make(map[string]bool, len(effective))
	names := make([]string, 0, len(effective))
	for _, p := range effective {
		prioritySet[p.Name] = true
		names = append(names, p.Name)
	}

	// Sorted iteration so a config with several bad keys always reports
	// the same one; Go map range order is randomised.
	for _, key := range slices.Sorted(maps.Keys(t.Scheduling.ByPriority)) {
		if !prioritySet[key] {
			return fmt.Errorf(
				"task.scheduling.by_priority references unknown priority %q; valid values: %s",
				key, strings.Join(names, ", "),
			)
		}
	}
	return nil
}

// validateRules checks one rule set against the declared status names.
// tag is the workflow override the rules came from, empty for the base
// state machine, and only shapes the error message.
func validateRules(def *WorkflowDefinition, statusSet map[string]bool, tag string) error {
	if def == nil {
		return nil
	}
	where := "state machine"
	if tag != "" {
		where = fmt.Sprintf("workflow override for tag %q", tag)
	}
	for _, from := range slices.Sorted(maps.Keys(def.Rules)) {
		if !statusSet[from] {
			return fmt.Errorf("%s rule references unknown status: %s", where, from)
		}
		for _, to := range def.Rules[from] {
			if !statusSet[to] {
				return fmt.Errorf("%s rule references unknown target status: %s", where, to)
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

// InboxConfig controls inbox-based task creation and transitions.
type InboxConfig struct {
	Dir         string `yaml:"dir,omitempty"`
	AutoProcess bool   `yaml:"auto_process"`
}

// Validate validates the inbox configuration.
func (ic *InboxConfig) Validate() error {
	return validateRelativePath("storage.inbox.dir", ic.Dir)
}

// InboxDir returns the configured inbox directory or the default "inbox".
func (ic *InboxConfig) InboxDir() string {
	if ic.Dir != "" {
		return ic.Dir
	}
	return "inbox"
}

// StorageConfig contains storage configuration.
type StorageConfig struct {
	Backend          string           `yaml:"backend"`
	DBPath           string           `yaml:"db_path"`
	ConnectionString string           `yaml:"connection_string"`
	Filesystem       FilesystemConfig `yaml:"filesystem"`
	Inbox            InboxConfig      `yaml:"inbox"`
}

// DBFilePath returns the configured database path or the default "db.sqlite".
func (s *StorageConfig) DBFilePath() string {
	if s.DBPath != "" {
		return s.DBPath
	}
	return "db.sqlite"
}

// FilesystemConfig controls filesystem projection of tasks.
// Accepts bool or object form in YAML:
//   - filesystem: true  → defaults (group_by: [status], sort_by: [id])
//   - filesystem: false → disabled
//   - filesystem: { group_by: [status, tag], sort_by: [priority, id] }
type FilesystemConfig struct {
	Enabled bool     `yaml:"-"`
	GroupBy []string `yaml:"-"`
	SortBy  []string `yaml:"-"`
}

// allowedGroupBy lists valid group_by field names.
var allowedGroupBy = map[string]bool{
	"status":      true,
	"tag":         true,
	"priority":    true,
	"effort":      true,
	"assigned_to": true,
	"track":       true,
}

// allowedSortBy lists valid sort_by field names.
var allowedSortBy = map[string]bool{
	"id":         true,
	"priority":   true,
	"effort":     true,
	"created_at": true,
	"updated_at": true,
	"title":      true,
}

// UnmarshalYAML handles bool or object form for FilesystemConfig.
func (f *FilesystemConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try bool first
	var boolVal bool
	if err := unmarshal(&boolVal); err == nil {
		f.Enabled = boolVal
		if boolVal {
			f.GroupBy = []string{"status"}
			f.SortBy = []string{"id"}
		}
		return nil
	}

	// Try object form
	type rawFilesystem struct {
		GroupBy []string `yaml:"group_by"`
		SortBy  []string `yaml:"sort_by"`
	}
	var raw rawFilesystem
	if err := unmarshal(&raw); err != nil {
		return fmt.Errorf("filesystem: must be bool or object with group_by/sort_by")
	}

	f.Enabled = true
	f.GroupBy = raw.GroupBy
	f.SortBy = raw.SortBy

	// Apply defaults for missing fields
	if len(f.GroupBy) == 0 {
		f.GroupBy = []string{"status"}
	}
	if len(f.SortBy) == 0 {
		f.SortBy = []string{"id"}
	}

	return nil
}

// Validate validates the storage configuration.
func (s *StorageConfig) Validate() error {
	switch s.Backend {
	case "sqlite", "local", "postgres", "":
	default:
		return fmt.Errorf("invalid storage backend: %s", s.Backend)
	}

	if s.Filesystem.Enabled {
		for _, g := range s.Filesystem.GroupBy {
			if !allowedGroupBy[g] {
				return fmt.Errorf(
					"storage.filesystem.group_by: invalid field %q; allowed: status, tag, priority, effort, assigned_to, track",
					g,
				)
			}
		}
		for _, sb := range s.Filesystem.SortBy {
			if !allowedSortBy[sb] {
				return fmt.Errorf(
					"storage.filesystem.sort_by: invalid field %q; allowed: id, priority, effort, created_at, updated_at, title",
					sb,
				)
			}
		}
	}

	return nil
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
