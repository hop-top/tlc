package core

import (
	"testing"

	"hop.top/tlc/internal/config"
)

func TestValidEffort(t *testing.T) {
	valid := []Effort{"", EffortXS, EffortS, EffortM, EffortL, EffortXL}
	for _, e := range valid {
		if !ValidEffort(e) {
			t.Errorf("expected %q to be valid", e)
		}
	}
	invalid := []Effort{"HUGE", "xs", "medium", "1"}
	for _, e := range invalid {
		if ValidEffort(e) {
			t.Errorf("expected %q to be invalid", e)
		}
	}
}

func TestValidPriority(t *testing.T) {
	valid := []Priority{"", PriorityP0, PriorityP1, PriorityP2, PriorityP3}
	for _, p := range valid {
		if !ValidPriority(p) {
			t.Errorf("expected %q to be valid", p)
		}
	}
	invalid := []Priority{"HIGH", "p0", "critical", "1", "A"}
	for _, p := range invalid {
		if ValidPriority(p) {
			t.Errorf("expected %q to be invalid", p)
		}
	}
}

// TestValidTaskStatus_HonoursConfiguredVocabulary proves the helper
// validates against the EFFECTIVE vocabulary rather than the built-in
// taskStatuses slice. Driven through the config provider — the same hook
// the CLI registers — rather than by reaching into package state, so the
// test exercises the path a real configured status travels.
//
// SKIPPED is the load-bearing negative: it is a built-in, so a
// built-in-only implementation accepts it here even though this config
// never declares it.
func TestValidTaskStatus_HonoursConfiguredVocabulary(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return &config.TaskConfig{
			Statuses: []config.StatusDefinition{
				{Name: "TODO", Label: "To Do", Role: "initial"},
				{Name: "IN_PROGRESS", Label: "In Progress", Role: "active"},
				{Name: "IN_REVIEW", Label: "In Review", Role: "active"},
				{Name: "DONE", Label: "Done", IsTerminal: true, Role: "completed"},
			},
		}
	})

	for _, s := range []TaskStatus{"", "TODO", "IN_PROGRESS", "IN_REVIEW", "DONE"} {
		if !ValidTaskStatus(s) {
			t.Errorf("configured status %q should be valid", s)
		}
	}
	for _, s := range []TaskStatus{"SKIPPED", "in_review", "NOPE"} {
		if ValidTaskStatus(s) {
			t.Errorf("undeclared status %q should be invalid", s)
		}
	}
}

// TestValidTaskStatus_NilProviderUsesBuiltins keeps library consumers and
// unit tests on the built-in four-status set when config declares none.
func TestValidTaskStatus_NilProviderUsesBuiltins(t *testing.T) {
	withTaskConfigProvider(t, nil)

	valid := []TaskStatus{"", StatusTodo, StatusInProgress, StatusDone, StatusSkipped}
	for _, s := range valid {
		if !ValidTaskStatus(s) {
			t.Errorf("built-in status %q should be valid", s)
		}
	}
	for _, s := range []TaskStatus{"IN_REVIEW", "todo", "1"} {
		if ValidTaskStatus(s) {
			t.Errorf("status %q should be invalid", s)
		}
	}
}
