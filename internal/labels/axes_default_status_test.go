package labels

import (
	"testing"

	"hop.top/tlc/internal/config"
)

// TestDefaultVocabularyStatusLabels pins the status axis for the seven
// default statuses: one label for each open, non-initial status, none
// for the initial or terminal ones, and a single status:blocked entry
// generated from the vocabulary.
func TestDefaultVocabularyStatusLabels(t *testing.T) {
	withStatuses(t, config.GetDefaultStatuses())

	labels, conflicts := GetTemplatesWithConflicts(TypeGeneric)
	if len(conflicts) != 0 {
		t.Errorf("default vocabulary produced conflicts: %+v", conflicts)
	}

	got := map[string]string{}
	for _, l := range labels {
		if len(l.Name) > 7 && l.Name[:7] == "status:" {
			got[l.Name] = l.Color
		}
	}
	for _, name := range []string{
		"status:backlog", "status:in-progress", "status:in-review", "status:blocked",
	} {
		if _, ok := got[name]; !ok {
			t.Errorf("missing %s; have %v", name, got)
		}
	}
	for _, name := range []string{"status:todo", "status:done", "status:cancelled"} {
		if _, ok := got[name]; ok {
			t.Errorf("%s generated; initial and terminal statuses get no label", name)
		}
	}
	if got["status:blocked"] != blockedLabelColor {
		t.Errorf("status:blocked color = %q, want %q", got["status:blocked"], blockedLabelColor)
	}
	if countByName(labels)["status:blocked"] != 1 {
		t.Errorf("status:blocked emitted %d times, want 1", countByName(labels)["status:blocked"])
	}
}

// TestStatusAxisGeneratesBlockedOnceForBlockedRole checks the axis itself,
// before any dedupe can hide a duplicate: a vocabulary with a blocked-role
// status must not also get the fixed flag label.
func TestStatusAxisGeneratesBlockedOnceForBlockedRole(t *testing.T) {
	n := 0
	for _, l := range statusAxis(config.GetDefaultStatuses()) {
		if l.Name == "status:blocked" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("status:blocked generated %d times by statusAxis, want 1", n)
	}
}
