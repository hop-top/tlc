package cli

// What the interactive create form OFFERS, as opposed to what it
// accepts.
//
// Every option the form shows is fed straight back into saveTask, which
// normalises against the CONFIGURED vocabulary. A hardcoded option list
// is therefore not cosmetic: under a renamed vocabulary EVERY offered
// value is rejected on submit and the whole filled-in form is lost.
// These tests drive the option builders directly, which is the only
// level at which "what was offered" is observable — huh's form runner
// needs a terminal.

import (
	"strings"
	"testing"

	"charm.land/huh/v2"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// withTaskConfig installs a task-config provider for one test and puts
// the previous one back afterwards. The CLI package registers
// loadTaskConfig in init(), so the restore matters.
func withTaskConfig(t *testing.T, cfg *config.TaskConfig) {
	t.Helper()
	core.SetTaskConfigProvider(func() *config.TaskConfig { return cfg })
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(loadTaskConfig)
		core.ResetDefaultWorkflow()
	})
}

// optionValues renders the option values in offered order.
func optionValues(opts []huh.Option[string]) []string {
	out := make([]string, 0, len(opts))
	for _, o := range opts {
		out = append(out, o.Value)
	}
	return out
}

// optionKeys renders the option LABELS in offered order.
func optionKeys(opts []huh.Option[string]) []string {
	out := make([]string, 0, len(opts))
	for _, o := range opts {
		out = append(out, o.Key)
	}
	return out
}

// renamedVocabTaskConfig is the config the hardcoded lists cannot
// survive: no priority is called P0, no effort is called M, and the tag
// policy is closed around a declared vocabulary that contains none of
// feat/fix/chore/docs/urgent.
func renamedVocabTaskConfig() *config.TaskConfig {
	return &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
		Priorities: []config.PriorityDefinition{
			{Name: "URGENT", Label: "Critical"},
			{Name: "NORMAL", Label: "Normal"},
			{Name: "LATER", Label: "Whenever"},
		},
		Efforts: []config.EffortDefinition{
			{Name: "QUICK", Label: "Quick"},
			{Name: "DEEP", Label: "Deep"},
		},
		Tags: config.TagsConfig{
			Policy:  config.TagPolicyClosed,
			Allowed: []string{"storage", "cli"},
		},
	}
}

// TestInteractivePriorityOptionsFollowConfig is the headline defect:
// with task.priorities renamed, every P0..P3 the form offered was
// rejected by saveTask's normaliser on submit.
func TestInteractivePriorityOptionsFollowConfig(t *testing.T) {
	withTaskConfig(t, renamedVocabTaskConfig())

	got := optionValues(interactivePriorityOptions())
	want := []string{"URGENT", "NORMAL", "LATER"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("priority options = %v, want %v", got, want)
	}

	// Every offered value must survive the same gate the form's submit
	// runs it through. This is the assertion that actually pins the bug.
	for _, v := range got {
		if _, ok := NormalizePriority(v); !ok {
			t.Errorf("offered priority %q is rejected by NormalizePriority", v)
		}
	}
}

// TestInteractivePriorityOptionsUseConfiguredLabel keeps the "P0
// (Critical)" affordance the hardcoded list had, sourced from the
// definition's Label instead of a literal.
func TestInteractivePriorityOptionsUseConfiguredLabel(t *testing.T) {
	withTaskConfig(t, renamedVocabTaskConfig())

	got := optionKeys(interactivePriorityOptions())
	want := []string{"URGENT (Critical)", "NORMAL (Normal)", "LATER (Whenever)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("priority labels = %v, want %v", got, want)
	}
}

// TestInteractivePriorityOptionsBareNameWithoutLabel: a definition with
// no label must not render a dangling "NAME ()".
func TestInteractivePriorityOptionsBareNameWithoutLabel(t *testing.T) {
	withTaskConfig(t, &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
		Priorities: []config.PriorityDefinition{
			{Name: "HOT"},
			{Name: "COLD", Label: "Cold"},
		},
	})

	got := optionKeys(interactivePriorityOptions())
	want := []string{"HOT", "COLD (Cold)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("priority labels = %v, want %v", got, want)
	}
}

// TestInteractiveEffortOptionsFollowConfig covers the field the form
// never had: create -e exists, so the form silently stored no effort at
// all. Offering it is only safe if the offer follows config.
func TestInteractiveEffortOptionsFollowConfig(t *testing.T) {
	withTaskConfig(t, renamedVocabTaskConfig())

	got := optionValues(interactiveEffortOptions())
	// A leading empty option keeps "no effort" reachable, since effort
	// is optional and a select has no other way to say nothing.
	if len(got) == 0 || got[0] != "" {
		t.Fatalf("effort options = %v, want a leading empty option", got)
	}
	if strings.Join(got[1:], ",") != "QUICK,DEEP" {
		t.Fatalf("effort options = %v, want ['', QUICK, DEEP]", got)
	}
	for _, v := range got[1:] {
		if _, ok := NormalizeEffort(v); !ok {
			t.Errorf("offered effort %q is rejected by NormalizeEffort", v)
		}
	}
}

