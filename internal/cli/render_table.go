package cli

import (
	"fmt"
	"io"
	"sync"

	"hop.top/kit/go/console/output"
)

// tableStyleMu guards the package-level default TableStyle. Reads happen
// on every renderStyledList call; writes happen once during root init.
// Mirrors the aps listing.SetTableStyle pattern (May 2026 rollout).
var (
	tableStyleMu  sync.RWMutex
	tableStyle    output.TableStyle
	tableStyleSet bool
)

// setTableStyle installs the default TableStyle that renderStyledList
// forwards to output.Render via output.WithTableStyle. Called once
// during cli init from kitRootInstance.TableStyle().
//
// kit/output gates the styled renderer on a TTY writer, so passing a
// style here is safe for tests piping into bytes.Buffer: the styled
// path falls back to the plain tabwriter renderer when the writer is
// not a *os.File terminal.
func setTableStyle(s output.TableStyle) {
	tableStyleMu.Lock()
	tableStyle = s
	tableStyleSet = true
	tableStyleMu.Unlock()
}

// activeTableStyle returns the installed style and whether one was set.
func activeTableStyle() (output.TableStyle, bool) {
	tableStyleMu.RLock()
	defer tableStyleMu.RUnlock()
	return tableStyle, tableStyleSet
}

// renderStyledList writes rows as a list in the requested format.
//
// format is one of output.Table, output.JSON, output.YAML. Empty
// defaults to output.Table. Unknown formats forward to kit/output
// which returns a descriptive error.
//
// rows is any slice — typically a typed []SomeRow with kit/output
// table:"" tags. The Table formatter inspects struct tags to derive
// headers; JSON/YAML use json/yaml tags.
//
// emphasis is a row-index → kit/output.EmphasisKind map. Pass nil for
// no emphasis. When the styled path activates (TTY writer + style set),
// each entry maps a row to RowEmphasis(idx, kind).
//
// When setTableStyle has installed a default style, renderStyledList
// forwards it via output.WithTableStyle. The styled path activates
// only on TTY writers; non-TTY writers (pipes, files, bytes.Buffer)
// keep emitting plain tabwriter output.
func renderStyledList[T any](w io.Writer, format string, rows []T, emphasis map[int]output.EmphasisKind) error {
	return renderStyledListCols(w, format, rows, emphasis, nil)
}

// renderStyledListCols renders rows with an explicit table-header
// allow-list. cols entries are table:"" header strings (e.g. "ID",
// "Title"). When cols is empty, it is identical to renderStyledList
// (styled + emphasis). When cols is non-empty, it renders via kit's
// cols-aware plain formatter: exact column selection, no row emphasis.
//
// Column projection is only meaningful for the table format; for
// json/yaml/csv the cols path is skipped and the standard path runs
// (structured formats emit full records).
//
// cols are forwarded verbatim to kit — they are matched case-sensitively
// against table:"" headers. Unknown headers produce an error.
func renderStyledListCols[T any](w io.Writer, format string, rows []T, emphasis map[int]output.EmphasisKind, cols []string) error {
	if format == "" {
		format = output.Table
	}
	if rows == nil {
		rows = []T{}
	}

	// Non-empty column selection: use kit's cols-aware plain formatter.
	if len(cols) > 0 && format == output.Table {
		f, ok := output.Default.Lookup(format)
		if !ok {
			return fmt.Errorf("unknown output format %q", format)
		}
		return f.Render(w, rows, nil, cols) //nolint:wrapcheck // kit typed errors surface verbatim
	}

	// Default path: styled + emphasis, unchanged from pre-refactor behavior.
	style, hasStyle := activeTableStyle()
	if !hasStyle && len(emphasis) == 0 {
		return output.Render(w, format, rows) //nolint:wrapcheck // pass-through helper; kit's typed errors surface verbatim
	}

	opts := make([]output.RenderOption, 0, 1+len(emphasis))
	if hasStyle {
		opts = append(opts, output.WithTableStyle(style))
	}
	for idx, kind := range emphasis {
		opts = append(opts, output.RowEmphasis(idx, kind))
	}
	return output.Render(w, format, rows, opts...) //nolint:wrapcheck // pass-through helper; kit's typed errors surface verbatim
}
