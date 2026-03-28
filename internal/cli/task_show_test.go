package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskShow tests task show and delete commands.
func TestTaskShow(t *testing.T) {
	t.Run("ShowTask", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Updated Title",
			Status: core.StatusInProgress,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001", "--logs"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show failed: %v", err)
		}

		output := buf.String()
		t.Logf("Captured output: %q", output)
		if !contains(output, "Updated Title") {
			t.Errorf("show output missing title. got: %s", output)
		}
	})

	t.Run("ShowTaskWithLogs", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		now := time.Now().UTC()
		task := &core.Task{
			ID:        "T-0001",
			Title:     "Show logs test",
			Status:    core.StatusTodo,
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.CreateTask(ctx, task)

		// Add a log entry (CreateTask doesn't auto-create one)
		s.AddLog(ctx, &core.LogEntry{
			TaskID:    "T-0001",
			Timestamp: now,
			By:        "test-user",
			Action:    "CREATED",
			Note:      "Task created",
		})

		viper.Set("output.format", "json")
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001", "--logs"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show with --logs failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Show logs test") {
			t.Error("expected task title in output")
		}
		if !contains(output, "CREATED") {
			t.Error("expected logs with CREATED entry in output")
		}
	})

	t.Run("ShowTaskJSONFormat", func(t *testing.T) {
		testShowTaskFormat(t, "json", "\"id\"", "\"title\"")
	})

	t.Run("ShowTaskYAMLFormat", func(t *testing.T) {
		testShowTaskFormat(t, "yaml", "id:", "title:")
	})

	t.Run("ShowTaskRelations", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Base task",
			Status: core.StatusTodo,
		})
		s.CreateTask(ctx, &core.Task{
			ID:     "T-0002",
			Title:  "Blocked task",
			Status: core.StatusTodo,
			Meta: map[string]interface{}{
				"blocked_by": []string{"T-0001"},
			},
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0002"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show relations failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Blocked By:") {
			t.Fatalf("expected Blocked By section, got: %s", output)
		}
		if !contains(output, "T-0001") || !contains(output, "Base task") {
			t.Fatalf("expected resolved blocker in output, got: %s", output)
		}

		cmd = newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf = new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show inverse relations failed: %v", err)
		}

		output = buf.String()
		if !contains(output, "Blocking:") {
			t.Fatalf("expected Blocking section, got: %s", output)
		}
		if !contains(output, "T-0002") || !contains(output, "Blocked task") {
			t.Fatalf("expected dependent task in output, got: %s", output)
		}
	})
}

// TestTaskDelete tests the delete command.
func TestTaskDelete(t *testing.T) {
	t.Run("DeleteTask", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Delete me", Status: core.StatusTodo})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "delete", "T-0001", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task delete failed: %v", err)
		}

		task, _ := s.GetTask(ctx, "T-0001")
		if task != nil {
			t.Error("task was not deleted")
		}
	})

	t.Run("DeleteTaskInteractiveConfirmation", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Delete confirm test",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "delete", "T-0001"})

		if err := cmd.Execute(); err == nil {
			t.Error("expected error or prompt for interactive delete, got nil")
		}
	})
}
