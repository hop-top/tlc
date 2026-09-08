package cli

// Regression coverage: aggregate output (`--counters`, `--summary`, and the
// same two spelled as `--format` / `-f` / config `output.format`) must report
// counts of the whole match set, never of the page the default `--limit 100`
// would return.
//
// The defect these tests pin was silent. A truncated count is
// indistinguishable from a real one -- no warning, no stderr note, exit 0 --
// so a caller has no signal to raise the limit and the wrong number gets
// used downstream. That means asserting today's output proves nothing: the
// seeded fixture deliberately holds more tasks than the default limit, so a
// count equal to the limit is a failure and only the true total passes.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

const (
	// aggSeedTotal exceeds taskListLimit's default of 100 so a
	// page-scoped count cannot coincide with the true count.
	aggSeedTotal = 150
	aggSeedDone  = 90
	aggSeedTodo  = 60
	// aggDefaultLimit mirrors the `--limit` default registered in
	// task_list.go. A count landing exactly here is the defect's
	// signature.
	aggDefaultLimit = 100
	aggProjectID    = "agg-fixture"
)

// aggFixtureDB returns a private copy of the shared aggregate fixture:
// aggSeedTotal tasks, more than the default limit, split across two statuses
// so the per-status breakdown is checked too and not just a single total.
func aggFixtureDB(t *testing.T) string {
	t.Helper()
	return seedTemplateDB(t, "aggregate", e2eTaskSeed{
		Total:   aggSeedTotal,
		Project: aggProjectID,
		Mutate: func(i int, task *core.Task) {
			if i < aggSeedDone {
				task.Status = core.StatusDone
			}
		},
	})
}

// aggFixture builds a run context over a private copy of the fixture.
func aggFixture(t *testing.T) (bin, cwd string, env []string) {
	t.Helper()
	bin = buildTLCBinary(t)
	dbPath := aggFixtureDB(t)
	env = append(e2eEnv(t, t.TempDir(), dbPath), "TLC_PROJECT_ID="+aggProjectID)
	return bin, t.TempDir(), env
}

// TestTaskListAggregate_CountsFullMatchSet is the headline regression.
// Every aggregate invocation must report the seeded totals, with and
// without an explicit -n, and must never report the default limit.
//
// Every spelling of an aggregate appears. The bool flags were the only
// spelling the pre-query resolution recognized, so `--format summary`,
// `-f counters` and a config `output.format: summary` stayed on the
// paginated path and reported the page size -- the same defect, wearing a
// different flag.
func TestTaskListAggregate_CountsFullMatchSet(t *testing.T) {
	bin, cwd, env := aggFixture(t)
	base := []string{"task", "list", "--archived", "--all-projects"}

	cases := []struct {
		name string
		args []string
	}{
		// No -n: the default limit of 100 applies, which is exactly
		// what silently truncated the count.
		{"CountersFlag", []string{"--counters"}},
		{"SummaryFlag", []string{"--summary"}},
		// The --format spellings, which reach the same renderers.
		{"FormatCounters", []string{"--format", "counters"}},
		{"FormatSummary", []string{"--format", "summary"}},
		{"ShortFormatCounters", []string{"-f", "counters"}},
		{"ShortFormatSummary", []string{"-f", "summary"}},
		// The config spelling, documented as an output.format enum
		// value.
		{"ConfigFormatCounters", []string{"-c", "output.format=counters"}},
		{"ConfigFormatSummary", []string{"-c", "output.format=summary"}},
		// Explicit -n above the true total: correct before the fix,
		// so it guards against a regression in the other direction.
		{"CountersHighLimit", []string{"--counters", "-n", "1000"}},
		{"FormatSummaryHighLimit", []string{"--format", "summary", "-n", "1000"}},
		// Explicit -n *below* the true total: the flags contradict,
		// and the aggregate must win rather than report 5.
		{"CountersLowLimit", []string{"--counters", "-n", "5"}},
		{"FormatCountersLowLimit", []string{"--format", "counters", "-n", "5"}},
		{"ShortFormatSummaryLowLimit", []string{"-f", "summary", "-n", "5"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runTLC(t, bin, cwd, env, append(append([]string{}, base...), tc.args...)...)
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}

			done := countFor(t, out, string(core.StatusDone))
			todo := countFor(t, out, string(core.StatusTodo))

			if done != aggSeedDone {
				t.Errorf("DONE = %d, want %d\n%s", done, aggSeedDone, out)
			}
			if todo != aggSeedTodo {
				t.Errorf("TODO = %d, want %d\n%s", todo, aggSeedTodo, out)
			}
			// The defect's signature: DONE+TODO summing to the page
			// size rather than the match set.
			if done+todo == aggDefaultLimit {
				t.Errorf("counts sum to the default limit %d -- aggregate counted the page, not the match set\n%s",
					aggDefaultLimit, out)
			}
		})
	}
}

