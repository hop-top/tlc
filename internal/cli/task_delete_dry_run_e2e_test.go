package cli

// `task delete` advertises dry-run support through its
// kit/side-effect: destructive-local tier, but its RunE carried no
// guard: the row was removed, the delete note was logged against it
// first, and the command printed "Deleted task T-NNNN" and exited 0.
//
// It is the most destructive leaf in the task surface, and the one a
// caller is most likely to rehearse with --dry-run before committing
// to it, so it is worth its own regression rather than only the
// coverage the conformance gate gives it.

import (
	"strings"
	"testing"
)

// TestTaskDeleteDryRunWritesNothing is the headline regression: the row
// survives, and so does the log table.
//
// The note is passed because the default delete-requires-note policy
// vetoes an empty one — a dry run has to clear the same gates a real
// run does, so the preview must be refused by the policy rather than
// by the absence of a note.
func TestTaskDeleteDryRunWritesNothing(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "delete me")
	runTLCOK(t, bin, home, env, "task", "create", "keep me")

	out := runTLCOK(t, bin, home, env,
		"task", "delete", "T-0001", "--note", "obsolete", "--no-prompt", "--dry-run")

	if !strings.Contains(out, "Dry run") {
		t.Errorf("dry-run output does not announce itself:\n%s", out)
	}
	if strings.Contains(out, "Deleted task") {
		t.Errorf("dry-run reported a delete:\n%s", out)
	}

	listed := runTLCOK(t, bin, home, env, "task", "list")
	if !strings.Contains(listed, "delete me") {
		t.Errorf("dry run deleted the task:\n%s", listed)
	}
	if !strings.Contains(listed, "keep me") {
		t.Errorf("the untargeted task went missing:\n%s", listed)
	}
}

// TestTaskDeleteDryRunWritesNoLog pins the second write. The delete
// note is recorded against the task BEFORE the row is removed, so a
// guard placed only around DeleteTask would leave an audit entry
// claiming a deletion that never happened.
func TestTaskDeleteDryRunWritesNoLog(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "delete me")
	before := runTLCOK(t, bin, home, env, "task", "show", "T-0001", "--format", "json")

	runTLCOK(t, bin, home, env,
		"task", "delete", "T-0001", "--note", "obsolete", "--no-prompt", "--dry-run")

	after := runTLCOK(t, bin, home, env, "task", "show", "T-0001", "--format", "json")
	if before != after {
		t.Errorf("dry run changed stored state.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if strings.Contains(after, "obsolete") {
		t.Errorf("dry run wrote the delete note to the log:\n%s", after)
	}
}

// TestTaskDeleteDryRunStillRefusedByPolicy keeps the flag from becoming
// an escape hatch around the delete-requires-note policy. A preview of
// a delete that policy would veto must be vetoed too, otherwise
// --dry-run reports a delete that cannot happen.
func TestTaskDeleteDryRunStillRefusedByPolicy(t *testing.T) {
	bin, home, env := statusVocabFixture(t, customStatusConfig)

	runTLCOK(t, bin, home, env, "task", "create", "delete me")

	out, code := runTLC(t, bin, home, env,
		"task", "delete", "T-0001", "--no-prompt", "--dry-run")
	if code == 0 {
		t.Errorf("dry-run delete without a note was accepted (exit 0):\n%s", out)
	}
}
