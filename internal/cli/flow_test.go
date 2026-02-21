package cli

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestFormatFlowStatus tests flow status formatting
// Verifies all flow statuses are formatted correctly and contain status text.
func TestFormatFlowStatus(t *testing.T) {
	tests := []struct {
		status core.FlowStatus
		want   string
	}{
		{core.FlowStatusQueued, "QUEUED"},
		{core.FlowStatusRunning, "RUNNING"},
		{core.FlowStatusSucceeded, "SUCCEEDED"},
		{core.FlowStatusFailed, "FAILED"},
		{core.FlowStatusCanceled, "CANCELED"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := formatFlowStatus(tt.status)
			// Just verify it doesn't panic and contains the status text
			if !strings.Contains(got, tt.want) {
				t.Errorf("formatFlowStatus(%q) = %q, want to contain %q", tt.status, got, tt.want)
			}
		})
	}
}
