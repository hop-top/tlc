package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	t.Run("ListTasksJSONFormat", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "JSON test task",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--format", "json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --format json failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "\"id\"") {
			t.Error("expected JSON output with 'id' field")
		}
		if !contains(output, "\"title\"") {
			t.Error("expected JSON output with 'title' field")
		}
	})

	t.Run("ListTasksYAMLFormat", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "YAML test task",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--format", "yaml"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --format yaml failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "id:") {
			t.Error("expected YAML output with 'id:' field")
		}
		if !contains(output, "title:") {
			t.Error("expected YAML output with 'title:' field")
		}
	})

	t.Run("ListTasksTLSFormat", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		assignee := "testuser"
		task := &core.Task{
			ID:         "T-0001",
			Title:      "TLS test task",
			Status:     core.StatusTodo,
			AssignedTo: &assignee,
			Tags:       []string{"bug", "urgent"},
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--format", "tls"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --format tls failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "[TODO]") {
			t.Error("expected TLS format with [status] bracket")
		}
		if !contains(output, "@testuser") {
			t.Error("expected TLS format with @assignee")
		}
		if !contains(output, "#bug") {
			t.Error("expected TLS format with #tag")
		}
	})

	t.Run("ShowTaskWithLogs", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Show logs test",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
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
		if !contains(output, "Logs") || !contains(output, "CREATED") {
			t.Error("expected logs section with CREATED entry")
		}
	})

	t.Run("DeleteTaskInteractiveConfirmation", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Delete confirm test",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "delete", "T-0001"})

		if err := cmd.Execute(); err == nil {
			t.Error("expected error or prompt for interactive delete, got nil")
		}
	})

	t.Run("ListTasksPagination", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		for i := 1; i <= 15; i++ {
			task := &core.Task{
				ID:     fmt.Sprintf("T-%04d", i),
				Title:  fmt.Sprintf("Task %d", i),
				Status: core.StatusTodo,
			}
			s.CreateTask(ctx, task)
		}

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--limit", "10", "--offset", "5"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with pagination failed: %v", err)
		}

		output := buf.String()
		taskCount := 0
		for i := 6; i <= 15; i++ {
			if contains(output, fmt.Sprintf("Task %d", i)) {
				taskCount++
			}
		}
		if taskCount != 10 {
			t.Errorf("expected 10 tasks (6-15), got %d", taskCount)
		}
	})

	t.Run("ListTasksSort", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task1 := &core.Task{
			ID:        "T-0001",
			Title:     "Task A",
			Status:    core.StatusTodo,
			CreatedAt: time.Now().Add(-2 * time.Hour),
		}
		task2 := &core.Task{
			ID:        "T-0002",
			Title:     "Task B",
			Status:    core.StatusTodo,
			CreatedAt: time.Now().Add(-1 * time.Hour),
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--sort-by", "created_at", "--sort-direction", "asc"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with sort failed: %v", err)
		}

		output := buf.String()
		taskAPos := bytes.Index([]byte(output), []byte("Task A"))
		taskBpos := bytes.Index([]byte(output), []byte("Task B"))
		if taskAPos == -1 || taskBpos == -1 {
			t.Fatal("tasks not found in output")
		}
		if taskAPos > taskBpos {
			t.Error("expected Task A before Task B (ascending by created_at)")
		}
	})

	t.Run("ListTasksFullTextSearch", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task1 := &core.Task{
			ID:     "T-0001",
			Title:  "Search match task",
			Status: core.StatusTodo,
		}
		task2 := &core.Task{
			ID:     "T-0002",
			Title:  "No match task",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "Search match"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with search term failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Search match task") {
			t.Error("expected matching task in output")
		}
		if contains(output, "No match task") {
			t.Error("did not expect non-matching task in output")
		}
	})

	t.Run("ListTasksAllProjects", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		project1 := "project-alpha"
		project2 := "project-beta"
		task1 := &core.Task{
			ID:        "T-0001",
			Title:     "Alpha task",
			Status:    core.StatusTodo,
			ProjectID: &project1,
		}
		task2 := &core.Task{
			ID:        "T-0002",
			Title:     "Beta task",
			Status:    core.StatusTodo,
			ProjectID: &project2,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--all-projects"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --all-projects failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Alpha task") {
			t.Error("expected Alpha task in output")
		}
		if !contains(output, "Beta task") {
			t.Error("expected Beta task in output")
		}
	})

	t.Run("ListTasksArchived", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task1 := &core.Task{
			ID:       "T-0001",
			Title:    "Archived task",
			Status:   core.StatusDone,
			Archived: true,
		}
		task2 := &core.Task{
			ID:       "T-0002",
			Title:    "Active task",
			Status:   core.StatusTodo,
			Archived: false,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--archived"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --archived failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Archived task") {
			t.Error("expected archived task in output")
		}
		if !contains(output, "Active task") {
			t.Error("expected active task in output")
		}
	})

	t.Run("CreateTaskWithCustomID", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Custom ID task", "--id", "T-9999"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with --id failed: %v", err)
		}

		task, _ := s.GetTask(ctx, "T-9999")
		if task == nil {
			t.Fatal("task not found after creation")
		}
		if task.ID != "T-9999" {
			t.Errorf("expected task ID T-9999, got %s", task.ID)
		}
	})

	t.Run("CreateTaskWithReference", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		refURL := "https://github.com/repo/issues/42"
		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Task with reference", "--reference", refURL})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with --reference failed: %v", err)
		}

		task, _ := s.GetTask(ctx, "T-0001")
		if task == nil {
			t.Fatal("task not found after creation")
		}
		if task.Reference != refURL {
			t.Errorf("expected reference %s, got %s", refURL, task.Reference)
		}
	})

	t.Run("ClaimTaskWithNote", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Claim test task",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "claim", "T-0001", "--note", "Starting implementation"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task claim with --note failed: %v", err)
		}

		task, _ = s.GetTask(ctx, "T-0001")
		if task == nil {
			t.Fatal("task not found after claim")
		}
		if task.Status != core.StatusInProgress {
			t.Errorf("expected status IN_PROGRESS, got %s", task.Status)
		}
	})

	t.Run("UnclaimTaskWithNote", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		assignee := "testuser"
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Unclaim test task",
			Status:     core.StatusInProgress,
			AssignedTo: &assignee,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "unclaim", "T-0001", "--note", "Blocked, releasing"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task unclaim with --note failed: %v", err)
		}

		task, _ = s.GetTask(ctx, "T-0001")
		if task == nil {
			t.Fatal("task not found after unclaim")
		}
		if task.Status != core.StatusTodo {
			t.Errorf("expected status TODO, got %s", task.Status)
		}
		if task.AssignedTo != nil {
			t.Error("expected assignee to be nil after unclaim")
		}
	})

	t.Run("ShowTaskJSONFormat", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Show JSON test",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001", "--format", "json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show with --format json failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "\"id\"") {
			t.Error("expected JSON output with 'id' field")
		}
		if !contains(output, "\"title\"") {
			t.Error("expected JSON output with 'title' field")
		}
	})

	t.Run("ShowTaskYAMLFormat", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Show YAML test",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(taskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001", "--format", "yaml"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show with --format yaml failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "id:") {
			t.Error("expected YAML output with 'id:' field")
		}
		if !contains(output, "title:") {
			t.Error("expected YAML output with 'title:' field")
		}
	})

	t.Run("VerifyIDSequencing", func(t *testing.T) {
		s, _ := getStorage()
		defer s.Close()

		cmd1 := newTestCmd()
		cmd1.AddCommand(taskCmd)
		buf1 := new(bytes.Buffer)
		cmd1.SetOut(buf1)
		cmd1.SetErr(buf1)
		cmd1.SetArgs([]string{"task", "create", "First task"})

		if err := cmd1.Execute(); err != nil {
			t.Fatalf("first task create failed: %v", err)
		}

		cmd2 := newTestCmd()
		cmd2.AddCommand(taskCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"task", "create", "Second task"})

		if err := cmd2.Execute(); err != nil {
			t.Fatalf("second task create failed: %v", err)
		}

		tasks, _ := s.ListTasks(ctx, core.Query{})
		if len(tasks) < 2 {
			t.Fatal("expected at least 2 tasks")
		}

		taskIDs := make([]string, len(tasks))
		for i, task := range tasks {
			taskIDs[i] = task.ID
		}

		if taskIDs[0] == taskIDs[1] {
			t.Error("expected different task IDs (no sequencing conflict)")
		}
	})
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

