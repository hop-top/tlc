package cli

import (
	"os"
	"testing"
)

// TestMain sets the COLORTERM env to truecolor before running the suite,
// so that lipgloss v2 detects TrueColor and tests exercise the same ANSI
// rendering path as a real terminal. This catches regressions where ANSI
// escape sequences embedded in table cell data corrupt runewidth
// calculations and truncate visible content.
func TestMain(m *testing.M) {
	os.Setenv("COLORTERM", "truecolor")
	os.Exit(m.Run())
}
