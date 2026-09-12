package cli

// End-to-end coverage for `task list --group-by` / `--group-limit`,
// driven through a real tlc binary.
//
// The unit tests in this package already pin the pieces: groupTasks'
// ordering and many-to-many tag behavior, renderGroupedTables' footer
// and truncation annotations, groupedPayload's nesting, the aggregate
// rejection, and the label resolution. What none of them can pin is
// whether a user typing the flags actually GETS any of it — flag
// registration, config defaults, the store round-trip, the labeler's
// store handle, and the rendering chokepoint all sit between the flag
// and the output, and every one of them is a place the feature can be
// wired up wrong while every unit test stays green.
//
// So these tests assert the same properties one layer out, against
// stdout of a subprocess. A helper that reimplemented the grouping to
// compute its expectations would only prove the helper agrees with
// itself; expectations here are literal, derived from the fixture.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

const (
	groupProjectID = "group-fixture"

	// Track IDs carry the stored typeid shape rather than a readable
	// stub, so the heading assertions below fail against raw-key
	// rendering. A fixture using "adoption" as the ID would render the
	// same text whether or not titles resolve.
	groupTrackAdoptionID = "track_01m29x2dnqfba9zqqsrpp8daxj"
	groupTrackBillingID  = "track_01m29x2j5vfsqs5g6q5dwseabx"
	// groupTrackGoneID is referenced by a task but has NO tracks row:
	// the deleted-track fallback, end to end.
	groupTrackGoneID = "track_01m29x2k7wgtrt6h7r6exteabc"

	groupTrackAdoptionTitle = "Adoption rollout"
	groupTrackBillingTitle  = "Billing migration"
)

// groupFixture seeds a listing whose every grouped property is
// observable: two named tracks plus one dangling reference, two
// assignees plus unassigned rows, overlapping tags, and enough rows in
// one track to be capped by --group-limit.
//
// Seeded through the storage layer rather than by running `tlc task
// create` repeatedly: the point of these tests is the READ path, and
// driving the write path through the CLI would make a task-create
// regression fail them for the wrong reason.
func groupFixture(t *testing.T) (bin, cwd string, env []string) {
	t.Helper()

	bin = buildTLCBinary(t)
	dbPath := groupFixtureDB(t)
	env = append(e2eEnv(t, t.TempDir(), dbPath), "TLC_PROJECT_ID="+groupProjectID)
	return bin, t.TempDir(), env
}

// groupSeedTask describes one fixture row in the terms the grouped
// listing reads.
type groupSeedTask struct {
	id       string
	title    string
	track    string
	assignee string
	tags     []string
	status   core.TaskStatus
}

// groupSeedTasks is the fixture, written out literally so the expected
// groupings below can be read off it rather than computed.
//
//	track   : Adoption(4 rows, incl. 1 unassigned) Billing(1) gone(1) none(1)
//	assignee: ann(3) bob(2) (none)(2)
//	tag      : api(3) urgent(2) docs(1) (none)(2)  -> 8 rows / 7 tasks
var groupSeedTasks = []groupSeedTask{
	{"T-0001", "wire metrics", groupTrackAdoptionID, "ann", []string{"api"}, core.StatusTodo},
	{"T-0002", "draft plan", groupTrackAdoptionID, "bob", []string{"api", "urgent"}, core.StatusTodo},
	{"T-0003", "measure funnel", groupTrackAdoptionID, "ann", []string{"docs"}, core.StatusInProgress},
	{"T-0004", "ship rollout", groupTrackAdoptionID, "", nil, core.StatusTodo},
	{"T-0005", "port schema", groupTrackBillingID, "bob", []string{"api"}, core.StatusTodo},
	{"T-0006", "orphan row", groupTrackGoneID, "ann", []string{"urgent"}, core.StatusTodo},
	{"T-0007", "untracked chore", "", "", nil, core.StatusTodo},
}

