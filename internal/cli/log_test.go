package cli

import (
	"strings"
	"testing"
)

func TestFormatLogAction(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		{"CREATED", "CREATED"},
		{"CLAIMED", "CLAIMED"},
		{"DONE", "DONE"},
		{"SYNC_PUSHED", "SYNC_PUSHED"},
		{"UNKNOWN_ACTION", "UNKNOWN_ACTION"},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			got := formatLogAction(tt.action)
			// Just verify it doesn't panic and contains the action text
			if !strings.Contains(got, tt.action) {
				t.Errorf("formatLogAction(%q) = %q, want to contain %q", tt.action, got, tt.action)
			}
		})
	}
}
