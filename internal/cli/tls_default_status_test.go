package cli

import (
	"testing"

	"hop.top/tlc/internal/core"
)

// TestTLS_DefaultMarkersRoundTrip writes one task per default status
// through formatTLS and reads it back with parseTLS. Every status must
// survive, which pins both the marker table and the decode path.
func TestTLS_DefaultMarkersRoundTrip(t *testing.T) {
	markers := map[core.TaskStatus]string{
		"backlog":     "[-]",
		"todo":        "[ ]",
		"in_progress": "[>]",
		"in_review":   "[?]",
		"blocked":     "[!]",
		"done":        "[x]",
		"cancelled":   "[~]", //nolint:misspell // status name
	}
	for status, marker := range markers {
		t.Run(string(status), func(t *testing.T) {
			task := &core.Task{ID: "T-0001", Title: "round trip", Status: status}
			line := formatTLS(task)
			if len(line) < 3 || line[:3] != marker {
				t.Fatalf("formatTLS(%s) = %q, want prefix %q", status, line, marker)
			}
			got, err := parseTLS(line)
			if err != nil {
				t.Fatalf("parseTLS(%q): %v", line, err)
			}
			if got.Status != status {
				t.Errorf("round trip status = %q, want %q", got.Status, status)
			}
		})
	}
}

// TestTLS_UnknownMarkerFallsBackByRole pins the legacy heuristics: an
// undeclared marker resolves through roles, never to an undeclared name.
func TestTLS_UnknownMarkerFallsBackByRole(t *testing.T) {
	wm := core.DefaultWorkflow()
	cases := map[string]core.TaskStatus{
		"":            "todo",
		"X":           "done",
		"DONE":        "done",
		"wip":         "todo",
		"in progress": "in_progress",
		"zzz":         "todo",
	}
	for content, want := range cases {
		if got := tlsStatusFor(wm, content); got != want {
			t.Errorf("tlsStatusFor(%q) = %q, want %q", content, got, want)
		}
	}
}
