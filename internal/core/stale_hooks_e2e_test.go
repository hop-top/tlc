package core_test

// End-to-end tests for RunStaleHooks.
// These tests exercise the full hook pipeline: template expansion → sh -c
// execution → real filesystem side-effects. No mocks.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

// TestRunStaleHooks_E2E verifies that a hook with a template command writes
// the task ID to a real temporary file — full path from RunStaleHooks → sh -c.
func TestRunStaleHooks_E2E(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "hook-output.txt")

	timeout := time.Hour
	task := &core.Task{
		ID:           "T-0094",
		Title:        "stale hook e2e",
		AssignedTo:   ptr("agent"),
		UpdatedAt:    time.Now().UTC().Add(-2 * time.Hour),
		StaleTimeout: &timeout,
	}
	hooks := []config.StaleHook{
		{Command: "echo {{.ID}} > " + outFile},
	}

	if err := core.RunStaleHooks(task, hooks); err != nil {
		t.Fatalf("RunStaleHooks returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("hook did not write output file: %v", err)
	}
	content := strings.TrimSpace(string(data))
	if content != "T-0094" {
		t.Fatalf("expected file content %q, got %q", "T-0094", content)
	}
}

// TestRunStaleHooks_E2E_AllTemplateVars verifies all StaleHookData fields
// expand correctly in a template command.
func TestRunStaleHooks_E2E_AllTemplateVars(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "vars.txt")

	timeout := 30 * time.Minute
	task := &core.Task{
		ID:           "T-0094",
		Title:        "e2e vars",
		AssignedTo:   ptr("bot"),
		UpdatedAt:    time.Now().UTC().Add(-90 * time.Minute),
		StaleTimeout: &timeout,
	}
	hooks := []config.StaleHook{
		// Write all expanded vars to file, separated by pipe
		{Command: `printf '%s|%s|%s' "{{.ID}}" "{{.Title}}" "{{.AssignedTo}}" > ` + outFile},
	}

	if err := core.RunStaleHooks(task, hooks); err != nil {
		t.Fatalf("RunStaleHooks returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("hook did not write output file: %v", err)
	}
	content := string(data)
	for _, want := range []string{"T-0094", "e2e vars", "bot"} {
		if !strings.Contains(content, want) {
			t.Errorf("expected %q in output, got: %q", want, content)
		}
	}
}
