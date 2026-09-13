package core

import (
	"reflect"
	"testing"
)

// TestTaskProvenance_RoundTrip pins the Meta keys recipe materialization
// writes and the executor/ledger read back. They are named here so a
// rename on either side breaks a test rather than silently orphaning
// tasks from their recipe.
func TestTaskProvenance_RoundTrip(t *testing.T) {
	task := &Task{ID: "task_x"}
	want := TaskProvenance{
		Recipe:        "code-review",
		RecipeVersion: "1.2.0",
		RecipeHash:    "sha256:abc",
		Subject:       "task_subject",
	}
	task.SetProvenance(want)

	if got := task.Provenance(); !reflect.DeepEqual(got, want) {
		t.Errorf("Provenance() = %+v; want %+v", got, want)
	}
	for key, want := range map[string]string{
		"recipe":         "code-review",
		"recipe_version": "1.2.0",
		"recipe_hash":    "sha256:abc",
		"subject":        "task_subject",
	} {
		if got, _ := task.Meta[key].(string); got != want {
			t.Errorf("Meta[%q] = %q; want %q", key, got, want)
		}
	}
}

// TestTaskSetProvenance_EmptyFieldsRemoveKeys mirrors SetBlockedBy: an
// empty value deletes its key instead of writing "", so Meta never
// carries blank provenance and unrelated keys are left alone.
func TestTaskSetProvenance_EmptyFieldsRemoveKeys(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{"blocked_by": []string{"task_a"}}}
	task.SetProvenance(TaskProvenance{
		Recipe: "r", RecipeVersion: "1", RecipeHash: "h", Subject: "s",
	})

	task.SetProvenance(TaskProvenance{Recipe: "r"})
	for _, key := range []string{"recipe_version", "recipe_hash", "subject"} {
		if _, ok := task.Meta[key]; ok {
			t.Errorf("Meta[%q] still present after clearing", key)
		}
	}
	if got := task.Provenance(); got.Recipe != "r" || got.RecipeVersion != "" {
		t.Errorf("Provenance() = %+v; want only Recipe=r", got)
	}

	task.SetProvenance(TaskProvenance{})
	if _, ok := task.Meta["recipe"]; ok {
		t.Error("Meta[recipe] still present after clearing everything")
	}
	if got := task.BlockedBy(); len(got) != 1 || got[0] != "task_a" {
		t.Errorf("unrelated blocked_by damaged: %v", got)
	}
}

// TestTaskProvenance_NilSafe: readers run over tasks from any source,
// including nil results of lookups, and must not panic.
func TestTaskProvenance_NilSafe(t *testing.T) {
	var nilTask *Task
	if got := nilTask.Provenance(); got != (TaskProvenance{}) {
		t.Errorf("nil task provenance = %+v; want zero", got)
	}
	nilTask.SetProvenance(TaskProvenance{Recipe: "r"}) // must not panic

	noMeta := &Task{}
	if got := noMeta.Provenance(); got != (TaskProvenance{}) {
		t.Errorf("nil-Meta provenance = %+v; want zero", got)
	}
	noMeta.SetProvenance(TaskProvenance{Recipe: "r"})
	if got := noMeta.Provenance().Recipe; got != "r" {
		t.Errorf("SetProvenance on nil Meta lost the value: %q", got)
	}
}

// TestTaskProvenance_IgnoresNonStringMeta: Meta is user-editable JSON;
// a non-string value under a provenance key reads as unset rather than
// panicking or stringifying garbage.
func TestTaskProvenance_IgnoresNonStringMeta(t *testing.T) {
	task := &Task{Meta: map[string]interface{}{"recipe": 42, "subject": []string{"x"}}}
	if got := task.Provenance(); got != (TaskProvenance{}) {
		t.Errorf("non-string meta read as %+v; want zero", got)
	}
}
