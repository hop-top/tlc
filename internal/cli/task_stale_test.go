package cli

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

// TestTaskStale_ListsStale seeds a stale task and asserts it appears in `task stale` output.
func TestTaskStale_ListsStale(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	shortTimeout := 5 * time.Minute
	staleTask := &core.Task{
		ID:           "T-0001",
		Title:        "Stale task title",
		Status:       core.StatusInProgress,
		UpdatedAt:    time.Now().Add(-3 * time.Hour), // 3h old >> 5m timeout
		StaleTimeout: &shortTimeout,
	}
	freshTask := &core.Task{
		ID:        "T-0002",
		Title:     "Fresh task title",
		Status:    core.StatusInProgress,
		UpdatedAt: time.Now(),
		// no StaleTimeout → IsStale() == false
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
		t.Fatalf("task stale failed: %v", err)
	}

	out := buf.String()
	if !contains(out, "T-0001") {
		t.Errorf("expected 'T-0001' in stale output; got:\n%s", out)
	}
	if contains(out, "T-0002") {
		t.Errorf("did not expect 'T-0002' (fresh) in stale output; got:\n%s", out)
	}
}

// TestTaskStale_NoStaleTasks asserts the "No stale tasks." message when none are stale.
func TestTaskStale_NoStaleTasks(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	freshTask := &core.Task{
		ID:        "T-0001",
		Title:     "Fresh task",
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
		t.Fatalf("task stale failed: %v", err)
	}

	out := buf.String()
	if !contains(out, "No stale tasks.") {
		t.Errorf("expected 'No stale tasks.' message; got:\n%s", out)
	}
}

// TestTaskStale_RunHooks asserts --run-hooks fires the hook and sets StaleFiredAt.
func TestTaskStale_RunHooks(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	shortTimeout := 5 * time.Minute
	staleTask := &core.Task{
		ID:           "T-0001",
		Title:        "Hook test task",
		Status:       core.StatusInProgress,
		UpdatedAt:    time.Now().Add(-3 * time.Hour),
		StaleTimeout: &shortTimeout,
	}
	if err := s.CreateTask(ctx, staleTask); err != nil {
		t.Fatalf("create stale task: %v", err)
	}

	// Write hook config: echo task ID into a temp file.
	tmpFile := filepath.Join(t.TempDir(), "hook-out.txt")
	hookCmd := "echo {{.ID}} > " + tmpFile

	// Set hook config via viper (mimics yaml: task.stale.hooks).
	viper.Set("task.stale.hooks", []map[string]string{{"command": hookCmd}})
	t.Cleanup(func() { viper.Set("task.stale.hooks", nil) })

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "stale", "--run-hooks"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task stale --run-hooks failed: %v", err)
	}

	// Assert hook file written.
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("hook output file not written: %v", err)
	}
	if !strings.Contains(string(data), "T-0001") {
		t.Errorf("hook output missing task ID; got: %s", string(data))
	}

	// Assert StaleFiredAt set on stored task.
	updated, err := s.GetTask(ctx, "T-0001")
	if err != nil {
		t.Fatalf("get task after run-hooks: %v", err)
	}
	if updated.StaleFiredAt == nil {
		t.Error("expected StaleFiredAt to be set after --run-hooks")
	}
}

// TestTaskStale_DefaultTimeout applies project default_timeout to tasks with nil StaleTimeout.
func TestTaskStale_DefaultTimeout(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	// Task with nil StaleTimeout but very old UpdatedAt — should become stale via project default.
	staleTask := &core.Task{
		ID:        "T-0001",
		Title:     "Default timeout stale",
		Status:    core.StatusTodo,
		UpdatedAt: time.Now().Add(-48 * time.Hour), // 48h old
		// StaleTimeout nil — will use project default
	}
	if err := s.CreateTask(ctx, staleTask); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Set a short project default timeout (1h).
	viper.Set("task.stale.default_timeout", "1h")
	t.Cleanup(func() { viper.Set("task.stale.default_timeout", nil) })

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "stale"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task stale failed: %v", err)
	}

	out := buf.String()
	if !contains(out, "T-0001") {
		t.Errorf("expected 'T-0001' using default timeout; got:\n%s", out)
	}
}