func groupFixtureDB(t *testing.T) string {
	t.Helper()

	tpl := groupTemplateDB(t)
	dst := filepath.Join(t.TempDir(), "seeded.db")
	data, err := os.ReadFile(tpl)
	if err != nil {
		t.Fatalf("read template db: %v", err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("write db copy: %v", err)
	}
	return dst
}

// groupTemplateDB seeds the fixture once per package run, matching the
// template-and-copy approach the aggregate fixture uses so the seeding
// transactions are not repaid per test.
func groupTemplateDB(t *testing.T) string {
	t.Helper()

	e2eTemplateMu.Lock()
	defer e2eTemplateMu.Unlock()

	if path, ok := e2eTemplateDirs["grouping"]; ok {
		return path
	}

	dir, err := os.MkdirTemp("", "tlc-e2e-group")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	path := filepath.Join(dir, "grouping.db")
	seedGroupFixture(t, path)
	e2eTemplateDirs["grouping"] = path
	return path
}

func seedGroupFixture(t *testing.T, dbPath string) {
	t.Helper()

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("open seed storage: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := t.Context()
	pid := groupProjectID

	// Two tracks exist; groupTrackGoneID deliberately does not, so the
	// fallback has something real to fall back from.
	for id, title := range map[string]string{
		groupTrackAdoptionID: groupTrackAdoptionTitle,
		groupTrackBillingID:  groupTrackBillingTitle,
	} {
		if err := s.CreateTrack(ctx, &core.Track{
			ID:        id,
			Slug:      strings.ToLower(strings.ReplaceAll(title, " ", "-")),
			Title:     title,
			Type:      "feature",
			Status:    core.TrackStatusPending,
			ProjectID: &pid,
		}); err != nil {
			t.Fatalf("seed track %s: %v", id, err)
		}
	}

	for i, seed := range groupSeedTasks {
		task := &core.Task{
			ID:        seed.id,
			Seq:       int64(i + 1),
			Title:     seed.title,
			Status:    seed.status,
			ProjectID: &pid,
			Tags:      seed.tags,
		}
		if seed.track != "" {
			track := seed.track
			task.TrackID = &track
		}
		if seed.assignee != "" {
			assignee := seed.assignee
			task.AssignedTo = &assignee
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", seed.id, err)
		}
	}
}

// groupListArgs is the common invocation: every status, every project
// off, so the fixture's IN_PROGRESS rows are not filtered out by the
// default unfinished-work filter differing between environments.
func groupListArgs(extra ...string) []string {
	return append([]string{"task", "list", "--archived"}, extra...)
}

// headings extracts the group headings from rendered output: the lines
// that are neither a table header, a data row, nor blank.
//
// Keyed off the "ID" column header that renderTable always emits, so a
// heading is "a non-empty line that is not part of a table". Stripping
// ANSI first — the headings are styled, and a raw comparison would be
// asserting on escape codes.
func headings(out string) []string {
	var found []string
	for _, line := range strings.Split(stripANSI(out), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "ID "), strings.HasPrefix(trimmed, "ID\t"):
			continue // table header
		case strings.HasPrefix(trimmed, "T-"):
			continue // data row
		case strings.Contains(trimmed, "distinct tasks"):
			continue // footer
		case strings.HasPrefix(trimmed, "note:"):
			continue
		default:
			found = append(found, trimmed)
		}
	}
	return found
}

// stripANSI removes SGR escape sequences so assertions compare text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// TestE2EGroupByEachKeyRendersExpectedGroups walks every --group-by
// dimension through the real command and checks the group SET each one
// produces.
//
// NEW at this layer: that the flag is registered, accepted, reaches the
// grouper, and that each dimension's headings survive the round-trip
// through storage. The unit tests group in-memory structs; nothing below
// them proves a stored NULL track_id or an empty tags column arrives as
// the "unset" the grouper recognizes.
func TestE2EGroupByEachKeyRendersExpectedGroups(t *testing.T) {
	bin, cwd, env := groupFixture(t)

	cases := []struct {
		key  string
		want []string
	}{{
		// Titles, not typeids — and the dangling reference keeps its
		// raw key rather than rendering blank.
		key:  "track",
		want: []string{groupTrackAdoptionTitle, groupTrackBillingTitle, groupTrackGoneID, noneGroupName},
	}, {
		key:  "assignee",
		want: []string{"ann", "bob", noneGroupName},
	}, {
		key:  "tag",
		want: []string{"api", "docs", "urgent", noneGroupName},
	}, {
		// Headings carry the CANONICAL vocabulary value ("TODO"), not
		// the table column's display label ("To Do"). That is the
		// spelling `--status` accepts, so a reader can retype a heading
		// as a filter; the column is free to prettify because nothing
		// consumes it.
		//
		// Order is LIFECYCLE, not alphabetical: TODO precedes
		// IN_PROGRESS in the declared vocabulary while "I" precedes "T"
		// in ASCII, so an alphabetical implementation fails here.
		key:  "status",
		want: []string{"TODO", "IN_PROGRESS"},
	}}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			out := runTLCOK(t, bin, cwd, env, groupListArgs("--group-by", tc.key)...)
			got := headings(out)

			if len(got) != len(tc.want) {
				t.Fatalf("--group-by %s headings = %q, want %q\nfull output:\n%s",
					tc.key, got, tc.want, out)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("--group-by %s heading %d = %q, want %q\nfull output:\n%s",
						tc.key, i, got[i], tc.want[i], out)
				}
			}
		})
	}
}

