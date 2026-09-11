package core

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
)

// customVocabConfig is a config whose statuses share not one name with
// the built-in TODO/IN_PROGRESS/DONE/SKIPPED set. Every test here turns
// on that disjointness: built-in rules leaking into such a config name
// statuses the user never declared, so the leak is visible rather than
// coincidentally harmless.
func customVocabConfig() *config.TaskConfig {
	return &config.TaskConfig{
		DefaultStatus: "BACKLOG",
		Statuses: []config.StatusDefinition{
			{Name: "BACKLOG", Label: "Backlog", Role: config.RoleInitial, TLSMarker: " "},
			{Name: "DOING", Label: "Doing", Role: config.RoleActive, TLSMarker: ">"},
			{Name: "SHIPPED", Label: "Shipped", IsTerminal: true, Role: config.RoleCompleted, TLSMarker: "x"},
		},
	}
}

// TestEmptyBaseRulesDoNotFreezeCustomVocabulary is the bug. A config
// that writes `state_machine:` but leaves `rules:` empty produces a
// non-nil WorkflowDefinition with nil Rules, which slipped past
// validation untouched and reached the workflow engine, where built-in
// TODO/IN_PROGRESS rules were installed over a vocabulary that has
// neither. Every task then sat frozen in its creation status, escapable
// only with --force, and the rejection named IN_PROGRESS — a status the
// user never declared.
func TestEmptyBaseRulesDoNotFreezeCustomVocabulary(t *testing.T) {
	cfg := customVocabConfig()
	cfg.StateMachine = &config.WorkflowDefinition{}

	if err := cfg.ValidateWorkflow(); err != nil {
		t.Fatalf("empty rules must not fail validation: %v", err)
	}

	wm, err := NewWorkflowManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No built-in status may appear in the installed rule set.
	for from, tos := range wm.rules {
		assertDeclared(t, cfg, from)
		for _, to := range tos {
			assertDeclared(t, cfg, to)
		}
	}

	// An unruled workflow permits movement rather than freezing every
	// task where it was created.
	if err := wm.ValidateTransition(TaskStatus("BACKLOG"), TaskStatus("DOING"), false); err != nil {
		t.Errorf("BACKLOG -> DOING must not require --force under an empty rule set: %v", err)
	}
	if err := wm.ValidateTransition(TaskStatus("DOING"), TaskStatus("SHIPPED"), false); err != nil {
		t.Errorf("DOING -> SHIPPED must not require --force under an empty rule set: %v", err)
	}
}

