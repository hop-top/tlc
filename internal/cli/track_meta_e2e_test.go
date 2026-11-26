package cli

// End-to-end coverage for writing and filtering track meta from the CLI:
// --meta on create and update, --unset-meta on update, --meta on list.
// Driven through the real binary so the flag wiring, the shared meta path
// and the stored row are all exercised, and read back via
// `track show --format json`.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func showTrackMeta(t *testing.T, bin, home string, env []string, id string) map[string]any {
	t.Helper()
	out := runTLCOK(t, bin, home, env, "track", "show", id, "--format", "json")
	var payload struct {
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode track show json: %v\n%s", err, out)
	}
	return payload.Meta
}

// writePlanFile writes a frontmatter-free plan under dir and returns its
// path; linking it puts the engine-owned "plans" key into track meta.
func writePlanFile(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("# Plan\n\nprose only\n"), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return p
}

func TestTrackMetaCLI_CreateWithMeta(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "track", "create", "With meta", "--id", "with-meta",
		"--meta", "owner=ana", "--meta", "url=https://x.test/p?a=1")
	got := showTrackMeta(t, bin, home, env, "with-meta")
	want := map[string]any{"owner": "ana", "url": "https://x.test/p?a=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("meta = %v, want %v", got, want)
	}
}

func TestTrackMetaCLI_UpdateSetOverwriteUnset(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "track", "create", "Meta track", "--id", "meta-track",
		"--meta", "keep=k", "--meta", "owner=ana")

	// Set a new key and overwrite an existing one; the rest is untouched.
	runTLCOK(t, bin, home, env, "track", "update", "meta-track",
		"--meta", "owner=bob", "--meta", "added=1")
	got := showTrackMeta(t, bin, home, env, "meta-track")
	want := map[string]any{"keep": "k", "owner": "bob", "added": "1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after set/overwrite meta = %v, want %v", got, want)
	}

	// Unset removes only the named keys; unknown keys are a no-op.
	runTLCOK(t, bin, home, env, "track", "update", "meta-track",
		"--unset-meta", "added", "--unset-meta", "never-there")
	got = showTrackMeta(t, bin, home, env, "meta-track")
	delete(want, "added")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after unset meta = %v, want %v", got, want)
	}

	// Set and unset in one invocation, alongside another field.
	runTLCOK(t, bin, home, env, "track", "update", "meta-track",
		"--meta", "fresh=y", "--unset-meta", "owner", "--title", "Renamed")
	got = showTrackMeta(t, bin, home, env, "meta-track")
	delete(want, "owner")
	want["fresh"] = "y"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after set+unset meta = %v, want %v", got, want)
	}
	if out := runTLCOK(t, bin, home, env, "track", "show", "meta-track", "--format", "json"); !strings.Contains(out, "Renamed") {
		t.Fatalf("--title alongside meta not applied:\n%s", out)
	}
}

func TestTrackMetaCLI_UpdateKeepsInternalKeys(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)
	plan := writePlanFile(t, home, "p.md")

	runTLCOK(t, bin, home, env, "track", "create", "Plan track", "--id", "plan-track")
	runTLCOK(t, bin, home, env, "track", "update", "plan-track", "--add-plan", plan)
	before := showTrackMeta(t, bin, home, env, "plan-track")
	if before["plans"] == nil {
		t.Fatalf("fixture lacks the plan link: %v", before)
	}

	runTLCOK(t, bin, home, env, "track", "update", "plan-track", "--meta", "owner=ana")
	got := showTrackMeta(t, bin, home, env, "plan-track")
	want := map[string]any{"owner": "ana"}
	for k, v := range before {
		want[k] = v
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("meta = %v, want %v (plans preserved)", got, want)
	}
}

func TestTrackMetaCLI_ReservedKeysRejected(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	// Rejected on create, and nothing is written.
	for _, key := range []string{"plans", "external_uid", "x_tlc", "due_date_only"} {
		out, code := runTLC(t, bin, home, env, "track", "create", "Bad", "--id", "bad-track", "--meta", key+"=x")
		if code == 0 {
			t.Fatalf("create --meta %s=x accepted:\n%s", key, out)
		}
		if !strings.Contains(out, `reserved meta key "`+key+`"`) {
			t.Errorf("create --meta %s: error should name the key as reserved, got:\n%s", key, out)
		}
	}
	if out, code := runTLC(t, bin, home, env, "track", "show", "bad-track"); code == 0 {
		t.Fatalf("a rejected create left a track behind:\n%s", out)
	}

	plan := writePlanFile(t, home, "p.md")
	runTLCOK(t, bin, home, env, "track", "create", "Good", "--id", "good-track", "--meta", "owner=ana")
	runTLCOK(t, bin, home, env, "track", "update", "good-track", "--add-plan", plan)
	before := showTrackMeta(t, bin, home, env, "good-track")

	// Rejected on update, set and unset, with the other changes in the
	// same invocation left unapplied (no partial write).
	cases := [][]string{
		{"--meta", "plans=x"},
		{"--unset-meta", "plans"},
		{"--unset-meta", "external_uid"},
		{"--meta", "owner=bob", "--meta", "x_tlc=u", "--title", "renamed"},
		{"--unset-meta", "owner", "--unset-meta", "due_date_only", "--title", "renamed"},
	}
	for _, args := range cases {
		out, code := runTLC(t, bin, home, env, append([]string{"track", "update", "good-track"}, args...)...)
		if code == 0 {
			t.Fatalf("update %v accepted:\n%s", args, out)
		}
		if !strings.Contains(out, "reserved meta key") {
			t.Errorf("update %v: error should say reserved meta key, got:\n%s", args, out)
		}
		if got := showTrackMeta(t, bin, home, env, "good-track"); !reflect.DeepEqual(got, before) {
			t.Fatalf("update %v partially applied: meta = %v, want %v", args, got, before)
		}
	}
	if out := runTLCOK(t, bin, home, env, "track", "show", "good-track", "--format", "json"); strings.Contains(out, "renamed") {
		t.Fatalf("rejected update still applied --title:\n%s", out)
	}
}

