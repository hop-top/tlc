package cli

import (
	"bytes"
	"context"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestTaskCreateCollateralStateLeak reproduces T-0231: creating task B after
// task A (with description + blocked-by) causes task B to inherit task A's
// description and blocking references, even though task B was created without
// those flags.
//
// Root cause: package-level flag variables (taskDescription, taskBlockedBy,
// etc.) are set once by cobra's init() and never reset between sequential
// command executions within the same process. When a second create omits
// those flags, cobra leaves the variables at whatever the first create set.
func TestTaskCreateCollateralStateLeak(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	ctx := context.Background()

	// --- Task A: create with description, tags, blocked-by, priority, effort ---
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	// Seed a blocker so --blocked-by is valid.
	if err := s.CreateTask(ctx, &core.Task{
		ID:     "T-0050",
		Title:  "Blocker task",
		Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	s.Close()

	cmd1 := newTestCmd()
	cmd1.AddCommand(TaskCmd)
	buf1 := new(bytes.Buffer)
	cmd1.SetOut(buf1)
	cmd1.SetErr(buf1)
	cmd1.SetArgs([]string{
		"task", "create", "Add task prompt",
		"--description", "Implement the interactive prompt for task creation",
		"--tag", "type:feat",
		"--priority", "P1",
		"--effort", "L",
		"--blocked-by", "T-0050",
	})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("task A create failed: %v", err)
	}

	// --- Task B: create with ONLY title, tag, priority, effort ---
	// Intentionally do NOT call resetTaskFlags() here.
	// This simulates the collateral state leak reported in T-0231.
	cmd2 := newTestCmd()
	cmd2.AddCommand(TaskCmd)
	buf2 := new(bytes.Buffer)
	cmd2.SetOut(buf2)
	cmd2.SetErr(buf2)
	cmd2.SetArgs([]string{
		"task", "create", "Fix collateral behavior",
		"--tag", "type:fix",
		"--priority", "P0",
		"--effort", "M",
	})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("task B create failed: %v", err)
	}

	// --- Verify task B has clean fields ---
	s2, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s2.Close()

	// Task B should be T-0002 (T-0050 was seeded, T-0001 was task A).
	tasks, err := s2.ListTasks(ctx, core.Query{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	var taskB *core.Task
	for _, tk := range tasks {
		if tk.Title == "Fix collateral behavior" {
			taskB = tk
			break
		}
	}
	if taskB == nil {
		t.Fatal("task B not found")
	}

	// Title must match exactly.
	if taskB.Title != "Fix collateral behavior" {
		t.Errorf("task B title = %q, want %q",
			taskB.Title, "Fix collateral behavior")
	}

	// Description must be empty (task B did not specify --description).
	if taskB.Description != "" {
		t.Errorf("task B description = %q, want empty (leaked from task A)",
			taskB.Description)
	}

	// Blocked-by must be empty (task B did not specify --blocked-by).
	blockedBy := taskB.BlockedBy()
	if len(blockedBy) != 0 {
		t.Errorf("task B blocked_by = %v, want empty (leaked from task A)",
			blockedBy)
	}

	// Priority should be what task B set.
	if string(taskB.Priority) != "P0" {
		t.Errorf("task B priority = %q, want %q", taskB.Priority, "P0")
	}

	// Effort should be what task B set.
	if string(taskB.Effort) != "M" {
		t.Errorf("task B effort = %q, want %q", taskB.Effort, "M")
	}

	// Tags should be exactly what task B set (not include task A's tags).
	if len(taskB.Tags) != 1 || taskB.Tags[0] != "type:fix" {
		t.Errorf("task B tags = %v, want [type:fix] (leaked from task A)",
			taskB.Tags)
	}
}
