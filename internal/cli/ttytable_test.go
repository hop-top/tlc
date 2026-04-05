package cli

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTTYTableRendersHeaders(t *testing.T) {
	buf := new(bytes.Buffer)
	cols := []string{"ID", "Name"}
	rows := [][]string{{"T-001", "Fix bug"}}
	renderTTYTable(buf, cols, rows, 60)
	out := buf.String()
	if !strings.Contains(out, "ID") {
		t.Fatal("missing header")
	}
	if !strings.Contains(out, "Fix bug") {
		t.Fatal("missing row")
	}
}

func TestTTYTableRespectsWidth(t *testing.T) {
	buf := new(bytes.Buffer)
	cols := []string{"A", "B"}
	rows := [][]string{{"hello", "world"}}
	renderTTYTable(buf, cols, rows, 40)
	for _, line := range strings.Split(buf.String(), "\n") {
		clean := stripAnsi(line)
		width := utf8.RuneCountInString(clean)
		if width > 45 { // small margin for border chars
			t.Errorf("line too wide (%d): %q", width, clean)
		}
	}
}
