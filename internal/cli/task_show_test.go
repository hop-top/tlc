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

// TestFindBlockingTasks_CrossProjectFalsePositive verifies that
// findBlockingTasks does NOT report tasks from other projects as
// blocking when they reference the same bare T-ID in their
// blocked_by (T-0436 regression).
func TestFindBlockingTasks_CrossProjectFalsePositive(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	projA := "proj-a"
	projB := "proj-b"

	// Project A: T-0001 (the task we will view)
	s.CreateTask(ctx, &core.Task{
		ID:        "T-0001",
		Title:     "Project A base",
		Status:    core.StatusTodo,
		ProjectID: &projA,
	})

	// Project A: T-0002 blocked by T-0001 (same project, legit)
	s.CreateTask(ctx, &core.Task{
		ID:        "T-0002",
		Title:     "Project A dependent",
		Status:    core.StatusTodo,
		ProjectID: &projA,
		Meta: map[string]interface{}{
			"blocked_by": []string{"T-0001"},
		},
	})

	// Project B: T-0001 (different task, same bare ID)
	s.CreateTask(ctx, &core.Task{
		ID:        "T-0001",
		Title:     "Project B base",
		Status:    core.StatusTodo,
		ProjectID: &projB,
	})

	// Project B: T-0003 blocked by T-0001 (refers to proj-B's
	// T-0001, NOT proj-A's). This must NOT appear when viewing
	// proj-A's T-0001.
	s.CreateTask(ctx, &core.Task{
		ID:        "T-0003",
		Title:     "Project B unrelated",
		Status:    core.StatusTodo,
		ProjectID: &projB,
		Meta: map[string]interface{}{
			"blocked_by": []string{"T-0001"},
		},
	})

	// Call findBlockingTasks for project A's T-0001.
	viewedTask := &core.Task{
		ID:        "T-0001",
		Title:     "Project A base",
		Status:    core.StatusTodo,
		ProjectID: &projA,
	}
	blocking := findBlockingTasks(ctx, s, viewedTask)

	// We expect ONLY project A's T-0002, not project B's T-0003.
	for _, b := range blocking {
		if contains(b.Title, "Project B") {
			t.Errorf(
				"findBlockingTasks returned cross-project false "+
					"positive: %s (%s); bare T-ID from another "+
					"project should not match",
				b.Ref, b.Title,
			)
		}
	}

	// Positive check: project A's T-0002 should still appear.
	foundLegit := false
	for _, b := range blocking {
		if contains(b.Title, "Project A dependent") {
			foundLegit = true
		}
	}
	if !foundLegit {
		t.Errorf(
			"findBlockingTasks missing same-project blocker; "+
				"got: %v", blocking,
		)
	}
}

// TestTaskShowMultipleIDs verifies that show accepts multiple IDs,
// prints each task, and returns a non-zero exit on any missing ID.
func TestTaskShowMultipleIDs(t *testing.T) {
	t.Run("ShowMultipleTasks", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "First Task", Status: core.StatusTodo})
		s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "Second Task", Status: core.StatusInProgress})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001", "T-0002"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show multiple failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "First Task") {
			t.Errorf("expected First Task in output, got: %s", output)
		}
		if !contains(output, "Second Task") {
			t.Errorf("expected Second Task in output, got: %s", output)
		}
	})

	t.Run("ShowPartialFailContinues", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Exists", Status: core.StatusTodo})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001", "T-9999"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error when one ID is missing, got nil")
		}
		output := buf.String()
		if !contains(output, "Exists") {
			t.Errorf("expected valid task to be printed before error, got: %s", output)
		}
		if !contains(err.Error(), "T-9999") {
			t.Errorf("expected missing ID in error, got: %s", err.Error())
		}
	})
}

// TestTaskShow_StaleFields_E2E is an end-to-end test: writes a task with stale
// fields directly to storage, then reads it back via "tlc task show" and verifies
// StaleTimeout, BlockedReason, and StaleFiredAt appear in the rendered output.
func TestTaskShow_StaleFields_E2E(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	timeout := 48 * time.Hour
	reason := "waiting for infra ticket"
	firedAt := time.Now().UTC().Truncate(time.Second)

	task := &core.Task{
		ID:            "T-0001",
		Title:         "E2E stale fields test",
		Status:        core.StatusInProgress,
		Reference:     "docs/stale-e2e.md",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC().Add(-50 * time.Hour),
		StaleTimeout:  &timeout,
		BlockedReason: &reason,
		StaleFiredAt:  &firedAt,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("setup CreateTask failed: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show failed: %v", err)
	}

	output := buf.String()
	t.Logf("show output: %s", output)

	if !contains(output, "48h0m0s") {
		t.Errorf("expected StaleTimeout '48h0m0s' in output, got: %s", output)
	}
	if !contains(output, reason) {
		t.Errorf("expected BlockedReason %q in output, got: %s", reason, output)
	}
	if !contains(output, firedAt.Format(time.RFC3339)) {
		t.Errorf("expected StaleFiredAt %q in output, got: %s", firedAt.Format(time.RFC3339), output)
	}
}

// TestTaskShowNotFound validates that showing a non-existent task returns an
// actionable error message telling the agent what to do next.
func TestTaskShowNotFound(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-9999"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-existent task, got nil")
	}

	msg := err.Error()
	if !contains(msg, "T-9999") {
		t.Errorf("expected task ID in error message, got: %q", msg)
	}
	if !contains(msg, "tlc task list") {
		t.Errorf("expected actionable hint 'tlc task list' in error message, got: %q", msg)
	}
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