// TestTaskListSummary_TotalIsMatchSetTotal pins the Total line
// specifically, in both spellings. --summary states Total as fact, so a
// truncated one is the most trust-damaging form of the defect.
func TestTaskListSummary_TotalIsMatchSetTotal(t *testing.T) {
	bin, cwd, env := aggFixture(t)

	for _, args := range [][]string{
		{"task", "list", "--summary", "--archived", "--all-projects"},
		{"task", "list", "--format", "summary", "--archived", "--all-projects"},
		{"task", "list", "-f", "summary", "--archived", "--all-projects"},
		{"task", "list", "--summary", "--archived", "--all-projects", "-n", "1000"},
	} {
		out, code := runTLC(t, bin, cwd, env, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d, want 0\n%s", args, code, out)
		}
		total := countFor(t, out, "Total")
		if total != aggSeedTotal {
			t.Errorf("%v: Total = %d, want %d\n%s", args, total, aggSeedTotal, out)
		}
		if total == aggDefaultLimit {
			t.Errorf("%v: Total equals the default limit -- page counted, not match set\n%s", args, out)
		}
	}
}

// TestTaskListAggregate_ConfigLimitIsAlsoIgnored covers a limit that never
// reaches pflag's Changed bit. A config `defaults.limit` is applied by
// applyConfigDefaults, so an aggregate has to drop it just the same -- and
// this is the case a Changed-keyed note reported nothing about.
func TestTaskListAggregate_ConfigLimitIsAlsoIgnored(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := aggFixtureDB(t)

	// A project config carrying both the DB path and a truncating limit.
	tlcDir := filepath.Join(cwd, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o750); err != nil {
		t.Fatalf("mkdir .tlc: %v", err)
	}
	cfg := "project:\n  id: " + aggProjectID + "\nstorage:\n  db_path: " + dbPath +
		"\ndefaults:\n  limit: 10\n"
	if err := os.WriteFile(filepath.Join(tlcDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	env := e2eEnv(t, home, dbPath)
	out, code := runTLC(t, bin, cwd, env, "task", "list", "--counters", "--archived")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	if got := countFor(t, out, string(core.StatusDone)); got != aggSeedDone {
		t.Errorf("DONE = %d, want %d -- a config limit truncated the count\n%s", got, aggSeedDone, out)
	}
	// A config limit that WOULD have truncated is exactly when the note
	// carries information, and the Changed-keyed version stayed silent.
	if !strings.Contains(out, "ignored") {
		t.Errorf("expected a note that the config limit was ignored, got:\n%s", out)
	}
}

// TestTaskListAggregate_NoteOnlyWhenInformative pins when the note fires.
//
// Silently resolving two contradictory flags produced the original defect,
// so the contradiction is surfaced -- but only when it mattered. An explicit
// -n above the match set could not have truncated anything, so a note there
// is noise that trains readers to ignore the real one.
func TestTaskListAggregate_NoteOnlyWhenInformative(t *testing.T) {
	bin, cwd, env := aggFixture(t)
	base := []string{"task", "list", "--counters", "--archived", "--all-projects"}

	cases := []struct {
		name     string
		args     []string
		wantNote bool
	}{
		// Below the match set: the limit would have truncated, so say so.
		{"LimitBelowTotal", []string{"-n", "5"}, true},
		{"OffsetRequested", []string{"--offset", "10"}, true},
		// At or above the match set: nothing was lost, so stay quiet.
		{"LimitAboveTotal", []string{"-n", "100000"}, false},
		{"LimitEqualsTotal", []string{"-n", "150"}, false},
		// No pagination requested at all.
		{"NoLimitFlag", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runTLC(t, bin, cwd, env, append(append([]string{}, base...), tc.args...)...)
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}
			// The count is right either way; only the note varies.
			if got := countFor(t, out, string(core.StatusDone)); got != aggSeedDone {
				t.Errorf("DONE = %d, want %d\n%s", got, aggSeedDone, out)
			}
			if gotNote := strings.Contains(out, "ignored"); gotNote != tc.wantNote {
				t.Errorf("note present = %v, want %v\n%s", gotNote, tc.wantNote, out)
			}
		})
	}
}

