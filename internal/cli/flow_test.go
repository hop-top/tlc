package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
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

// TestFlowListDefaultLimit verifies that `flow list` applies a default limit
// when no --limit flag is provided. Without a default limit, all flow runs are
// returned which causes excessive output (observed: 94 runs / 42KB).
func TestFlowListDefaultLimit(t *testing.T) {
	t.Skip("TODO(T-0314): flow-list default limit not yet implemented; see T-0757")
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	// Seed 50 flow runs — more than any reasonable default limit (expected: 25)
	const totalRuns = 50
	const expectedDefault = 25
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < totalRuns; i++ {
		run := &core.FlowRun{
			ID:        fmt.Sprintf("run:%04d", i),
			FlowID:    "flow:test:1.0",
			Status:    core.FlowStatusSucceeded,
			StartedAt: base.Add(time.Duration(i) * time.Minute),
		}
		ended := run.StartedAt.Add(10 * time.Second)
		run.EndedAt = &ended
		if err := s.CreateFlowRun(ctx, run); err != nil {
			t.Fatalf("CreateFlowRun[%d]: %v", i, err)
		}
	}

	viper.Set("output.format", "table")

	cmd := newTestCmd()
	cmd.AddCommand(FlowCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"flow", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("flow list failed: %v", err)
	}

	out := buf.String()

	// The footer line reports the count. It should respect the default limit.
	wantFooter := fmt.Sprintf("Showing %d flow runs", expectedDefault)
	if !strings.Contains(out, wantFooter) {
		// Extract actual count from footer for a clear failure message
		actual := "unknown"
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "Showing") && strings.Contains(line, "flow runs") {
				actual = strings.TrimSpace(line)
				break
			}
		}
		t.Errorf("flow list without --limit should default to %d;\ngot footer: %s\nwant footer to contain: %s",
			expectedDefault, actual, wantFooter)
	}
}

// TestFlowListLimitFlag verifies that `flow list --limit N` caps output to N runs.
func TestFlowListLimitFlag(t *testing.T) {
	t.Skip("TODO(T-0314): flow-list --limit flag not yet implemented; see T-0757")
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	const totalRuns = 20
	const requestedLimit = 5
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < totalRuns; i++ {
		run := &core.FlowRun{
			ID:        fmt.Sprintf("run:%04d", i),
			FlowID:    "flow:test:1.0",
			Status:    core.FlowStatusFailed,
			StartedAt: base.Add(time.Duration(i) * time.Minute),
		}
		ended := run.StartedAt.Add(5 * time.Second)
		run.EndedAt = &ended
		if err := s.CreateFlowRun(ctx, run); err != nil {
			t.Fatalf("CreateFlowRun[%d]: %v", i, err)
		}
	}

	viper.Set("output.format", "table")

	cmd := newTestCmd()
	cmd.AddCommand(FlowCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"flow", "list", "--limit", fmt.Sprintf("%d", requestedLimit)})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("flow list --limit %d failed: %v", requestedLimit, err)
	}

	out := buf.String()
	wantFooter := fmt.Sprintf("Showing %d flow runs", requestedLimit)
	if !strings.Contains(out, wantFooter) {
		t.Errorf("flow list --limit %d: output does not contain %q;\ngot: %s",
			requestedLimit, wantFooter, out)
	}
}
