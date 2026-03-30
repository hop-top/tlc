package cli

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain sets the lipgloss color profile to TrueColor before running the
// suite, so that tests exercise the same ANSI rendering path as a real
// terminal. This catches regressions where ANSI escape sequences embedded in
// table cell data corrupt runewidth calculations and truncate visible content
// (e.g. leading "T" stripped from task IDs in rows 3+).
//
// TrueColor is safe for all tests because every test that checks output uses
// contains() on raw bytes — ANSI sequences don't break string containment
// checks, they only break cell-width accounting in the table renderer.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}
