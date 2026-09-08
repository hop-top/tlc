package cli

// `tlc task list` auto-fires stale hooks once per crossing, persisting
// StaleFiredAt (docs/task-crud-spec-0.1.md). An aggregate wants numbers, not
// bookkeeping -- it never reads StaleFiredAt -- and the two paths through the
// aggregate code disagreed about it in opposite directions:
//
//   - the counting path returned before the hook block, so `--counters`
//     silently stopped firing hooks that a plain `task list` fires
//   - the slice path ran the hook block with the limit cleared, so
//     `--counters --blocked` fired hooks UNBOUNDED, one hook exec plus one
//     write transaction per stale task in the whole store, off a command
//     annotated read-only
//
// So the boundary is the aggregate, not the branch: no aggregate does stale
// bookkeeping, and a plain list still does.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

const staleHookProjectID = "stale-hook-fixture"

// staleHookFixture seeds a DB where every task is already stale, writes a
// project config with a hook that appends to a witness file, and returns the
// run context plus the witness path.
func staleHookFixture(t *testing.T) (bin, cwd string, env []string, dbPath, witness string) {
	t.Helper()

	bin = buildTLCBinary(t)
	home := t.TempDir()
	cwd = t.TempDir()
	dbPath = filepath.Join(home, "stale.db")
	witness = filepath.Join(home, "fired.log")

	// Every task last updated well past the 1s timeout below, and blocked,
	// so the --blocked variant matches the same rows.
	old := timeAgo(72)
	reason := "waiting on review"
	seedTasks(t, dbPath, e2eTaskSeed{
		Total:   3,
		Project: staleHookProjectID,
		Mutate: func(_ int, task *core.Task) {
			r := reason
			task.BlockedReason = &r
			task.CreatedAt = old
			task.UpdatedAt = old
		},
	})

	tlcDir := filepath.Join(cwd, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o750); err != nil {
		t.Fatalf("mkdir .tlc: %v", err)
	}
	cfg := "project:\n  id: " + staleHookProjectID +
		"\nstorage:\n  db_path: " + dbPath +
		"\ntask:\n  stale:\n    default_timeout: 1s\n    hooks:\n" +
		"      - command: 'echo fired >> " + witness + "'\n"
	if err := os.WriteFile(filepath.Join(tlcDir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	env = e2eEnv(t, home, dbPath)
	return bin, cwd, env, dbPath, witness
}

// hookFireCount reports how many times the witness file was appended to.
func hookFireCount(t *testing.T, witness string) int {
	t.Helper()
	data, err := os.ReadFile(witness)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("read witness: %v", err)
	}
	return strings.Count(strings.TrimSpace(string(data)), "fired")
}

// staleFiredCount reports how many stored tasks carry a StaleFiredAt.
func staleFiredCount(t *testing.T, dbPath string) int {
	t.Helper()

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer func() { _ = s.Close() }()

	tasks, err := s.ListTasks(t.Context(), core.Query{AllProjects: true, IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	n := 0
	for _, task := range tasks {
		if task.StaleFiredAt != nil {
			n++
		}
	}
	return n
}

// TestTaskListAggregate_FiresNoStaleHooks is the F3 regression. Both
// aggregate paths -- the store count and the slice fallback -- must leave
// StaleFiredAt untouched and run no hook.
func TestTaskListAggregate_FiresNoStaleHooks(t *testing.T) {
	cases := map[string][]string{
		// Counting path: no Go-side filter, so the SQL count serves it.
		"CountersCountingPath": {"task", "list", "--counters", "--archived"},
		"SummaryCountingPath":  {"task", "list", "--summary", "--archived"},
		// Slice fallback: --stale is filtered in Go, and the limit is
		// cleared, so this is the branch that fired hooks unbounded.
		"CountersSliceFallback": {"task", "list", "--counters", "--stale", "--archived"},
		"SummarySliceFallback":  {"task", "list", "--summary", "--stale", "--archived"},
		// The format spellings reach the same paths.
		"FormatCounters": {"task", "list", "--format", "counters", "--archived"},
		"FormatSummary":  {"task", "list", "-f", "summary", "--archived"},
	}

	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			bin, cwd, env, dbPath, witness := staleHookFixture(t)

			out, code := runTLC(t, bin, cwd, env, args...)
			if code != 0 {
				t.Fatalf("exit %d, want 0\n%s", code, out)
			}

			if n := hookFireCount(t, witness); n != 0 {
				t.Errorf("%d stale hook(s) fired; an aggregate is annotated read-only and never reads StaleFiredAt\n%s",
					n, out)
			}
			if n := staleFiredCount(t, dbPath); n != 0 {
				t.Errorf("%d task(s) got a StaleFiredAt written; an aggregate must not persist bookkeeping", n)
			}
		})
	}
}

// TestTaskList_StillFiresStaleHooks guards the other side: row output keeps
// the documented auto-fire, once per crossing.
func TestTaskList_StillFiresStaleHooks(t *testing.T) {
	bin, cwd, env, dbPath, witness := staleHookFixture(t)

	out, code := runTLC(t, bin, cwd, env, "task", "list", "--archived", "--format", "tls")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	// Three stale tasks, three fires, three timestamps persisted.
	if n := hookFireCount(t, witness); n != 3 {
		t.Errorf("stale hooks fired %d time(s), want 3 -- plain list must keep firing\n%s", n, out)
	}
	if n := staleFiredCount(t, dbPath); n != 3 {
		t.Errorf("%d task(s) carry StaleFiredAt, want 3", n)
	}

	// Second run: StaleFiredAt guards the re-fire, so the count holds.
	if _, code := runTLC(t, bin, cwd, env, "task", "list", "--archived", "--format", "tls"); code != 0 {
		t.Fatalf("second run: exit %d", code)
	}
	if n := hookFireCount(t, witness); n != 3 {
		t.Errorf("stale hooks fired %d time(s) after a second list, want 3 (once per crossing)", n)
	}
}
