package main

import (
	"testing"

	"github.com/google/go-github/v69/github"
)

// customVocab is a vocabulary that shares no value name with the
// built-in one, so any assertion it satisfies is satisfied by config
// rather than by a hardcoded table that happens to agree.
func customVocab() *Vocabulary {
	return &Vocabulary{
		PriorityToLabel: map[string]string{
			"DROP_EVERYTHING": "priority:drop-everything",
			"NORMAL":          "priority:normal",
			"LATER":           "priority:later",
		},
		EffortToLabel: map[string]string{
			"TINY": "effort:tiny",
			"HUGE": "effort:huge",
		},
		ActiveStatuses: map[string]string{
			"DOING":     "status:doing",
			"IN_REVIEW": "status:in-review",
		},
	}
}

// TestCustomPriorityRoundTrip is the regression this whole change
// exists for: a task carrying a configured priority must come back
// from the forge in the TYPED Priority field, not degrade into a tag.
func TestCustomPriorityRoundTrip(t *testing.T) {
	v := customVocab()

	task := &Task{Status: "todo", Priority: "DROP_EVERYTHING"}
	pushed := buildPushLabelsWith(task, v)

	var found bool
	for _, l := range pushed {
		if l == "priority:drop-everything" {
			found = true
		}
	}
	if !found {
		t.Fatalf("push dropped the configured priority: got %v", pushed)
	}

	// Pull the same label back.
	got := &Task{}
	ghLabels := make([]*github.Label, 0, len(pushed))
	for i := range pushed {
		ghLabels = append(ghLabels, &github.Label{Name: &pushed[i]})
	}
	mapLabelsToTaskWith(ghLabels, got, v)

	if got.Priority != "DROP_EVERYTHING" {
		t.Errorf("Priority = %q, want %q", got.Priority, "DROP_EVERYTHING")
	}
	for _, tag := range got.Tags {
		if tag == "priority:drop-everything" {
			t.Errorf("priority degraded into a tag: %v", got.Tags)
		}
	}
}

// TestCustomEffortRoundTrip is the effort axis equivalent.
func TestCustomEffortRoundTrip(t *testing.T) {
	v := customVocab()

	task := &Task{Status: "todo", Effort: "HUGE"}
	pushed := buildPushLabelsWith(task, v)

	got := &Task{}
	ghLabels := make([]*github.Label, 0, len(pushed))
	for i := range pushed {
		ghLabels = append(ghLabels, &github.Label{Name: &pushed[i]})
	}
	mapLabelsToTaskWith(ghLabels, got, v)

	if got.Effort != "HUGE" {
		t.Errorf("Effort = %q, want %q", got.Effort, "HUGE")
	}
}

// TestCustomActiveStatusPushed proves a renamed active status still
// gets a status label, which a name-matched in_progress check cannot do.
func TestCustomActiveStatusPushed(t *testing.T) {
	v := customVocab()

	pushed := buildPushLabelsWith(&Task{Status: "DOING"}, v)
	var found bool
	for _, l := range pushed {
		if l == "status:doing" {
			found = true
		}
	}
	if !found {
		t.Errorf("push dropped the configured active status: got %v", pushed)
	}
}

// TestNilVocabularyMatchesBuiltin is the behavior-identical guarantee:
// with no vocabulary on the wire the plugin must emit exactly what it
// emitted before this change.
func TestNilVocabularyMatchesBuiltin(t *testing.T) {
	blocked := "blocked by #1"
	cases := []*Task{
		{Status: "in_progress", Priority: "P0", Effort: "XS"},
		{Status: "todo", Priority: "P3", Effort: "XL"},
		{Status: "todo", BlockedReason: &blocked},
		{Status: "done", Priority: "P2", Effort: "M", Tags: []string{"domain:cli"}},
	}
	for _, task := range cases {
		want := buildPushLabels(task)
		got := buildPushLabelsWith(task, nil)
		if len(want) != len(got) {
			t.Fatalf("len mismatch for %+v: want %v got %v", task, want, got)
		}
		for i := range want {
			if want[i] != got[i] {
				t.Errorf("label %d for %+v: want %q got %q", i, task, want[i], got[i])
			}
		}
	}
}

// TestBuiltinPullUnchangedUnderNilVocabulary pins the pull direction.
func TestBuiltinPullUnchangedUnderNilVocabulary(t *testing.T) {
	names := []string{"priority:critical", "effort:xs", "status:in-progress", "domain:cli"}
	ghLabels := make([]*github.Label, 0, len(names))
	for i := range names {
		ghLabels = append(ghLabels, &github.Label{Name: &names[i]})
	}

	task := &Task{}
	inProgress, blocked := mapLabelsToTaskWith(ghLabels, task, nil)

	if task.Priority != "P0" {
		t.Errorf("Priority = %q, want P0", task.Priority)
	}
	if task.Effort != "XS" {
		t.Errorf("Effort = %q, want XS", task.Effort)
	}
	if !inProgress {
		t.Error("in-progress flag not set")
	}
	if blocked {
		t.Error("blocked flag set unexpectedly")
	}
	if len(task.Tags) != 1 || task.Tags[0] != "domain:cli" {
		t.Errorf("Tags = %v, want [domain:cli]", task.Tags)
	}
}

// TestBlockedStatusLabelResolvesThroughVocabulary pins that a vocabulary
// declaring a blocked status turns the status:blocked label into that
// status on pull, and writes the same label on push, instead of the
// blocked-flag fallback.
func TestBlockedStatusLabelResolvesThroughVocabulary(t *testing.T) {
	v := &Vocabulary{
		ActiveStatuses: map[string]string{
			"in_progress": "status:in-progress",
			"blocked":     "status:blocked",
		},
		InitialStatus: "todo",
	}

	name := "status:blocked"
	task := &Task{}
	active, hasBlocked := classifyLabels([]*github.Label{{Name: &name}}, task, v)
	if active != "blocked" || hasBlocked {
		t.Errorf("classifyLabels = (%q, %v), want (blocked, false)", active, hasBlocked)
	}

	pushed := buildPushLabelsWith(&Task{Status: "blocked"}, v)
	if len(pushed) != 1 || pushed[0] != "status:blocked" {
		t.Errorf("push labels = %v, want [status:blocked]", pushed)
	}

	// No blocked status in the vocabulary: the flag fallback holds.
	active, hasBlocked = classifyLabels([]*github.Label{{Name: &name}}, &Task{}, customVocab())
	if active != "" || !hasBlocked {
		t.Errorf("fallback classifyLabels = (%q, %v), want (\"\", true)", active, hasBlocked)
	}
}
