package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskLifecycle tests claim, unclaim, complete, and reopen operations.
func TestTaskLifecycle(t *testing.T) {
	t.Run("ClaimTaskWithNote", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Claim test task",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
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
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		assignee := testUser
		task := &core.Task{
			ID:         "T-0001",
			Title:      "Unclaim test task",
			Status:     core.StatusInProgress,
			AssignedTo: &assignee,
		}
		s.CreateTask(ctx, task)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
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
}

// TestTaskAssign tests the assign subcommand.
func TestTaskAssign(t *testing.T) {
	t.Run("AssignTask", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		// Create task via CLI (ensures DB is properly initialized)
		createCmd := newTestCmd()
		createCmd.AddCommand(TaskCmd)
		createBuf := new(bytes.Buffer)
		createCmd.SetOut(createBuf)
		createCmd.SetErr(createBuf)
		createCmd.SetArgs([]string{"task", "create", "Assign test task"})
		if err := createCmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "assign", "T-0001", testEngineer1})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task assign failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Assigned task T-0001 to engineer-1") {
			t.Errorf("unexpected output: %s", output)
		}

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("failed to get storage: %v", err)
		}
		defer s.Close()

		updatedTask, err := s.GetTask(ctx, "T-0001")
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if updatedTask == nil {
			t.Fatal("task not found after assign")
		}
		if updatedTask.AssignedTo == nil {
			t.Fatal("assignee is nil after assign")
		}
		if *updatedTask.AssignedTo != testEngineer1 {
			t.Errorf("assignee is %s, expected %s", *updatedTask.AssignedTo, testEngineer1)
		}
		// Status should NOT change (unlike claim)
		if updatedTask.Status != core.StatusTodo {
			t.Errorf("expected status TODO (unchanged), got %s", updatedTask.Status)
		}
	})

	t.Run("AssignTaskWithNote", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		// Create task via CLI
		createCmd := newTestCmd()
		createCmd.AddCommand(TaskCmd)
		createBuf := new(bytes.Buffer)
		createCmd.SetOut(createBuf)
		createCmd.SetErr(createBuf)
		createCmd.SetArgs([]string{"task", "create", "Assign note test"})
		if err := createCmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "assign", "T-0001", testEngineer2, "--note", "Reassigning for review"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task assign with note failed: %v", err)
		}

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("failed to get storage: %v", err)
		}
		defer s.Close()

		updatedTask, err := s.GetTask(ctx, "T-0001")
		if err != nil {
			t.Fatalf("failed to get task: %v", err)
		}
		if updatedTask == nil {
			t.Fatal("task not found after assign")
		}
		if *updatedTask.AssignedTo != testEngineer2 {
			t.Errorf("assignee is %s, expected %s", *updatedTask.AssignedTo, testEngineer2)
		}
		// Status should remain TODO (no transition)
		if updatedTask.Status != core.StatusTodo {
			t.Errorf("expected status TODO (unchanged), got %s", updatedTask.Status)
		}

		// Verify REASSIGNED log entry exists
		logs, _ := s.GetLogs(ctx, "T-0001", "desc")
		if len(logs) == 0 {
			t.Fatal("expected at least one log entry")
		}
		found := false
		for _, l := range logs {
			if l.Action == core.ActionReassigned {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected REASSIGNED log entry")
		}
	})

	t.Run("AssignTaskNotFound", func(t *testing.T) {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "assign", "T-9999", testEngineer1})

		if err := cmd.Execute(); err == nil {
			t.Fatal("expected error for non-existent task")
		}
	})
}

