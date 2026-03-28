package cli

import (
	"bytes"
	"testing"
	"time"

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

func TestTaskUpdateBlockedBy(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	task := &core.Task{
		ID:     "T-0001",
		Title:  "Blocked task",
		Status: core.StatusTodo,
		Meta: map[string]interface{}{
			"blocked_by": []string{"T-0009"},
		},
	}
	s.CreateTask(ctx, task)
	s.CreateTask(ctx, &core.Task{ID: "T-0009", Title: "Blocker 1", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0010", Title: "Blocker 2", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "update", "T-0001",
		"--add-blocked-by", "T-0010",
		"--add-blocked-by", "T-0009",
		"--remove-blocked-by", "T-0009",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update blockers failed: %v", err)
	}

	updatedTask, _ := s.GetTask(ctx, "T-0001")
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	blockedBy := updatedTask.BlockedBy()
	if len(blockedBy) != 1 || blockedBy[0] != "T-0010" {
		t.Fatalf("blocked_by = %v, want [T-0010]", blockedBy)
	}
}

func TestTaskUpdateClearBlockedBy(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	task := &core.Task{
		ID:     "T-0001",
		Title:  "Blocked task",
		Status: core.StatusTodo,
		Meta: map[string]interface{}{
			"blocked_by": []string{"T-0001", "T-0002"},
		},
	}
	s.CreateTask(ctx, task)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--clear-blocked-by"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --clear-blocked-by failed: %v", err)
	}

	updatedTask, _ := s.GetTask(ctx, "T-0001")
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}
	if len(updatedTask.BlockedBy()) != 0 {
		t.Fatalf("expected blocked_by to be cleared, got %v", updatedTask.BlockedBy())
	}
	if _, ok := updatedTask.Meta["blocked_by"]; ok {
		t.Fatalf("blocked_by key should be removed after clear, got %v", updatedTask.Meta["blocked_by"])
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

// TestTaskUpdate_BlockedReason tests setting a blocked reason via --blocked flag.
func TestTaskUpdate_BlockedReason(t *testing.T) {
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
	cmd.SetArgs([]string{"task", "update", "T-0001", "--blocked", "waiting on T-0002"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --blocked failed: %v", err)
	}

	updated, _ := s.GetTask(ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.BlockedReason == nil {
		t.Fatal("BlockedReason is nil, expected 'waiting on T-0002'")
	}
	if *updated.BlockedReason != "waiting on T-0002" {
		t.Errorf("BlockedReason = %q, want 'waiting on T-0002'", *updated.BlockedReason)
	}
}

// TestTaskUpdate_Unblock tests clearing a blocked reason via --unblock flag.
func TestTaskUpdate_Unblock(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	reason := "waiting on T-0002"
	s.CreateTask(ctx, &core.Task{
		ID:            "T-0001",
		Title:         "Blocked Task",
		Status:        core.StatusTodo,
		BlockedReason: &reason,
	})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--unblock"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --unblock failed: %v", err)
	}

	updated, _ := s.GetTask(ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.BlockedReason != nil {
		t.Errorf("BlockedReason = %q, expected nil after --unblock", *updated.BlockedReason)
	}
}

// TestTaskUpdate_Timeout tests setting a stale timeout via --timeout flag.
func TestTaskUpdate_Timeout(t *testing.T) {
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
	cmd.SetArgs([]string{"task", "update", "T-0001", "--timeout", "2h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --timeout failed: %v", err)
	}

	updated, _ := s.GetTask(ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.StaleTimeout == nil {
		t.Fatal("StaleTimeout is nil, expected 2h")
	}
	expected := 2 * time.Hour
	if *updated.StaleTimeout != expected {
		t.Errorf("StaleTimeout = %v, want 2h", *updated.StaleTimeout)
	}
}

// TestTaskUpdate_TimeoutInvalid tests that an invalid --timeout value is rejected.
func TestTaskUpdate_TimeoutInvalid(t *testing.T) {
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
	cmd.SetArgs([]string{"task", "update", "T-0001", "--timeout", "notaduration"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for invalid --timeout, got nil")
	}
}

// TestTaskCreate_Timeout tests setting stale timeout on task create via --timeout flag.
func TestTaskCreate_Timeout(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Timeout Task", "--timeout", "2h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create --timeout failed: %v", err)
	}

	tasks, _ := s.ListTasks(ctx, core.Query{})
	if len(tasks) == 0 {
		t.Fatal("no tasks found after create")
	}
	if tasks[0].StaleTimeout == nil {
		t.Fatal("StaleTimeout is nil, expected 2h")
	}
	expected := 2 * time.Hour
	if *tasks[0].StaleTimeout != expected {
		t.Errorf("StaleTimeout = %v, want 2h", *tasks[0].StaleTimeout)
	}
}
