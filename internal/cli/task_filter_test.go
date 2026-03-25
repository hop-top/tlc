package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskFilterByTag tests standalone filter by tag.
func TestTaskFilterByTag(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
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
	cmd.AddCommand(TaskCmd)
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

// TestTaskFilterByAssignee tests standalone filter by assignee.
func TestTaskFilterByAssignee(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	assignee1 := testEngineer1
	assignee2 := testEngineer2
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
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--assigned-to", testEngineer1})

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

// TestTaskFilterByStatus tests standalone filter by status.
func TestTaskFilterByStatus(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
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
	cmd.AddCommand(TaskCmd)
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

// TestTaskFilterByAssigneeMe tests standalone filter by current user.
func TestTaskFilterByAssigneeMe(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	currentUser := core.GetCurrentUser()
	if currentUser == "" {
		currentUser = testUser
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
	cmd.AddCommand(TaskCmd)
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

// TestTaskListSummary tests the summary output of task list.
func TestTaskListSummary(t *testing.T) {
	t.Run("TaskListSummaryFormat", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		proj := "test-project"
		s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Task 1", Status: core.StatusTodo, ProjectID: &proj})
		s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Task 2", Status: core.StatusInProgress, ProjectID: &proj})
		s.CreateTask(ctx, &core.Task{ID: "T-0003", Title: "Task 3", Status: core.StatusTodo, ProjectID: &proj})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--summary"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --summary failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Project:") {
			t.Errorf("expected 'Project:' header in summary output, got: %s", output)
		}
		if !contains(output, "TODO") {
			t.Errorf("expected 'TODO' status in summary output, got: %s", output)
		}
		if !contains(output, "IN_PROGRESS") {
			t.Errorf("expected 'IN_PROGRESS' status in summary output, got: %s", output)
		}
		if !contains(output, "Total") {
			t.Errorf("expected 'Total' in summary output, got: %s", output)
		}
	})
}

// TestTaskListFormat tests the format flag on task list.
func TestTaskListFormat(t *testing.T) {
	t.Run("TaskListFormatSummary", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		proj := "fmt-project"
		s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Task A", Status: core.StatusTodo, ProjectID: &proj})
		s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Task B", Status: core.StatusInProgress, ProjectID: &proj})

		viper.Set("output.format", "summary")
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list -f summary failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Project: fmt-project") {
			t.Errorf("expected 'Project: fmt-project' in output, got: %s", output)
		}
		if !contains(output, "TODO") {
			t.Errorf("expected 'TODO' in output, got: %s", output)
		}
		if !contains(output, "IN_PROGRESS") {
			t.Errorf("expected 'IN_PROGRESS' in output, got: %s", output)
		}
		if !contains(output, "Total") {
			t.Errorf("expected 'Total' in output, got: %s", output)
		}
	})
}
