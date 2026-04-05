package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/term"
	"hop.top/tlc/internal/core"
)

// defaultTermWidth is the fallback terminal width when detection fails.
const defaultTermWidth = 100

// tableOpts holds optional rendering hints for renderTTYTable.
type tableOpts struct {
	// primaryRows marks row indices that match the first filter value.
	// When non-nil, primary rows render with full color; others are muted.
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

	// Build column role lookup: "id" columns get pink, "title" columns
	// get green, everything else gets white.
	idCols := make(map[int]bool, len(headers))
	titleCols := make(map[int]bool, len(headers))
	for i, h := range headers {
		switch strings.ToLower(h) {
		case "id", "run id", "task id", "flow id":
			idCols[i] = true
		case "title", "label", "name":
			titleCols[i] = true
		}
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
			if o.primaryRows != nil {
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
			}
			switch {
			case titleCols[col]:
				return lipgloss.NewStyle().Foreground(theme.Accent)
			case idCols[col]:
				return lipgloss.NewStyle().Foreground(theme.Secondary)
			default:
				return lipgloss.NewStyle()
			}
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

// filterFieldToColumn maps query filter fields to table header names.
var filterFieldToColumn = map[string]string{
	"status":      "Status",
	"assigned_to": "Assigned",
	"priority":    "Priority",
	"track_id":    "Track",
}

// primaryRowsFromFilters finds the first filter field that appears more
// than once, then marks rows whose column value matches the first filter
// value as primary. Returns nil when no multi-value filter exists.
func primaryRowsFromFilters(headers []string, rows [][]string, filters []core.FieldFilter) map[int]bool {
	if len(filters) < 2 {
		return nil
	}

	// Count occurrences per field; remember insertion order.
	type fieldInfo struct {
		count      int
		firstValue string
	}
	seen := make(map[string]*fieldInfo)
	var order []string
	for _, f := range filters {
		info, ok := seen[f.Field]
		if !ok {
			info = &fieldInfo{firstValue: fmt.Sprint(f.Value)}
			seen[f.Field] = info
			order = append(order, f.Field)
		}
		info.count++
	}

	// Find first field with >1 values.
	var matchField, matchValue string
	for _, field := range order {
		if seen[field].count > 1 {
			matchField = field
			matchValue = seen[field].firstValue
			break
		}
	}
	if matchField == "" {
		return nil
	}

	// Normalize status filter values to display labels.
	if matchField == "status" {
		matchValue = formatStatusPlain(core.TaskStatus(matchValue))
	}

	// Resolve column header name.
	colName := filterFieldToColumn[matchField]
	if colName == "" {
		return nil
	}

	// Find column index.
	colIdx := -1
	for i, h := range headers {
		if strings.EqualFold(h, colName) {
			colIdx = i
			break
		}
	}
	if colIdx < 0 {
		return nil
	}

	// Mark rows matching the first filter value.
	primary := make(map[int]bool)
	for i, row := range rows {
		if colIdx < len(row) && strings.EqualFold(row[colIdx], matchValue) {
			primary[i] = true
		}
	}
	if len(primary) == len(rows) {
		return nil
	}
	return primary
}
