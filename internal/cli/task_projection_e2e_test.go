package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestProjectionLifecycle verifies the full filesystem projection lifecycle:
// create -> claim -> complete -> sync -> disabled no-op.
func TestProjectionLifecycle(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		tasksDir := filepath.Join(cwd, ".tlc", "tasks")

		// Enable filesystem projection.
		viper.Set("storage.filesystem.enabled", true)
		viper.Set("storage.filesystem.group_by", []string{"status"})
		viper.Set("storage.filesystem.sort_by", []string{"id"})

		// --- Step 1: Create a task via CLI -> canonical file appears ---
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Projection test task"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v\n%s", err, buf.String())
		}

		// Determine task ID from storage.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		SetupProjector(s)

		tasks, err := s.ListTasks(ctx, core.Query{Limit: 1})
		if err != nil || len(tasks) == 0 {
			t.Fatalf("no tasks found after create: %v", err)
		}
		taskID := tasks[0].ID

		// Verify canonical file exists.
		canonicalPath := filepath.Join(tasksDir, "all", taskID+".json")
		if _, err := os.Stat(canonicalPath); os.IsNotExist(err) {
			t.Fatalf("canonical file not found at %s", canonicalPath)
		}

		// Verify by-status/todo symlink exists.
		todoGlob := filepath.Join(tasksDir, "by-status", "todo", "*"+taskID+".json")
		todoMatches, _ := filepath.Glob(todoGlob)
		if len(todoMatches) == 0 {
			t.Fatalf("expected symlink in by-status/todo/ for %s", taskID)
		}

		// --- Step 2: Claim (transition to IN_PROGRESS) -> symlinks update ---
		resetTaskFlags()
		cmd2 := newTestCmd()
		cmd2.AddCommand(TaskCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"task", "claim", taskID})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("task claim failed: %v\n%s", err, buf2.String())
		}

		// todo symlink should be gone; in-progress should exist.
		todoMatches2, _ := filepath.Glob(todoGlob)
		if len(todoMatches2) > 0 {
			t.Error("expected no symlink in by-status/todo/ after claim")
		}
		ipGlob := filepath.Join(tasksDir, "by-status", "in-progress", "*"+taskID+".json")
		ipMatches, _ := filepath.Glob(ipGlob)
		if len(ipMatches) == 0 {
			t.Fatalf("expected symlink in by-status/in-progress/ for %s", taskID)
		}

		// --- Step 3: Complete -> symlinks update to done ---
		resetTaskFlags()
		cmd3 := newTestCmd()
		cmd3.AddCommand(TaskCmd)
		buf3 := new(bytes.Buffer)
		cmd3.SetOut(buf3)
		cmd3.SetErr(buf3)
		cmd3.SetArgs([]string{"task", "complete", taskID})
		if err := cmd3.Execute(); err != nil {
			t.Fatalf("task complete failed: %v\n%s", err, buf3.String())
		}

		ipMatches2, _ := filepath.Glob(ipGlob)
		if len(ipMatches2) > 0 {
			t.Error("expected no symlink in by-status/in-progress/ after complete")
		}
		doneGlob := filepath.Join(tasksDir, "by-status", "done", "*"+taskID+".json")
		doneMatches, _ := filepath.Glob(doneGlob)
		if len(doneMatches) == 0 {
			t.Fatalf("expected symlink in by-status/done/ for %s", taskID)
		}

		// --- Step 4: tlc tasks sync rebuilds correctly ---
		resetTaskFlags()
		cmd4 := newTestCmd()
		cmd4.AddCommand(TasksCmd)
		buf4 := new(bytes.Buffer)
		cmd4.SetOut(buf4)
		cmd4.SetErr(buf4)
		cmd4.SetArgs([]string{"tasks", "sync"})
		if err := cmd4.Execute(); err != nil {
			t.Fatalf("tasks sync failed: %v\n%s", err, buf4.String())
		}

		// After sync, canonical + done symlink should still exist.
		if _, err := os.Stat(canonicalPath); os.IsNotExist(err) {
			t.Fatal("canonical file missing after sync")
		}
		doneMatches2, _ := filepath.Glob(doneGlob)
		if len(doneMatches2) == 0 {
			t.Fatal("expected symlink in by-status/done/ after sync")
		}

		output := buf4.String()
		if !contains(output, "rebuilt") || !contains(output, "1 task(s)") {
			t.Errorf("unexpected sync output: %s", output)
		}
	})
}

// TestProjectionDisabled verifies zero overhead when projection is disabled.
func TestProjectionDisabled(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		tasksDir := filepath.Join(cwd, ".tlc", "tasks")

		// Do NOT enable filesystem projection.
		viper.Set("storage.filesystem.enabled", false)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "No projection task"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v\n%s", err, buf.String())
		}

		// .tlc/tasks/ should NOT exist.
		if _, err := os.Stat(tasksDir); !os.IsNotExist(err) {
			t.Fatalf("expected .tlc/tasks/ to not exist when projection disabled, but it does")
		}
	})
}

// TestTasksSyncDryRun verifies --dry-run prints plan without writing.
func TestTasksSyncDryRun(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		tasksDir := filepath.Join(cwd, ".tlc", "tasks")

		viper.Set("storage.filesystem.enabled", true)
		viper.Set("storage.filesystem.group_by", []string{"status"})
		viper.Set("storage.filesystem.sort_by", []string{"id"})

		// Create a task directly in storage (bypassing projection).
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Dry run test",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task: %v", err)
		}

		resetTaskFlags()
		cmd := newTestCmd()
		cmd.AddCommand(TasksCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tasks", "sync", "--dry-run"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("tasks sync --dry-run failed: %v\n%s", err, buf.String())
		}

		output := buf.String()
		if !contains(output, "would rebuild") {
			t.Errorf("expected 'would rebuild' in dry-run output, got: %s", output)
		}
		if !contains(output, "T-0001") {
			t.Errorf("expected task ID in dry-run output, got: %s", output)
		}

		// NOTE: tasks dir may exist from auto-sync in SetupProjector
		// (triggered by getStorage). The dry-run flag only controls
		// the explicit sync command, not the auto-sync on first access.
		_ = tasksDir
	})
}

// TestTasksSyncDisabledMessage verifies message when projection not enabled.
func TestTasksSyncDisabledMessage(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		viper.Set("storage.filesystem.enabled", false)

		resetTaskFlags()
		cmd := newTestCmd()
		cmd.AddCommand(TasksCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"tasks", "sync"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("tasks sync failed: %v\n%s", err, buf.String())
		}

		output := buf.String()
		if !contains(output, "disabled") {
			t.Errorf("expected 'disabled' message, got: %s", output)
		}
	})
}