// TestOmittedStateMachineMatchesEmptyRules removes the inversion. A
// config that omits `state_machine:` entirely is strictly LESS specified
// than one that writes the key with no rules, so it must not fail
// harder. Omitting it used to substitute the built-in rules and then
// reject them against the custom vocabulary — a hard startup error for
// the less-specified config, while the more-specified one merely froze.
func TestOmittedStateMachineMatchesEmptyRules(t *testing.T) {
	omitted := customVocabConfig()
	if err := omitted.ValidateWorkflow(); err != nil {
		t.Fatalf("omitting state_machine must not be a hard error: %v", err)
	}

	wm, err := NewWorkflowManager(customVocabConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := wm.ValidateTransition(TaskStatus("BACKLOG"), TaskStatus("DOING"), false); err != nil {
		t.Errorf("BACKLOG -> DOING rejected with state_machine omitted: %v", err)
	}
}

// TestEmptyRulesKeepBuiltinWorkflowIntact is the other half: dropping
// the built-in rules where they do not belong must not drop them where
// they do. A config on the built-in vocabulary with an empty rule set
// still gets the built-in state machine, TODO -> DONE still refused.
func TestEmptyRulesKeepBuiltinWorkflowIntact(t *testing.T) {
	cfg := &config.TaskConfig{
		DefaultStatus: "TODO",
		Statuses:      config.GetDefaultStatuses(),
		StateMachine:  &config.WorkflowDefinition{},
	}
	if err := cfg.ValidateWorkflow(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wm, err := NewWorkflowManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := wm.ValidateTransition(StatusTodo, StatusInProgress, false); err != nil {
		t.Errorf("built-in TODO -> IN_PROGRESS must stay allowed: %v", err)
	}
	if err := wm.ValidateTransition(StatusTodo, StatusDone, false); err == nil {
		t.Error("built-in rules must still refuse TODO -> DONE")
	}
}

// TestOverrideWithEmptyRulesInheritsBase covers the per-tag variant. An
// override declaring `state_machine:` with no rules used to install the
// built-in TODO/IN_PROGRESS set, so a tagged task was judged against
// statuses from neither the override nor the user's base workflow. With
// nothing of its own to say, the override must defer to the base it
// replaces.
func TestOverrideWithEmptyRulesInheritsBase(t *testing.T) {
	cfg := customVocabConfig()
	cfg.StateMachine = &config.WorkflowDefinition{
		Rules: map[string][]string{
			"BACKLOG": {"DOING"},
			"DOING":   {"SHIPPED", "BACKLOG"},
		},
	}
	cfg.Workflows = map[string]config.WorkflowOverride{
		"urgent": {StateMachine: &config.WorkflowDefinition{}},
	}

	if err := cfg.ValidateWorkflow(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wm, err := NewWorkflowManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagWM, err := wm.GetWorkflowForTags([]string{"urgent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The base rules answer, in full, for an override that declares none.
	if err := tagWM.ValidateTransition(TaskStatus("BACKLOG"), TaskStatus("DOING"), false); err != nil {
		t.Errorf("empty override must inherit base BACKLOG -> DOING: %v", err)
	}
	if err := tagWM.ValidateTransition(TaskStatus("DOING"), TaskStatus("BACKLOG"), false); err != nil {
		t.Errorf("empty override must inherit the base's full rule set: %v", err)
	}
	// Including the base's refusals: inheriting is not permitting.
	err = tagWM.ValidateTransition(TaskStatus("BACKLOG"), TaskStatus("SHIPPED"), false)
	if err == nil {
		t.Fatal("empty override must inherit the base's refusals too")
	}
	// And no built-in status may surface in the rejection.
	for _, builtin := range []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"} {
		if strings.Contains(err.Error(), builtin) {
			t.Errorf("rejection names undeclared built-in status %q: %v", builtin, err)
		}
	}

	// The rule set itself must carry nothing the user did not declare.
	// The transition assertions above can pass on a leaked built-in set
	// by accident: rules keyed TODO/IN_PROGRESS match no custom status,
	// so the stranded-status fallback answers and hides the leak.
	for from, tos := range tagWM.rules {
		assertDeclared(t, cfg, from)
		for _, to := range tos {
			assertDeclared(t, cfg, to)
		}
	}
}

// TestOverrideWithEmptyRulesInheritsBaseOnBuiltinVocabulary is the same
// per-tag defect where the stranded-status fallback cannot hide it. On
// the built-in vocabulary a leaked built-in rule set DOES match the
// current status, so the fallback never fires and the override answers
// with rules the user wrote nowhere. The base narrows TODO to DOING-less
// SKIPPED only; an override with no rules of its own must narrow the
// same way, not re-widen to the built-in TODO -> IN_PROGRESS.
func TestOverrideWithEmptyRulesInheritsBaseOnBuiltinVocabulary(t *testing.T) {
	cfg := &config.TaskConfig{
		DefaultStatus: "TODO",
		Statuses:      config.GetDefaultStatuses(),
		StateMachine: &config.WorkflowDefinition{
			Rules: map[string][]string{"TODO": {"SKIPPED"}},
		},
		Workflows: map[string]config.WorkflowOverride{
			"urgent": {StateMachine: &config.WorkflowDefinition{}},
		},
	}
	if err := cfg.ValidateWorkflow(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wm, err := NewWorkflowManager(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tagWM, err := wm.GetWorkflowForTags([]string{"urgent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := tagWM.ValidateTransition(StatusTodo, StatusSkipped, false); err != nil {
		t.Errorf("empty override must inherit base TODO -> SKIPPED: %v", err)
	}
	if err := tagWM.ValidateTransition(StatusTodo, StatusInProgress, false); err == nil {
		t.Error("empty override must inherit the base's narrowing, not the built-in rules")
	}
}

// assertDeclared fails when name is not one of the config's statuses.
func assertDeclared(t *testing.T, cfg *config.TaskConfig, name string) {
	t.Helper()
	for _, s := range cfg.Statuses {
		if s.Name == name {
			return
		}
	}
	t.Errorf("rule set names undeclared status %q", name)
}
