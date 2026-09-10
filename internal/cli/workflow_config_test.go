package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// customStatusYAML declares a five-status workflow with a custom
// IN_REVIEW status and rules routing IN_PROGRESS through it. Rule keys
// are written upper-case, exactly as a user would; viper lower-cases
// them on read, which is what normalizeStateMachineKeys undoes.
const customStatusYAML = `task:
  default_status: TODO
  statuses:
    - name: TODO
      label: To Do
      is_terminal: false
      role: initial
      tls_marker: " "
    - name: IN_PROGRESS
      label: In Progress
      is_terminal: false
      role: active
      tls_marker: ">"
    - name: IN_REVIEW
      label: In Review
      is_terminal: false
      role: active
      tls_marker: "r"
    - name: DONE
      label: Done
      is_terminal: true
      role: completed
      tls_marker: "x"
    - name: SKIPPED
      label: Skipped
      is_terminal: true
      role: completed
      tls_marker: "-"
  state_machine:
    rules:
      TODO: [IN_PROGRESS, SKIPPED]
      IN_PROGRESS: [IN_REVIEW, TODO, SKIPPED]
      IN_REVIEW: [DONE, IN_PROGRESS, SKIPPED]
`

// loadWorkflowConfigFile points viper at a config file holding yamlBody
// and clears the workflow singleton so the next DefaultWorkflow call
// rebuilds from it. Mirrors what initConfig does in production.
func loadWorkflowConfigFile(t *testing.T, yamlBody string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yamlBody), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	viper.Reset()
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("read config: %v", err)
	}

	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		viper.Reset()
		core.ResetDefaultWorkflow()
	})
}

// TestDefaultWorkflow_UsesLoadedConfig proves the user's configured
// statuses and transition rules reach DefaultWorkflow. It deliberately
// goes through a real config file rather than constructing a
// WorkflowManager directly — the direct-construction tests in
// internal/core passed while the config was being ignored entirely.
func TestDefaultWorkflow_UsesLoadedConfig(t *testing.T) {
	loadWorkflowConfigFile(t, customStatusYAML)

	wm, err := core.DefaultWorkflowE()
	if err != nil {
		t.Fatalf("DefaultWorkflowE: %v", err)
	}

	statuses := wm.GetAllStatuses()
	if len(statuses) != 5 {
		t.Fatalf("expected 5 configured statuses, got %d: %v", len(statuses), statuses)
	}
	if statuses[2] != "IN_REVIEW" {
		t.Errorf("expected IN_REVIEW at index 2, got %q (%v)", statuses[2], statuses)
	}

	if _, err := wm.GetStatusDef(core.TaskStatus("IN_REVIEW")); err != nil {
		t.Errorf("custom status IN_REVIEW not registered: %v", err)
	}

	if status, ok := wm.StatusForTLSMarker("r"); !ok || status != core.TaskStatus("IN_REVIEW") {
		t.Errorf("TLS marker \"r\" = (%q, %v), want (IN_REVIEW, true)", status, ok)
	}
}

// TestDefaultWorkflow_ConfiguredRulesApply covers the transition rules
// themselves, not just the status list.
func TestDefaultWorkflow_ConfiguredRulesApply(t *testing.T) {
	loadWorkflowConfigFile(t, customStatusYAML)

	wm, err := core.DefaultWorkflowE()
	if err != nil {
		t.Fatalf("DefaultWorkflowE: %v", err)
	}

	// Configured: IN_PROGRESS -> IN_REVIEW allowed.
	if err := wm.ValidateTransition(
		core.TaskStatus("IN_PROGRESS"), core.TaskStatus("IN_REVIEW"), false,
	); err != nil {
		t.Errorf("IN_PROGRESS -> IN_REVIEW should be allowed: %v", err)
	}

	// Configured: IN_REVIEW -> DONE allowed.
	if err := wm.ValidateTransition(
		core.TaskStatus("IN_REVIEW"), core.TaskStatus("DONE"), false,
	); err != nil {
		t.Errorf("IN_REVIEW -> DONE should be allowed: %v", err)
	}

	// The built-in default machine allows IN_PROGRESS -> DONE; this
	// config does not. Fails if built-in rules leaked through.
	if err := wm.ValidateTransition(
		core.TaskStatus("IN_PROGRESS"), core.TaskStatus("DONE"), false,
	); err == nil {
		t.Error("IN_PROGRESS -> DONE should be rejected under configured rules")
	}

	// Not in the configured rules for TODO.
	if err := wm.ValidateTransition(
		core.StatusTodo, core.TaskStatus("IN_REVIEW"), false,
	); err == nil {
		t.Error("TODO -> IN_REVIEW should be rejected")
	}
}

