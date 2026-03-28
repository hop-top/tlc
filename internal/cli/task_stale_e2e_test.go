package cli

// End-to-end tests for `tlc task stale`.
// Exercises the full CLI pipeline: flag parsing → storage query → stale filter → render.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskStale_E2E_ListsStale seeds a stale task via CLI storage, runs `task stale`,
// and asserts the stale task ID appears in output while the fresh one does not.
func TestTaskStale_E2E_ListsStale(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	shortTimeout := 5 * time.Minute
	staleTask := &core.Task{
		ID:           "T-0010",
		Title:        "E2E stale cmd task",
		Status:       core.StatusInProgress,
		UpdatedAt:    time.Now().Add(-4 * time.Hour),
		StaleTimeout: &shortTimeout,
	}
	freshTask := &core.Task{
		ID:        "T-0011",
		Title:     "E2E fresh cmd task",
		Status:    core.StatusInProgress,
		UpdatedAt: time.Now(),
	}
	if err := s.CreateTask(ctx, staleTask); err != nil {
		t.Fatalf("create stale task: %v", err)
	}
	if err := s.CreateTask(ctx, freshTask); err != nil {
		t.Fatalf("create fresh task: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "stale"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task stale e2e: %v", err)
	}

	out := buf.String()
	if !contains(out, "T-0010") {
		t.Errorf("expected T-0010 in stale output; got:\n%s", out)
	}
	if contains(out, "T-0011") {
		t.Errorf("did not expect fresh T-0011 in stale output; got:\n%s", out)
	}
}

// TestTaskStale_E2E_RunHooks runs `task stale --run-hooks` with a hook writing to a tmpfile,
// then asserts the file was written and StaleFiredAt is set in storage.
func TestTaskStale_E2E_RunHooks(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	shortTimeout := 5 * time.Minute
	staleTask := &core.Task{
		ID:           "T-0020",
		Title:        "E2E hook task",
		Status:       core.StatusTodo,
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
		StaleTimeout: &shortTimeout,
	}
	if err := s.CreateTask(ctx, staleTask); err != nil {
		t.Fatalf("create stale task: %v", err)
	}

	outFile := filepath.Join(t.TempDir(), "stale-hook.txt")
	viper.Set("task.stale.hooks", []map[string]string{{"command": "echo {{.ID}} > " + outFile}})
	t.Cleanup(func() { viper.Set("task.stale.hooks", nil) })

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "stale", "--run-hooks"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task stale --run-hooks e2e: %v", err)
	}

	// Hook file written.
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("hook output file missing: %v", err)
	}
	if !strings.Contains(string(data), "T-0020") {
		t.Errorf("hook file missing task ID; got: %q", string(data))
	}

	// StaleFiredAt persisted.
	updated, err := s.GetTask(ctx, "T-0020")
	if err != nil {
		t.Fatalf("get task post run-hooks: %v", err)
	}
	if updated.StaleFiredAt == nil {
		t.Error("expected StaleFiredAt set after --run-hooks")
	}
}

// TestTaskStale_E2E_EmptyMessage asserts "No stale tasks." when nothing is stale.
func TestTaskStale_E2E_EmptyMessage(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	freshTask := &core.Task{
		ID:        "T-0030",
		Title:     "E2E all fresh",
		Status:    core.StatusInProgress,
		UpdatedAt: time.Now(),
	}
	if err := s.CreateTask(ctx, freshTask); err != nil {
		t.Fatalf("create fresh task: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "stale"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task stale e2e empty: %v", err)
	}

	out := buf.String()
	if !contains(out, "No stale tasks.") {
		t.Errorf("expected 'No stale tasks.' message; got:\n%s", out)
	}
}
