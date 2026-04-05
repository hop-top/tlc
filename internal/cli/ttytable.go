package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"

	lipgloss "charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/term"
)

// defaultTermWidth is the fallback terminal width when detection fails.
const defaultTermWidth = 100

// renderTTYTable renders rows using lipgloss/v2/table with
// content-based auto-sized columns constrained to the given width.
func renderTTYTable(w io.Writer, headers []string, rows [][]string, width int) {
	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Width(width).
		Border(lipgloss.NormalBorder()).
		BorderRow(false).
		BorderColumn(true).
		BorderHeader(true).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("240"))
			}
			return lipgloss.NewStyle()
		})
	_, _ = fmt.Fprintln(w, t.Render())
}

// termWidth returns the current terminal width, or defaultTermWidth.
func termWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return defaultTermWidth
	}
	return w
}

// ansiRe matches ANSI escape sequences.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripAnsi removes ANSI escape sequences from a string (for testing).
func stripAnsi(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}
