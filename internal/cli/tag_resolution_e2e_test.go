package cli

// End-to-end coverage for `tlc tag <task-ref> <tag…>` task resolution.
//
// `T-NNNN` is a DISPLAY ALIAS, rendered on demand from (project_id, seq)
// and never stored; the durable identity is the TypeID. `tlc tag` used to
// hand its argument straight to uri.Resolver, which looks up stored
// identifiers only — so the single form users actually type returned
// NOT_FOUND for a task that plainly existed, under the default tag policy
// as much as a closed one.
//
// Every case here asserts the tag LANDED, read back through `task show`,
// rather than only that the command exited 0. A command that resolved the
// task and then silently failed to write would exit 0 too.

import (
	"encoding/json"
	"strings"
	"testing"
)

// tagResolutionFixture builds an isolated world under the built-in
// vocabulary — this defect has nothing to do with the status vocabulary
// and must be shown to be independent of it.
func tagResolutionFixture(t *testing.T) (bin, home string, env []string) {
	t.Helper()
	return statusVocabFixture(t, defaultStatusConfig)
}

// tagsOf reads a task's tags back through `task show`, so the assertion
// is on persisted state rather than on the command's own report of
// itself.
func tagsOf(t *testing.T, bin, home string, env []string, ref string) string {
	t.Helper()
	return runTLCOK(t, bin, home, env, "task", "show", ref)
}

// TestTagResolvesDisplayAlias is the headline regression: the form the
// help text tells users to type must work.
func TestTagResolvesDisplayAlias(t *testing.T) {
	bin, home, env := tagResolutionFixture(t)

	runTLCOK(t, bin, home, env, "task", "create", "tag me")

	out, code := runTLC(t, bin, home, env, "tag", "T-0001", "sometag")
	if code != 0 {
		t.Fatalf("`tlc tag T-0001 sometag` failed (exit %d):\n%s", code, out)
	}

	// Assert the tag actually persisted. Exiting 0 is not evidence: the
	// defect's whole shape was a command that looked fine and did not
	// reach the task.
	if shown := tagsOf(t, bin, home, env, "T-0001"); !strings.Contains(shown, "sometag") {
		t.Errorf("tag did not land on T-0001:\n%s", shown)
	}
}

// TestTagResolvesTypeID pins the durable identity. The fix must not trade
// one identifier for the other.
//
// This form was broken differently and more quietly: a TypeID did not
// match the router pattern at all, so `tlc tag task_01h… sometag` was
// read as a request to FILTER by two tags and answered with an empty
// listing at exit 0 — no error, no write, nothing to notice.
func TestTagResolvesTypeID(t *testing.T) {
	bin, home, env := tagResolutionFixture(t)

	runTLCOK(t, bin, home, env, "task", "create", "tag me by typeid")
	typeID := taskTypeIDOf(t, bin, home, env, "T-0001")

	out, code := runTLC(t, bin, home, env, "tag", typeID, "typeidtag")
	if code != 0 {
		t.Fatalf("`tlc tag %s typeidtag` failed (exit %d):\n%s", typeID, code, out)
	}
	if shown := tagsOf(t, bin, home, env, "T-0001"); !strings.Contains(shown, "typeidtag") {
		t.Errorf("tag did not land when addressed by TypeID:\n%s", shown)
	}
}

// TestTagMergesWithExistingTags pins that resolution feeds the merge
// rather than replacing the row: a task addressed by alias keeps the tags
// it already had.
func TestTagMergesWithExistingTags(t *testing.T) {
	bin, home, env := tagResolutionFixture(t)

	runTLCOK(t, bin, home, env, "task", "create", "already tagged", "--tag", "existing")
	runTLCOK(t, bin, home, env, "tag", "T-0001", "added")

	shown := tagsOf(t, bin, home, env, "T-0001")
	for _, want := range []string{"existing", "added"} {
		if !strings.Contains(shown, want) {
			t.Errorf("expected tag %q on the task, got:\n%s", want, shown)
		}
	}
}

// TestTagUnknownAliasStillFails is the negative control. Making aliases
// resolve must not make every alias resolve — a reference to a task that
// does not exist has to stay an error, or the fix would have turned a
// wrong-ID typo into a silent no-op.
func TestTagUnknownAliasStillFails(t *testing.T) {
	bin, home, env := tagResolutionFixture(t)

	runTLCOK(t, bin, home, env, "task", "create", "the only task")

	out, code := runTLC(t, bin, home, env, "tag", "T-9999", "sometag")
	if code == 0 {
		t.Fatalf("`tlc tag T-9999` should fail — no such task:\n%s", out)
	}
	if !strings.Contains(out, "T-9999") {
		t.Errorf("the error should name the ref the user typed, got:\n%s", out)
	}
}

// TestTagFilterModeStillRoutes guards the router. `tlc tag` is
// overloaded — the same verb means "add tags" or "filter by tags"
// depending on whether the first argument looks like a task reference —
// so widening what counts as a task reference risks swallowing tag names.
//
// A bare number is deliberately NOT a task reference here: it is far more
// plausibly a tag, and routing it to add mode would break filtering on
// numeric tags to fix a form nothing documents.
func TestTagFilterModeStillRoutes(t *testing.T) {
	bin, home, env := tagResolutionFixture(t)

	runTLCOK(t, bin, home, env, "task", "create", "numeric tag holder", "--tag", "42")
	runTLCOK(t, bin, home, env, "task", "create", "word tag holder", "--tag", "backend")

	// Two tag names, neither a task ref → filter mode, ANDed.
	out := runTLCOK(t, bin, home, env, "tag", "backend")
	if !strings.Contains(out, "T-0002") {
		t.Errorf("filter mode should find the backend-tagged task, got:\n%s", out)
	}

	// A bare number stays a tag, not a task reference.
	numeric := runTLCOK(t, bin, home, env, "tag", "42")
	if !strings.Contains(numeric, "T-0001") {
		t.Errorf("`tlc tag 42` should filter by the tag \"42\", got:\n%s", numeric)
	}
}

// TestTagResolvesLowercaseAlias pins the casing the router used to drop.
// Every other task-argument command accepts `t-0001`; `tlc tag` routed it
// to filter mode and answered "no tasks tagged t-0001" at exit 0.
func TestTagResolvesLowercaseAlias(t *testing.T) {
	bin, home, env := tagResolutionFixture(t)

	runTLCOK(t, bin, home, env, "task", "create", "lowercase ref")
	runTLCOK(t, bin, home, env, "tag", "t-0001", "lowertag")

	if shown := tagsOf(t, bin, home, env, "T-0001"); !strings.Contains(shown, "lowertag") {
		t.Errorf("tag did not land when addressed as t-0001:\n%s", shown)
	}
}

// taskTypeIDOf digs the durable TypeID out of `task show --format json`,
// so a test can address a task by the identity that is actually stored.
//
// Parsed as JSON rather than scanned for a "task_" prefix: the log
// entries carry a `task_id` KEY, and a scan picks up the key name itself
// as if it were an id. That produced a fake failure — `tlc tag task_id
// sometag` routes to filter mode, finds nothing, and exits 0, which
// reads exactly like a resolution bug.
func taskTypeIDOf(t *testing.T, bin, home string, env []string, alias string) string {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "task", "show", alias, "--format", "json")

	var payload struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse task show json: %v\n%s", err, out)
	}
	if !strings.HasPrefix(payload.Task.ID, "task_") {
		t.Fatalf("task id %q is not a TypeID:\n%s", payload.Task.ID, out)
	}
	return payload.Task.ID
}