// TestTaskListAggregate_ListOutputStillPaginates guards the other side of
// the fix. Pagination on a list is correct and load-bearing; only
// aggregates were meant to escape it.
func TestTaskListAggregate_ListOutputStillPaginates(t *testing.T) {
	bin, cwd, env := aggFixture(t)

	out, code := runTLC(t, bin, cwd, env,
		"task", "list", "--archived", "--all-projects", "-n", "7", "--format", "tls")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	rows := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.Contains(line, "seeded task") {
			rows++
		}
	}
	if rows != 7 {
		t.Errorf("list rows = %d, want 7 (--limit must still bound list output)\n%s", rows, out)
	}
}

// TestTaskListSummary_GroupsByProjectOutsideAProject is the F2 regression.
//
// Outside a project, nothing scopes the query to one project, so a summary
// built from flat status counts collapsed every project into a single
// "(no project)" bucket -- a strictly worse answer than the per-project
// breakdown that shipped before, and one that looks plausible.
func TestTaskListSummary_GroupsByProjectOutsideAProject(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir() // no .tlc/, no git: not a project
	dbPath := filepath.Join(home, "multi.db")

	// Three named projects plus genuinely project-less rows, so the
	// no-project bucket is distinguishable from the collapse.
	projects := []string{"proj-alpha", "proj-beta", "proj-gamma"}
	seedTasks(t, dbPath, e2eTaskSeed{
		Total:   12,
		Project: "",
		Mutate: func(i int, task *core.Task) {
			if i%4 == 3 {
				task.ProjectID = nil
				return
			}
			pid := projects[i%3]
			task.ProjectID = &pid
		},
	})

	env := e2eEnv(t, home, dbPath)
	out, code := runTLC(t, bin, cwd, env, "task", "list", "--summary", "--archived")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	// One block per project, and the collapse's signature is exactly one
	// block that is the no-project one.
	if n := strings.Count(out, "Project: "); n < len(projects) {
		t.Errorf("got %d project blocks, want at least %d -- projects collapsed into one bucket\n%s",
			n, len(projects), out)
	}
	for _, p := range projects {
		if !strings.Contains(out, "Project: "+p) {
			t.Errorf("missing block for %q\n%s", p, out)
		}
	}
	if !strings.Contains(out, "Project: "+noProject) {
		t.Errorf("genuinely project-less tasks lost their %q block\n%s", noProject, out)
	}
}

