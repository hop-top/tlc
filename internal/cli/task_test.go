package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/oss-tlc-cli/internal/core"
	"github.com/spf13/viper"
)

func TestTaskCommands(t *testing.T) {
	// Setup temporary directory for test
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", tmpDir)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	todoPath := filepath.Join(tmpDir, "TODO")

	// Setup viper for test
	viper.Reset()
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)
	viper.Set("task.todo_file", todoPath)
	viper.Set("output.format", "json")

	ctx := context.Background()

	t.Run("CreateTask", func(t *testing.T) {
		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Test Task", "--description", "Test Description", "--tag", "test", "--tag", "cli"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v", err)
		}

		output := buf.String()
		t.Logf("Create output: %s", output)

		// Verify in storage
		s, _ := getStorage()
		defer s.Close()
		tasks, _ := s.ListTasks(ctx, core.Query{})
		if len(tasks) != 1 {
			t.Errorf("expected 1 task, got %d", len(tasks))
		}
	})

	t.Run("ListTasks", func(t *testing.T) {
		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list failed: %v", err)
		}

		output := buf.String()
		t.Logf("Captured output: %q", output)
		if !contains(output, "Test Task") {
			t.Errorf("list output missing task title. got: %s", output)
		}
	})

	t.Run("UpdateTask", func(t *testing.T) {
		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--status", "IN_PROGRESS", "--title", "Updated Title"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update failed: %v", err)
		}
	})

	t.Run("ShowTask", func(t *testing.T) {
		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
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
		if !contains(output, "CREATED") {
			t.Errorf("show output missing logs. got: %s", output)
		}
	})

	t.Run("DeleteTask", func(t *testing.T) {
		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "delete", "T-0001", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task delete failed: %v", err)
		}

		s, _ := getStorage()
		defer s.Close()
		task, _ := s.GetTask(ctx, "T-0001")
		if task != nil {
			t.Error("task was not deleted")
		}
	})
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