// TestLoadTaskConfig_NormalizesStateMachineKeys pins the map-key defect
// directly at the provider: viper lower-cases map keys, so the rules
// arrive as "todo"/"in_progress" and must be re-cased before they reach
// the workflow. Without normalization TaskConfig.Validate rejects them
// ("state machine rule references unknown status: todo").
func TestLoadTaskConfig_NormalizesStateMachineKeys(t *testing.T) {
	loadWorkflowConfigFile(t, customStatusYAML)

	// Precondition: viper really did lower-case the keys, so this test
	// exercises the defect rather than a config that never had it.
	raw := viper.GetStringMap("task.state_machine.rules")
	if _, lowered := raw["todo"]; !lowered {
		t.Fatalf("expected viper to lower-case rule keys, got %v", keysOf(raw))
	}

	cfg := loadTaskConfig()
	if cfg == nil {
		t.Fatal("loadTaskConfig returned nil")
	}
	if cfg.StateMachine == nil {
		t.Fatal("state machine missing from decoded config")
	}

	for _, want := range []string{"TODO", "IN_PROGRESS", "IN_REVIEW"} {
		if _, ok := cfg.StateMachine.Rules[want]; !ok {
			t.Errorf("rule key %q missing after normalization; have %v",
				want, keysOf2(cfg.StateMachine.Rules))
		}
	}
	for _, bad := range []string{"todo", "in_progress", "in_review"} {
		if _, ok := cfg.StateMachine.Rules[bad]; ok {
			t.Errorf("lower-cased rule key %q survived normalization", bad)
		}
	}

	// The whole point of normalizing: validation must accept the result.
	if err := cfg.Validate(); err != nil {
		t.Errorf("configured task section should validate, got: %v", err)
	}
}

// TestLoadTaskConfig_DecodesSnakeCaseKeys pins the missing-mapstructure-tag
// defect on the provider's own decode path: without TagName = "yaml"
// every snake_case key decodes to its zero value.
func TestLoadTaskConfig_DecodesSnakeCaseKeys(t *testing.T) {
	loadWorkflowConfigFile(t, customStatusYAML)

	cfg := loadTaskConfig()
	if cfg == nil {
		t.Fatal("loadTaskConfig returned nil")
	}
	if cfg.DefaultStatus != "TODO" {
		t.Errorf("default_status = %q, want TODO", cfg.DefaultStatus)
	}
	if cfg.StateMachine == nil {
		t.Fatal("state_machine decoded as nil")
	}
	if len(cfg.Statuses) != 5 {
		t.Fatalf("statuses = %d, want 5", len(cfg.Statuses))
	}
	// tls_marker and is_terminal are snake_case too.
	if cfg.Statuses[2].TLSMarker != "r" {
		t.Errorf("statuses[2].tls_marker = %q, want r", cfg.Statuses[2].TLSMarker)
	}
	if !cfg.Statuses[3].IsTerminal {
		t.Error("statuses[3].is_terminal = false, want true (DONE)")
	}
}

// TestDefaultWorkflow_NoConfigUsesBuiltins guards the fallback: an empty
// task section must still produce the built-in four-status workflow.
func TestDefaultWorkflow_NoConfigUsesBuiltins(t *testing.T) {
	loadWorkflowConfigFile(t, "output:\n  format: table\n")

	wm, err := core.DefaultWorkflowE()
	if err != nil {
		t.Fatalf("DefaultWorkflowE: %v", err)
	}
	if got := len(wm.GetAllStatuses()); got != 4 {
		t.Errorf("expected 4 built-in statuses, got %d: %v", got, wm.GetAllStatuses())
	}
	if err := wm.ValidateTransition(core.StatusTodo, core.StatusInProgress, false); err != nil {
		t.Errorf("built-in TODO -> IN_PROGRESS should be allowed: %v", err)
	}
}

// TestDefaultWorkflowE_ReportsInvalidConfig proves a broken workflow
// config surfaces as an error rather than a silent fall back to the
// built-in statuses.
func TestDefaultWorkflowE_ReportsInvalidConfig(t *testing.T) {
	loadWorkflowConfigFile(t, `task:
  statuses:
    - name: TODO
      label: To Do
      role: initial
    - name: IN_PROGRESS
      label: In Progress
      role: active
  state_machine:
    rules:
      TODO: [NOPE]
`)

	wm, err := core.DefaultWorkflowE()
	if err == nil {
		t.Fatalf("expected error for rule targeting undeclared status, got workflow %v", wm)
	}
	if wm != nil {
		t.Errorf("expected nil workflow alongside error, got %v", wm)
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysOf2(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
