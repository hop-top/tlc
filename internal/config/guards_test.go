package config

import (
	"strings"
	"testing"
)

func guardCfg(g ...GuardDefinition) *TaskConfig {
	return &TaskConfig{StateMachine: &WorkflowDefinition{Guards: g}}
}

func TestGuards_DefaultsPopulated(t *testing.T) {
	c := &TaskConfig{}
	if err := c.ValidateWorkflow(); err != nil {
		t.Fatal(err)
	}
	if len(c.StateMachine.Guards) != 4 {
		t.Fatalf("default guards = %d, want 4", len(c.StateMachine.Guards))
	}
}

func TestGuards_Validation(t *testing.T) {
	tests := []struct {
		name string
		g    GuardDefinition
		want string // "" = valid
	}{
		{"valid", GuardDefinition{To: "done", Require: []string{CheckNote}}, ""},
		{"unknown check", GuardDefinition{To: "done", Require: []string{"nope"}}, "unknown check"},
		{"undeclared to", GuardDefinition{To: "zzz", Require: []string{CheckNote}}, "unknown status"},
		{"undeclared from", GuardDefinition{From: "zzz", Require: []string{CheckNote}}, "unknown status"},
		{"terminal from", GuardDefinition{From: "done", Require: []string{CheckNote}}, "terminal status"},
		{"terminal to is fine", GuardDefinition{To: "done", Require: []string{CheckNote}}, ""},
		{"neither", GuardDefinition{Require: []string{CheckNote}}, "neither from nor to"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := guardCfg(tt.g).ValidateWorkflow()
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestGuards_PerTagValidated(t *testing.T) {
	c := &TaskConfig{Workflows: map[string]WorkflowOverride{
		"hotfix": {StateMachine: &WorkflowDefinition{
			Guards: []GuardDefinition{{To: "done", Require: []string{"nope"}}},
		}},
	}}
	err := c.ValidateWorkflow()
	if err == nil || !strings.Contains(err.Error(), "hotfix") {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultStatuses_Seven(t *testing.T) {
	want := []struct {
		name, role, marker string
		terminal           bool
	}{
		{"backlog", RoleBacklog, "-", false},
		{"todo", RoleInitial, " ", false},
		{"in_progress", RoleActive, ">", false},
		{"in_review", RoleReview, "?", false},
		{"blocked", RoleBlocked, "!", false},
		{"done", RoleCompleted, "x", true},
		{"cancelled", RoleSkipped, "~", true},
	}
	got := GetDefaultStatuses()
	if len(got) != len(want) {
		t.Fatalf("len = %d", len(got))
	}
	for i, w := range want {
		g := got[i]
		if g.Name != w.name || g.Role != w.role || g.TLSMarker != w.marker || g.IsTerminal != w.terminal {
			t.Errorf("status %d = %+v, want %+v", i, g, w)
		}
	}
}
