package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskFilterStatusCaseInsensitiveTodo verifies --status=todo (lowercase) matches TODO.
func TestTaskFilterStatusCaseInsensitiveTodo(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Todo task", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Done task", Status: core.StatusDone})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "todo"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --status todo failed: %v", err)
	}
	out := buf.String()
	if !contains(out, "Todo task") {
		t.Errorf("--status todo: expected 'Todo task', got: %s", out)
	}
	if contains(out, "Done task") {
		t.Errorf("--status todo: unexpected 'Done task', got: %s", out)
	}
}

// TestTaskFilterStatusCaseInsensitiveMixed verifies --status=Todo (mixed case) matches TODO.
func TestTaskFilterStatusCaseInsensitiveMixed(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Todo task", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "Todo"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --status Todo failed: %v", err)
	}
	if !contains(buf.String(), "Todo task") {
		t.Errorf("--status Todo: expected 'Todo task'; got: %s", buf.String())
	}
}

// TestTaskFilterStatusAliasComplete verifies --status=complete maps to DONE.
func TestTaskFilterStatusAliasComplete(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Finished task", Status: core.StatusDone, UpdatedAt: now, CreatedAt: now})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Open task", Status: core.StatusTodo, UpdatedAt: now, CreatedAt: now})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "complete"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --status complete failed: %v", err)
	}
	out := buf.String()
	if !contains(out, "Finished task") {
		t.Errorf("--status complete: expected 'Finished task'; got: %s", out)
	}
	if contains(out, "Open task") {
		t.Errorf("--status complete: unexpected 'Open task'; got: %s", out)
	}
}

// TestTaskFilterStatusAliasOpen verifies --status=open maps to TODO.
func TestTaskFilterStatusAliasOpen(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Finished task", Status: core.StatusDone, UpdatedAt: now, CreatedAt: now})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Open task", Status: core.StatusTodo, UpdatedAt: now, CreatedAt: now})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "open"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --status open failed: %v", err)
	}
	out := buf.String()
	if !contains(out, "Open task") {
		t.Errorf("--status open: expected 'Open task'; got: %s", out)
	}
	if contains(out, "Finished task") {
		t.Errorf("--status open: unexpected 'Finished task'; got: %s", out)
	}
}

// TestTaskFilterStatusAliasToDo verifies --status=to-do maps to TODO.
func TestTaskFilterStatusAliasToDo(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Open task", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "to-do"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --status to-do failed: %v", err)
	}
	if !contains(buf.String(), "Open task") {
		t.Errorf("--status to-do: expected 'Open task'; got: %s", buf.String())
	}
}

// TestTaskFilterPriorityCaseInsensitiveLower verifies --priority=p0 matches P0.
func TestTaskFilterPriorityCaseInsensitiveLower(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "P0 task", Status: core.StatusTodo, Priority: core.PriorityP0})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "P2 task", Status: core.StatusTodo, Priority: core.PriorityP2})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--priority", "p0"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --priority p0 failed: %v", err)
	}
	out := buf.String()
	if !contains(out, "P0 task") {
		t.Errorf("--priority p0: expected 'P0 task'; got: %s", out)
	}
	if contains(out, "P2 task") {
		t.Errorf("--priority p0: unexpected 'P2 task'; got: %s", out)
	}
}

// TestTaskFilterPriorityAliasCritical verifies --priority=critical maps to P0.
func TestTaskFilterPriorityAliasCritical(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Critical task", Status: core.StatusTodo, Priority: core.PriorityP0})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Low task", Status: core.StatusTodo, Priority: core.PriorityP3})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--priority", "critical"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --priority critical failed: %v", err)
	}
	out := buf.String()
	if !contains(out, "Critical task") {
		t.Errorf("--priority critical: expected 'Critical task'; got: %s", out)
	}
	if contains(out, "Low task") {
		t.Errorf("--priority critical: unexpected 'Low task'; got: %s", out)
	}
}

// TestTaskFilterPriorityAliasLow verifies --priority=low maps to P3.
func TestTaskFilterPriorityAliasLow(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Critical task", Status: core.StatusTodo, Priority: core.PriorityP0})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Low task", Status: core.StatusTodo, Priority: core.PriorityP3})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--priority", "low"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --priority low failed: %v", err)
	}
	out := buf.String()
	if !contains(out, "Low task") {
		t.Errorf("--priority low: expected 'Low task'; got: %s", out)
	}
	if contains(out, "Critical task") {
		t.Errorf("--priority low: unexpected 'Critical task'; got: %s", out)
	}
}

// TestTaskFilterPriorityAliasNumeric verifies --priority=0 maps to P0.
func TestTaskFilterPriorityAliasNumeric(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Critical task", Status: core.StatusTodo, Priority: core.PriorityP0})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Low task", Status: core.StatusTodo, Priority: core.PriorityP3})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--priority", "0"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --priority 0 failed: %v", err)
	}
	out := buf.String()
	if !contains(out, "Critical task") {
		t.Errorf("--priority 0: expected 'Critical task'; got: %s", out)
	}
	if contains(out, "Low task") {
		t.Errorf("--priority 0: unexpected 'Low task'; got: %s", out)
	}
}

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