// TestTaskListSummary_AllProjectsGroupsByProject pins the same grouping for
// an explicit --all-projects, which the allowlist used to bounce off the
// counting path entirely.
func TestTaskListSummary_AllProjectsGroupsByProject(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "multi.db")

	projects := []string{"proj-one", "proj-two"}
	seedTasks(t, dbPath, e2eTaskSeed{
		Total:   6,
		Project: "",
		Mutate: func(i int, task *core.Task) {
			pid := projects[i%2]
			task.ProjectID = &pid
		},
	})

	env := e2eEnv(t, home, dbPath)
	out, code := runTLC(t, bin, cwd, env,
		"task", "list", "--summary", "--archived", "--all-projects")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	for _, p := range projects {
		if !strings.Contains(out, "Project: "+p) {
			t.Errorf("missing block for %q\n%s", p, out)
		}
	}
}

// TestTaskListCounters_FlattensAcrossProjects checks --counters' side of the
// per-project counts: it is a flat table by definition, so the projects sum.
func TestTaskListCounters_FlattensAcrossProjects(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "multi.db")

	seedTasks(t, dbPath, e2eTaskSeed{
		Total:   6,
		Project: "",
		Mutate: func(i int, task *core.Task) {
			pid := []string{"proj-one", "proj-two"}[i%2]
			task.ProjectID = &pid
		},
	})

	env := e2eEnv(t, home, dbPath)
	out, code := runTLC(t, bin, cwd, env,
		"task", "list", "--counters", "--archived", "--all-projects")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if strings.Contains(out, "Project:") {
		t.Errorf("--counters must not group by project\n%s", out)
	}
	if got := countFor(t, out, string(core.StatusTodo)); got != 6 {
		t.Errorf("TODO = %d, want 6 (summed across projects)\n%s", got, out)
	}
}

// TestTaskListOverdue_CountsInStore pins --overdue on the counting path.
// It is a column predicate now -- due_at in the past AND the task still
// open -- so it narrows the SQL count rather than filtering a page of rows.
func TestTaskListOverdue_CountsInStore(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "overdue.db")

	// 120 rows, above the default limit: 40 overdue TODO, 40 overdue but
	// DONE (finished, so not late), 40 not yet due.
	past := timeAgo(48)
	future := timeAhead(48)
	seedTasks(t, dbPath, e2eTaskSeed{
		Total:   120,
		Project: aggProjectID,
		Mutate: func(i int, task *core.Task) {
			switch {
			case i < 40:
				task.DueAt = &past
			case i < 80:
				task.Status = core.StatusDone
				task.DueAt = &past
			default:
				task.DueAt = &future
			}
		},
	})

	env := append(e2eEnv(t, home, dbPath), "TLC_PROJECT_ID="+aggProjectID)
	out, code := runTLC(t, bin, cwd, env,
		"task", "list", "--counters", "--overdue", "--archived", "--all-projects")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	if got := countFor(t, out, string(core.StatusTodo)); got != 40 {
		t.Errorf("overdue TODO = %d, want 40\n%s", got, out)
	}
	// A DONE task past its due date is not overdue -- one definition,
	// shared with `tlc status`.
	if strings.Contains(out, string(core.StatusDone)) {
		t.Errorf("finished tasks counted as overdue\n%s", out)
	}
}

// TestTaskListBlocked_CountsInStore pins --blocked on the counting path,
// where it belongs: blocked_reason is a column.
func TestTaskListBlocked_CountsInStore(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "blocked.db")

	// 120 rows so a page-scoped count would be visibly wrong.
	reason := "waiting on review"
	seedTasks(t, dbPath, e2eTaskSeed{
		Total:   120,
		Project: aggProjectID,
		Mutate: func(i int, task *core.Task) {
			if i < 70 {
				r := reason
				task.BlockedReason = &r
			}
		},
	})

	env := append(e2eEnv(t, home, dbPath), "TLC_PROJECT_ID="+aggProjectID)
	out, code := runTLC(t, bin, cwd, env,
		"task", "list", "--counters", "--blocked", "--archived", "--all-projects")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if got := countFor(t, out, string(core.StatusTodo)); got != 70 {
		t.Errorf("blocked TODO = %d, want 70 (whole match set)\n%s", got, out)
	}
}
