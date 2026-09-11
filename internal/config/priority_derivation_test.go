package config

// Config-level coverage for the priority-derivation rule set: which
// validation path it rides, and what it does to a config that declares
// none.

import (
	"strings"
	"testing"
	"time"
)

// TestPriorityDerivationRidesTheAdvisoryPath pins the split that keeps a
// broken rule set from bricking the tool.
//
// Validate (the CLI's warning-only path) reports it, so the user is told.
// ValidateWorkflow (DefaultWorkflow's FATAL path) does NOT, so `task
// list` still runs. Inverting either half is a real regression: the fatal
// version turns a cosmetic rule typo into a tool that refuses to start,
// and dropping it from Validate makes the mistake silent everywhere the
// derivation command is not run.
func TestPriorityDerivationRidesTheAdvisoryPath(t *testing.T) {
	cfg := &TaskConfig{
		PriorityDeriv: PriorityDerivationConfig{
			Rules: []PriorityDerivationRule{
				{Name: "a", DueWithin: time.Hour, Then: "P0"},
				{Name: "a", MinAge: time.Hour, Then: "P1"},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Error("Validate accepted a duplicate rule name; the advisory path must report it")
	} else if !strings.Contains(err.Error(), "duplicate rule name") {
		t.Errorf("Validate error = %v, want a duplicate-rule-name diagnosis", err)
	}

	if err := cfg.ValidateWorkflow(); err != nil {
		t.Errorf("ValidateWorkflow rejected a broken rule set (%v); a derivation "+
			"typo must not stop every command in the tool", err)
	}
}

// TestNoDerivationRulesValidatesClean pins that derivation off is not an
// error on any path.
func TestNoDerivationRulesValidatesClean(t *testing.T) {
	cfg := &TaskConfig{}
	if err := cfg.ValidatePriorityDerivation(); err != nil {
		t.Errorf("an undeclared rule set was rejected: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate rejected a config with no derivation rules: %v", err)
	}
}

// TestDerivationRulesValidateAgainstConfiguredVocabulary pins that `then`
// is checked against the USER's priority names, not the built-in four. A
// check against the built-ins would reject every rule a user with a
// renamed vocabulary writes.
func TestDerivationRulesValidateAgainstConfiguredVocabulary(t *testing.T) {
	cfg := &TaskConfig{
		Priorities: []PriorityDefinition{{Name: "URGENT"}, {Name: "LATER"}},
		PriorityDeriv: PriorityDerivationConfig{
			Rules: []PriorityDerivationRule{
				{Name: "soon", DueWithin: time.Hour, Then: "URGENT"},
			},
		},
	}
	if err := cfg.ValidatePriorityDerivation(); err != nil {
		t.Errorf("a rule naming a configured priority was rejected: %v", err)
	}

	// And a built-in name is now the INVALID one, since the user replaced
	// the vocabulary.
	cfg.PriorityDeriv.Rules[0].Then = "P0"
	err := cfg.ValidatePriorityDerivation()
	if err == nil {
		t.Fatal("a rule naming P0 was accepted against a vocabulary that has no P0")
	}
	if !strings.Contains(err.Error(), "URGENT") {
		t.Errorf("rejection did not name the configured vocabulary: %v", err)
	}
}

// TestConditionKeyExcludesNameAndVerdict pins what "identical conditions"
// compares. Two rules assigning the SAME priority on different conditions
// are fine; two rules assigning DIFFERENT priorities on the same
// condition are the ambiguity.
func TestConditionKeyExcludesNameAndVerdict(t *testing.T) {
	sameVerdictDifferentConditions := &TaskConfig{
		PriorityDeriv: PriorityDerivationConfig{
			Rules: []PriorityDerivationRule{
				{Name: "a", DueWithin: time.Hour, Then: "P0"},
				{Name: "b", MinDependents: 3, Then: "P0"},
			},
		},
	}
	if err := sameVerdictDifferentConditions.ValidatePriorityDerivation(); err != nil {
		t.Errorf("two rules assigning the same priority on different conditions "+
			"were rejected: %v", err)
	}

	sameConditionDifferentVerdicts := &TaskConfig{
		PriorityDeriv: PriorityDerivationConfig{
			Rules: []PriorityDerivationRule{
				{Name: "a", DueWithin: time.Hour, Then: "P0"},
				{Name: "b", DueWithin: time.Hour, Then: "P1"},
			},
		},
	}
	if err := sameConditionDifferentVerdicts.ValidatePriorityDerivation(); err == nil {
		t.Error("two rules with identical conditions were accepted")
	}
}
