package cli

import (
	"bytes"
	"testing"

	"hop.top/tlc/internal/core"
)

// TestTaskUpdateCommands tests update, tag management, and assignee changes.
func TestTaskUpdateCommands(t *testing.T) {
	t.Run("UpdateTask", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Original Title", Status: core.StatusTodo})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--status", "IN_PROGRESS", "--title", "Updated Title"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update failed: %v", err)
		}
	})

	t.Run("UpdateAssignee", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		assignee1 := testEngineer1
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Task to reassign",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--assigned-to", testEngineer2})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update assignee failed: %v", err)
		}

		updatedTask, _ := s.GetTask(ctx, "T-0001")
		if updatedTask.AssignedTo == nil {
			t.Error("assignee field is nil after update")
		} else if *updatedTask.AssignedTo != testEngineer2 {
			t.Errorf("assignee is %s, expected 'engineer-2'", *updatedTask.AssignedTo)
		}
	})

	t.Run("ClearAssigneeWithNull", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		assignee1 := testEngineer1
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Task with assignee",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
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
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		assignee1 := testEngineer1
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Task with assignee",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
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
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Task without urgent tag",
			Status: core.StatusTodo,
			Tags:   []string{testTagBug},
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
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
			if tag == testTagBug {
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
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Task with chore tag",
			Status: core.StatusTodo,
			Tags:   []string{"chore", testTagBug},
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
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
		if len(updatedTask.Tags) > 0 && updatedTask.Tags[0] != testTagBug {
			t.Errorf("expected 'bug' tag, got '%s'", updatedTask.Tags[0])
		}
	})
}

// TestTaskUpdateAssignee tests standalone update of assignee.
func TestTaskUpdateAssignee(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	assignee1 := testEngineer1
	task := &core.Task{
		ID:         "T-0001",
		Title:      "Task to reassign",
		Status:     core.StatusTodo,
		AssignedTo: &assignee1,
	}
	s.CreateTask(ctx, task)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--assigned-to", testEngineer2})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update assignee failed: %v", err)
	}

	updatedTask, _ := s.GetTask(ctx, "T-0001")
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	if updatedTask.AssignedTo == nil {
		t.Error("assignee field is nil after update")
	} else if *updatedTask.AssignedTo != testEngineer2 {
		t.Errorf("assignee field is %s, expected 'engineer-2'", *updatedTask.AssignedTo)
	}
}

// TestTaskCreateWithPriority tests creating a task with --priority flag.
func TestTaskCreateWithPriority(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Priority Task", "--priority", "P0"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create --priority failed: %v", err)
	}

	tasks, _ := s.ListTasks(ctx, core.Query{})
	if len(tasks) == 0 {
		t.Fatal("no tasks found after create")
	}
	if tasks[0].Priority != core.PriorityP0 {
		t.Errorf("expected priority P0, got %q", tasks[0].Priority)
	}
}

// TestTaskCreateWithPriorityInvalid tests that an invalid priority value is rejected.
func TestTaskCreateWithPriorityInvalid(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	t.Cleanup(func() { taskPriority = "" })

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Bad Priority Task", "--priority", "HIGH"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid priority, got nil")
	}
}

// TestTaskClearAssigneeNull tests clearing assignee with "null".
func TestTaskClearAssigneeNull(t *testing.T) {
	testClearAssignee(t, "null")
}

// TestTaskClearAssigneeDash tests clearing assignee with "-".
func TestTaskClearAssigneeDash(t *testing.T) {
	testClearAssignee(t, "-")
}

// TestTaskUpdate tests adding a tag via update.
func TestTaskUpdate(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	task := &core.Task{
		ID:     "T-0001",
		Title:  "Task without urgent tag",
		Status: core.StatusTodo,
		Tags:   []string{testTagBug},
	}
	s.CreateTask(ctx, task)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
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
		if tag == testTagBug {
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

// TestTaskRemoveTag tests standalone remove-tag via update.
func TestTaskRemoveTag(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	task := &core.Task{
		ID:     "T-0001",
		Title:  "Task with chore tag",
		Status: core.StatusTodo,
		Tags:   []string{"chore", testTagBug},
	}
	s.CreateTask(ctx, task)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
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
	if len(updatedTask.Tags) > 0 && updatedTask.Tags[0] != testTagBug {
		t.Errorf("expected 'bug' tag, got '%s'", updatedTask.Tags[0])
	}
}

// TestTaskUpdateEffort tests setting effort on a task via update.
func TestTaskUpdateEffort(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Task", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--effort", "M"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --effort failed: %v", err)
	}

	updated, _ := s.GetTask(ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.Effort != core.EffortM {
		t.Errorf("expected effort M, got %q", updated.Effort)
	}
}

// TestTaskUpdateEffortInvalid tests that an invalid effort value is rejected.
func TestTaskUpdateEffortInvalid(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Task", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--effort", "HUGE"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid effort, got nil")
	}
}

// TestTaskUpdatePriority tests setting priority on a task via update.
func TestTaskUpdatePriority(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Task", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--priority", "P1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --priority failed: %v", err)
	}

	updated, _ := s.GetTask(ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.Priority != core.PriorityP1 {
		t.Errorf("expected priority P1, got %q", updated.Priority)
	}
}

// TestTaskUpdatePriorityInvalid tests that an invalid priority value is rejected.
func TestTaskUpdatePriorityInvalid(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	t.Cleanup(func() { taskUpdatePriority = "" })
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Task", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--priority", "CRITICAL"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid priority, got nil")
	}
}

// TestTaskCreateWithEffort tests creating a task with --effort flag.
func TestTaskCreateWithEffort(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Effort Task", "--effort", "L"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create --effort failed: %v", err)
	}

	tasks, _ := s.ListTasks(ctx, core.Query{})
	if len(tasks) == 0 {
		t.Fatal("no tasks found after create")
	}
	if tasks[0].Effort != core.EffortL {
		t.Errorf("expected effort L, got %q", tasks[0].Effort)
	}
}
