package uri

import (
	"strings"
	"testing"
)

func TestErrTaskNotFound_SimpleID(t *testing.T) {
	err := &ErrTaskNotFound{ID: "T-0042"}
	msg := err.Error()

	if !strings.Contains(msg, "T-0042") {
		t.Errorf("expected task ID in error: %q", msg)
	}
	if !strings.Contains(msg, "tlc task list") {
		t.Errorf("expected actionable hint 'tlc task list' in error: %q", msg)
	}
}

func TestErrTaskNotFound_WithProject(t *testing.T) {
	err := &ErrTaskNotFound{ID: "T-0007", ProjectID: "hop-top/tlc"}
	msg := err.Error()

	if !strings.Contains(msg, "T-0007") {
		t.Errorf("expected task ID in error: %q", msg)
	}
	if !strings.Contains(msg, "hop-top/tlc") {
		t.Errorf("expected project ID in error: %q", msg)
	}
	if !strings.Contains(msg, "tlc task list") {
		t.Errorf("expected actionable hint 'tlc task list' in error: %q", msg)
	}
}

func TestErrProjectNotFound(t *testing.T) {
	err := &ErrProjectNotFound{ProjectID: "hop-top/tlc"}
	msg := err.Error()

	if !strings.Contains(msg, "hop-top/tlc") {
		t.Errorf("expected project ID in error: %q", msg)
	}
	if !strings.Contains(msg, "tlc init") {
		t.Errorf("expected actionable hint 'tlc init' in error: %q", msg)
	}
}
