package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/displaytime"
)

// TestTaskShowRendersDueAtInConfiguredTimezone is the e2e golden for
// spec §6: when ui.timezone resolves to a non-UTC IANA zone, `task
// show` must render absolute timestamps converted into that zone, not
// the stored UTC instant.
//
// We use America/New_York which is UTC-04:00 in summer (EDT) and a
// UTC instant whose local-time conversion is unambiguous (14:30 UTC →
// 10:30 EDT).
func TestTaskShowRendersDueAtInConfiguredTimezone(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	prev := viper.GetString("ui.timezone")
	t.Cleanup(func() {
		viper.Set("ui.timezone", prev)
		displaytime.Reset()
	})
	viper.Set("ui.timezone", "America/New_York")
	displaytime.Reset()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	due := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	task := &core.Task{
		ID:     "T-0001",
		Title:  "tz-binding due render",
		Status: core.StatusTodo,
		DueAt:  &due,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show failed: %v", err)
	}

	out := buf.String()
	const wantEDT = "2026-06-15T10:30:00-04:00"
	if !strings.Contains(out, wantEDT) {
		t.Fatalf("expected DueAt rendered in America/New_York (%q) in output, got:\n%s",
			wantEDT, out)
	}
	const dontWantUTC = "2026-06-15T14:30:00Z"
	if strings.Contains(out, dontWantUTC) {
		t.Fatalf("DueAt leaked stored UTC (%q) into display output:\n%s",
			dontWantUTC, out)
	}
}

// TestTaskShowHandlesInvalidTimezone covers the spec §6 fallback: an
// invalid IANA name in ui.timezone must not break rendering. We don't
// assert the exact warning text (logger format) — only that the
// command completes and renders something parseable for DueAt.
func TestTaskShowHandlesInvalidTimezone(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	prev := viper.GetString("ui.timezone")
	t.Cleanup(func() {
		viper.Set("ui.timezone", prev)
		displaytime.Reset()
	})
	viper.Set("ui.timezone", "Not/A_Real_Zone")
	displaytime.Reset()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	due := time.Date(2026, 6, 15, 14, 30, 0, 0, time.UTC)
	task := &core.Task{
		ID:     "T-0001",
		Title:  "tz-binding bad zone",
		Status: core.StatusTodo,
		DueAt:  &due,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show with invalid tz should not fail: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "2026-06-15") {
		t.Fatalf("expected DueAt date in output, got:\n%s", out)
	}
}
