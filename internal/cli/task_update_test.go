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

		updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

		updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

		updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

		updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

		updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

	updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

	updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

	updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

// TestTaskCreateWithPriorityInvalid tests that an unresolvable priority
// value is rejected on create.
//
// Story 069 added "high" → P1 to the priority alias table, and story 070
// (this work) wires NormalizePriority into the create path. So uppercase
// inputs like "HIGH" now resolve to P1 and are accepted — that is the
// intended new behavior. This test uses a value that the alias table,
// lowercase canonical match, and fuzzy fallback all reject.
func TestTaskCreateWithPriorityInvalid(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	t.Cleanup(func() { taskPriority = "" })

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Bad Priority Task", "--priority", "garbage"})

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

	updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

	updatedTask := getTaskByAlias(t, ctx, "T-0001")
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

	updated := getTaskByAlias(t, ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.Effort != core.EffortM {
		t.Errorf("expected effort M, got %q", updated.Effort)
	}
}

// TestTaskUpdateEffortInvalid tests that an unresolvable effort value
// is rejected on update.
//
// Story 070 (T-1351) added "huge" → XL to the effort alias table, so
// "HUGE" → huge → XL is now valid (the intended new behavior). This
// test uses a value that the alias table, lowercase canonical match,
// and fuzzy fallback all reject.
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
	cmd.SetArgs([]string{"task", "update", "T-0001", "--effort", "garbage"})

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

	updated := getTaskByAlias(t, ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.Priority != core.PriorityP1 {
		t.Errorf("expected priority P1, got %q", updated.Priority)
	}
}

// TestTaskUpdatePriorityInvalid tests that an unresolvable priority
// value is rejected on update.
//
// Story 069 added "critical" → P0 to the priority alias table, so
// "CRITICAL" → critical → P0 is now valid (the intended behavior).
// This test uses a value that the alias table, lowercase canonical
// match, and fuzzy fallback all reject.
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
	cmd.SetArgs([]string{"task", "update", "T-0001", "--priority", "garbage"})

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

	updated := getTaskByAlias(t, ctx, "T-0001")
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

	updated := getTaskByAlias(t, ctx, "T-0001")
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

	updated := getTaskByAlias(t, ctx, "T-0001")
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

func TestTaskUpdate_MultipleIDs(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	cmd.SetArgs([]string{"task", "update", "T-0001", "T-0002", "--assigned-to", "alice"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, id := range []string{"T-0001", "T-0002"} {
		task, _ := s.GetTask(ctx, id)
		if task.AssignedTo == nil || *task.AssignedTo != "alice" {
			t.Errorf("expected %s assigned to alice", id)
		}
	}
}

func TestTaskDelete_MultipleIDs_WithNoPrompt(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()
	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "A", Status: core.StatusTodo})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "B", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	cmd.SetArgs([]string{"task", "delete", "T-0001", "T-0002", "--no-prompt"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, id := range []string{"T-0001", "T-0002"} {
		task, _ := s.GetTask(ctx, id)
		if task != nil {
			t.Errorf("expected %s to be deleted", id)
		}
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

// TestTaskUpdateStatusWithNote verifies --note|-n is plumbed to the
// state-machine transition log when --status changes (T-1178).
func TestTaskUpdateStatusWithNote(t *testing.T) {
	t.Run("StatusChangeWithNoteIsRecorded", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Note plumbing",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--status", "IN_PROGRESS",
			"--note", "Picking this up now",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --status --note failed: %v", err)
		}

		s2, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s2.Close()

		updated := getTaskByAlias(t, ctx, "T-0001")
		if updated == nil {
			t.Fatal("task not found after update")
		}
		if updated.Status != core.StatusInProgress {
			t.Errorf("status = %s, want IN_PROGRESS", updated.Status)
		}

		logs, err := s2.GetLogs(ctx, updated.ID, "desc")
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		if len(logs) == 0 {
			t.Fatal("expected at least one log entry")
		}
		found := false
		for _, l := range logs {
			if contains(l.Note, "Picking this up now") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected note to be recorded against transition log; got logs: %+v", logs)
		}
	})

	t.Run("StatusChangeWithShortNoteFlag", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Short flag plumbing",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--status", "IN_PROGRESS",
			"-n", "Short flag works",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --status -n failed: %v", err)
		}

		s2, _ := getStorageRaw()
		defer s2.Close()

		updated := getTaskByAlias(t, ctx, "T-0001")
		if updated == nil {
			t.Fatal("task not found after update")
		}
		logs, _ := s2.GetLogs(ctx, updated.ID, "desc")
		found := false
		for _, l := range logs {
			if contains(l.Note, "Short flag works") {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected note from -n short flag to be recorded")
		}
	})

	t.Run("StatusChangeWithoutNoteStillWorks", func(t *testing.T) {
		// Negative test: omitting --note must continue to work,
		// since this task does not enforce non-empty note (T-1192).
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "No note still works",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--status", "IN_PROGRESS",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update without --note failed: %v", err)
		}

		updated := getTaskByAlias(t, ctx, "T-0001")
		if updated == nil {
			t.Fatal("task not found after update")
		}
		if updated.Status != core.StatusInProgress {
			t.Errorf("status = %s, want IN_PROGRESS", updated.Status)
		}
	})

	t.Run("NoteFlagAcceptedWithoutStatusChange", func(t *testing.T) {
		// Ergonomics: --note is accepted unconditionally even when
		// --status is not the operation being performed.
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Original",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--title", "Renamed",
			"--note", "Just renaming",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --title --note failed: %v", err)
		}

		updated := getTaskByAlias(t, ctx, "T-0001")
		if updated == nil {
			t.Fatal("task not found after update")
		}
		if updated.Title != "Renamed" {
			t.Errorf("title = %q, want %q", updated.Title, "Renamed")
		}
	})
}

