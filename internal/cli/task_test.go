package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/IdeaCraftersLabs/oss-tlc-cli/internal/core"
	"github.com/spf13/viper"
)

// TestTaskCommands tests task CRUD operations through CLI
// Tests create, list, update, show, and delete task commands
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

	t.Run("CreateTaskWithAssignee", func(t *testing.T) {
		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Task with Assignee", "--assigned-to", "engineer-1"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with assignee failed: %v", err)
		}

		s, _ := getStorage()
		defer s.Close()
		task, _ := s.GetTask(ctx, "T-0001")
		if task == nil {
			t.Fatal("task was not created")
		}
		if task.AssignedTo == nil {
			t.Error("assignee field is nil, expected 'engineer-1'")
		} else if *task.AssignedTo != "engineer-1" {
			t.Errorf("assignee field is %s, expected 'engineer-1'", *task.AssignedTo)
		}
	})

	t.Run("CreateTaskWithTags", func(t *testing.T) {
		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Task with Tags", "--tag", "auth", "--tag", "security"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with tags failed: %v", err)
		}

		s, _ := getStorage()
		defer s.Close()
		task, _ := s.GetTask(ctx, "T-0001")
		if task == nil {
			t.Fatal("task was not created")
		}
		if len(task.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(task.Tags))
		}
		hasAuth := false
		hasSecurity := false
		for _, tag := range task.Tags {
			if tag == "auth" {
				hasAuth = true
			}
			if tag == "security" {
				hasSecurity = true
			}
		}
		if !hasAuth {
			t.Error("missing 'auth' tag")
		}
		if !hasSecurity {
			t.Error("missing 'security' tag")
		}
	})

	t.Run("FilterTasksByTag", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task1 := &core.Task{
			ID:     "T-0001",
			Title:  "Auth task",
			Status: core.StatusTodo,
			Tags:   []string{"auth", "security"},
		}
		task2 := &core.Task{
			ID:     "T-0002",
			Title:  "Docs task",
			Status: core.StatusTodo,
			Tags:   []string{"docs"},
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--tag", "auth"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with tag filter failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Auth task") {
			t.Error("expected 'Auth task' in output")
		}
		if contains(output, "Docs task") {
			t.Error("did not expect 'Docs task' in output (filtered by tag 'auth')")
		}
	})

	t.Run("FilterTasksByAssignee", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		assignee1 := "engineer-1"
		assignee2 := "engineer-2"
		task1 := &core.Task{
			ID:         "T-0001",
			Title:      "Task for engineer-1",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		task2 := &core.Task{
			ID:         "T-0002",
			Title:      "Task for engineer-2",
			Status:     core.StatusTodo,
			AssignedTo: &assignee2,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--assigned-to", "engineer-1"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with assignee filter failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Task for engineer-1") {
			t.Error("expected 'Task for engineer-1' in output")
		}
		if contains(output, "Task for engineer-2") {
			t.Error("did not expect 'Task for engineer-2' in output (filtered by assignee)")
		}
	})

	t.Run("FilterTasksByStatus", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task1 := &core.Task{
			ID:     "T-0001",
			Title:  "TODO task",
			Status: core.StatusTodo,
		}
		task2 := &core.Task{
			ID:     "T-0002",
			Title:  "IN_PROGRESS task",
			Status: core.StatusInProgress,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--status", "TODO"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with status filter failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "TODO task") {
			t.Error("expected 'TODO task' in output")
		}
		if contains(output, "IN_PROGRESS task") {
			t.Error("did not expect 'IN_PROGRESS task' in output (filtered by status TODO)")
		}
	})

	t.Run("UpdateAssignee", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		assignee1 := "engineer-1"
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Task to reassign",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--assigned-to", "engineer-2"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update assignee failed: %v", err)
		}

		updatedTask, _ := s.GetTask(ctx, "T-0001")
		if updatedTask.AssignedTo == nil {
			t.Error("assignee field is nil after update")
		} else if *updatedTask.AssignedTo != "engineer-2" {
			t.Errorf("assignee is %s, expected 'engineer-2'", *updatedTask.AssignedTo)
		}
	})

	t.Run("ClearAssigneeWithNull", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		assignee1 := "engineer-1"
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Task with assignee",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--assigned-to", "null"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update to clear assignee failed: %v", err)
		}

		updatedTask, _ := s.GetTask(ctx, "T-0001")
		if updatedTask.AssignedTo != nil {
			t.Errorf("assignee field is %s, expected nil (cleared)", *updatedTask.AssignedTo)
		}
	})

	t.Run("ClearAssigneeWithDash", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		assignee1 := "engineer-1"
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Task with assignee",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--assigned-to", "-"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update to clear assignee with dash failed: %v", err)
		}

		updatedTask, _ := s.GetTask(ctx, "T-0001")
		if updatedTask.AssignedTo != nil {
			t.Errorf("assignee field is %s, expected nil (cleared)", *updatedTask.AssignedTo)
		}
	})

	t.Run("AddTagToTask", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Task without urgent tag",
			Status: core.StatusTodo,
			Tags:   []string{"bug"},
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--add-tag", "urgent"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update to add tag failed: %v", err)
		}

		updatedTask, _ := s.GetTask(ctx, "T-0001")
		if len(updatedTask.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(updatedTask.Tags))
		}

		hasUrgent := false
		hasBug := false
		for _, tag := range updatedTask.Tags {
			if tag == "urgent" {
				hasUrgent = true
			}
			if tag == "bug" {
				hasBug = true
			}
		}
		if !hasUrgent {
			t.Error("missing 'urgent' tag after add")
		}
		if !hasBug {
			t.Error("missing 'bug' tag (should be preserved)")
		}
	})

	t.Run("RemoveTagFromTask", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Task with chore tag",
			Status: core.StatusTodo,
			Tags:   []string{"chore", "bug"},
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--remove-tag", "chore"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update to remove tag failed: %v", err)
		}

		updatedTask, _ := s.GetTask(ctx, "T-0001")
		if len(updatedTask.Tags) != 1 {
			t.Errorf("expected 1 tag after removal, got %d", len(updatedTask.Tags))
		}
		if len(updatedTask.Tags) > 0 && updatedTask.Tags[0] != "bug" {
			t.Errorf("expected 'bug' tag, got '%s'", updatedTask.Tags[0])
		}
	})

	t.Run("FilterTasksByAssigneeMe", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		currentUser := core.GetCurrentUser()
		if currentUser == "" {
			currentUser = "testuser"
		}

		assignee1 := currentUser
		assignee2 := "other-user"
		task1 := &core.Task{
			ID:         "T-0001",
			Title:      "My task",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		task2 := &core.Task{
			ID:         "T-0002",
			Title:      "Other's task",
			Status:     core.StatusTodo,
			AssignedTo: &assignee2,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--assigned-to", "me"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --assigned-to me failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "My task") {
			t.Errorf("expected 'My task' in output (assigned to current user: %s)", currentUser)
		}
		if contains(output, "Other's task") {
			t.Error("did not expect 'Other's task' in output (filtered by current user)")
		}
	})
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
