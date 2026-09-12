package cli

// E2E coverage for --cols / --columns on the list commands.
//
// Subprocess, not in-process: the flag reaches the reader through the
// global viper singleton, which any earlier test in this package can
// pre-populate. An in-process assertion therefore passes against a
// binary that ignores the flag entirely. Only a fresh process proves
// the user-visible behavior.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// colsHeaderFields returns the whitespace-separated fields of the first
// non-empty output line, i.e. the table header row.
func colsHeaderFields(t *testing.T, out string) []string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		return strings.Fields(line)
	}
	t.Fatalf("no header row in output:\n%s", out)
	return nil
}

func assertHeaders(t *testing.T, out string, want []string) {
	t.Helper()
	got := colsHeaderFields(t, out)
	if len(got) != len(want) {
		t.Fatalf("header %v, want %v\nfull output:\n%s", got, want, out)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("header %v, want %v\nfull output:\n%s", got, want, out)
		}
	}
}

// colsFixture seeds a project + two tasks and returns (bin, cwd, env).
func colsFixture(t *testing.T) (bin, cwd string, env []string) {
	t.Helper()
	bin = buildTLCBinary(t)
	home := t.TempDir()
	cwd = t.TempDir()
	dbPath := filepath.Join(home, "test.db")
	env = e2eEnv(t, home, dbPath)

	runTLCOK(t, bin, cwd, env, "init")
	runTLCOK(t, bin, cwd, env, "task", "create", "alpha task")
	runTLCOK(t, bin, cwd, env, "task", "create", "beta task")
	return bin, cwd, env
}

func TestTaskListColsFlag_RestrictsColumns(t *testing.T) {
	bin, cwd, env := colsFixture(t)

	for _, flag := range []string{"--cols", "--columns"} {
		t.Run(flag, func(t *testing.T) {
			out := runTLCOK(t, bin, cwd, env, "task", "list", flag, "id,title")
			assertHeaders(t, out, []string{"ID", "Title"})
		})
	}
}

func TestTaskListColsFlag_RepeatableAndReordered(t *testing.T) {
	bin, cwd, env := colsFixture(t)

	out := runTLCOK(t, bin, cwd, env,
		"task", "list", "--cols", "status", "--cols", "id")
	assertHeaders(t, out, []string{"Status", "ID"})
}

func TestTrackListColsFlag_RestrictsColumns(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "test.db")
	env := e2eEnv(t, home, dbPath)

	runTLCOK(t, bin, cwd, env, "init")
	runTLCOK(t, bin, cwd, env, "track", "create", "demo-track", "--type", "feature")

	out := runTLCOK(t, bin, cwd, env, "track", "list", "--cols", "id,title")
	assertHeaders(t, out, []string{"ID", "Title"})
}

// TestTaskListColumns_ConfigStillApplies pins the config-file ladder that
// already worked before the flag did: the flag fix must not cost it.
func TestTaskListColumns_ConfigStillApplies(t *testing.T) {
	bin, cwd, env := colsFixture(t)

	cfg := filepath.Join(cwd, ".tlc", "config.yaml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	extra := string(data) + "\ntask:\n  list:\n    columns: [id, status]\n"
	if err := os.WriteFile(cfg, []byte(extra), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := runTLCOK(t, bin, cwd, env, "task", "list")
	assertHeaders(t, out, []string{"ID", "Status"})
}

// TestTaskListColsFlag_OverridesConfig pins precedence: an explicit flag
// beats the configured column set.
func TestTaskListColsFlag_OverridesConfig(t *testing.T) {
	bin, cwd, env := colsFixture(t)

	cfg := filepath.Join(cwd, ".tlc", "config.yaml")
	data, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	extra := string(data) + "\ntask:\n  list:\n    columns: [id, status]\n"
	if err := os.WriteFile(cfg, []byte(extra), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := runTLCOK(t, bin, cwd, env, "task", "list", "--cols", "id,title")
	assertHeaders(t, out, []string{"ID", "Title"})
}
