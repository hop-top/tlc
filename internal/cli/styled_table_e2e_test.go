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
	env := e2eEnv(t, home, dbPath)

	// Seed: init + one track + one task so listings have content.
	runTLCOK(t, bin, cwd, env, "init")
	runTLCOK(t, bin, cwd, env, "track", "create", "demo-track", "--type", "feature")
	runTLCOK(t, bin, cwd, env, "task", "create", "alpha task")

	// Non-TTY path.
	plain := runTLCOK(t, bin, cwd, env, "task", "list")
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
