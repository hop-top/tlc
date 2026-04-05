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

// tableOpts holds optional rendering hints for renderTTYTable.
type tableOpts struct {
	// primaryRows marks row indices that match the first filter value.
	// When non-nil, enables row-emphasis mode: primary rows render green,
	// non-primary rows render white (or pink/muted if blocker/blocked).
	primaryRows map[int]bool
	// blockerRows marks tasks that block other tasks (render in pink).
	blockerRows map[int]bool
	// blockedRows marks tasks that are themselves blocked (render muted).
	blockedRows map[int]bool
}

// TableOption configures renderTTYTable.
type TableOption func(*tableOpts)

// WithPrimaryRows marks the given row indices as primary (emphasized).
func WithPrimaryRows(indices map[int]bool) TableOption {
	return func(o *tableOpts) { o.primaryRows = indices }
}

// WithBlockerRows marks rows that are blocking other tasks (pink).
func WithBlockerRows(indices map[int]bool) TableOption {
	return func(o *tableOpts) { o.blockerRows = indices }
}

// WithBlockedRows marks rows that are themselves blocked (muted).
func WithBlockedRows(indices map[int]bool) TableOption {
	return func(o *tableOpts) { o.blockedRows = indices }
}

// renderTTYTable renders rows using lipgloss/v2/table with
// content-based auto-sized columns constrained to the given width.
func renderTTYTable(w io.Writer, headers []string, rows [][]string, width int, opts ...TableOption) {
	theme := kitRootInstance.Theme

	var o tableOpts
	for _, fn := range opts {
		fn(&o)
	}

	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Width(width).
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(theme.Muted)).
		BorderRow(false).
		BorderColumn(true).
		BorderHeader(true).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).Foreground(theme.Muted)
			}
			if o.primaryRows[row] {
				return lipgloss.NewStyle().Foreground(theme.Accent)
			}
			if o.blockerRows[row] {
				return lipgloss.NewStyle().Foreground(theme.Secondary)
			}
			if o.blockedRows[row] {
				return lipgloss.NewStyle().Foreground(theme.Muted)
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