// TestTaskUnassignRequiresNote verifies unassign requires --note flag.
func TestTaskUnassignRequiresNote(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	cmd1 := newTestCmd()
	cmd1.AddCommand(TaskCmd)
	buf1 := new(bytes.Buffer)
	cmd1.SetOut(buf1)
	cmd1.SetErr(buf1)
	cmd1.SetArgs([]string{"task", "create", "Task to unassign",
		"--assigned-to", testEngineer1, "--status", "IN_PROGRESS"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("task create failed: %v", err)
	}

	// Unassign WITHOUT --note should fail
	cmd2 := newTestCmd()
	cmd2.AddCommand(TaskCmd)
	buf2 := new(bytes.Buffer)
	cmd2.SetOut(buf2)
	cmd2.SetErr(buf2)
	cmd2.SetArgs([]string{"task", "unassign", "T-0001"})

	err := cmd2.Execute()
	if err == nil {
		t.Fatal("expected error when unassign called without --note")
	}
	if !contains(err.Error(), "note") {
		t.Errorf("error should mention --note, got: %v", err)
	}
}

// TestTaskUnassignAppendsNoteToDescription verifies note is appended to description.
func TestTaskUnassignAppendsNoteToDescription(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	cmd1 := newTestCmd()
	cmd1.AddCommand(TaskCmd)
	buf1 := new(bytes.Buffer)
	cmd1.SetOut(buf1)
	cmd1.SetErr(buf1)
	cmd1.SetArgs([]string{"task", "create", "Task to unassign",
		"--description", "Original description",
		"--assigned-to", testEngineer1, "--status", "IN_PROGRESS"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("task create failed: %v", err)
	}

	// Unassign WITH --note
	cmd2 := newTestCmd()
	cmd2.AddCommand(TaskCmd)
	buf2 := new(bytes.Buffer)
	cmd2.SetOut(buf2)
	cmd2.SetErr(buf2)
	cmd2.SetArgs([]string{"task", "unassign", "T-0001", "--note", "Blocked on dependency"})

	if err := cmd2.Execute(); err != nil {
		t.Fatalf("task unassign failed: %v", err)
	}

	// Verify note was appended to description
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("failed to get storage: %v", err)
	}
	defer s.Close()

	task, err := s.GetTask(ctx, "T-0001")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task == nil {
		t.Fatal("task not found after unassign")
	}
	if !contains(task.Description, "Original description") {
		t.Errorf("expected original description preserved, got: %s", task.Description)
	}
	if !contains(task.Description, "Blocked on dependency") {
		t.Errorf("expected note appended to description, got: %s", task.Description)
	}
}

// TestTaskUnassign tests the unassign subcommand fully.
func TestTaskUnassign(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	cmd1 := newTestCmd()
	cmd1.AddCommand(TaskCmd)
	buf1 := new(bytes.Buffer)
	cmd1.SetOut(buf1)
	cmd1.SetErr(buf1)
	cmd1.SetArgs([]string{"task", "create", "Task to unassign",
		"--assigned-to", testEngineer1, "--status", "IN_PROGRESS"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("task create failed: %v", err)
	}

	// Unassign the task (with required note)
	cmd2 := newTestCmd()
	cmd2.AddCommand(TaskCmd)
	buf2 := new(bytes.Buffer)
	cmd2.SetOut(buf2)
	cmd2.SetErr(buf2)
	cmd2.SetArgs([]string{"task", "unassign", "T-0001", "--note", "No longer needed"})

	if err := cmd2.Execute(); err != nil {
		t.Fatalf("task unassign failed: %v", err)
	}

	output := buf2.String()
	if !contains(output, "Unassigned task T-0001") {
		t.Errorf("expected confirmation message, got: %s", output)
	}

	// Verify task state
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("failed to get storage: %v", err)
	}
	defer s.Close()

	updatedTask, err := s.GetTask(ctx, "T-0001")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if updatedTask == nil {
		t.Fatal("task not found after unassign")
	}
	if updatedTask.AssignedTo != nil {
		t.Errorf("expected assignee nil, got %s", *updatedTask.AssignedTo)
	}
	// Status should NOT change (unlike unclaim)
	if updatedTask.Status != core.StatusInProgress {
		t.Errorf("expected status IN_PROGRESS (unchanged), got %s",
			updatedTask.Status)
	}

	// Verify log entry
	logs, _ := s.GetLogs(ctx, "T-0001", "desc")
	if len(logs) == 0 {
		t.Fatal("expected at least one log entry")
	}
	found := false
	for _, l := range logs {
		if l.Action == core.ActionReassigned && contains(l.Note, "No longer needed") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected REASSIGNED log entry with note containing 'No longer needed'")
	}
}

// TestTaskUnassignNotFound verifies error for missing task.
func TestTaskUnassignNotFound(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "unassign", "T-9999"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// TestTaskReopenRequiresNote verifies reopen requires --note flag.
func TestTaskReopenRequiresNote(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Reopen WITHOUT --note should fail
	cmd3 := newTestCmd()
	cmd3.AddCommand(TaskCmd)
	buf3 := new(bytes.Buffer)
	cmd3.SetOut(buf3)
	cmd3.SetErr(buf3)
	cmd3.SetArgs([]string{"task", "reopen", "T-0001"})

	err := cmd3.Execute()
	if err == nil {
		t.Fatal("expected error when reopen called without --note")
	}
	if !contains(err.Error(), "note") {
		t.Errorf("error should mention --note, got: %v", err)
	}
}

// TestTaskReopenAppendsNoteToDescription verifies note is appended on reopen.
func TestTaskReopenAppendsNoteToDescription(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	createAndCompleteTask(t, "Reopen test task", "Initial work done")

	// Reopen WITH --note
	cmd3 := newTestCmd()
	cmd3.AddCommand(TaskCmd)
	buf3 := new(bytes.Buffer)
	cmd3.SetOut(buf3)
	cmd3.SetErr(buf3)
	cmd3.SetArgs([]string{"task", "reopen", "T-0001", "--note", "Tests failing after merge"})
	if err := cmd3.Execute(); err != nil {
		t.Fatalf("task reopen failed: %v", err)
	}

	// Verify note was appended to description
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("failed to get storage: %v", err)
	}
	defer s.Close()

	task, err := s.GetTask(ctx, "T-0001")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task == nil {
		t.Fatal("task not found after reopen")
	}
	if !contains(task.Description, "Initial work done") {
		t.Errorf("expected original description preserved, got: %s", task.Description)
	}
	if !contains(task.Description, "Tests failing after merge") {
		t.Errorf("expected note appended to description, got: %s", task.Description)
	}
}

// TestClaimComplete_TaskDuplicatedAcrossProjectBuckets is a regression test for
// the bug where claim/complete silently updated the wrong row when the same task
// ID existed in multiple project_id buckets ('' global + project-scoped sync rows).
//
// Repro sequence:
//  1. Task created outside project context → project_id=''
//  2. Sync duplicates it into a project-scoped row (project_id='proj-a')
//  3. claim runs → GetTask without filter returns project-scoped row first (by rowid)
//     → UpdateTask updates that row → '' row stays TODO
//  4. complete reads '' row (still TODO) → "invalid transition from TODO to DONE"
//
// Fix: GetTask (no-project path) now orders by CASE WHEN project_id='' THEN 0 ELSE 1
// so the global row is preferred, and UpdateTask uses WHERE project_id=? (not IS NULL).
func TestClaimComplete_TaskDuplicatedAcrossProjectBuckets(t *testing.T) {
	withTestLock(func() {
		dbPath := resetTestDB(t)
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()

		// Step 1: insert project-scoped duplicate FIRST so it gets a lower rowid.
		// An unordered SELECT WHERE id=? would return this row first, proving the fix
		// is needed to prefer the global (project_id='') row.
		projID := "proj-a"
		_ = s.CreateTask(ctx, &core.Task{
			ID:        "T-0001",
			Title:     "Global task",
			ProjectID: &projID,
			Status:    core.StatusTodo,
			Reference: "ref",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})

		// Step 2: create global task (project_id='') second — higher rowid.
		// Without the fix an unordered SELECT would miss this row.
		_ = s.CreateTask(ctx, &core.Task{
			ID:        "T-0001",
			Title:     "Global task",
			Status:    core.StatusTodo,
			Reference: "ref",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})

		viper.Set("storage.db_path", dbPath)

		// Step 3: claim — must update the '' row, not the project-scoped one
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "claim", "T-0001"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("claim failed: %v", err)
		}

		// '' row must be IN_PROGRESS
		global, err := s.GetTaskInProject(ctx, "T-0001", "")
		if err != nil || global == nil {
			t.Fatalf("GetTaskInProject('') after claim: %v", err)
		}
		if global.Status != core.StatusInProgress {
			t.Errorf("global '' row: expected IN_PROGRESS after claim, got %s", global.Status)
		}

		// project-scoped row must be untouched
		scoped, err := s.GetTaskInProject(ctx, "T-0001", projID)
		if err != nil || scoped == nil {
			t.Fatalf("GetTaskInProject(projID) after claim: %v", err)
		}
		if scoped.Status != core.StatusTodo {
			t.Errorf("scoped row: expected TODO (unaffected), got %s", scoped.Status)
		}

		// Step 4: complete — must not fail with "invalid transition from TODO to DONE"
		resetTaskFlags()
		cmd2 := newTestCmd()
		cmd2.AddCommand(TaskCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"task", "complete", "T-0001"})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("complete failed (regression: reads stale '' row as TODO): %v", err)
		}

		global2, err := s.GetTaskInProject(ctx, "T-0001", "")
		if err != nil || global2 == nil {
			t.Fatalf("GetTaskInProject('') after complete: %v", err)
		}
		if global2.Status != core.StatusDone {
			t.Errorf("global '' row: expected DONE after complete, got %s", global2.Status)
		}
	})
}
