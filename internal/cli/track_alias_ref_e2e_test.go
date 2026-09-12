package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// The user-visible contract of the L-NNNN track alias: every command that
// accepts a track reference must treat "L-0002" and that track's slug as
// the same track. These run the real binary so the resolution path under
// test is the one a user actually hits, not an in-process shortcut.

// aliasFixture creates an isolated project with two tracks (seq 1 and 2)
// and one task attached to each, returning the binary, cwd, env and the
// slugs in seq order.
func aliasFixture(t *testing.T) (bin, cwd string, env []string, slugs []string) {
	t.Helper()

	bin = buildTLCBinary(t)
	home := t.TempDir()
	cwd = t.TempDir()
	dbPath := filepath.Join(home, "alias.db")
	env = e2eEnv(t, home, dbPath)

	slugs = []string{"alpha-track-one", "beta-track-two"}
	for _, slug := range slugs {
		runTLCOK(t, bin, cwd, env, "track", "create", "Track "+slug,
			"--id", slug)
		runTLCOK(t, bin, cwd, env, "task", "create",
			"task for "+slug, "--track", slug)
	}
	return bin, cwd, env, slugs
}

// taskTitles pulls the task titles out of `task list --format json`,
// tolerating either a bare array or an envelope with a data array.
func taskTitles(t *testing.T, out string) []string {
	t.Helper()

	trimmed := strings.TrimSpace(out)
	start := strings.IndexAny(trimmed, "[{")
	if start < 0 {
		t.Fatalf("no JSON in output:\n%s", out)
	}
	trimmed = trimmed[start:]

	var direct []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &direct); err == nil {
		return titlesOf(direct)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		t.Fatalf("parse task list JSON: %v\n%s", err, out)
	}
	for _, key := range []string{"data", "tasks", "items", "results"} {
		raw, ok := env[key]
		if !ok {
			continue
		}
		encoded, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var rows []map[string]any
		if err := json.Unmarshal(encoded, &rows); err == nil {
			return titlesOf(rows)
		}
	}
	t.Fatalf("no task array in JSON envelope:\n%s", out)
	return nil
}

func titlesOf(rows []map[string]any) []string {
	titles := make([]string, 0, len(rows))
	for _, row := range rows {
		if title, ok := row["title"].(string); ok {
			titles = append(titles, title)
		}
	}
	return titles
}

// TestTaskListTrackAliasMatchesSlug pins the headline contract: filtering
// by the L-NNNN alias returns exactly the rows the slug returns.
func TestTaskListTrackAliasMatchesSlug(t *testing.T) {
	bin, cwd, env, slugs := aliasFixture(t)

	for i, slug := range slugs {
		alias := aliasFor(i + 1)
		bySlug := taskTitles(t, runTLCOK(t, bin, cwd, env,
			"task", "list", "--track", slug, "--format", "json"))
		byAlias := taskTitles(t, runTLCOK(t, bin, cwd, env,
			"task", "list", "--track", alias, "--format", "json"))

		if len(bySlug) == 0 {
			t.Fatalf("slug %q returned no tasks; fixture is broken", slug)
		}
		if strings.Join(bySlug, "|") != strings.Join(byAlias, "|") {
			t.Errorf("--track %s returned %v, want %v (same as --track %s)",
				alias, byAlias, bySlug, slug)
		}
	}
}

// TestTaskListTrackAliasLowercase covers the documented case-insensitive
// alias form through the CLI surface.
func TestTaskListTrackAliasLowercase(t *testing.T) {
	bin, cwd, env, slugs := aliasFixture(t)

	bySlug := taskTitles(t, runTLCOK(t, bin, cwd, env,
		"task", "list", "--track", slugs[1], "--format", "json"))
	byAlias := taskTitles(t, runTLCOK(t, bin, cwd, env,
		"task", "list", "--track", "l-0002", "--format", "json"))

	if strings.Join(bySlug, "|") != strings.Join(byAlias, "|") {
		t.Errorf("--track l-0002 returned %v, want %v", byAlias, bySlug)
	}
}

// TestTaskCreateTrackAliasLinksExistingTrack guards the auto-create trap:
// --track with a valid alias must attach to that track, never silently
// mint a new track literally named "L-0002".
func TestTaskCreateTrackAliasLinksExistingTrack(t *testing.T) {
	bin, cwd, env, slugs := aliasFixture(t)

	runTLCOK(t, bin, cwd, env, "task", "create",
		"created via alias", "--track", "L-0002")

	titles := taskTitles(t, runTLCOK(t, bin, cwd, env,
		"task", "list", "--track", slugs[1], "--format", "json"))
	found := false
	for _, title := range titles {
		if title == "created via alias" {
			found = true
		}
	}
	if !found {
		t.Errorf("task created with --track L-0002 not under %q; got %v",
			slugs[1], titles)
	}

	listOut := runTLCOK(t, bin, cwd, env, "track", "list")
	if strings.Contains(strings.ToLower(listOut), "l-0002\n") &&
		strings.Count(listOut, "l-0002") > 1 {
		t.Errorf("alias appears to have auto-created a track:\n%s", listOut)
	}
}

