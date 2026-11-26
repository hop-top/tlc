package cli

import (
	"context"
	"errors"
	"testing"

	"hop.top/tlc/internal/core"
)

// A rejected meta write must stop applyTaskFieldChanges before ANY field
// is touched. The status branch logs its transition as it runs, so a
// check placed after it would leave a log row and a moved status behind
// an update that reports failure.
func TestApplyTaskFieldChanges_ReservedMetaWritesNothing(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		task := &core.Task{ID: core.NewTaskID(), Title: "orig", Status: core.StatusTodo}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
		logsBefore, _ := s.GetTaskLogs(ctx, task.ID)

		next, title := "IN_PROGRESS", "renamed"
		_, err = applyTaskFieldChanges(context.Background(), s, s, task, TaskFieldChanges{
			Title:  &title,
			Status: &next,
			Meta:   map[string]any{"eva": []any{"x"}},
		})
		if !errors.Is(err, core.ErrReservedMetaKey) {
			t.Fatalf("err = %v, want ErrReservedMetaKey", err)
		}
		if task.Title != "orig" || task.Status != core.StatusTodo || task.Meta != nil {
			t.Fatalf("task mutated in memory: %+v", task)
		}
		logsAfter, _ := s.GetTaskLogs(ctx, task.ID)
		if len(logsAfter) != len(logsBefore) {
			t.Fatalf("rejected update wrote %d log row(s)", len(logsAfter)-len(logsBefore))
		}
	})
}
