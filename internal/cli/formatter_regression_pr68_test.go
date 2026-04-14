package cli

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// T-0595: Regression — buildTaskReference with nil project produces triple-slash.
// A nil project must yield "tlc:///T-XXXX" (triple slash = local ref),
// not "tlc://T-XXXX" (double slash = authority-style, wrong).
func TestBuildTaskReference_NilProject_TripleSlash(t *testing.T) {
	got := buildTaskReference("T-0001", nil)
	want := "tlc:///T-0001"
	if got != want {
		t.Fatalf("buildTaskReference(T-0001, nil) = %q, want %q", got, want)
	}

	// Verify triple slash specifically (not double).
	if !strings.HasPrefix(got, "tlc:///") {
		t.Fatalf("expected triple-slash prefix tlc:///, got %q", got)
	}
}

// T-0595 (continued): Empty ProjectID also produces triple slash.
func TestBuildTaskReference_EmptyProjectID_TripleSlash(t *testing.T) {
	proj := &core.ProjectDetection{ProjectID: ""}
	got := buildTaskReference("T-0042", proj)
	want := "tlc:///T-0042"
	if got != want {
		t.Fatalf("buildTaskReference(T-0042, empty) = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, "tlc:///") {
		t.Fatalf("expected triple-slash prefix, got %q", got)
	}
}

// T-0596: Regression — isDefaultRef suppresses all known local reference forms.
// The TLS formatter must suppress ref: output for any default-generated
// local reference, regardless of historical scheme variant.
func TestIsDefaultRef_SuppressesAllForms(t *testing.T) {
	tests := []struct {
		ref    string
		taskID string
		want   bool
	}{
		// Triple-slash (canonical)
		{"tlc:///T-0001", "T-0001", true},
		// Double-slash (legacy)
		{"tlc://T-0001", "T-0001", true},
		// task:// scheme (legacy)
		{"task://T-0001", "T-0001", true},
		// Non-default: different task ID
		{"tlc:///T-0099", "T-0001", false},
		// Non-default: project-qualified
		{"tlc://myorg/myrepo/T-0001", "T-0001", false},
		// Non-default: completely unrelated
		{"https://example.com/T-0001", "T-0001", false},
		// Empty ref
		{"", "T-0001", false},
	}
	for _, tt := range tests {
		name := tt.ref
		if name == "" {
			name = "<empty>"
		}
		t.Run(name, func(t *testing.T) {
			got := isDefaultRef(tt.ref, tt.taskID)
			if got != tt.want {
				t.Errorf("isDefaultRef(%q, %q) = %v, want %v",
					tt.ref, tt.taskID, got, tt.want)
			}
		})
	}
}
