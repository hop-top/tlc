package storage

import (
	"context"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestLogOrderingTiebreak pins the ordering contract for task_logs reads
// when several rows share a timestamp.
//
// task_logs.timestamp is RFC3339 TEXT at one-second resolution, so any
// two log rows written inside the same second compare equal. Ordering by
// timestamp alone leaves those ties to SQLite's discretion, which made
// "the newest log entry" non-deterministic — `task update --amend` reads
// the newest row with LIMIT 1 and could rewrite an unrelated older entry.
// id is INTEGER PRIMARY KEY AUTOINCREMENT, so it is a strictly monotonic
// stand-in for insertion order and breaks the tie deterministically.
func TestLogOrderingTiebreak(t *testing.T) {
	// One fixed timestamp for every row: the tie is the whole point, and
	// hardcoding it removes any dependence on how fast the test runs.
	ts := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

	// Insertion order, oldest first. Actions are distinct so the
	// assertions can name the row that should have won.
	actions := []string{"CREATED", "IN_PROGRESS", "UPDATED", "DONE"}

	newFixture := func(t *testing.T) (*SQLiteStorage, context.Context, string) {
		t.Helper()
		s, err := NewSQLiteStorage(":memory:")
		if err != nil {
			t.Fatalf("NewSQLiteStorage: %v", err)
		}
		t.Cleanup(func() { _ = s.Close() })

		ctx := context.Background()

		// AddLog re-detects the project from cwd, so the task must carry
		// the detected ID or the scoped WHERE clause won't match.
		proj := core.DetectProject()
		var projPtr *string
		if proj != nil && proj.InProject && proj.ProjectID != "" {
			pid := proj.ProjectID
			projPtr = &pid
		}

		task := &core.Task{
			ID:        core.NewTaskID(),
			Title:     "ordering fixture",
			Status:    core.StatusTodo,
			ProjectID: projPtr,
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		for _, action := range actions {
			entry := &core.LogEntry{
				TaskID:    task.ID,
				Timestamp: ts,
				By:        "noor",
				Action:    action,
				Note:      "note for " + action,
			}
			if err := s.AddLog(ctx, entry); err != nil {
				t.Fatalf("AddLog(%s): %v", action, err)
			}
		}
		return s, ctx, task.ID
	}

	newest := actions[len(actions)-1]
	oldest := actions[0]

	t.Run("GetLogsDescNewestFirst", func(t *testing.T) {
		s, ctx, taskID := newFixture(t)

		logs, err := s.GetLogs(ctx, taskID, "desc")
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		if len(logs) != len(actions) {
			t.Fatalf("got %d logs; want %d", len(logs), len(actions))
		}
		// This is what --amend reads with LIMIT 1.
		if logs[0].Action != newest {
			t.Errorf("desc logs[0].Action = %q; want %q (newest row must win the timestamp tie)", logs[0].Action, newest)
		}
		if !descendingIDs(logs) {
			t.Errorf("desc log IDs not monotonically decreasing: %v", logIDs(logs))
		}
	})

	t.Run("GetLogsAscOldestFirst", func(t *testing.T) {
		s, ctx, taskID := newFixture(t)

		logs, err := s.GetLogs(ctx, taskID, "asc")
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		if len(logs) != len(actions) {
			t.Fatalf("got %d logs; want %d", len(logs), len(actions))
		}
		if logs[0].Action != oldest {
			t.Errorf("asc logs[0].Action = %q; want %q", logs[0].Action, oldest)
		}
		if !ascendingIDs(logs) {
			t.Errorf("asc log IDs not monotonically increasing: %v", logIDs(logs))
		}
	})

	t.Run("ListLogsDescLimit1IsNewest", func(t *testing.T) {
		// The exact query shape amendLatestLogNote issues.
		s, ctx, taskID := newFixture(t)

		logs, err := s.ListLogs(ctx, core.LogQuery{
			TaskID:        taskID,
			Limit:         1,
			SortDirection: "desc",
		})
		if err != nil {
			t.Fatalf("ListLogs: %v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("got %d logs; want 1", len(logs))
		}
		if logs[0].Action != newest {
			t.Errorf("ListLogs desc limit 1 returned %q; want %q — --amend would rewrite the wrong row", logs[0].Action, newest)
		}
	})

	t.Run("ListLogsAscOldestFirst", func(t *testing.T) {
		s, ctx, taskID := newFixture(t)

		logs, err := s.ListLogs(ctx, core.LogQuery{
			TaskID:        taskID,
			SortDirection: "asc",
		})
		if err != nil {
			t.Fatalf("ListLogs: %v", err)
		}
		if len(logs) != len(actions) {
			t.Fatalf("got %d logs; want %d", len(logs), len(actions))
		}
		if logs[0].Action != oldest {
			t.Errorf("asc logs[0].Action = %q; want %q", logs[0].Action, oldest)
		}
		if !ascendingIDs(logs) {
			t.Errorf("asc log IDs not monotonically increasing: %v", logIDs(logs))
		}
	})
}

func logIDs(logs []*core.LogEntry) []int64 {
	ids := make([]int64, 0, len(logs))
	for _, l := range logs {
		ids = append(ids, l.ID)
	}
	return ids
}

func ascendingIDs(logs []*core.LogEntry) bool {
	for i := 1; i < len(logs); i++ {
		if logs[i].ID <= logs[i-1].ID {
			return false
		}
	}
	return true
}

func descendingIDs(logs []*core.LogEntry) bool {
	for i := 1; i < len(logs); i++ {
		if logs[i].ID >= logs[i-1].ID {
			return false
		}
	}
	return true
}