// TestE2EGroupByTrackShowsTitlesNotTypeIDs is the label regression
// stated as its own failure. Folding it into the case table above would
// report "heading 0 wrong" for a defect whose real name is "the typeid
// leaked into the UI".
func TestE2EGroupByTrackShowsTitlesNotTypeIDs(t *testing.T) {
	bin, cwd, env := groupFixture(t)
	out := stripANSI(runTLCOK(t, bin, cwd, env, groupListArgs("--group-by", "track")...))

	for _, title := range []string{groupTrackAdoptionTitle, groupTrackBillingTitle} {
		if !strings.Contains(out, title) {
			t.Errorf("track title %q missing from headings:\n%s", title, out)
		}
	}
	// The two RESOLVABLE typeids must be gone. The third must remain:
	// its track row does not exist, and a blank heading would strip the
	// attribution from the row beneath it.
	for _, id := range []string{groupTrackAdoptionID, groupTrackBillingID} {
		if strings.Contains(out, id) {
			t.Errorf("resolved track still renders its typeid %q:\n%s", id, out)
		}
	}
	if !strings.Contains(out, groupTrackGoneID) {
		t.Errorf("deleted track must fall back to its raw key %q:\n%s", groupTrackGoneID, out)
	}
}

// TestE2EGroupByTagDuplicatesAndReportsDistinct is the many-to-many
// case end to end: a multi-tagged task appears under each of its tags,
// and the footer states the distinct count so the duplication does not
// read as a bug.
//
// NEW at this layer: the footer arithmetic over rows that came from
// STORAGE. The unit test computes it over hand-built structs whose Tags
// slice it controls; nothing below this proves the tags column
// round-trips into the same membership.
func TestE2EGroupByTagDuplicatesAndReportsDistinct(t *testing.T) {
	bin, cwd, env := groupFixture(t)
	out := stripANSI(runTLCOK(t, bin, cwd, env, groupListArgs("--group-by", "tag")...))

	// T-0002 carries [api urgent] and must appear under BOTH.
	apiSection := sectionFor(t, out, "api")
	urgentSection := sectionFor(t, out, "urgent")
	if !strings.Contains(apiSection, "T-0002") {
		t.Errorf("T-0002 missing from the api group:\n%s", out)
	}
	if !strings.Contains(urgentSection, "T-0002") {
		t.Errorf("T-0002 missing from the urgent group:\n%s", out)
	}

	// 8 memberships over 7 distinct tasks — read off groupSeedTasks.
	const wantFooter = "8 rows, 7 distinct tasks"
	if !strings.Contains(out, wantFooter) {
		t.Errorf("expected footer %q in:\n%s", wantFooter, out)
	}
}

// TestE2EGroupByWithoutDuplicationOmitsFooter is the other half of the
// footer rule: when rows and distinct tasks agree the line states
// nothing and must not print.
func TestE2EGroupByWithoutDuplicationOmitsFooter(t *testing.T) {
	bin, cwd, env := groupFixture(t)
	out := runTLCOK(t, bin, cwd, env, groupListArgs("--group-by", "assignee")...)

	if strings.Contains(out, "distinct tasks") {
		t.Errorf("assignee grouping duplicates nothing; footer must be absent:\n%s", out)
	}
}

