package cli

// End-to-end tests for `tlc task list --stale` and `tlc task list --blocked`.
// Exercises the full CLI pipeline: flag parsing → storage query → post-filter → render.

import (
	"bytes"
	"testing"
	"time"

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