// TestTaskUpdateTrackAlias covers the update path, which shares the
// auto-create-on-not-found branch with create.
func TestTaskUpdateTrackAlias(t *testing.T) {
	bin, cwd, env, slugs := aliasFixture(t)

	runTLCOK(t, bin, cwd, env, "task", "create", "floating task")
	runTLCOK(t, bin, cwd, env, "task", "update", "T-0003",
		"--track", "L-0001")

	titles := taskTitles(t, runTLCOK(t, bin, cwd, env,
		"task", "list", "--track", slugs[0], "--format", "json"))
	found := false
	for _, title := range titles {
		if title == "floating task" {
			found = true
		}
	}
	if !found {
		t.Errorf("task update --track L-0001 did not attach to %q; got %v",
			slugs[0], titles)
	}
}

// TestTrackSubcommandsAcceptAlias walks the read-only track subcommands
// that take a track reference.
func TestTrackSubcommandsAcceptAlias(t *testing.T) {
	bin, cwd, env, slugs := aliasFixture(t)

	for _, sub := range []string{"show", "graph"} {
		aliasOut, aliasCode := runTLC(t, bin, cwd, env, "track", sub, "L-0002")
		if aliasCode != 0 {
			t.Errorf("track %s L-0002: exit %d\n%s", sub, aliasCode, aliasOut)
			continue
		}
		if !strings.Contains(aliasOut, slugs[1]) {
			t.Errorf("track %s L-0002 does not mention %q:\n%s",
				sub, slugs[1], aliasOut)
		}
	}
}

// TestTrackUpdateAcceptsAlias mutates through the alias and reads back
// through the slug, so a wrong-track resolution cannot pass.
func TestTrackUpdateAcceptsAlias(t *testing.T) {
	bin, cwd, env, slugs := aliasFixture(t)

	out, code := runTLC(t, bin, cwd, env, "track", "update", "L-0002",
		"--title", "retitled via alias")
	if code != 0 {
		t.Fatalf("track update L-0002: exit %d\n%s", code, out)
	}

	showOut := runTLCOK(t, bin, cwd, env, "track", "show", slugs[1])
	if !strings.Contains(showOut, "retitled via alias") {
		t.Errorf("update via alias did not land on %q:\n%s", slugs[1], showOut)
	}
}

// TestTrackAliasMissWithoutSlugIsNotFound pins the resolution contract's
// edge: an uppercase alias that matches no seq must be a clean not-found,
// never a fuzzy match onto an unrelated track.
func TestTrackAliasMissWithoutSlugIsNotFound(t *testing.T) {
	bin, cwd, env, slugs := aliasFixture(t)

	out, code := runTLC(t, bin, cwd, env, "track", "show", "L-0042")
	if code == 0 {
		t.Fatalf("track show L-0042 unexpectedly succeeded:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "not found") {
		t.Errorf("want a not-found error for L-0042, got:\n%s", out)
	}
	// A fuzzy fallback would happily resolve "L-0042" onto a real track
	// and print its detail instead of an error.
	for _, slug := range slugs {
		if strings.Contains(out, slug) {
			t.Errorf("L-0042 resolved onto track %q:\n%s", slug, out)
		}
	}
}

// TestTaskCreateAliasMissDoesNotAutoCreate pins the other half of the miss
// contract: --track with an unmatched alias must not mint a track named
// after the alias, which is what the auto-create branch would do if the
// alias arrived there as if it were an ordinary slug.
func TestTaskCreateAliasMissDoesNotAutoCreate(t *testing.T) {
	bin, cwd, env, _ := aliasFixture(t)

	before := runTLCOK(t, bin, cwd, env, "track", "list")
	out, code := runTLC(t, bin, cwd, env, "task", "create",
		"alias miss task", "--track", "L-0042")
	after := runTLCOK(t, bin, cwd, env, "track", "list")

	if code == 0 && after != before {
		t.Errorf("--track L-0042 created a track:\nbefore:\n%s\nafter:\n%s\n%s",
			before, after, out)
	}
	if strings.Contains(strings.ToLower(after), "l-0042") {
		t.Errorf("track list now contains an alias-named track:\n%s", after)
	}
}

// aliasFor renders the display alias for a track sequence number, spelled
// out locally so the test pins the literal user-facing format rather than
// re-deriving it from the code under test.
func aliasFor(seq int) string {
	return fmt.Sprintf("L-%04d", seq)
}
