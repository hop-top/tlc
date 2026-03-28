package cli

// End-to-end tests for `tlc task list --stale` and `tlc task list --blocked`.
// Exercises the full CLI pipeline: flag parsing → storage query → post-filter → render.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskList_StaleFilter_E2E seeds two tasks (one stale via short StaleTimeout +
// old UpdatedAt, one not), runs `task list --stale`, and asserts correct filtering.
func TestTaskList_StaleFilter_E2E(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	shortTimeout := 5 * time.Minute
	staleTask := &core.Task{
		ID:           "T-0001",
		Title:        "E2E stale task",
		Status:       core.StatusInProgress,
		UpdatedAt:    time.Now().Add(-3 * time.Hour), // 3h old >> 5m timeout
		StaleTimeout: &shortTimeout,
	}
	freshTask := &core.Task{
		ID:        "T-0002",
		Title:     "E2E fresh task",
		Status:    core.StatusInProgress,
		UpdatedAt: time.Now(),
		// no StaleTimeout → IsStale() always returns false
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
	cmd.SetArgs([]string{"task", "list", "--stale", "--status", "IN_PROGRESS"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --stale e2e: %v", err)
	}

	out := buf.String()
	if !contains(out, "E2E stale task") {
		t.Errorf("expected 'E2E stale task' in output; got:\n%s", out)
	}
	if contains(out, "E2E fresh task") {
		t.Errorf("did not expect 'E2E fresh task' in --stale output; got:\n%s", out)
	}
}

// TestTaskList_BlockedFilter_E2E seeds two tasks (one with BlockedReason, one without),
// runs `task list --blocked`, and asserts only the blocked task appears.
func TestTaskList_BlockedFilter_E2E(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	reason := "waiting on design approval"
	blockedTask := &core.Task{
		ID:            "T-0001",
		Title:         "E2E blocked task",
		Status:        core.StatusTodo,
		BlockedReason: &reason,
	}
	freeTask := &core.Task{
		ID:     "T-0002",
		Title:  "E2E unblocked task",
		Status: core.StatusTodo,
	}
	if err := s.CreateTask(ctx, blockedTask); err != nil {
		t.Fatalf("create blocked task: %v", err)
	}
	if err := s.CreateTask(ctx, freeTask); err != nil {
		t.Fatalf("create free task: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--blocked", "--status", "TODO"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --blocked e2e: %v", err)
	}

	out := buf.String()
	if !contains(out, "E2E blocked task") {
		t.Errorf("expected 'E2E blocked task' in output; got:\n%s", out)
	}
	if contains(out, "E2E unblocked task") {
		t.Errorf("did not expect 'E2E unblocked task' in --blocked output; got:\n%s", out)
	}
}

// TestTaskList_AutoFireHook_E2E verifies that `tlc task list` auto-fires stale
// hooks (once per crossing) when hooks are configured. The hook writes the task
// ID to a tmpfile; the test asserts the file is written and StaleFiredAt is set.
func TestTaskList_AutoFireHook_E2E(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	// Write hook output to a temp file so we can assert it fired.
	hookOut := filepath.Join(t.TempDir(), "hook-fired.txt")

	// Configure stale hook via viper so task_list.go picks it up.
	// Use 5m timeout; task is 3h old → clearly stale.
	viper.Set("task.stale.hooks", []map[string]any{
		{"command": "echo {{.ID}} > " + hookOut},
	})

	shortTimeout := 5 * time.Minute
	staleTask := &core.Task{
		ID:           "T-0010",
		Title:        "E2E auto-fire hook task",
		Status:       core.StatusInProgress,
		UpdatedAt:    time.Now().Add(-3 * time.Hour), // stale: 3h > 5m
		StaleTimeout: &shortTimeout,                  // per-task timeout; avoids config default
	}
	if err := s.CreateTask(ctx, staleTask); err != nil {
		t.Fatalf("create task: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "IN_PROGRESS"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list: %v", err)
	}

	// Assert hook fired: output file must exist and contain the task ID.
	data, err := os.ReadFile(hookOut)
	if err != nil {
		t.Fatalf("hook output file not written: %v", err)
	}
	if !contains(string(data), "T-0010") {
		t.Errorf("expected hook output to contain T-0010; got: %q", string(data))
	}

	// Assert StaleFiredAt is persisted on the task.
	got, err := s.GetTask(ctx, "T-0010")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.StaleFiredAt == nil {
		t.Error("expected StaleFiredAt to be set after auto-fire")
	}
}

// TestTaskUpdate_ClearsStaleFiredAt_E2E verifies that any `tlc task update`
// clears StaleFiredAt, resetting the stale crossing state.
func TestTaskUpdate_ClearsStaleFiredAt_E2E(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	// Seed a task with StaleFiredAt already set.
	firedAt := time.Now().UTC().Add(-1 * time.Hour)
	task := &core.Task{
		ID:           "T-0020",
		Title:        "E2E stale-fired task",
		Status:       core.StatusInProgress,
		UpdatedAt:    time.Now().Add(-3 * time.Hour),
		StaleFiredAt: &firedAt,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Run `tlc task update` with a title change — any change should clear StaleFiredAt.
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0020", "--title", "E2E stale-fired task updated"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update: %v", err)
	}

	// Assert StaleFiredAt is now nil.
	got, err := s.GetTask(ctx, "T-0020")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.StaleFiredAt != nil {
		t.Errorf("expected StaleFiredAt to be nil after update; got %v", got.StaleFiredAt)
	}
}
