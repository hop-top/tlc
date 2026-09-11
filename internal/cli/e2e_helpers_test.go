package cli

// Shared plumbing for the subprocess e2e tests in this package: one build,
// one env builder, one runner, one seeder.
//
// These had grown three near-copies apiece. Copies drift, and one already
// had: the env builder set TLC_DB, which nothing reads (viper honors
// TLC_STORAGE_DB_PATH), so tests believing themselves pinned to a temp DB
// were resolving whatever DB the ambient config named.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var (
	// tlcBinaryOnce guards a single build per test binary. Every e2e test
	// here runs the same binary, and the comment on the old helper claimed
	// it was cached while it rebuilt per test — six new callers turned that
	// into six redundant compiles.
	tlcBinaryOnce sync.Once
	tlcBinaryPath string
	tlcBinaryErr  error
)

// buildTLCBinary returns the path to a tlc binary built once per package
// run. Subprocess tests use a real binary rather than the in-process command
// tree so exit codes are real (go run masks them) and the default flag
// values are the ones a user actually gets.
func buildTLCBinary(t *testing.T) string {
	t.Helper()

	tlcBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "tlc-e2e-bin")
		if err != nil {
			tlcBinaryErr = fmt.Errorf("tempdir: %w", err)
			return
		}
		binPath := filepath.Join(dir, "tlc")
		// context.Background, not t.Context: the build is shared across
		// every test in the package, so it must outlive the first
		// caller's context.
		cmd := exec.CommandContext(context.Background(), "go", "build",
			"-buildvcs=false", "-o", binPath, "./cmd/tlc")
		cmd.Dir = testRepoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			tlcBinaryErr = fmt.Errorf("go build tlc: %w\n%s", err, out)
			return
		}
		tlcBinaryPath = binPath
	})

	if tlcBinaryErr != nil {
		t.Fatalf("%v", tlcBinaryErr)
	}
	return tlcBinaryPath
}

// e2eEnv returns an env for subprocess tlc runs, isolated from the
// developer's real state. HOME/XDG are redirected and the DB path is pinned
// via TLC_STORAGE_DB_PATH — the key viper actually reads — so no run can
// reach the user's real store.
//
// GIT_* vars are stripped: project detection consults git, and a subprocess
// inheriting GIT_DIR/GIT_INDEX_FILE would resolve project context against
// whatever repo invoked the test rather than its own tempdir.
func e2eEnv(t *testing.T, home, dbPath string) []string {
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
		"TLC_USER":            true,
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

// runTLC runs the built binary with stdout as a pipe (non-TTY) and returns
// combined output plus the exit code, asserting nothing itself so callers
// can check either.
func runTLC(t *testing.T, bin, cwd string, env []string, args ...string) (string, int) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Env = env
	cmd.Dir = cwd
	cmd.Stdin = nil
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

// runTLCOK runs the binary and fails the test on a non-zero exit, for the
// callers that only care about output.
func runTLCOK(t *testing.T, bin, cwd string, env []string, args ...string) string {
	t.Helper()
	out, code := runTLC(t, bin, cwd, env, args...)
	if code != 0 {
		t.Fatalf("tlc %v: exit %d\n%s", args, code, out)
	}
	return out
}

// e2eTaskSeed describes one seeded task in fixture terms.
type e2eTaskSeed struct {
	Total   int
	Project string
	// Mutate customizes the task built for index i, so each fixture
	// expresses only what makes it different.
	Mutate func(i int, task *core.Task)
}

// seedTasks writes Total tasks into dbPath, applying Mutate per index.
// Replaces three near-identical seeding loops.
func seedTasks(t *testing.T, dbPath string, seed e2eTaskSeed) {
	t.Helper()

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("open seed storage: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := t.Context()
	for i := range seed.Total {
		pid := seed.Project
		task := &core.Task{
			ID:        fmt.Sprintf("T-%04d", i+1),
			Title:     fmt.Sprintf("seeded task %d", i+1),
			Status:    core.StatusTodo,
			ProjectID: &pid,
		}
		if seed.Mutate != nil {
			seed.Mutate(i, task)
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %d: %v", i+1, err)
		}
	}
}

// seedTemplateDB seeds a fixture once per package run and hands each caller
// a private copy of the file. Seeding runs one CreateTask transaction per
// task, so a 150-task fixture rebuilt per test dominated the suite's runtime;
// a file copy does not.
func seedTemplateDB(t *testing.T, key string, seed e2eTaskSeed) string {
	t.Helper()

	tpl := e2eTemplateDB(t, key, seed)
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

var (
	e2eTemplateMu   sync.Mutex
	e2eTemplateDirs = map[string]string{}
)

// e2eTemplateDB builds (or returns) the template DB for a fixture key.
func e2eTemplateDB(t *testing.T, key string, seed e2eTaskSeed) string {
	t.Helper()

	e2eTemplateMu.Lock()
	defer e2eTemplateMu.Unlock()

	if path, ok := e2eTemplateDirs[key]; ok {
		return path
	}

	dir, err := os.MkdirTemp("", "tlc-e2e-tpl")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	path := filepath.Join(dir, key+".db")
	seedTasks(t, path, seed)
	e2eTemplateDirs[key] = path
	return path
}

// timeAgo and timeAhead build fixture due dates relative to now.
func timeAgo(hours int) time.Time {
	return time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
}

func timeAhead(hours int) time.Time {
	return time.Now().UTC().Add(time.Duration(hours) * time.Hour)
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