// TestE2ENoneGroupSortsLast pins placement through the command. "(none)"
// begins with "(", which sorts ahead of every letter, so a plain sort
// opens the listing with the least informative section.
func TestE2ENoneGroupSortsLast(t *testing.T) {
	bin, cwd, env := groupFixture(t)

	for _, key := range []string{"track", "assignee", "tag"} {
		t.Run(key, func(t *testing.T) {
			got := headings(runTLCOK(t, bin, cwd, env, groupListArgs("--group-by", key)...))
			if len(got) == 0 {
				t.Fatalf("--group-by %s produced no headings", key)
			}
			if last := got[len(got)-1]; last != noneGroupName {
				t.Errorf("--group-by %s: %q must sort last, got %q (headings: %q)",
					key, noneGroupName, last, got)
			}
		})
	}
}

// TestE2EGroupLimitCapsRowsAndMarksTruncation drives the cap through the
// real flag: a group over the cap shows only the capped rows and says so
// in its heading; a group under it is NOT annotated, because "(1 of 1)"
// states nothing and an unconditional annotation stops meaning "there is
// more".
func TestE2EGroupLimitCapsRowsAndMarksTruncation(t *testing.T) {
	bin, cwd, env := groupFixture(t)
	out := stripANSI(runTLCOK(t, bin, cwd, env,
		groupListArgs("--group-by", "track", "--group-limit", "2")...))

	// Adoption holds 4 rows, capped to 2.
	adoption := sectionFor(t, out, groupTrackAdoptionTitle)
	if n := strings.Count(adoption, "T-0"); n != 2 {
		t.Errorf("--group-limit 2 must render 2 rows for %q, got %d:\n%s",
			groupTrackAdoptionTitle, n, out)
	}
	if !strings.Contains(out, groupTrackAdoptionTitle+" (2 of 4)") {
		t.Errorf("capped group must announce truncation as %q:\n%s",
			groupTrackAdoptionTitle+" (2 of 4)", out)
	}

	// Billing holds 1 row, under the cap, so it carries no annotation.
	if strings.Contains(out, groupTrackBillingTitle+" (") {
		t.Errorf("uncapped group must not be annotated:\n%s", out)
	}
}

// TestE2EGroupLimitWithoutGroupByIsRejected: the flag caps rows WITHIN a
// group, so with no grouping dimension there is nothing to cap.
// Accepting it would exit 0 having done nothing, leaving the user
// believing a cap applied.
func TestE2EGroupLimitWithoutGroupByIsRejected(t *testing.T) {
	bin, cwd, env := groupFixture(t)
	out, code := runTLC(t, bin, cwd, env, groupListArgs("--group-limit", "2")...)

	if code == 0 {
		t.Fatalf("--group-limit without --group-by must fail; got exit 0:\n%s", out)
	}
	if !strings.Contains(out, "--group-by") {
		t.Errorf("rejection must name the flag that fixes it:\n%s", out)
	}
}