// Helper function for test setup
func setupTestDir(t *testing.T) (ctx context.Context, cleanup func()) {
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", tmpDir)
	}

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	todoPath := filepath.Join(tmpDir, "TODO")

	viper.Reset()
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)
	viper.Set("task.todo_file", todoPath)
	viper.Set("output.format", "json")

	ctx = context.Background()

	cleanup = func() {
		os.RemoveAll(tmpDir)
	}

	return
}

// Test Task Creation with Assignee
func TestTaskCreateWithAssignee(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

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
	task, _ := s.GetTask(ctx, "T-0001")
	if task == nil {
		t.Fatal("task was not created")
	}
	if task.AssignedTo == nil {
		t.Error("assignee field is nil, expected 'engineer-1'")
	} else if *task.AssignedTo != "engineer-1" {
		t.Errorf("assignee field is %s, expected 'engineer-1'", *task.AssignedTo)
	}
}

// Test Task Creation with Tags
func TestTaskCreateWithTags(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

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
}

// Test Filter Tasks by Tag
func TestTaskFilterByTag(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
}

// Test Filter Tasks by Assignee
func TestTaskFilterByAssignee(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
}

// Test Filter Tasks by Status
func TestTaskFilterByStatus(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
}

// Test Filter Tasks by Current User (assigned-to me)
func TestTaskFilterByAssigneeMe(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
}

// Test Update Assignee
func TestTaskUpdateAssignee(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	if updatedTask.AssignedTo == nil {
		t.Error("assignee field is nil after update")
	} else if *updatedTask.AssignedTo != "engineer-2" {
		t.Errorf("assignee field is %s, expected 'engineer-2'", *updatedTask.AssignedTo)
	}
}

// Test Clear Assignee with null
func TestTaskClearAssigneeNull(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
		t.Fatalf("task update to clear assignee with null failed: %v", err)
	}

	updatedTask, _ := s.GetTask(ctx, "T-0001")
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	if updatedTask.AssignedTo != nil {
		t.Errorf("assignee field is %s, expected nil (cleared)", *updatedTask.AssignedTo)
	}
}

// Test Clear Assignee with dash
func TestTaskClearAssigneeDash(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	if updatedTask.AssignedTo != nil {
		t.Errorf("assignee field is %s, expected nil (cleared)", *updatedTask.AssignedTo)
	}
}

// Test Add Tag
func TestTaskAddTag(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

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
}

// Test Remove Tag
func TestTaskRemoveTag(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorage()
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
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	if len(updatedTask.Tags) != 1 {
		t.Errorf("expected 1 tag after removal, got %d", len(updatedTask.Tags))
	}
	if len(updatedTask.Tags) > 0 && updatedTask.Tags[0] != "bug" {
		t.Errorf("expected 'bug' tag, got '%s'", updatedTask.Tags[0])
	}
}
