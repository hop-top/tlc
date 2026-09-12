package config

import (
	"path/filepath"
	"strings"
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
			name: testInvalid + " github sync_direction",
			setup: func(c *Config) {
				c.Sync.GitHub.SyncDirection = testInvalid
			},
			wantErr: true,
		},
		{
			name: "valid github sync_direction",
			setup: func(c *Config) {
				c.Sync.GitHub.SyncDirection = "bidirectional"
			},
			wantErr: false,
		},
		{
			name: "empty github sync_direction",
			setup: func(c *Config) {
				c.Sync.GitHub.SyncDirection = ""
			},
			wantErr: false,
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

// -- Configurable path fields -------------------------------------------------

func TestTrackConfig_TracksDir_Default(t *testing.T) {
	tc := &TrackConfig{}
	if got := tc.TracksDir(); got != "tracks" {
		t.Errorf("TracksDir() = %q, want %q", got, "tracks")
	}
}

func TestTrackConfig_TracksDir_Custom(t *testing.T) {
	tc := &TrackConfig{Dir: "my-tracks"}
	if got := tc.TracksDir(); got != "my-tracks" {
		t.Errorf("TracksDir() = %q, want %q", got, "my-tracks")
	}
}

func TestFlowConfig_FlowsDir_Default(t *testing.T) {
	fc := &FlowConfig{}
	want := filepath.Join("examples", "flows")
	if got := fc.FlowsDir(); got != want {
		t.Errorf("FlowsDir() = %q, want %q", got, want)
	}
}

func TestFlowConfig_FlowsDir_Custom(t *testing.T) {
	fc := &FlowConfig{Dir: "flows"}
	if got := fc.FlowsDir(); got != "flows" {
		t.Errorf("FlowsDir() = %q, want %q", got, "flows")
	}
}

func TestFlowConfig_AssigneesDirectory_Default(t *testing.T) {
	fc := &FlowConfig{}
	want := filepath.Join("examples", "assignees")
	if got := fc.AssigneesDirectory(); got != want {
		t.Errorf("AssigneesDirectory() = %q, want %q", got, want)
	}
}

func TestFlowConfig_AssigneesDirectory_Custom(t *testing.T) {
	fc := &FlowConfig{AssigneesDir: "people"}
	if got := fc.AssigneesDirectory(); got != "people" {
		t.Errorf("AssigneesDirectory() = %q, want %q", got, "people")
	}
}

func TestInboxConfig_InboxDir_Default(t *testing.T) {
	ic := &InboxConfig{}
	if got := ic.InboxDir(); got != "inbox" {
		t.Errorf("InboxDir() = %q, want %q", got, "inbox")
	}
}

func TestInboxConfig_InboxDir_Custom(t *testing.T) {
	ic := &InboxConfig{Dir: "my-inbox"}
	if got := ic.InboxDir(); got != "my-inbox" {
		t.Errorf("InboxDir() = %q, want %q", got, "my-inbox")
	}
}

func TestTaskConfig_ProjectionDirectory_Default(t *testing.T) {
	tc := &TaskConfig{}
	if got := tc.ProjectionDirectory(); got != "tasks" {
		t.Errorf("ProjectionDirectory() = %q, want %q", got, "tasks")
	}
}

func TestTaskConfig_ProjectionDirectory_Custom(t *testing.T) {
	tc := &TaskConfig{ProjectionDir: "my-tasks"}
	if got := tc.ProjectionDirectory(); got != "my-tasks" {
		t.Errorf("ProjectionDirectory() = %q, want %q", got, "my-tasks")
	}
}

func TestTaskConfig_TodoFilePath_Default(t *testing.T) {
	tc := &TaskConfig{}
	if got := tc.TodoFilePath(); got != "todo.txt" {
		t.Errorf("TodoFilePath() = %q, want %q", got, "todo.txt")
	}
}

func TestTaskConfig_TodoFilePath_Custom(t *testing.T) {
	tc := &TaskConfig{TodoFile: "my-todo.txt"}
	if got := tc.TodoFilePath(); got != "my-todo.txt" {
		t.Errorf("TodoFilePath() = %q, want %q", got, "my-todo.txt")
	}
}

func TestStorageConfig_DBFilePath_Default(t *testing.T) {
	sc := &StorageConfig{}
	if got := sc.DBFilePath(); got != "db.sqlite" {
		t.Errorf("DBFilePath() = %q, want %q", got, "db.sqlite")
	}
}

func TestStorageConfig_DBFilePath_Custom(t *testing.T) {
	sc := &StorageConfig{DBPath: "data/main.db"}
	if got := sc.DBFilePath(); got != "data/main.db" {
		t.Errorf("DBFilePath() = %q, want %q", got, "data/main.db")
	}
}

func TestValidateRelativePath_RejectsAbsolute(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Config)
	}{
		{"tracks.dir absolute", func(c *Config) { c.Tracks.Dir = "/abs/tracks" }},
		{"flow.dir absolute", func(c *Config) { c.Flow.Dir = "/abs/flows" }},
		{"flow.assignees_dir absolute", func(c *Config) { c.Flow.AssigneesDir = "/abs/people" }},
		{"storage.inbox.dir absolute", func(c *Config) { c.Storage.Inbox.Dir = "/abs/inbox" }},
		{"task.projection_dir absolute", func(c *Config) { c.Task.ProjectionDir = "/abs/tasks" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.setup(cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("expected error for absolute path, got nil")
			}
		})
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

// TestTaskConfig_PriorityVocabularyValidation covers the priority
// vocabulary's own structural checks — the ones that are fatal because
// DefaultWorkflow builds on them.
func TestTaskConfig_PriorityVocabularyValidation(t *testing.T) {
	tests := []struct {
		name       string
		priorities []PriorityDefinition
		wantErr    bool
	}{
		{"empty falls back to built-ins", nil, false},
		{
			"custom vocabulary accepted",
			[]PriorityDefinition{{Name: "URGENT"}, {Name: "LATER"}},
			false,
		},
		{
			"duplicate name rejected",
			[]PriorityDefinition{{Name: "URGENT"}, {Name: "URGENT"}},
			true,
		},
		{
			"unnamed definition rejected",
			[]PriorityDefinition{{Name: "URGENT"}, {Label: "no name"}},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Task.Priorities = tt.priorities
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestTaskConfig_EmptyPrioritiesNotMaterialised pins the deliberate
// asymmetry with statuses: Validate fills in default STATUSES but must
// leave Priorities empty, so "declared none" stays distinguishable from
// "declared exactly the built-in four".
func TestTaskConfig_EmptyPrioritiesNotMaterialised(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.Priorities = nil
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Task.Priorities) != 0 {
		t.Errorf("Validate materialized %d priorities; it must leave the list empty",
			len(cfg.Task.Priorities))
	}
	if got := len(cfg.Task.EffectivePriorities()); got != 4 {
		t.Errorf("EffectivePriorities() = %d entries, want the 4 built-ins", got)
	}
}

// TestTaskConfig_SchedulingKeyOutsideVocabulary covers the silent no-op:
// a by_priority key naming a priority outside the vocabulary is
// unreachable at runtime and must be reported rather than ignored.
func TestTaskConfig_SchedulingKeyOutsideVocabulary(t *testing.T) {
	tests := []struct {
		name       string
		priorities []PriorityDefinition
		key        string
		wantErr    bool
	}{
		{"built-in key under built-in vocabulary", nil, "P0", false},
		{"unknown key under built-in vocabulary", nil, "NOSUCH", true},
		{
			"declared key under custom vocabulary",
			[]PriorityDefinition{{Name: "URGENT"}, {Name: "LATER"}},
			"URGENT", false,
		},
		{
			"built-in key under custom vocabulary is now unknown",
			[]PriorityDefinition{{Name: "URGENT"}, {Name: "LATER"}},
			"P0", true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Task.Priorities = tt.priorities
			cfg.Task.Scheduling.ByPriority = map[string]PriorityScheduleRule{
				tt.key: {Due: time.Hour},
			}
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestTaskConfig_ValidateWorkflowSkipsSchedulingKeys pins the split that
// keeps a stale scheduling key off DefaultWorkflow's FATAL path. Validate
// must reject it; ValidateWorkflow must not, or a cosmetic config mistake
// would stop every command in the tool from running.
func TestTaskConfig_ValidateWorkflowSkipsSchedulingKeys(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.Scheduling.ByPriority = map[string]PriorityScheduleRule{
		"NOSUCH": {Due: time.Hour},
	}
	if err := cfg.Task.ValidateWorkflow(); err != nil {
		t.Errorf("ValidateWorkflow must ignore scheduling keys, got: %v", err)
	}
	if err := cfg.Task.Validate(); err == nil {
		t.Error("Validate must reject an out-of-vocabulary scheduling key")
	}
}

// A rule whose FROM status is terminal can never fire: ValidateTransition
// refuses every transition out of a terminal status before it ever
// consults the rules, and `tlc task reopen` is the sanctioned way out.
// Accepting such a rule silently left the user with a rule they wrote,
// config validation blessed, and nothing honored. Reject it instead, so
// the config fails loudly and names the status.
func TestTaskConfig_StateMachineRuleFromTerminalStatusRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.StateMachine.Rules["DONE"] = []string{"TODO"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for state machine rule out of terminal status DONE, got nil")
	}
	if !strings.Contains(err.Error(), "DONE") {
		t.Errorf("error must name the offending status, got: %v", err)
	}
	if !strings.Contains(err.Error(), "reopen") {
		t.Errorf("error must point at the sanctioned way out, got: %v", err)
	}
}

// The per-tag overrides get the same check. This is the case that
// motivated it: `workflows.reopenable.state_machine.rules.DONE` was
// accepted here and inert at runtime.
func TestTaskConfig_WorkflowOverrideRuleFromTerminalStatusRejected(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.Workflows = map[string]WorkflowOverride{
		"reopenable": {
			StateMachine: &WorkflowDefinition{
				Rules: map[string][]string{"DONE": {"TODO"}},
			},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for override rule out of terminal status DONE, got nil")
	}
	if !strings.Contains(err.Error(), "reopenable") {
		t.Errorf("error must name the override tag, got: %v", err)
	}
}

// Terminal statuses remain legal as rule TARGETS — that is how a task
// reaches DONE at all. Only the FROM side is rejected, so this check
// cannot creep into forbidding completion.
func TestTaskConfig_TerminalStatusAllowedAsRuleTarget(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Task.StateMachine.Rules["TODO"] = []string{"IN_PROGRESS", "DONE", "SKIPPED"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("terminal status as a rule target must stay valid, got: %v", err)
	}
}

// The built-in rule set keys only off TODO and IN_PROGRESS, both
// non-terminal, so the default config must survive the new check.
func TestTaskConfig_DefaultsSurviveTerminalFromCheck(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config must validate, got: %v", err)
	}
}

func TestTrackConfig_SlugMaxLen(t *testing.T) {
	tests := []struct {
		name    string
		set     int
		want    int
		wantErr bool
	}{
		{name: "unset defaults to 24", set: 0, want: 24},
		{name: "explicit min", set: 3, want: 3},
		{name: "explicit max", set: 64, want: 64},
		{name: "below min rejected", set: 2, wantErr: true},
		{name: "negative rejected", set: -1, wantErr: true},
		{name: "above max rejected", set: 65, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &TrackConfig{SlugMaxLen: tc.set}
			err := cfg.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() expected error for slug_max_len=%d", tc.set)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
			if got := cfg.SlugMaxLenOrDefault(); got != tc.want {
				t.Errorf("SlugMaxLenOrDefault() = %d, want %d", got, tc.want)
			}
		})
	}
}