func TestTrackMetaCLI_BadInput(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	if out, code := runTLC(t, bin, home, env, "track", "create", "X", "--meta", "novalue"); code == 0 {
		t.Fatalf("--meta without = accepted:\n%s", out)
	}
	runTLCOK(t, bin, home, env, "track", "create", "X", "--id", "x-track", "--meta", "a=1")
	if out, code := runTLC(t, bin, home, env, "track", "update", "x-track",
		"--meta", "a=2", "--unset-meta", "a"); code == 0 {
		t.Fatalf("same key set and unset accepted:\n%s", out)
	}
	if got := showTrackMeta(t, bin, home, env, "x-track"); got["a"] != "1" {
		t.Fatalf("conflicting write applied: %v", got)
	}
	out, code := runTLC(t, bin, home, env, "track", "create", "--recipe", "r", "--meta", "a=1")
	if code == 0 || !strings.Contains(out, "--meta cannot be combined with --recipe") {
		t.Fatalf("--meta with --recipe: exit %d, want rejection naming the conflict:\n%s", code, out)
	}
}

func TestTrackMetaCLI_UpdateDryRun(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "track", "create", "Dry", "--id", "dry-track", "--meta", "a=1")
	out := runTLCOK(t, bin, home, env, "track", "update", "dry-track", "--meta", "a=2", "--dry-run")
	if !strings.Contains(out, "Meta") {
		t.Errorf("dry run should report the meta change:\n%s", out)
	}
	if got := showTrackMeta(t, bin, home, env, "dry-track"); got["a"] != "1" {
		t.Fatalf("dry run wrote meta: %v", got)
	}
	if out, code := runTLC(t, bin, home, env, "track", "update", "dry-track", "--meta", "plans=x", "--dry-run"); code == 0 {
		t.Fatalf("dry run accepted a reserved key:\n%s", out)
	}
}

func trackSlugsFromList(t *testing.T, out string) []string {
	t.Helper()
	var recs []struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal([]byte(out), &recs); err != nil {
		t.Fatalf("decode track list json: %v\n%s", err, out)
	}
	slugs := []string{}
	for _, r := range recs {
		slugs = append(slugs, r.Slug)
	}
	return slugs
}

func TestTrackMetaCLI_ListFilter(t *testing.T) {
	bin, home, env := effortVocabFixture(t, defaultEffortConfig)

	runTLCOK(t, bin, home, env, "track", "create", "One", "--id", "one-track", "--meta", "owner=ana", "--meta", "env=prod")
	runTLCOK(t, bin, home, env, "track", "create", "Two", "--id", "two-track", "--meta", "owner=bob", "--meta", "env=prod")
	runTLCOK(t, bin, home, env, "track", "create", "Three", "--id", "three-track")

	list := func(args ...string) []string {
		out := runTLCOK(t, bin, home, env, append([]string{"track", "list", "--format", "json"}, args...)...)
		return trackSlugsFromList(t, out)
	}
	if got := list("--meta", "owner=ana"); !reflect.DeepEqual(got, []string{"one-track"}) {
		t.Errorf("owner=ana: %v", got)
	}
	if got := list("--meta", "env=prod"); len(got) != 2 {
		t.Errorf("env=prod: %v, want two tracks", got)
	}
	if got := list("--meta", "env=prod", "--meta", "owner=bob"); !reflect.DeepEqual(got, []string{"two-track"}) {
		t.Errorf("env=prod AND owner=bob: %v", got)
	}
	if got := list("--meta", "owner=nobody"); len(got) != 0 {
		t.Errorf("no match: %v, want none", got)
	}
	if got := list(); len(got) != 3 {
		t.Errorf("no filter: %v, want all three", got)
	}

	// An invalid key is a usage error, not an empty list, and not SQL.
	for _, k := range []string{"a.b", "x') OR 1=1 --", "a b"} {
		out, code := runTLC(t, bin, home, env, "track", "list", "--meta", k+"=1")
		if code == 0 {
			t.Errorf("--meta %q=1 accepted:\n%s", k, out)
		}
		if !strings.Contains(out, "meta filter key") {
			t.Errorf("--meta %q=1: error should name the meta filter key, got:\n%s", k, out)
		}
	}
	if got := list(); len(got) != 3 {
		t.Errorf("tracks after rejected filters: %v", got)
	}
}
