package cli

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestBatchE2E_CompleteAllWithGlob(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create 4 tasks via CLI with IN_PROGRESS status so complete transition is valid.
	for i := 1; i <= 4; i++ {
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		cmd.SetArgs([]string{"task", "create", fmt.Sprintf("Task %d", i), "--status", "IN_PROGRESS"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	// Complete all with *
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"task", "complete", "*", "--no-prompt"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("complete *: %v", err)
	}

	// List and verify all DONE
	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(context.Background(), core.Query{Limit: 100})
	for _, task := range tasks {
		if task.Status != core.StatusDone {
			t.Errorf("expected %s DONE, got %s", task.ID, task.Status)
		}
	}
}

func TestBatchE2E_AssignRegexNoPrompt(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	for _, id := range []string{"T-0010", "T-0011", "T-0020"} {
		s.CreateTask(ctx, &core.Task{ID: id, Title: id, Status: core.StatusTodo})
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	cmd.SetArgs([]string{"task", "assign", "carol", `T-001\d`, "--no-prompt"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("assign: %v", err)
	}

	for _, id := range []string{"T-0010", "T-0011"} {
		task, _ := s.GetTask(ctx, id)
		if task.AssignedTo == nil || *task.AssignedTo != "carol" {
			t.Errorf("expected %s → carol", id)
		}
	}
	// T-0020 was seeded directly via storage with that literal as
	// task.ID, so look it up by the same string (not via seq alias).
	task, _ := s.GetTask(ctx, "T-0020")
	if task == nil {
		t.Fatal("T-0020 not found")
	}
	if task.AssignedTo != nil {
		t.Errorf("expected T-0020 unassigned")
	}
}

func TestBatchE2E_DeleteRequiresConfirmOrNoPrompt(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

	// Without --no-prompt in non-TTY, should return error or aborted.
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	cmd.SetArgs([]string{"task", "delete", "T-0001", "T-0002"})
	err := cmd.Execute()
	if err == nil {
		t.Error("expected error without --no-prompt in non-TTY batch delete")
	}

	// With --no-prompt it should succeed.
	cmd2 := newTestCmd()
	cmd2.AddCommand(TaskCmd)
	cmd2.SetArgs([]string{"task", "delete", "T-0001", "T-0002", "--no-prompt"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("delete with --no-prompt: %v", err)
	}
}
