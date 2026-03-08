package core

import (
	"testing"

	"hop.top/tlc/internal/config"
)

func defaultTestConfig() *config.TaskConfig {
	return &config.TaskConfig{
		DefaultStatus: "TODO",
		Statuses:      config.GetDefaultStatuses(),
		StateMachine:  config.GetDefaultStateMachine(),
	}
}

func TestValidateTransition_DefaultConfig(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error creating WorkflowManager: %v", err)
	}

	tests := []struct {
		name    string
		current TaskStatus
		next    TaskStatus
		wantErr bool
	}{
		{"TODO to IN_PROGRESS", StatusTodo, StatusInProgress, false},
		{"TODO to SKIPPED", StatusTodo, StatusSkipped, false},
		{"IN_PROGRESS to DONE", StatusInProgress, StatusDone, false},
		{"IN_PROGRESS to TODO", StatusInProgress, StatusTodo, false},
		{"IN_PROGRESS to SKIPPED", StatusInProgress, StatusSkipped, false},
		{"same status", StatusTodo, StatusTodo, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wm.ValidateTransition(tt.current, tt.next, false)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition(%s, %s) error = %v, wantErr %v",
					tt.current, tt.next, err, tt.wantErr)
			}
		})
	}
}

func TestValidateTransition_RejectsTerminalToAnything(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	terminalStatuses := []TaskStatus{StatusDone, StatusSkipped}
	targets := []TaskStatus{StatusTodo, StatusInProgress, StatusDone, StatusSkipped}

	for _, from := range terminalStatuses {
		for _, to := range targets {
			if from == to {
				continue // same-status is always allowed
			}
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				err := wm.ValidateTransition(from, to, false)
				if err == nil {
					t.Errorf("expected error for terminal %s -> %s", from, to)
				}
			})
		}
	}
}

func TestValidateTransition_RejectsTodoToDone(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = wm.ValidateTransition(StatusTodo, StatusDone, false)
	if err == nil {
		t.Error("expected error for TODO -> DONE without rule, got nil")
	}
}

func TestValidateTransition_ForceBypassesRules(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// TODO -> DONE is normally forbidden, but force=true bypasses
	err = wm.ValidateTransition(StatusTodo, StatusDone, true)
	if err != nil {
		t.Errorf("expected force=true to bypass rules, got error: %v", err)
	}
}

func TestValidateTransition_ForceBypassesTerminal(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// DONE -> TODO is normally blocked (terminal), but force=true bypasses
	err = wm.ValidateTransition(StatusDone, StatusTodo, true)
	if err != nil {
		t.Errorf("expected force=true to bypass terminal immutability, got error: %v", err)
	}
}

func TestValidateTransition_ForceRejectsUnknownStatuses(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Even with force=true, unknown statuses should be rejected
	err = wm.ValidateTransition(StatusTodo, "NONEXISTENT", true)
	if err == nil {
		t.Error("expected error for unknown target status even with force=true")
	}

	err = wm.ValidateTransition("NONEXISTENT", StatusTodo, true)
	if err == nil {
		t.Error("expected error for unknown source status even with force=true")
	}
}

func TestStatusForRole(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		role     string
		expected TaskStatus
		wantErr  bool
	}{
		{"initial", StatusTodo, false},
		{"active", StatusInProgress, false},
		{"completed", StatusDone, false},
		{"nonexistent", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			status, err := wm.StatusForRole(tt.role)
			if (err != nil) != tt.wantErr {
				t.Errorf("StatusForRole(%q) error = %v, wantErr %v", tt.role, err, tt.wantErr)
				return
			}
			if !tt.wantErr && status != tt.expected {
				t.Errorf("StatusForRole(%q) = %s, want %s", tt.role, status, tt.expected)
			}
		})
	}
}

func TestStatusForTLSMarker(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		marker   string
		expected TaskStatus
		found    bool
	}{
		{" ", StatusTodo, true},
		{">", StatusInProgress, true},
		{"x", StatusDone, true},
		{"-", StatusSkipped, true},
		{"?", "", false},
	}

	for _, tt := range tests {
		t.Run("marker_"+tt.marker, func(t *testing.T) {
			status, found := wm.StatusForTLSMarker(tt.marker)
			if found != tt.found {
				t.Errorf("StatusForTLSMarker(%q) found = %v, want %v", tt.marker, found, tt.found)
				return
			}
			if found && status != tt.expected {
				t.Errorf("StatusForTLSMarker(%q) = %s, want %s", tt.marker, status, tt.expected)
			}
		})
	}
}

func TestGetAllStatuses_PreservesOrder(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"}
	got := wm.GetAllStatuses()

	if len(got) != len(expected) {
		t.Fatalf("expected %d statuses, got %d", len(expected), len(got))
	}

	for i, s := range expected {
		if got[i] != s {
			t.Errorf("status[%d] = %s, want %s", i, got[i], s)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		status   TaskStatus
		terminal bool
	}{
		{StatusTodo, false},
		{StatusInProgress, false},
		{StatusDone, true},
		{StatusSkipped, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := wm.IsTerminal(tt.status); got != tt.terminal {
				t.Errorf("IsTerminal(%s) = %v, want %v", tt.status, got, tt.terminal)
			}
		})
	}
}

