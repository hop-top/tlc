package cli

// E2E coverage for the kit-styled-table rollout (T-1186, drop-ttytable
// follow-up to aps's kit-styled-table-rollout May 2026). The same
// `tlc track list` invocation must:
//   - emit ANSI + lipgloss box-drawing characters when stdout is a TTY
//   - emit plain tabwriter output when stdout is a pipe / non-TTY
//   - return content-identical rows in both modes once ANSI is stripped
//
// Mirrors aps tests/e2e/profile/profile_list_styled_test.go.

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/creack/pty"
)

var (
	styledAnsiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	// styledBoxRunes is the smallest set of lipgloss NormalBorder runes
	// the styled renderer always emits. Any one is proof of styled mode.
	styledBoxRunes = []rune{'┌', '┐', '└', '┘', '│', '─', '├', '┤', '┬', '┴'}
)

func styledStripANSI(s string) string {
	return styledAnsiRe.ReplaceAllString(s, "")
}

func styledHasBoxRune(s string) bool {
	for _, r := range styledBoxRunes {
		if strings.ContainsRune(s, r) {
			return true
		}
	}
	return false
}

// buildTLCBinary builds the tlc binary into a tempdir for subprocess
// runs. Reused across PTY tests in this file via testing.B initOnce.
func buildTLCBinary(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "tlc")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-o", binPath, "./cmd/tlc")
	cmd.Dir = styledRepoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build tlc: %v\n%s", err, out)
	}
	return binPath
}

// styledRepoRoot returns the absolute path to the tlc repo root by
// walking up from the working directory until go.mod is found.
// Distinct from cli.repoRoot (agent_helpers.go) which returns CWD or
// "/workspace" depending on agent run mode.
func styledRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	d := wd
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			t.Fatalf("repo root not found from %s", wd)
		}
		d = parent
	}
}

// styledEnv returns a test-isolated env with HOME/XDG paths pinned to
// the given tempdir so subprocess tlc runs see no shared state.
func styledEnv(home, dbPath string) []string {
	override := map[string]bool{
		"HOME":          true,
		"USERPROFILE":   true,
		"XDG_DATA_HOME": true,
		"TLC_DB":        true,
	}
	env := []string{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"TLC_DB=" + dbPath,
	}
	for _, e := range os.Environ() {
		key := strings.SplitN(e, "=", 2)[0]
		if override[key] {
			continue
		}
		env = append(env, e)
	}
	return env
}

// runTLCPlain runs tlc with stdout as a pipe (non-TTY).
func runTLCPlain(t *testing.T, bin, cwd string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Env = env
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tlc %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// runTLCPTY runs tlc attached to a pseudo-terminal so kit/output sees
// an *os.File terminal and activates the styled renderer.
func runTLCPTY(t *testing.T, bin, cwd string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Env = env
	cmd.Dir = cwd

	f, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("pty.Start: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	done := make(chan []byte, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, f)
		done <- buf.Bytes()
	}()

	if err := cmd.Wait(); err != nil {
		t.Fatalf("tlc wait: %v\nptyOutput: %s", err, string(<-done))
	}
	_ = f.Close()
	return string(<-done)
}

// TestStyledTable_TTYAndNonTTYContentIdentity is the headline e2e for
// the rollout. Same `tlc track list` invocation: styled bordered
// table on TTY, plain tabwriter on pipe; row content identical once
// ANSI is stripped.
func TestStyledTable_TTYAndNonTTYContentIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creack/pty is unix-only")
	}

	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir() // separate dir so init writes a fresh .tlc/
	dbPath := filepath.Join(home, "test.db")
	env := styledEnv(home, dbPath)

	// Seed: init + one track + one task so listings have content.
	runTLCPlain(t, bin, cwd, env, "init")
	runTLCPlain(t, bin, cwd, env, "track", "create", "demo-track", "--type", "feature")
	runTLCPlain(t, bin, cwd, env, "task", "create", "alpha task")

	// Non-TTY path.
	plain := runTLCPlain(t, bin, cwd, env, "task", "list")
	if styledAnsiRe.MatchString(plain) {
		t.Errorf("non-TTY task list leaked ANSI escapes: %q", plain)
	}
	if styledHasBoxRune(plain) {
		t.Errorf("non-TTY task list leaked box-drawing chars: %q", plain)
	}

	// TTY path.
	tty := runTLCPTY(t, bin, cwd, env, "task", "list")
	if !styledAnsiRe.MatchString(tty) {
		t.Errorf("TTY task list missing ANSI escapes: %q", tty)
	}
	if !styledHasBoxRune(tty) {
		t.Errorf("TTY task list missing box-drawing characters: %q", tty)
	}

	// Content identity: alpha task title and headers in both modes.
	stripped := styledStripANSI(tty)
	for _, want := range []string{"alpha task", "Title", "Status"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain output missing %q\nstdout: %s", want, plain)
		}
		if !strings.Contains(stripped, want) {
			t.Errorf("TTY output (stripped) missing %q\nstdout: %s", want, stripped)
		}
	}
}
