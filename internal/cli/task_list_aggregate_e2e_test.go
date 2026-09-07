package cli

// Regression coverage: aggregate output (`--counters`, `--summary`) must
// report counts of the whole match set, never of the page the default
// `--limit 100` would return.
//
// The defect these tests pin was silent. A truncated count is
// indistinguishable from a real one -- no warning, no stderr note, exit 0 --
// so a caller has no signal to raise the limit and the wrong number gets
// used downstream. That means asserting today's output proves nothing: the
// seeded fixture deliberately holds more tasks than the default limit, so a
// count equal to the limit is a failure and only the true total passes.
//
// Runs against a built binary, not the in-process command tree, so exit
// codes are real (`go run` masks them) and the default flag values are the
// ones a user actually gets.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
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
)

// aggEnv returns an env for subprocess tlc runs, isolated from the
// developer's real state. HOME/XDG are redirected and the DB path is
// pinned, so no run can reach ~/.local/share/tlc/db.sqlite.
//
// GIT_* vars are stripped: tests in this repo have leaked git state into
// the user's config when run under hooks, and a subprocess inheriting
// GIT_DIR/GIT_INDEX_FILE would resolve project context against whatever
// repo invoked the test rather than its own tempdir.
func aggEnv(t *testing.T, home, dbPath string) []string {
	t.Helper()

	drop := map[string]bool{
		"HOME":                true,
		"USERPROFILE":         true,
		"XDG_DATA_HOME":       true,
		"XDG_CONFIG_HOME":     true,
		"TLC_DB":              true,
		"TLC_CONFIG":          true,
		"TLC_STORAGE_DB_PATH": true,
		"TLC_PROJECT_ID":      true,
		"TLC_OUTPUT_FORMAT":   true,
	}

	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"TLC_STORAGE_DB_PATH=" + dbPath,
	}
	for _, e := range os.Environ() {
		key := strings.SplitN(e, "=", 2)[0]
		if drop[key] || strings.HasPrefix(key, "GIT_") {
			continue
		}
		env = append(env, e)
	}
	return env
}

// seedAggregateDB creates a DB holding aggSeedTotal tasks, more than the
// default limit, split across two statuses so the per-status breakdown is
// checked too and not just a single total.
func seedAggregateDB(t *testing.T, dbPath, projectID string) {
	t.Helper()

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("open seed storage: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := t.Context()
	for i := range aggSeedTotal {
		status := core.StatusDone
		if i >= aggSeedDone {
			status = core.StatusTodo
		}
		pid := projectID
		task := &core.Task{
			ID:        fmt.Sprintf("T-%04d", i+1),
			Title:     fmt.Sprintf("seeded task %d", i+1),
			Status:    status,
			ProjectID: &pid,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %d: %v", i+1, err)
		}
	}
}

// runAgg runs the built binary and returns stdout+stderr and the exit
// code, asserting nothing itself so callers can check both.
func runAgg(t *testing.T, bin, cwd string, env []string, args ...string) (string, int) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Env = env
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()

	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("tlc %v: %v\n%s", args, err, out)
		}
		code = exitErr.ExitCode()
	}
	return string(out), code
}

// countFor extracts the integer printed against a status label in the
// aggregate tables, both of which render as "  <LABEL>  <n>".
func countFor(t *testing.T, out, label string) int {
	t.Helper()

	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != label {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(fields[1], "%d", &n); err != nil {
			t.Fatalf("parse count for %q from %q: %v", label, line, err)
		}
		return n
	}
	t.Fatalf("no %q row in aggregate output:\n%s", label, out)
	return 0
}

// TestTaskListAggregate_CountsFullMatchSet is the headline regression.
// Every aggregate invocation must report the seeded totals, with and
// without an explicit -n, and must never report the default limit.
func TestTaskListAggregate_CountsFullMatchSet(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "agg.db")
	const projectID = "agg-fixture"

	seedAggregateDB(t, dbPath, projectID)
	env := append(aggEnv(t, home, dbPath), "TLC_PROJECT_ID="+projectID)

	cases := []struct {
		name string
		args []string
	}{
		// No -n: the default limit of 100 applies, which is exactly
		// what silently truncated the count.
		{"CountersDefaultLimit", []string{"task", "list", "--counters", "--archived", "--all-projects"}},
		{"SummaryDefaultLimit", []string{"task", "list", "--summary", "--archived", "--all-projects"}},
		// Explicit -n above the true total: correct before the fix,
		// so it guards against a regression in the other direction.
		{"CountersExplicitHighLimit", []string{"task", "list", "--counters", "--archived", "--all-projects", "-n", "1000"}},
		{"SummaryExplicitHighLimit", []string{"task", "list", "--summary", "--archived", "--all-projects", "-n", "1000"}},
		// Explicit -n *below* the true total: the flags contradict,
		// and the aggregate must win rather than report 5.
		{"CountersExplicitLowLimit", []string{"task", "list", "--counters", "--archived", "--all-projects", "-n", "5"}},
		{"SummaryExplicitLowLimit", []string{"task", "list", "--summary", "--archived", "--all-projects", "-n", "5"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runAgg(t, bin, cwd, env, tc.args...)
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
// specifically. --summary states Total as fact, so a truncated one is the
// most trust-damaging form of the defect.
func TestTaskListSummary_TotalIsMatchSetTotal(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "agg.db")
	const projectID = "agg-fixture"

	seedAggregateDB(t, dbPath, projectID)
	env := append(aggEnv(t, home, dbPath), "TLC_PROJECT_ID="+projectID)

	for _, args := range [][]string{
		{"task", "list", "--summary", "--archived", "--all-projects"},
		{"task", "list", "--summary", "--archived", "--all-projects", "-n", "1000"},
	} {
		out, code := runAgg(t, bin, cwd, env, args...)
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

// TestTaskListAggregate_NotesIgnoredLimit asserts the contradiction is
// surfaced rather than silently resolved. Silently picking one of two
// contradictory flags is what produced the original defect, so an
// explicit --limit alongside an aggregate says so on stderr -- while
// stdout still carries the correct count.
func TestTaskListAggregate_NotesIgnoredLimit(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "agg.db")
	const projectID = "agg-fixture"

	seedAggregateDB(t, dbPath, projectID)
	env := append(aggEnv(t, home, dbPath), "TLC_PROJECT_ID="+projectID)

	out, code := runAgg(t, bin, cwd, env,
		"task", "list", "--counters", "--archived", "--all-projects", "-n", "5")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "--limit/--offset ignored") {
		t.Errorf("expected a note that --limit was ignored, got:\n%s", out)
	}

	// Without an explicit --limit there is no contradiction, so no note.
	quiet, code := runAgg(t, bin, cwd, env,
		"task", "list", "--counters", "--archived", "--all-projects")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, quiet)
	}
	if strings.Contains(quiet, "ignored") {
		t.Errorf("unexpected ignore note without an explicit --limit:\n%s", quiet)
	}
}

// TestTaskListAggregate_ListOutputStillPaginates guards the other side of
// the fix. Pagination on a list is correct and load-bearing; only
// aggregates were meant to escape it.
func TestTaskListAggregate_ListOutputStillPaginates(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "agg.db")
	const projectID = "agg-fixture"

	seedAggregateDB(t, dbPath, projectID)
	env := append(aggEnv(t, home, dbPath), "TLC_PROJECT_ID="+projectID)

	out, code := runAgg(t, bin, cwd, env,
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