// TestInteractiveStatusOptionsFollowConfig re-pins the status list that
// was already fixed on this branch, so a later edit cannot regress the
// pattern the other two now copy.
func TestInteractiveStatusOptionsFollowConfig(t *testing.T) {
	withTaskConfig(t, &config.TaskConfig{
		Statuses: []config.StatusDefinition{
			{Name: "OPEN", Label: "Open", Role: config.RoleInitial},
			{Name: "WORKING", Label: "Working", Role: "active"},
			{Name: "SHIPPED", Label: "Shipped", IsTerminal: true, Role: "completed"},
		},
		StateMachine: &config.WorkflowDefinition{
			Rules: map[string][]string{"OPEN": {"WORKING"}, "WORKING": {"SHIPPED"}},
		},
	})

	got := optionValues(interactiveStatusOptions(core.DefaultWorkflow()))
	want := []string{"OPEN", "WORKING", "SHIPPED"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("status options = %v, want %v", got, want)
	}
}

// TestInteractiveTagOptionsClosedPolicy is the second rejection path:
// under tags.policy closed, ValidateTags refuses anything outside the
// vocabulary, so the hardcoded feat/fix/chore/docs/urgent list failed on
// submit exactly the way priority did.
func TestInteractiveTagOptionsClosedPolicy(t *testing.T) {
	withTaskConfig(t, renamedVocabTaskConfig())

	got := optionValues(interactiveTagOptions())
	if len(got) == 0 {
		t.Fatal("closed policy offered no tags at all")
	}
	// Every offered tag must pass the very gate saveTask applies.
	if err := core.ValidateTags(got); err != nil {
		t.Errorf("offered tags rejected by the policy that gates submit: %v", err)
	}
	// The declared project vocabulary must actually be reachable.
	for _, want := range []string{"storage", "cli"} {
		if !containsString(got, want) {
			t.Errorf("declared tag %q not offered: %v", want, got)
		}
	}
	// The bare pre-rename spellings this branch removed must be gone.
	for _, gone := range []string{"feat", "fix", "chore", "docs", "urgent"} {
		if containsString(got, gone) {
			t.Errorf("bare pre-rename tag %q still offered: %v", gone, got)
		}
	}
}

// TestInteractiveTagOptionsClosedPolicyDropsWildcards: a `domain:*`
// entry opens a namespace, it is not itself a taggable value. Offering
// the literal string `domain:*` would seed a tag the policy rejects.
func TestInteractiveTagOptionsClosedPolicyDropsWildcards(t *testing.T) {
	withTaskConfig(t, &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
		Tags: config.TagsConfig{
			Policy:  config.TagPolicyClosed,
			Allowed: []string{"domain:*", "storage"},
		},
	})

	got := interactiveTagOptions()
	values := optionValues(got)
	if containsString(values, "domain:*") {
		t.Errorf("wildcard opener offered as a literal tag: %v", values)
	}
	if err := core.ValidateTags(values); err != nil {
		t.Errorf("offered tags rejected by the policy: %v", err)
	}
}

// TestInteractiveTagOptionsOpenPolicy: with no vocabulary declared, the
// composed axes are still the right suggestions — they are the tags
// tlc's own `label init` and `sync` write — and they must be the RENAMED
// ones, never the bare feat/fix the branch removed.
func TestInteractiveTagOptionsOpenPolicy(t *testing.T) {
	withTaskConfig(t, &config.TaskConfig{
		Statuses:     config.GetDefaultStatuses(),
		StateMachine: config.GetDefaultStateMachine(),
	})

	got := optionValues(interactiveTagOptions())
	if !containsString(got, "type:feat") {
		t.Errorf("open policy should still suggest the composed axes: %v", got)
	}
	for _, gone := range []string{"feat", "fix", "chore", "docs", "urgent"} {
		if containsString(got, gone) {
			t.Errorf("bare pre-rename tag %q still offered: %v", gone, got)
		}
	}
	// Open policy admits everything, so the suggestions cannot be wrong,
	// but they must still round-trip.
	if err := core.ValidateTags(got); err != nil {
		t.Errorf("suggested tags rejected: %v", err)
	}
}

// TestInteractiveTagOptionsFollowRenamedStatusVocabulary proves the tag
// suggestions are COMPOSED from config rather than a second hardcoded
// list one rename behind.
func TestInteractiveTagOptionsFollowRenamedStatusVocabulary(t *testing.T) {
	withTaskConfig(t, &config.TaskConfig{
		Statuses: []config.StatusDefinition{
			{Name: "OPEN", Label: "Open", Role: config.RoleInitial},
			{Name: "WORKING", Label: "Working", Role: "active"},
			{Name: "SHIPPED", Label: "Shipped", IsTerminal: true, Role: "completed"},
		},
		StateMachine: &config.WorkflowDefinition{
			Rules: map[string][]string{"OPEN": {"WORKING"}, "WORKING": {"SHIPPED"}},
		},
		Priorities: []config.PriorityDefinition{{Name: "URGENT"}, {Name: "LATER"}},
	})

	got := optionValues(interactiveTagOptions())
	for _, want := range []string{"status:working", "priority:urgent"} {
		if !containsString(got, want) {
			t.Errorf("renamed axis value %q not offered: %v", want, got)
		}
	}
	if containsString(got, "status:in-progress") {
		t.Errorf("built-in status axis leaked under a renamed vocabulary: %v", got)
	}
}
