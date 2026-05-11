package cli

// End-to-end tests for `tlc task list` temporal filters (T-0908).
// Covers --due-before / --due-after / --overdue / --no-due, including
// mutual-exclusion validation. Exercises flag parsing → SQL pushdown →
// post-query filter → render.

import (
	"bytes"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// seedTemporalTasks creates four canonical fixtures in a fresh DB:
//
//	T-0001 overdue        IN_PROGRESS, DueAt = now - 48h
//	T-0002 future         TODO,        DueAt = now + 48h
//	T-0003 no-due         TODO,        DueAt = nil
//	T-0004 done-overdue   DONE,        DueAt = now - 72h
//
// Returns the cleanup func from setupTestDir so subtests can defer it.
func seedTemporalTasks(t *testing.T) func() {
	t.Helper()
	ctx, cleanup := setupTestDir(t)
	s, _ := getStorageRaw()
	t.Cleanup(func() { _ = s.Close() })

	now := time.Now().UTC()
	past := now.Add(-48 * time.Hour)
	future := now.Add(48 * time.Hour)
	pastDone := now.Add(-72 * time.Hour)

	// UpdatedAt must be recent on every fixture so the auto-archive
	// path (DONE/SKIPPED older than task.archive_threshold) does not
	// archive T-0004 before the test reads it back.
	tasks := []*core.Task{
		{ID: "T-0001", Title: "Overdue task", Status: core.StatusInProgress, DueAt: &past, CreatedAt: now, UpdatedAt: now},
		{ID: "T-0002", Title: "Future task", Status: core.StatusTodo, DueAt: &future, CreatedAt: now, UpdatedAt: now},
		{ID: "T-0003", Title: "No-due task", Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now},
		{ID: "T-0004", Title: "Done-overdue task", Status: core.StatusDone, DueAt: &pastDone, CreatedAt: now, UpdatedAt: now},
	}
	for _, task := range tasks {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}
	return cleanup
}

func TestTaskList_TemporalFilters_E2E(t *testing.T) {
	t.Run("DueBefore_IncludesAllPastDue", func(t *testing.T) {
		defer seedTemporalTasks(t)()

		// --due-before tomorrow: plain temporal filter, no status
		// exclusion — DONE-overdue IS included per spec. Pass
		// --status TODO,IN_PROGRESS,DONE to override the default
		// IN_PROGRESS/TODO-only filter so DONE-overdue shows.
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "list",
			"--due-before", "tomorrow",
			"--status", "TODO",
			"--status", "IN_PROGRESS",
			"--status", "DONE",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --due-before: %v", err)
		}
		out := buf.String()
		if !contains(out, "Overdue task") {
			t.Errorf("expected 'Overdue task' in output; got:\n%s", out)
		}
		if !contains(out, "Done-overdue task") {
			t.Errorf("expected 'Done-overdue task' (plain --due-before does not exclude DONE); got:\n%s", out)
		}
		if contains(out, "Future task") {
			t.Errorf("did not expect 'Future task'; got:\n%s", out)
		}
		if contains(out, "No-due task") {
			t.Errorf("did not expect 'No-due task' (NULL due_at fails comparison); got:\n%s", out)
		}
	})

	t.Run("Overdue_ExcludesDoneAndSkipped", func(t *testing.T) {
		defer seedTemporalTasks(t)()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		// Pass --status to include DONE in the SQL pool; --overdue's
		// own post-query filter must still exclude it.
		cmd.SetArgs([]string{
			"task", "list",
			"--overdue",
			"--status", "TODO",
			"--status", "IN_PROGRESS",
			"--status", "DONE",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --overdue: %v", err)
		}
		out := buf.String()
		if !contains(out, "Overdue task") {
			t.Errorf("expected 'Overdue task' in --overdue output; got:\n%s", out)
		}
		if contains(out, "Done-overdue task") {
			t.Errorf("did not expect 'Done-overdue task' in --overdue output; got:\n%s", out)
		}
		if contains(out, "Future task") {
			t.Errorf("did not expect 'Future task'; got:\n%s", out)
		}
		if contains(out, "No-due task") {
			t.Errorf("did not expect 'No-due task'; got:\n%s", out)
		}
	})

	t.Run("DueAfter_ReturnsFutureOnly", func(t *testing.T) {
		defer seedTemporalTasks(t)()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--due-after", "tomorrow"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --due-after: %v", err)
		}
		out := buf.String()
		if !contains(out, "Future task") {
			t.Errorf("expected 'Future task' in --due-after output; got:\n%s", out)
		}
		if contains(out, "Overdue task") {
			t.Errorf("did not expect 'Overdue task' in --due-after output; got:\n%s", out)
		}
		if contains(out, "No-due task") {
			t.Errorf("did not expect 'No-due task' (NULL fails comparison); got:\n%s", out)
		}
	})

	t.Run("NoDue_ReturnsNullDueAtOnly", func(t *testing.T) {
		defer seedTemporalTasks(t)()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--no-due"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --no-due: %v", err)
		}
		out := buf.String()
		if !contains(out, "No-due task") {
			t.Errorf("expected 'No-due task' in --no-due output; got:\n%s", out)
		}
		if contains(out, "Overdue task") || contains(out, "Future task") {
			t.Errorf("did not expect tasks with due_at in --no-due output; got:\n%s", out)
		}
	})

	t.Run("OverdueWithDueBefore_Errors", func(t *testing.T) {
		defer seedTemporalTasks(t)()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "list",
			"--overdue",
			"--due-before", "tomorrow",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error when combining --overdue and --due-before; got nil")
		}
		if !contains(err.Error(), "mutually exclusive") {
			t.Errorf("expected 'mutually exclusive' in error; got: %v", err)
		}
	})

	t.Run("NoDueWithDueAfter_Errors", func(t *testing.T) {
		defer seedTemporalTasks(t)()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"task", "list",
			"--no-due",
			"--due-after", "tomorrow",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error when combining --no-due and --due-after; got nil")
		}
		if !contains(err.Error(), "mutually exclusive") {
			t.Errorf("expected 'mutually exclusive' in error; got: %v", err)
		}
	})

	t.Run("InvalidDueBeforeValue_Errors", func(t *testing.T) {
		defer seedTemporalTasks(t)()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--due-before", "not-a-time"})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected parse error on --due-before garbage; got nil")
		}
		if !contains(err.Error(), "--due-before") {
			t.Errorf("expected error to mention --due-before; got: %v", err)
		}
	})
}