// TestE2EGroupedJSONNestsGroups pins the structured shape a script
// consumes, parsed as JSON rather than string-matched: the assertion is
// about the SHAPE, and a substring check would pass on a payload that
// merely mentions the right words.
func TestE2EGroupedJSONNestsGroups(t *testing.T) {
	bin, cwd, env := groupFixture(t)
	out := runTLCOK(t, bin, cwd, env,
		groupListArgs("--group-by", "track", "-f", "json")...)

	var payload struct {
		Groups []struct {
			Name  string `json:"name"`
			Tasks []struct {
				ID string `json:"id"`
			} `json:"tasks"`
		} `json:"groups"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("grouped JSON must parse as {groups:[...]}: %v\n%s", err, out)
	}
	if len(payload.Groups) != 4 {
		t.Fatalf("expected 4 groups, got %d:\n%s", len(payload.Groups), out)
	}

	// Group names are LABELED the same way the headings are, so a script
	// reading .groups[].name and a human reading the table agree.
	if payload.Groups[0].Name != groupTrackAdoptionTitle {
		t.Errorf("JSON group name = %q, want %q", payload.Groups[0].Name, groupTrackAdoptionTitle)
	}
	if got := payload.Groups[len(payload.Groups)-1].Name; got != noneGroupName {
		t.Errorf("JSON groups must end with %q, got %q", noneGroupName, got)
	}
	if len(payload.Groups[0].Tasks) != 4 {
		t.Errorf("%q must carry 4 tasks, got %d", groupTrackAdoptionTitle,
			len(payload.Groups[0].Tasks))
	}
}

// TestE2EUngroupedJSONStaysAFlatArray is the compatibility guard for
// every existing consumer. Without --group-by the payload must remain
// the top-level array they iterate; wrapping it unconditionally would
// break all of them at once, and `jq '.[]'` fails on an object exactly
// the way it failed on the null the empty-list fix removed.
func TestE2EUngroupedJSONStaysAFlatArray(t *testing.T) {
	bin, cwd, env := groupFixture(t)
	out := runTLCOK(t, bin, cwd, env, groupListArgs("-f", "json")...)

	var flat []map[string]any
	if err := json.Unmarshal([]byte(out), &flat); err != nil {
		t.Fatalf("ungrouped JSON must stay a flat array: %v\n%s", err, out)
	}
	if len(flat) != len(groupSeedTasks) {
		t.Errorf("expected %d tasks, got %d", len(groupSeedTasks), len(flat))
	}
}

// TestE2EGroupByWithAggregateIsRejected: --group-by lists rows in one
// table per dimension, an aggregate collapses the match set into counts.
// There is no reading of the pair that satisfies either, so it is
// rejected rather than silently resolved in one flag's favor.
//
// Every spelling is exercised. The bool flags are only ONE spelling of
// an aggregate; `-f summary` and `--format counters` leave those bools
// false, and a gate keyed on them would let exactly those through.
func TestE2EGroupByWithAggregateIsRejected(t *testing.T) {
	bin, cwd, env := groupFixture(t)

	for _, extra := range [][]string{
		{"--summary"},
		{"--counters"},
		{"-f", "summary"},
		{"--format", "counters"},
	} {
		t.Run(strings.Join(extra, " "), func(t *testing.T) {
			args := groupListArgs(append([]string{"--group-by", "track"}, extra...)...)
			out, code := runTLC(t, bin, cwd, env, args...)

			if code == 0 {
				t.Fatalf("--group-by with %v must fail; got exit 0:\n%s", extra, out)
			}
			if !strings.Contains(out, "--group-by") {
				t.Errorf("rejection must name --group-by:\n%s", out)
			}
		})
	}
}

// TestE2EGroupOrderingIsStableAcrossRuns is the determinism guard, and
// it can only live at this layer: Go randomizes map iteration order PER
// PROCESS, so repeated in-process calls share one randomization seed and
// a map-ordered implementation can pass a unit test that loops. Separate
// subprocesses genuinely re-roll it.
func TestE2EGroupOrderingIsStableAcrossRuns(t *testing.T) {
	bin, cwd, env := groupFixture(t)

	for _, key := range []string{"track", "assignee", "tag", "status"} {
		t.Run(key, func(t *testing.T) {
			args := groupListArgs("--group-by", key)
			first := headings(runTLCOK(t, bin, cwd, env, args...))

			// Five fresh processes: enough that a map-ordered
			// implementation over four-plus groups is overwhelmingly
			// unlikely to repeat the same order every time.
			for run := range 5 {
				got := headings(runTLCOK(t, bin, cwd, env, args...))
				if len(got) != len(first) {
					t.Fatalf("run %d: group count changed: %q vs %q", run, got, first)
				}
				for i := range first {
					if got[i] != first[i] {
						t.Fatalf("run %d: group order changed at %d: %q vs %q",
							run, i, got, first)
					}
				}
			}
		})
	}
}

// sectionFor returns the rendered text belonging to one group: from its
// heading up to the next blank-line-separated section.
func sectionFor(t *testing.T, out, heading string) string {
	t.Helper()

	start := strings.Index(out, heading)
	if start < 0 {
		t.Fatalf("heading %q not found in:\n%s", heading, out)
	}
	rest := out[start:]
	if end := strings.Index(rest, "\n\n"); end >= 0 {
		return rest[:end]
	}
	return rest
}
