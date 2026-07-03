package cli

import (
	"bytes"
	"strings"
	"testing"

	"hop.top/kit/go/console/output"
)

// colTestRow is a local test struct with table tags for col-projection tests.
type colTestRow struct {
	ID    string `table:"ID"`
	Title string `table:"Title"`
	Extra string `table:"Extra"`
}

func TestRenderStyledListCols_RestrictsColumns(t *testing.T) {
	rows := []colTestRow{{"a", "b", "c"}}
	var buf bytes.Buffer

	err := renderStyledListCols(&buf, output.Table, rows, nil, []string{"ID", "Title"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "ID") {
		t.Errorf("expected output to contain %q, got:\n%s", "ID", got)
	}
	if !strings.Contains(got, "Title") {
		t.Errorf("expected output to contain %q, got:\n%s", "Title", got)
	}
	if strings.Contains(got, "Extra") {
		t.Errorf("expected output NOT to contain %q, got:\n%s", "Extra", got)
	}
	if strings.Contains(got, "c") {
		t.Errorf("expected output NOT to contain Extra cell value %q, got:\n%s", "c", got)
	}
}

func TestRenderStyledListCols_EmptyColsAllColumns(t *testing.T) {
	rows := []colTestRow{{"a", "b", "c"}}

	var buf1 bytes.Buffer
	if err := renderStyledListCols(&buf1, output.Table, rows, nil, nil); err != nil {
		t.Fatalf("renderStyledListCols nil cols: %v", err)
	}

	var buf2 bytes.Buffer
	if err := renderStyledList(&buf2, output.Table, rows, nil); err != nil {
		t.Fatalf("renderStyledList: %v", err)
	}

	if buf1.String() != buf2.String() {
		t.Errorf("renderStyledListCols(nil cols) output differs from renderStyledList:\ngot:  %q\nwant: %q",
			buf1.String(), buf2.String())
	}

	got := buf1.String()
	for _, header := range []string{"ID", "Title", "Extra"} {
		if !strings.Contains(got, header) {
			t.Errorf("expected output to contain %q, got:\n%s", header, got)
		}
	}
}

func TestRenderStyledListCols_UnknownColumnErrors(t *testing.T) {
	rows := []colTestRow{{"a", "b", "c"}}
	var buf bytes.Buffer

	err := renderStyledListCols(&buf, output.Table, rows, nil, []string{"Nope"})
	if err == nil {
		t.Fatal("expected error for unknown column, got nil")
	}
}
