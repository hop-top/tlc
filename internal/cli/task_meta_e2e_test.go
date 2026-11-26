package cli

// End-to-end coverage for writing task meta from the CLI: --meta on
// create and update, --unset-meta on update. Driven through the real
// binary so the flag wiring, the shared field-change path and the stored
// row are all exercised, and read back via `task show --format json`.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func showTaskMeta(t *testing.T, bin, home string, env []string, id string) map[string]any {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "task", "show", id, "--format", "json")
	var payload struct {
		Task struct {
			Meta map[string]any `json:"meta"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode task show json: %v\n%s", err, out)
	}
	return payload.Task.Meta
}

func TestTaskMetaCLI_CreateWithMeta(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "task", "create", "with meta",
		"--meta", "owner=ana", "--meta", "url=https://x.test/p?a=1")
	got := showTaskMeta(t, bin, home, env, "T-0001")
	if got["owner"] != "ana" || got["url"] != "https://x.test/p?a=1" {
		t.Fatalf("meta = %v, want owner=ana and url preserved verbatim", got)
	}
}

func TestTaskMetaCLI_CreateKeepsInternalKeys(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "task", "create", "a", "--eva", "check")
	runTLCOK(t, bin, home, env, "task", "create", "b",
		"--blocked-by", "T-0001", "--eva", "e1", "--meta", "owner=ana")
	got := showTaskMeta(t, bin, home, env, "T-0002")
	if got["owner"] != "ana" || got["eva"] == nil || got["blocked_by"] == nil {
		t.Fatalf("meta = %v, want owner plus eva and blocked_by", got)
	}
}

func TestTaskMetaCLI_UpdateSetOverwriteUnset(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "task", "create", "t", "-p", "P1",
		"--eva", "e1", "--meta", "keep=k", "--meta", "owner=ana")

	before := showTaskMeta(t, bin, home, env, "T-0001")
	if before["priority_source"] == nil || before["eva"] == nil {
		t.Fatalf("fixture lacks internal keys: %v", before)
	}

	// Set a new key and overwrite an existing one; the rest is untouched.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001",
		"--meta", "owner=bob", "--meta", "added=1")
	got := showTaskMeta(t, bin, home, env, "T-0001")
	want := map[string]any{}
	for k, v := range before {
		want[k] = v
	}
	want["owner"] = "bob"
	want["added"] = "1"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after set/overwrite meta = %v, want %v", got, want)
	}

	// Unset removes only the named keys; unknown keys are a no-op.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001",
		"--unset-meta", "added", "--unset-meta", "never-there")
	got = showTaskMeta(t, bin, home, env, "T-0001")
	delete(want, "added")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after unset meta = %v, want %v", got, want)
	}

	// Set and unset in one invocation.
	runTLCOK(t, bin, home, env, "task", "update", "T-0001",
		"--meta", "fresh=y", "--unset-meta", "owner")
	got = showTaskMeta(t, bin, home, env, "T-0001")
	delete(want, "owner")
	want["fresh"] = "y"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after set+unset meta = %v, want %v", got, want)
	}
}

func TestTaskMetaCLI_UpdateMultipleTasks(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "task", "create", "one", "--meta", "own=1")
	runTLCOK(t, bin, home, env, "task", "create", "two", "--meta", "own=2")
	runTLCOK(t, bin, home, env, "task", "update", "T-0001", "T-0002", "--meta", "batch=yes")
	for id, own := range map[string]string{"T-0001": "1", "T-0002": "2"} {
		got := showTaskMeta(t, bin, home, env, id)
		if got["batch"] != "yes" || got["own"] != own {
			t.Errorf("%s meta = %v, want batch=yes own=%s", id, got, own)
		}
	}
}

func TestTaskMetaCLI_ReservedKeysRejected(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	// Rejected on create, and nothing is written.
	for _, key := range []string{"eva", "blocked_by", "priority_source", "origin_id"} {
		out, code := runTLC(t, bin, home, env, "task", "create", "bad", "--meta", key+"=x")
		if code == 0 {
			t.Fatalf("create --meta %s=x accepted:\n%s", key, out)
		}
		if !strings.Contains(out, `reserved meta key "`+key+`"`) {
			t.Errorf("create --meta %s: error should name the key as reserved, got:\n%s", key, out)
		}
	}
	if out, code := runTLC(t, bin, home, env, "task", "show", "T-0001"); code == 0 {
		t.Fatalf("a rejected create left a task behind:\n%s", out)
	}

	runTLCOK(t, bin, home, env, "task", "create", "good", "-p", "P1", "--eva", "e1", "--meta", "owner=ana")
	before := showTaskMeta(t, bin, home, env, "T-0001")

	// Rejected on update, set and unset, with other changes in the same
	// invocation left unapplied (no partial write).
	cases := [][]string{
		{"--meta", "eva=x"},
		{"--unset-meta", "eva"},
		{"--unset-meta", "priority_source"},
		{"--meta", "owner=bob", "--meta", "origin_url=u", "--title", "renamed"},
		{"--unset-meta", "owner", "--unset-meta", "blocked_by", "--title", "renamed"},
	}
	for _, args := range cases {
		out, code := runTLC(t, bin, home, env, append([]string{"task", "update", "T-0001"}, args...)...)
		if code == 0 {
			t.Fatalf("update %v accepted:\n%s", args, out)
		}
		if !strings.Contains(out, "reserved meta key") {
			t.Errorf("update %v: error should say reserved meta key, got:\n%s", args, out)
		}
		if got := showTaskMeta(t, bin, home, env, "T-0001"); !reflect.DeepEqual(got, before) {
			t.Fatalf("update %v partially applied: meta = %v, want %v", args, got, before)
		}
	}
	if out := runTLCOK(t, bin, home, env, "task", "show", "T-0001", "--format", "json"); strings.Contains(out, "renamed") {
		t.Fatalf("rejected update still applied --title:\n%s", out)
	}
}

func TestTaskMetaCLI_BadInput(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	if out, code := runTLC(t, bin, home, env, "task", "create", "x", "--meta", "novalue"); code == 0 {
		t.Fatalf("--meta without = accepted:\n%s", out)
	}
	runTLCOK(t, bin, home, env, "task", "create", "x", "--meta", "a=1")
	if out, code := runTLC(t, bin, home, env, "task", "update", "T-0001",
		"--meta", "a=2", "--unset-meta", "a"); code == 0 {
		t.Fatalf("same key set and unset accepted:\n%s", out)
	}
	if got := showTaskMeta(t, bin, home, env, "T-0001"); got["a"] != "1" {
		t.Fatalf("conflicting write applied: %v", got)
	}
	out, code := runTLC(t, bin, home, env, "task", "create", "--recipe", "r", "--meta", "a=1")
	if code == 0 || !strings.Contains(out, "--meta cannot be combined with --recipe") {
		t.Fatalf("--meta with --recipe: exit %d, want rejection naming the conflict:\n%s", code, out)
	}
}
