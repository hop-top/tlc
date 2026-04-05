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
	// primaryRows marks row indices that should receive primary emphasis
	// using the theme accent color.
	primaryRows map[int]bool
	// secondaryRows marks row indices that should receive secondary emphasis
	// using the theme secondary color.
	secondaryRows map[int]bool
	// mutedRows marks row indices that should render in a muted style
	// using the theme muted color.
	mutedRows map[int]bool
}

// TableOption configures renderTTYTable.
type TableOption func(*tableOpts)

// WithPrimaryRows marks the given row indices for primary emphasis.
func WithPrimaryRows(indices map[int]bool) TableOption {
	return func(o *tableOpts) { o.primaryRows = indices }
}

// WithSecondaryRows marks the given row indices for secondary emphasis.
func WithSecondaryRows(indices map[int]bool) TableOption {
	return func(o *tableOpts) { o.secondaryRows = indices }
}

// WithMutedRows marks the given row indices to render in a muted style.
func WithMutedRows(indices map[int]bool) TableOption {
	return func(o *tableOpts) { o.mutedRows = indices }
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
			if o.secondaryRows[row] {
				return lipgloss.NewStyle().Foreground(theme.Secondary)
			}
			if o.mutedRows[row] {
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
