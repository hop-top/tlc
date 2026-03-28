package core_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

func TestRunStaleHooks_WritesFile(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.txt")

	task := &core.Task{
		ID:           "T-0001",
		Title:        "test task",
		UpdatedAt:    time.Now().UTC().Add(-2 * time.Hour),
		StaleTimeout: ptr(time.Hour),
	}
	hooks := []config.StaleHook{
		{Command: "echo {{.ID}} > " + outFile},
	}

	if err := core.RunStaleHooks(task, hooks); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Fatal("expected hook output in file")
	}
	if !strings.Contains(string(data), "T-0001") {
		t.Fatalf("expected T-0001 in output, got: %q", string(data))
	}
}

func TestRunStaleHooks_FailureNonFatal(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "second.txt")

	task := &core.Task{
		ID:    "T-0002",
		Title: "x",
	}
	hooks := []config.StaleHook{
		{Command: "false"},
		{Command: "echo done > " + outFile},
	}
	// First hook fails; should not return error; second hook must run.
	if err := core.RunStaleHooks(task, hooks); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("second hook did not run: %v", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		t.Fatal("expected second hook output")
	}
}

func TestRunStaleHooks_TemplateExpansion(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "tmpl.txt")

	timeout := 2 * time.Hour
	task := &core.Task{
		ID:           "T-0003",
		Title:        "template test",
		AssignedTo:   ptr("alice"),
		UpdatedAt:    time.Now().UTC().Add(-3 * time.Hour),
		StaleTimeout: &timeout,
	}
	hooks := []config.StaleHook{
		{Command: `echo "{{.ID}} {{.Title}} {{.AssignedTo}}" > ` + outFile},
	}

	if err := core.RunStaleHooks(task, hooks); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(string(data))
	if !strings.Contains(out, "T-0003") {
		t.Errorf("expected T-0003 in output, got: %q", out)
	}
	if !strings.Contains(out, "alice") {
		t.Errorf("expected alice in output, got: %q", out)
	}
}
