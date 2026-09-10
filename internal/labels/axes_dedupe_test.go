package labels

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// withStatuses installs a status vocabulary for one test and restores
// whatever was registered before.
//
// It drives core.ConfiguredTaskStatusDefinitions, which is what
// statusAxis reads, so a test can state what a USER's config produces
// rather than only what the built-ins produce. The built-in vocabulary
// cannot exhibit either collision below — that is precisely why both
// shipped unnoticed.
func withStatuses(t *testing.T, defs []config.StatusDefinition) {
	t.Helper()
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{
			Statuses:     defs,
			StateMachine: config.GetDefaultStateMachine(),
		}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})
}

// countByName tallies how many times each label name appears.
func countByName(labels []Label) map[string]int {
	n := make(map[string]int, len(labels))
	for _, l := range labels {
		n[l.Name]++
	}
	return n
}

// TestDeclaredBlockedStatusDoesNotDuplicateFixedEntry pins collision (a).
//
// statusAxis generates a label for every non-terminal, non-initial
// status and THEN appends a fixed `status:blocked`. A vocabulary that
// declares its own non-terminal BLOCKED therefore emits the name twice
// with two different colors, and the comment above the fixed entry
// anticipates exactly that config without guarding it.
//
// The harm is not cosmetic. `sync push` creates the label from the first
// entry and then updates it from the second, so which color survives is
// decided by slice order rather than by anything the user declared.
func TestDeclaredBlockedStatusDoesNotDuplicateFixedEntry(t *testing.T) {
	withStatuses(t, []config.StatusDefinition{
		{Name: "TODO", Label: "To Do", Role: config.RoleInitial, TLSMarker: " "},
		{Name: "BLOCKED", Label: "Blocked", Color: "purple", Role: config.RoleActive, TLSMarker: "b"},
		{Name: "DONE", Label: "Done", IsTerminal: true, Role: config.RoleCompleted, TLSMarker: "x"},
	})

	got := countByName(GetTemplates(TypeGeneric))
	if got["status:blocked"] != 1 {
		t.Errorf("status:blocked emitted %d times, want 1", got["status:blocked"])
	}
}

// TestDistinctStatusNamesCollapsingToOneLabelAreDeduped pins collision (b).
//
// labelValue lowercases and turns `_` into `-`, so IN_REVIEW and
// in-review are two names config validation accepts as distinct — it
// rejects duplicate status NAMES, and these are not duplicates — that
// render to one label value. Nothing between validation and the forge
// notices.
func TestDistinctStatusNamesCollapsingToOneLabelAreDeduped(t *testing.T) {
	withStatuses(t, []config.StatusDefinition{
		{Name: "TODO", Label: "To Do", Role: config.RoleInitial, TLSMarker: " "},
		{Name: "IN_REVIEW", Label: "In Review", Color: "purple", Role: config.RoleActive, TLSMarker: "r"},
		{Name: "in-review", Label: "In review", Color: "green", Role: config.RoleActive, TLSMarker: "R"},
		{Name: "DONE", Label: "Done", IsTerminal: true, Role: config.RoleCompleted, TLSMarker: "x"},
	})

	got := countByName(GetTemplates(TypeGeneric))
	if got["status:in-review"] != 1 {
		t.Errorf("status:in-review emitted %d times, want 1", got["status:in-review"])
	}
}

// TestDedupeKeepsFirstOccurrence states WHICH duplicate survives.
//
// Skipping the later entry rather than the earlier one is what makes the
// outcome depend on declaration order the user controls instead of on
// the order the axes happen to be concatenated in. For a declared
// BLOCKED that means the user's own color wins over the fixed entry's,
// which is the answer a user who bothered to declare the status would
// expect.
func TestDedupeKeepsFirstOccurrence(t *testing.T) {
	withStatuses(t, []config.StatusDefinition{
		{Name: "TODO", Label: "To Do", Role: config.RoleInitial, TLSMarker: " "},
		{Name: "BLOCKED", Label: "Blocked", Color: "purple", Role: config.RoleActive, TLSMarker: "b"},
		{Name: "DONE", Label: "Done", IsTerminal: true, Role: config.RoleCompleted, TLSMarker: "x"},
	})

	var got string
	for _, l := range GetTemplates(TypeGeneric) {
		if l.Name == "status:blocked" {
			got = l.Color
			break
		}
	}
	// "purple" resolves to 5319E7; the fixed entry carries its own color.
	if want := resolveColor("purple"); got != want {
		t.Errorf("status:blocked color = %q, want the declared status's %q", got, want)
	}
}

// TestDedupeReportsWhatItDropped is the reason a skip is tolerable at
// all.
//
// resolveColor argues that one off-palette label beats a `label init`
// that refuses to run, and the same argument applies here — but a
// SILENTLY dropped label with a silently different color is the defect,
// not the fix. The conflict list is what lets the CLI say so out loud
// while still emitting a usable set.
func TestDedupeReportsWhatItDropped(t *testing.T) {
	withStatuses(t, []config.StatusDefinition{
		{Name: "TODO", Label: "To Do", Role: config.RoleInitial, TLSMarker: " "},
		{Name: "BLOCKED", Label: "Blocked", Color: "purple", Role: config.RoleActive, TLSMarker: "b"},
		{Name: "DONE", Label: "Done", IsTerminal: true, Role: config.RoleCompleted, TLSMarker: "x"},
	})

	_, conflicts := GetTemplatesWithConflicts(TypeGeneric)
	if len(conflicts) != 1 {
		t.Fatalf("got %d conflicts, want 1: %+v", len(conflicts), conflicts)
	}
	c := conflicts[0]
	if c.Name != "status:blocked" {
		t.Errorf("conflict name = %q, want status:blocked", c.Name)
	}
	if c.Kept == c.Dropped {
		t.Errorf("conflict reports identical colors %q; a same-color duplicate is not worth reporting", c.Kept)
	}
	if !strings.Contains(c.String(), "status:blocked") {
		t.Errorf("conflict message %q does not name the label", c.String())
	}
}

// TestNoConflictsForBuiltinVocabulary guards the warning against noise.
//
// A warning that fires on the default config would train users to
// ignore it, so the built-in vocabulary must produce none.
func TestNoConflictsForBuiltinVocabulary(t *testing.T) {
	for _, pt := range allProjectTypes {
		if _, conflicts := GetTemplatesWithConflicts(pt); len(conflicts) != 0 {
			t.Errorf("%s: built-in vocabulary reports conflicts %+v", pt, conflicts)
		}
	}
}
