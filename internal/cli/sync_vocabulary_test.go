package cli

import (
	"testing"

	"hop.top/tlc/internal/config"
)

// TestBuiltinVocabularyKeepsAliases pins the behavior-identical
// guarantee: with no config the wire payload must carry exactly the
// labels the plugins hardcoded before this change.
func TestBuiltinVocabularyKeepsAliases(t *testing.T) {
	v := buildSyncVocabulary()

	wantPriority := map[string]string{
		"P0": "priority:critical",
		"P1": "priority:high",
		"P2": "priority:medium",
		"P3": "priority:low",
	}
	for name, label := range wantPriority {
		if got := v.PriorityToLabel[name]; got != label {
			t.Errorf("priority %s -> %q, want %q", name, got, label)
		}
	}

	wantEffort := map[string]string{
		"XS": "effort:xs",
		"S":  "effort:s",
		"M":  "effort:m",
		"L":  "effort:l",
		"XL": "effort:xl",
	}
	for name, label := range wantEffort {
		if got := v.EffortToLabel[name]; got != label {
			t.Errorf("effort %s -> %q, want %q", name, got, label)
		}
	}

	if got := v.ActiveStatuses["IN_PROGRESS"]; got != "status:in-progress" {
		t.Errorf("IN_PROGRESS -> %q, want status:in-progress", got)
	}
	if v.InitialStatus != "TODO" {
		t.Errorf("InitialStatus = %q, want TODO", v.InitialStatus)
	}
	if got := v.TerminalStatuses["DONE"]; got != "completed" {
		t.Errorf("DONE -> %q, want completed", got)
	}
	if got := v.TerminalStatuses["SKIPPED"]; got != "not_planned" {
		t.Errorf("SKIPPED -> %q, want not_planned", got)
	}

	// A terminal or initial status must never earn a status label.
	for _, name := range []string{"TODO", "DONE", "SKIPPED"} {
		if _, ok := v.ActiveStatuses[name]; ok {
			t.Errorf("%s must not carry a status label", name)
		}
	}
}

// TestCustomPriorityVocabularyDropsAlias is the bug this change fixes:
// a declared vocabulary must render from its own names, not through the
// built-in alias table.
func TestCustomPriorityVocabularyDropsAlias(t *testing.T) {
	defs := []config.PriorityDefinition{
		{Name: "URGENT"},
		{Name: "NORMAL"},
		{Name: "LATER"},
	}
	if isBuiltinPriorityVocabulary(defs) {
		t.Fatal("custom vocabulary misidentified as built-in")
	}

	for _, tc := range []struct{ name, want string }{
		{"URGENT", "urgent"},
		{"NORMAL", "normal"},
		{"LATER", "later"},
	} {
		if got := labelValue(tc.name); got != tc.want {
			t.Errorf("labelValue(%s) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestLabelValueConvention pins the shouty-underscore to
// lowercase-hyphen rule the forge labels rely on.
func TestLabelValueConvention(t *testing.T) {
	cases := map[string]string{
		"IN_PROGRESS": "in-progress",
		"IN_REVIEW":   "in-review",
		"DOING":       "doing",
		"XS":          "xs",
	}
	for in, want := range cases {
		if got := labelValue(in); got != want {
			t.Errorf("labelValue(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestBuiltinDetectionIsOrderSensitive guards the alias short-circuit:
// a reordered vocabulary is a DIFFERENT vocabulary, because declaration
// order is rank order.
func TestBuiltinDetectionIsOrderSensitive(t *testing.T) {
	reordered := []config.PriorityDefinition{
		{Name: "P1"}, {Name: "P0"}, {Name: "P2"}, {Name: "P3"},
	}
	if isBuiltinPriorityVocabulary(reordered) {
		t.Error("reordered vocabulary must not count as built-in")
	}

	if !isBuiltinPriorityVocabulary(config.GetDefaultPriorities()) {
		t.Error("the default vocabulary must count as built-in")
	}
}
