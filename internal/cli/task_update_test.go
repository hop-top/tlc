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
