package cli

import (
	"strings"
	"testing"

	"github.com/google/oss-tlc-cli/internal/core"
)

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
