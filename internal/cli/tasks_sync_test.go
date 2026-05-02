package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskSyncProjectionRunsRebuild verifies the new
// `tlc task sync-projection` command rebuilds the filesystem projection.
func TestTaskSyncProjectionRunsRebuild(t *testing.T) {
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

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0001",
			Title:  "Sync projection test",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task: %v", err)
		}

		resetTaskFlags()
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(errBuf)
		cmd.SetArgs([]string{"task", "sync-projection"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("task sync-projection failed: %v\n%s", err, buf.String())
		}

		out := buf.String()
		if !contains(out, "rebuilt") || !contains(out, "1 task(s)") {
			t.Errorf("unexpected sync output: %s", out)
		}

		// Canonical file should exist after rebuild.
		canonical := filepath.Join(tasksDir, "all", "T-0001.json")
		if _, err := os.Stat(canonical); os.IsNotExist(err) {
			t.Errorf("expected canonical file at %s", canonical)
		}

		// New command must NOT print the deprecation warning to stderr.
		if contains(errBuf.String(), "deprecated") {
			t.Errorf("unexpected deprecation warning on new command: %s",
				errBuf.String())
		}
	})
}

// TestTasksSyncDeprecatedAliasForwards verifies that `tlc tasks sync` still
// works, prints a deprecation warning to stderr, and forwards to the same
// rebuild handler.
func TestTasksSyncDeprecatedAliasForwards(t *testing.T) {
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

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		task := &core.Task{
			ID:     "T-0002",
			Title:  "Deprecated alias forwards",
			Status: core.StatusTodo,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task: %v", err)
		}

		resetTaskFlags()
		cmd := newTestCmd()
		cmd.AddCommand(TasksCmd)
		out := new(bytes.Buffer)
		errOut := new(bytes.Buffer)
		cmd.SetOut(out)
		cmd.SetErr(errOut)
		cmd.SetArgs([]string{"tasks", "sync"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("tasks sync failed: %v\nstdout: %s\nstderr: %s",
				err, out.String(), errOut.String())
		}

		// Deprecation warning must land on stderr.
		errStr := errOut.String()
		if !contains(errStr, "deprecated") {
			t.Errorf("expected deprecation warning on stderr, got: %s", errStr)
		}
		if !contains(errStr, "task sync-projection") {
			t.Errorf("expected stderr to point at 'task sync-projection', got: %s",
				errStr)
		}

		// Rebuild output should still appear on stdout.
		stdoutStr := out.String()
		if !contains(stdoutStr, "rebuilt") || !contains(stdoutStr, "1 task(s)") {
			t.Errorf("unexpected sync output: %s", stdoutStr)
		}

		// Canonical file should exist after rebuild via deprecated alias.
		canonical := filepath.Join(tasksDir, "all", "T-0002.json")
		if _, err := os.Stat(canonical); os.IsNotExist(err) {
			t.Errorf("expected canonical file at %s", canonical)
		}
	})
}
