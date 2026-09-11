package core

import (
	"testing"

	"hop.top/tlc/internal/config"
)

// withTaskConfigProvider installs a provider and clears the workflow
// singleton for the duration of one test.
func withTaskConfigProvider(t *testing.T, fn func() *config.TaskConfig) {
	t.Helper()
	prev := taskConfigProvider
	SetTaskConfigProvider(fn)
	ResetDefaultWorkflow()
	t.Cleanup(func() {
		SetTaskConfigProvider(prev)
		ResetDefaultWorkflow()
	})
}

// TestDefaultWorkflow_ProviderSuppliesStatuses proves the registered
// provider — not the built-in constants — determines the workflow.
func TestDefaultWorkflow_ProviderSuppliesStatuses(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return &config.TaskConfig{
			Statuses: []config.StatusDefinition{
				{Name: "TODO", Label: "To Do", Role: "initial", TLSMarker: " "},
				{Name: "IN_PROGRESS", Label: "In Progress", Role: "active", TLSMarker: ">"},
				{Name: "IN_REVIEW", Label: "In Review", Role: "active", TLSMarker: "r"},
				{Name: "DONE", Label: "Done", IsTerminal: true, Role: "completed", TLSMarker: "x"},
			},
			StateMachine: &config.WorkflowDefinition{
				Rules: map[string][]string{
					"TODO":        {"IN_PROGRESS"},
					"IN_PROGRESS": {"IN_REVIEW"},
					"IN_REVIEW":   {"DONE"},
				},
			},
		}
	})

	wm := DefaultWorkflow()
	if got := len(wm.GetAllStatuses()); got != 4 {
		t.Fatalf("statuses = %d, want 4: %v", got, wm.GetAllStatuses())
	}
	if _, err := wm.GetStatusDef(TaskStatus("IN_REVIEW")); err != nil {
		t.Errorf("provider status IN_REVIEW missing: %v", err)
	}
	if err := wm.ValidateTransition(
		TaskStatus("IN_PROGRESS"), TaskStatus("IN_REVIEW"), false,
	); err != nil {
		t.Errorf("IN_PROGRESS -> IN_REVIEW should be allowed: %v", err)
	}
}

// TestDefaultWorkflow_NilProviderUsesBuiltins keeps library consumers and
// unit tests on the built-in four-status workflow.
func TestDefaultWorkflow_NilProviderUsesBuiltins(t *testing.T) {
	withTaskConfigProvider(t, nil)

	wm := DefaultWorkflow()
	if got := len(wm.GetAllStatuses()); got != 4 {
		t.Fatalf("statuses = %d, want 4: %v", got, wm.GetAllStatuses())
	}
	if err := wm.ValidateTransition(StatusTodo, StatusInProgress, false); err != nil {
		t.Errorf("built-in TODO -> IN_PROGRESS should be allowed: %v", err)
	}
}

// TestDefaultWorkflow_FatalOnInvalidConfig pins the replacement for the
// old panic: a workflow that cannot be built from user config terminates
// loudly instead of silently serving built-in statuses.
func TestDefaultWorkflow_FatalOnInvalidConfig(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return &config.TaskConfig{
			Statuses: []config.StatusDefinition{
				{Name: "TODO", Label: "To Do", Role: "initial"},
				{Name: "IN_PROGRESS", Label: "In Progress", Role: "active"},
			},
			StateMachine: &config.WorkflowDefinition{
				Rules: map[string][]string{"TODO": {"NOPE"}},
			},
		}
	})

	prevExit := exit
	var code int
	called := false
	exit = func(c int) { code, called = c, true }
	t.Cleanup(func() { exit = prevExit })

	// DefaultWorkflow returns nil after the stubbed exit; the real
	// os.Exit would not return at all.
	if wm := DefaultWorkflow(); wm != nil {
		t.Errorf("expected nil workflow on invalid config, got %v", wm)
	}
	if !called {
		t.Fatal("expected DefaultWorkflow to exit on invalid config")
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