// TestTaskUpdateFieldOnlyNote covers notes supplied on updates that are
// not status transitions. Before this, --note was read only inside the
// Changed("status") branch, so SHA-linkage notes on ordinary edits were
// dropped silently at exit 0. Per docs/state-change-notes-design.md
// §189-195, a note is valid on ANY update and lands on an UPDATED log
// row.
func TestTaskUpdateFieldOnlyNote(t *testing.T) {
	t.Run("NoteWithFieldEditIsRecorded", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Tag plus note",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--add-tag", "wip",
			"--note", "def5678 second",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --add-tag --note failed: %v", err)
		}

		s2, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s2.Close()

		updated := getTaskByAlias(t, ctx, "T-0001")
		if updated == nil {
			t.Fatal("task not found after update")
		}
		// The field edit must still land.
		hasTag := false
		for _, tag := range updated.Tags {
			if tag == "wip" {
				hasTag = true
			}
		}
		if !hasTag {
			t.Errorf("tags = %v, want to contain %q", updated.Tags, "wip")
		}
		if updated.Status != core.StatusTodo {
			t.Errorf("status = %s, want TODO (note must not force a transition)", updated.Status)
		}

		logs, err := s2.GetLogs(ctx, updated.ID, "desc")
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		found := false
		for _, l := range logs {
			if contains(l.Note, "def5678 second") {
				found = true
				if l.Action != core.ActionUpdated {
					t.Errorf("action = %q, want %q", l.Action, core.ActionUpdated)
				}
			}
		}
		if !found {
			t.Errorf("expected note on UPDATED log row; got logs: %+v", logs)
		}
	})

	t.Run("NoteAloneIsRecorded", func(t *testing.T) {
		// The original report: --note as the sole flag exited 0 and
		// wrote nothing, because --note never set changed = true.
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Note alone",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--note", "abc1234 sha linkage",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --note failed: %v", err)
		}

		s2, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s2.Close()

		updated := getTaskByAlias(t, ctx, "T-0001")
		if updated == nil {
			t.Fatal("task not found after update")
		}
		if updated.Status != core.StatusTodo {
			t.Errorf("status = %s, want TODO", updated.Status)
		}

		logs, err := s2.GetLogs(ctx, updated.ID, "desc")
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		found := false
		for _, l := range logs {
			if contains(l.Note, "abc1234 sha linkage") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected bare --note to be recorded; got logs: %+v", logs)
		}
	})

	t.Run("StatusAndFieldEditShareOneNote", func(t *testing.T) {
		// design doc §189: a single note attaches to BOTH the
		// transition row and the field-update row.
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Both",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "update", "T-0001",
			"--status", "IN_PROGRESS",
			"--priority", "P0",
			"--note", "escalating now",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --status --priority --note failed: %v", err)
		}

		s2, _ := getStorageRaw()
		defer s2.Close()

		updated := getTaskByAlias(t, ctx, "T-0001")
		if updated == nil {
			t.Fatal("task not found after update")
		}
		if updated.Status != core.StatusInProgress {
			t.Errorf("status = %s, want IN_PROGRESS", updated.Status)
		}

		logs, _ := s2.GetLogs(ctx, updated.ID, "desc")
		var sawTransition, sawUpdate bool
		for _, l := range logs {
			if !contains(l.Note, "escalating now") {
				continue
			}
			if l.Action == core.ActionUpdated {
				sawUpdate = true
			} else {
				sawTransition = true
			}
		}
		if !sawTransition {
			t.Errorf("expected note on the transition row; got logs: %+v", logs)
		}
		if !sawUpdate {
			t.Errorf("expected note on the UPDATED row; got logs: %+v", logs)
		}
	})

	t.Run("FieldEditWithoutNoteWritesNoUpdatedLog", func(t *testing.T) {
		// Guard against log spam: only noted edits get an UPDATED row.
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "No note",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "update", "T-0001", "--priority", "P2"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task update --priority failed: %v", err)
		}

		s2, _ := getStorageRaw()
		defer s2.Close()

		updated := getTaskByAlias(t, ctx, "T-0001")
		if updated == nil {
			t.Fatal("task not found after update")
		}
		logs, _ := s2.GetLogs(ctx, updated.ID, "desc")
		for _, l := range logs {
			if l.Action == core.ActionUpdated {
				t.Errorf("unexpected UPDATED log row for un-noted edit: %+v", l)
			}
		}
	})
}

