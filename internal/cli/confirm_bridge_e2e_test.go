package cli

// E2E coverage for the kit 12fcc-leak --confirm gate and tlc's local
// bridge (T-1392). Verifies, against a built tlc binary running in a
// non-TTY pipe:
//
//   - destructive command WITHOUT --confirm refuses with exit 5
//     (UNAUTHORIZED) — proves kit's gate is wired
//   - destructive command WITH --confirm=yes proceeds past the gate
//     (exit code != 5; downstream "not found" errors are acceptable)
//   - destructive command WITH the local skip flag also proceeds —
//     proves the bridge translates the legacy flag
//
// Three commands are picked, one per side-effect batch grouping:
//   - task delete (C1, has local --yes/--no-prompt → bridge under test)
//   - flow cancel (C2, no local flag → kit gate is the only surface)
//   - auth logout (C3, recently converted Run → RunE so the gate fires)

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// confirmGateExitCode runs `bin args...` with a non-TTY stdin/stdout
// and returns the exit code. -1 if exec fails for non-exit reasons.
func confirmGateExitCode(t *testing.T, bin, cwd string, env []string, args ...string) int {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Env = env
	cmd.Dir = cwd
	cmd.Stdin = nil
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		t.Fatalf("tlc %v: unexpected exec error: %v", args, err)
	}
	return 0
}

// TestConfirmGate_NonTTYRefuses asserts that the three representative
// destructive commands return UNAUTHORIZED (exit 5) when invoked in a
// non-TTY context without --confirm.
func TestConfirmGate_NonTTYRefuses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-TTY pipe assumptions are unix-only")
	}
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "test.db")
	env := styledEnv(home, dbPath)
	_ = os.MkdirAll(cwd, 0o755)

	// Seed init so storage paths exist (some commands open storage
	// before the gate would fire; for those we still expect exit 5
	// because the gate runs in PreRunE-equivalent slot before RunE).
	_ = exec.Command(bin, "init").Run() // best-effort; not assertive

	cases := []struct {
		name string
		args []string
	}{
		{"task delete", []string{"task", "delete", "T-NONE"}},
		{"flow cancel", []string{"flow", "cancel", "run-X"}},
		{"auth logout", []string{"auth", "logout", "github"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := confirmGateExitCode(t, bin, cwd, env, tc.args...)
			if rc != 5 {
				t.Errorf("exit code = %d, want 5 (UNAUTHORIZED); args=%v", rc, tc.args)
			}
		})
	}
}

// TestConfirmGate_NonTTYProceedsWithConfirmYes asserts that the same
// three destructives proceed past the gate when --confirm=yes is set.
// The downstream RunE may still fail (target not found etc.), but the
// exit code must not be 5 (UNAUTHORIZED).
func TestConfirmGate_NonTTYProceedsWithConfirmYes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-TTY pipe assumptions are unix-only")
	}
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "test.db")
	env := styledEnv(home, dbPath)

	cases := []struct {
		name string
		args []string
	}{
		{"task delete", []string{"task", "delete", "T-NONE", "--confirm=yes"}},
		{"flow cancel", []string{"flow", "cancel", "run-X", "--confirm=yes"}},
		{"auth logout", []string{"auth", "logout", "github", "--confirm=yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := confirmGateExitCode(t, bin, cwd, env, tc.args...)
			if rc == 5 {
				t.Errorf("exit code = 5 (gate still refused) with --confirm=yes; args=%v", tc.args)
			}
		})
	}
}

// TestConfirmGate_LocalFlagBridges asserts that the legacy local
// "skip" flags continue to work — the bridge must translate them into
// --confirm=yes before the kit gate runs.
func TestConfirmGate_LocalFlagBridges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-TTY pipe assumptions are unix-only")
	}
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "test.db")
	env := styledEnv(home, dbPath)

	cases := []struct {
		name string
		args []string
	}{
		{"task delete --yes", []string{"task", "delete", "T-NONE", "--yes"}},
		{"task delete --no-prompt", []string{"task", "delete", "T-NONE", "--no-prompt"}},
		{"track abandon --no-prompt", []string{"track", "abandon", "T-NONE", "--no-prompt"}},
		{"project prune --yes", []string{"project", "prune", "--yes"}},
		{"project prune -y", []string{"project", "prune", "-y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := confirmGateExitCode(t, bin, cwd, env, tc.args...)
			if rc == 5 {
				t.Errorf("local flag did not bridge to --confirm=yes; exit code 5 (UNAUTHORIZED); args=%v", tc.args)
			}
		})
	}
}