func TestGetWorkflowForTags(t *testing.T) {
	cfg := defaultTestConfig()
	cfg.Workflows = map[string]config.WorkflowOverride{
		"urgent": {
			StateMachine: &config.WorkflowDefinition{
				Rules: map[string][]string{
					"TODO":        {"IN_PROGRESS", "DONE", "SKIPPED"},
					"IN_PROGRESS": {"DONE", "TODO", "SKIPPED"},
				},
			},
		},
	}

	wm, err := NewWorkflowManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("matching tag returns override", func(t *testing.T) {
		tagWM := wm.GetWorkflowForTags([]string{"urgent"})
		if tagWM == wm {
			t.Error("expected different WorkflowManager for matching tag")
		}
		// The "urgent" override allows TODO -> DONE
		err := tagWM.ValidateTransition(StatusTodo, StatusDone, false)
		if err != nil {
			t.Errorf("expected TODO -> DONE to be allowed for urgent tag, got: %v", err)
		}
	})

	t.Run("non-matching tag returns self", func(t *testing.T) {
		tagWM := wm.GetWorkflowForTags([]string{"normal"})
		if tagWM != wm {
			t.Error("expected same WorkflowManager for non-matching tag")
		}
	})

	t.Run("empty tags returns self", func(t *testing.T) {
		tagWM := wm.GetWorkflowForTags(nil)
		if tagWM != wm {
			t.Error("expected same WorkflowManager for nil tags")
		}
	})
}

func TestCustomConfig_WithInReviewStatus(t *testing.T) {
	cfg := &config.TaskConfig{
		DefaultStatus: "TODO",
		Statuses: []config.StatusDefinition{
			{Name: "TODO", Label: "To Do", IsTerminal: false, Color: "yellow", Role: "initial", TLSMarker: " "},
			{Name: "IN_PROGRESS", Label: "In Progress", IsTerminal: false, Color: "blue", Role: "active", TLSMarker: ">"},
			{Name: "IN_REVIEW", Label: "In Review", IsTerminal: false, Color: "purple", Role: "active", TLSMarker: "r"},
			{Name: "DONE", Label: "Done", IsTerminal: true, Color: "green", Role: "completed", TLSMarker: "x"},
			{Name: "SKIPPED", Label: "Skipped", IsTerminal: true, Color: "gray", Role: "completed", TLSMarker: "-"},
		},
		StateMachine: &config.WorkflowDefinition{
			Rules: map[string][]string{
				"TODO":        {"IN_PROGRESS", "SKIPPED"},
				"IN_PROGRESS": {"IN_REVIEW", "TODO", "SKIPPED"},
				"IN_REVIEW":   {"DONE", "IN_PROGRESS", "SKIPPED"},
			},
		},
	}

	wm, err := NewWorkflowManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// IN_PROGRESS -> IN_REVIEW should be allowed
	err = wm.ValidateTransition(TaskStatus("IN_PROGRESS"), TaskStatus("IN_REVIEW"), false)
	if err != nil {
		t.Errorf("expected IN_PROGRESS -> IN_REVIEW to be allowed: %v", err)
	}

	// IN_REVIEW -> DONE should be allowed
	err = wm.ValidateTransition(TaskStatus("IN_REVIEW"), TaskStatus("DONE"), false)
	if err != nil {
		t.Errorf("expected IN_REVIEW -> DONE to be allowed: %v", err)
	}

	// TODO -> IN_REVIEW should NOT be allowed
	err = wm.ValidateTransition(StatusTodo, TaskStatus("IN_REVIEW"), false)
	if err == nil {
		t.Error("expected TODO -> IN_REVIEW to be rejected")
	}

	// GetAllStatuses should have 5 entries in order
	allStatuses := wm.GetAllStatuses()
	if len(allStatuses) != 5 {
		t.Fatalf("expected 5 statuses, got %d", len(allStatuses))
	}
	if allStatuses[2] != "IN_REVIEW" {
		t.Errorf("expected IN_REVIEW at index 2, got %s", allStatuses[2])
	}

	// TLS marker lookup
	status, found := wm.StatusForTLSMarker("r")
	if !found {
		t.Error("expected to find status for marker 'r'")
	}
	if status != TaskStatus("IN_REVIEW") {
		t.Errorf("expected IN_REVIEW for marker 'r', got %s", status)
	}
}

func TestDefaultWorkflow(t *testing.T) {
	wm := DefaultWorkflow()
	if wm == nil {
		t.Fatal("DefaultWorkflow() returned nil")
	}

	// Should be able to do standard transitions
	err := wm.ValidateTransition(StatusTodo, StatusInProgress, false)
	if err != nil {
		t.Errorf("expected TODO -> IN_PROGRESS on default workflow, got: %v", err)
	}
}

func TestGetStatusDef(t *testing.T) {
	wm, err := NewWorkflowManager(defaultTestConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("existing status", func(t *testing.T) {
		def, err := wm.GetStatusDef(StatusTodo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if def.Name != "TODO" {
			t.Errorf("expected name TODO, got %s", def.Name)
		}
		if def.Label != "To Do" {
			t.Errorf("expected label 'To Do', got %s", def.Label)
		}
	})

	t.Run("unknown status", func(t *testing.T) {
		_, err := wm.GetStatusDef("NONEXISTENT")
		if err == nil {
			t.Error("expected error for unknown status")
		}
	})
}

func TestNewWorkflowManager_NilConfig(t *testing.T) {
	_, err := NewWorkflowManager(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}
