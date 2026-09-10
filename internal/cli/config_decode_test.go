package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
)

// loadViper seeds the global viper with a YAML document. Callers must run
// under a test that resets viper afterwards.
func loadViper(t *testing.T, yaml string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader(yaml)); err != nil {
		t.Fatalf("seed viper: %v", err)
	}
}

// Guards the confirmed user-visible loss: task.stale.default_timeout was
// silently dropped, so the 6h built-in from TaskConfig.Validate always won.
func TestUnmarshalConfigKeyDecodesStaleDefaultTimeout(t *testing.T) {
	loadViper(t, `
task:
  default_status: TODO
  stale:
    default_timeout: 3h
`)

	var taskCfg config.TaskConfig
	if err := unmarshalConfigKey("task", &taskCfg); err != nil {
		t.Fatalf("unmarshalConfigKey: %v", err)
	}

	if got, want := taskCfg.Stale.DefaultTimeout, 3*time.Hour; got != want {
		t.Errorf("task.stale.default_timeout = %v, want %v", got, want)
	}
	// Single-word keys always worked; assert they still do.
	if got, want := taskCfg.DefaultStatus, "TODO"; got != want {
		t.Errorf("task.default_status = %q, want %q", got, want)
	}
}

// The built-in default must not silently override a configured timeout:
// this is the exact symptom users reported.
func TestStaleDefaultTimeoutSurvivesValidate(t *testing.T) {
	loadViper(t, `
task:
  stale:
    default_timeout: 3h
`)

	var taskCfg config.TaskConfig
	if err := unmarshalConfigKey("task", &taskCfg); err != nil {
		t.Fatalf("unmarshalConfigKey: %v", err)
	}
	if err := taskCfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if got := taskCfg.Stale.DefaultTimeout; got != 3*time.Hour {
		t.Errorf("default_timeout = %v, want 3h (6h built-in leaked through)", got)
	}
}

// Guards the second confirmed loss: workspaces[].wsm_id was dropped.
func TestUnmarshalConfigKeyDecodesWorkspaceWsmID(t *testing.T) {
	loadViper(t, `
workspaces:
  - name: primary
    wsm_id: ws-123
    default: true
    spaces:
      - uri: /tmp/one
`)

	var workspaces []config.WorkspaceConfig
	if err := unmarshalConfigKey("workspaces", &workspaces); err != nil {
		t.Fatalf("unmarshalConfigKey: %v", err)
	}

	if len(workspaces) != 1 {
		t.Fatalf("got %d workspaces, want 1", len(workspaces))
	}
	ws := workspaces[0]
	if got, want := ws.WsmID, "ws-123"; got != want {
		t.Errorf("workspaces[0].wsm_id = %q, want %q", got, want)
	}
	if got, want := ws.Name, "primary"; got != want {
		t.Errorf("workspaces[0].name = %q, want %q", got, want)
	}
	if !ws.Default {
		t.Error("workspaces[0].default = false, want true")
	}
	if len(ws.Spaces) != 1 || ws.Spaces[0].URI != "/tmp/one" {
		t.Errorf("workspaces[0].spaces = %+v, want one space with uri /tmp/one", ws.Spaces)
	}
}

// Whole-config decode (root.go, config.go, doctor.go, task_validation.go)
// must pick up snake_case keys across every section.
func TestUnmarshalConfigDecodesSnakeCaseKeys(t *testing.T) {
	loadViper(t, `
project:
  id: demo
  fallback_mode: detected
  duplicate_id_strategy: unique
task:
  default_status: TODO
  id_format: "T-{seq:04d}"
  require_reference: true
  stale:
    default_timeout: 90m
tracks:
  stale_threshold: 12h
  health:
    max_active: 7
    min_progress_to_start: 25
storage:
  db_path: /tmp/custom.sqlite
git:
  branch:
    zero_pad_issue: 6
ui:
  date_format: "2006-01-02"
  log_sort_direction: desc
validation:
  create:
    required_fields: [title, priority]
workspaces:
  - name: primary
    wsm_id: ws-123
`)

	var cfg config.Config
	if err := unmarshalConfig(&cfg); err != nil {
		t.Fatalf("unmarshalConfig: %v", err)
	}

	checks := []struct {
		key       string
		got, want any
	}{
		{"project.fallback_mode", cfg.Project.FallbackMode, "detected"},
		{"project.duplicate_id_strategy", cfg.Project.DuplicateIDStrategy, "unique"},
		{"task.id_format", cfg.Task.IDFormat, "T-{seq:04d}"},
		{"task.require_reference", cfg.Task.RequireReference, true},
		{"task.stale.default_timeout", cfg.Task.Stale.DefaultTimeout, 90 * time.Minute},
		{"tracks.stale_threshold", cfg.Tracks.StaleThreshold, 12 * time.Hour},
		{"tracks.health.max_active", cfg.Tracks.Health.MaxActive, 7},
		{"tracks.health.min_progress_to_start", cfg.Tracks.Health.MinProgressToStart, 25},
		{"storage.db_path", cfg.Storage.DBPath, "/tmp/custom.sqlite"},
		{"git.branch.zero_pad_issue", cfg.Git.Branch.ZeroPadIssue, 6},
		{"ui.date_format", cfg.UI.DateFormat, "2006-01-02"},
		{"ui.log_sort_direction", cfg.UI.LogSortDirection, "desc"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.key, c.got, c.want)
		}
	}

	if got := cfg.Validation.Create.RequiredFields; len(got) != 2 || got[0] != "title" || got[1] != "priority" {
		t.Errorf("validation.create.required_fields = %v, want [title priority]", got)
	}
	if len(cfg.Workspaces) != 1 || cfg.Workspaces[0].WsmID != "ws-123" {
		t.Errorf("workspaces = %+v, want one with wsm_id ws-123", cfg.Workspaces)
	}
}

// getValidationConfig is the task_validation.go call site; validation rules
// live behind the snake_case key required_fields.
func TestGetValidationConfigDecodesRequiredFields(t *testing.T) {
	loadViper(t, `
validation:
  update:
    required_fields: [status]
    rules:
      - field: title
        pattern: "^.{3,}$"
`)

	vc := getValidationConfig()

	if got := vc.Update.RequiredFields; len(got) != 1 || got[0] != "status" {
		t.Errorf("validation.update.required_fields = %v, want [status]", got)
	}
	if len(vc.Update.Rules) != 1 || vc.Update.Rules[0].Field != "title" {
		t.Errorf("validation.update.rules = %+v, want one rule on title", vc.Update.Rules)
	}
}
