package core

import (
	"testing"

	"hop.top/tlc/internal/config"
)

// effortVocab installs a provider declaring the given effort names, in
// order, for the duration of one test.
func effortVocab(t *testing.T, names ...string) {
	t.Helper()
	defs := make([]config.EffortDefinition, 0, len(names))
	for _, n := range names {
		defs = append(defs, config.EffortDefinition{Name: n})
	}
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return &config.TaskConfig{Efforts: defs}
	})
}

// TestValidEffort_ConfiguredVocabulary pins the write-path gate to the
// DECLARED vocabulary.
//
// This is the gate the importers use — internal/vtodo drops an effort it
// considers invalid, and internal/inbox rejects the row — so a gate still
// reading the built-ins would silently discard a user's own sizes on
// every import while the CLI accepted them.
func TestValidEffort_ConfiguredVocabulary(t *testing.T) {
	effortVocab(t, "TINY", "SMALL", "BIG")

	for _, e := range []Effort{"TINY", "SMALL", "BIG"} {
		if !ValidEffort(e) {
			t.Errorf("declared effort %q rejected", e)
		}
	}
	// The built-ins are not declared here, so they are not legal.
	for _, e := range []Effort{EffortXS, EffortXL} {
		if ValidEffort(e) {
			t.Errorf("built-in %q accepted under a renamed vocabulary", e)
		}
	}
	// Effort stays optional regardless of vocabulary.
	if !ValidEffort("") {
		t.Error("empty effort must stay valid: effort is optional")
	}
}

// TestConfiguredEffortStrings_FallsBackToBuiltins pins that a config
// declaring no efforts is indistinguishable from today.
func TestConfiguredEffortStrings_FallsBackToBuiltins(t *testing.T) {
	withTaskConfigProvider(t, func() *config.TaskConfig {
		return &config.TaskConfig{}
	})

	got := ConfiguredEffortStrings()
	want := []string{"XS", "S", "M", "L", "XL"}
	if len(got) != len(want) {
		t.Fatalf("vocabulary = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("vocabulary = %v, want %v", got, want)
		}
	}
}

// TestEffortRank pins that rank is the DECLARATION index, and that the
// unset effort is outside the vocabulary rather than ranked first.
func TestEffortRank(t *testing.T) {
	effortVocab(t, "TINY", "SMALL", "BIG")

	for i, name := range []string{"TINY", "SMALL", "BIG"} {
		got, ok := EffortRank(Effort(name))
		if !ok {
			t.Fatalf("EffortRank(%q) not found", name)
		}
		if got != i {
			t.Errorf("EffortRank(%q) = %d, want %d", name, got, i)
		}
	}

	// Unset is not a size: callers order it last, which they can only do
	// if it reports as absent rather than as rank 0.
	if _, ok := EffortRank(""); ok {
		t.Error("empty effort must report as unranked, not as rank 0")
	}
	if _, ok := EffortRank("XS"); ok {
		t.Error("an undeclared effort must report as unranked")
	}
}

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
