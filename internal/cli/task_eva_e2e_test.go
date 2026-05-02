package cli

import (
	"bytes"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestTaskCreateWithEva(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "create", "Eva task",
			"--eva", "hint-a",
			"--eva", "hint-b",
			"--eva", "hint-a",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create --eva failed: %v", err)
		}

		tasks, err := s.ListTasks(ctx, core.Query{})
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}

		eva := tasks[0].Eva()
		if len(eva) != 2 {
			t.Fatalf("expected 2 unique eva values, got %d (%v)", len(eva), eva)
		}
		if eva[0] != "hint-a" || eva[1] != "hint-b" {
			t.Fatalf("eva = %v, want [hint-a hint-b]", eva)
		}
	})
}

func TestTaskUpdateAddAndRemoveEva(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Task with eva",
			Status: core.StatusTodo,
			Meta:   map[string]interface{}{"eva": []string{"a", "b"}},
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--add-eva", "c",
			"--remove-eva", "a",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update eva failed: %v", err)
		}

		updatedTask := getTaskByAlias(t, ctx, "T-0001")
		if updatedTask == nil {
			t.Fatal("task not found after update")
		}
		eva := updatedTask.Eva()
		if len(eva) != 2 {
			t.Fatalf("expected 2 eva values, got %d (%v)", len(eva), eva)
		}
		if eva[0] != "b" || eva[1] != "c" {
			t.Fatalf("eva = %v, want [b c]", eva)
		}
	})
}

func TestTaskUpdateClearEva(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Task with eva",
			Status: core.StatusTodo,
			Meta:   map[string]interface{}{"eva": []string{"a", "b"}},
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--clear-eva"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --clear-eva failed: %v", err)
		}

		updatedTask := getTaskByAlias(t, ctx, "T-0001")
		if updatedTask == nil {
			t.Fatal("task not found after update")
		}
		if len(updatedTask.Eva()) != 0 {
			t.Fatalf("expected eva to be cleared, got %v", updatedTask.Eva())
		}
		if _, ok := updatedTask.Meta["eva"]; ok {
			t.Fatal("expected eva key to be removed from meta")
		}
	})
}

func TestTaskShowEvaHiddenForAssignee(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		currentUser := core.GetCurrentUser()
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Task with eva",
			Status:     core.StatusTodo,
			AssignedTo: &currentUser,
			Meta:       map[string]interface{}{"eva": []string{"secret-hint"}},
		}
		if err := s.CreateTask(nil, task); err != nil {
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

		output := buf.String()
		if contains(output, "Eva:") {
			t.Error("Eva field should be hidden when viewer is the assignee")
		}
		if contains(output, "secret-hint") {
			t.Error("Eva values should be hidden when viewer is the assignee")
		}
	})
}

func TestTaskShowEvaVisibleForNonAssignee(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		other := "someone-else"
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Task with eva",
			Status:     core.StatusTodo,
			AssignedTo: &other,
			Meta:       map[string]interface{}{"eva": []string{"visible-hint"}},
		}
		if err := s.CreateTask(nil, task); err != nil {
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

		output := buf.String()
		if !contains(output, "Eva:") {
			t.Error("Eva field should be visible when viewer is not the assignee")
		}
		if !contains(output, "visible-hint") {
			t.Error("Eva values should be visible when viewer is not the assignee")
		}
	})
}