// TestTaskDeleteWithNote verifies --note|-n is plumbed to a DELETED
// log entry written before the row mutation. Post-T-1232 the cascade
// is dropped, so the log entry must remain queryable after delete.
func TestTaskDeleteWithNote(t *testing.T) {
	t.Run("DeleteWithNoteRecordsLog", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "About to be deleted",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "delete", "T-0001",
			"--yes",
			"--note", "Out of scope",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task delete --yes --note failed: %v", err)
		}

		// Task itself is gone…
		gone := getTaskByAlias(t, ctx, "T-0001")
		if gone != nil {
			t.Errorf("expected task removed, found: %+v", gone)
		}

		// …but the DELETED log entry with the note survives
		// (T-1232: cascade dropped from task_logs.task_id).
		s2, _ := getStorageRaw()
		defer s2.Close()
		logs, err := s2.GetLogs(ctx, "T-0001", "asc")
		if err != nil {
			t.Fatalf("GetLogs after delete: %v", err)
		}
		var found bool
		for _, le := range logs {
			if le.Action == core.ActionDeleted && le.Note == "Out of scope" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected DELETED log with note %q to survive task delete; got %d entries: %+v",
				"Out of scope", len(logs), logs)
		}
	})

	t.Run("DeleteWithoutNoteStillWorks", func(t *testing.T) {
		// Negative test: --note is optional; the flag is plumbed
		// but not enforced (enforcement lands in T-1192).
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Delete without note",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "delete", "T-0001", "--yes"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task delete --yes failed: %v", err)
		}

		gone := getTaskByAlias(t, ctx, "T-0001")
		if gone != nil {
			t.Errorf("expected task removed, found: %+v", gone)
		}
	})

	t.Run("DeleteWithShortNoteFlag", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{
			ID:     "T-0001",
			Title:  "Short flag",
			Status: core.StatusTodo,
		})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "delete", "T-0001",
			"--yes",
			"-n", "Short flag delete",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task delete --yes -n failed: %v", err)
		}

		gone := getTaskByAlias(t, ctx, "T-0001")
		if gone != nil {
			t.Errorf("expected task removed, found: %+v", gone)
		}
	})
}
