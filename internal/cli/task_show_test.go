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

func TestShortenTaskRef(t *testing.T) {
	tests := []struct {
		name             string
		ref              string
		currentProjectID string
		want             string
	}{
		{
			name:             "same project slash form",
			ref:              "hop-top/tlc/T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "T-0013",
		},
		{
			name:             "cross project slash form",
			ref:              "hop-top/c12n/T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "hop-top/c12n#T-0013",
		},
		{
			name:             "bare task ID unchanged",
			ref:              "T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "T-0013",
		},
		{
			name:             "tlc URI same project",
			ref:              "tlc://hop-top/tlc/T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "T-0013",
		},
		{
			name:             "tlc URI cross project",
			ref:              "tlc://hop-top/c12n/T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "hop-top/c12n#T-0013",
		},
		{
			name:             "legacy task URI same project",
			ref:              "task://hop-top/tlc/T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "T-0013",
		},
		{
			name:             "legacy task URI cross project",
			ref:              "task://hop-top/c12n/T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "hop-top/c12n#T-0013",
		},
		{
			name:             "local tlc URI triple slash",
			ref:              "tlc:///T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "T-0013",
		},
		{
			name:             "hash form same project",
			ref:              "hop-top/tlc#T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "T-0013",
		},
		{
			name:             "hash form cross project",
			ref:              "hop-top/c12n#T-0013",
			currentProjectID: "hop-top/tlc",
			want:             "hop-top/c12n#T-0013",
		},
		{
			name:             "empty project context converts slash to hash form",
			ref:              "hop-top/tlc/T-0013",
			currentProjectID: "",
			want:             "hop-top/tlc#T-0013",
		},
		{
			name:             "http URL returned unchanged",
			ref:              "https://github.com/org/repo/issues/42",
			currentProjectID: "hop-top/tlc",
			want:             "https://github.com/org/repo/issues/42",
		},
		{
			name:             "non-task tail returned unchanged",
			ref:              "hop-top/tlc/not-a-task",
			currentProjectID: "hop-top/tlc",
			want:             "hop-top/tlc/not-a-task",
		},
		{
			name:             "custom URI scheme returned unchanged",
			ref:              "myapp://some/resource",
			currentProjectID: "hop-top/tlc",
			want:             "myapp://some/resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortenTaskRef(tt.ref, tt.currentProjectID)
			if got != tt.want {
				t.Errorf("shortenTaskRef(%q, %q) = %q, want %q",
					tt.ref, tt.currentProjectID, got, tt.want)
			}
		})
	}
}

func TestRenderTaskRelations_ShortRefs(t *testing.T) {
	buf := new(bytes.Buffer)
	blockedBy := []relatedTaskSummary{
		{Ref: "hop-top/tlc/T-0001", Title: "Same proj", Status: core.StatusTodo},
		{Ref: "hop-top/c12n/T-0005", Title: "Cross proj", Status: core.StatusInProgress},
	}
	blocking := []relatedTaskSummary{
		{Ref: "T-0099", Title: "Bare ref", Status: core.StatusDone},
	}

	renderTaskRelations(buf, blockedBy, blocking, "hop-top/tlc")
	output := buf.String()

	// Same-project ref should be shortened
	if !contains(output, "T-0001") {
		t.Errorf("expected short ref T-0001 in output, got: %s", output)
	}
	if contains(output, "hop-top/tlc/T-0001") {
		t.Errorf("expected same-project ref to be shortened, got: %s", output)
	}

	// Cross-project ref should use hash form
	if !contains(output, "hop-top/c12n#T-0005") {
		t.Errorf("expected cross-project ref hop-top/c12n#T-0005 in output, got: %s", output)
	}

	// Bare ref should pass through
	if !contains(output, "T-0099") {
		t.Errorf("expected bare ref T-0099 in output, got: %s", output)
	}
}

// TestTaskShow_BlockedBy_RendersDisplayID is a regression test for the
// "Blocked By" / "Blocking" sections leaking raw typeids (task_01k…) when
// blocked_by entries are stored as durable typeids. After the fix, the
// renderer must convert them to display IDs (T-NNNN) via FormatTaskDisplay.
func TestTaskShow_BlockedBy_RendersDisplayID(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	// Base task — stored with a durable typeid as its ID and Seq=1 so
	// FormatTaskDisplay yields T-0001. This mirrors production rows
	// where Task.ID is the typeid and Task.Seq drives display.
	baseID := "task_01kqnzg72be3zsg5zmh73y94c9"
	if err := s.CreateTask(ctx, &core.Task{
		ID:     baseID,
		Seq:    1,
		Title:  "Base task",
		Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("setup CreateTask(base) failed: %v", err)
	}

	// Dependent task — stores blocked_by as the raw typeid, exactly
	// like the live aps T-0586 scenario.
	depID := "task_01kqnzg72be4096bzj8r90h0gz"
	if err := s.CreateTask(ctx, &core.Task{
		ID:     depID,
		Seq:    2,
		Title:  "Dependent task",
		Status: core.StatusTodo,
		Meta: map[string]interface{}{
			"blocked_by": []string{baseID},
		},
	}); err != nil {
		t.Fatalf("setup CreateTask(dep) failed: %v", err)
	}

	// Show the dependent — exercises resolveBlockedBySummaries.
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0002"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show dep failed: %v", err)
	}
	output := buf.String()
	if !contains(output, "Blocked By:") {
		t.Fatalf("expected Blocked By section, got: %s", output)
	}
	if !contains(output, "T-0001") {
		t.Errorf("expected display ID T-0001 in Blocked By, got: %s", output)
	}
	if contains(output, "task_01k") || contains(output, "task_") {
		t.Errorf("Blocked By leaked raw typeid; expected display ID, got: %s", output)
	}

	// Show the base — exercises relatedTaskRef (Blocking section).
	cmd = newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf = new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show base failed: %v", err)
	}
	output = buf.String()
	if !contains(output, "Blocking:") {
		t.Fatalf("expected Blocking section, got: %s", output)
	}
	if !contains(output, "T-0002") {
		t.Errorf("expected display ID T-0002 in Blocking, got: %s", output)
	}
	if contains(output, "task_01k") || contains(output, "task_") {
		t.Errorf("Blocking leaked raw typeid; expected display ID, got: %s", output)
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

		task := getTaskByAlias(t, ctx, "T-0001")
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
