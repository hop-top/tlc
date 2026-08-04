package storage

import (
	"context"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// tiedLogTimestamp is the single timestamp every fixture row shares: the
// tie is the whole point of these tests, and hardcoding it removes any
// dependence on how fast the test runs.
var tiedLogTimestamp = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

// tiedLogActions is the fixture's insertion order, oldest first. Actions
// are distinct so assertions can name the row that should have won.
var tiedLogActions = []string{"CREATED", "IN_PROGRESS", "UPDATED", "DONE"}

// newTiedLogFixture builds an in-memory store holding one task whose log
// rows all carry tiedLogTimestamp, and returns the task's ID.
func newTiedLogFixture(t *testing.T) (*SQLiteStorage, context.Context, string) {
	t.Helper()
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()

	// AddLog re-detects the project from cwd, so the task must carry the
	// detected ID or the scoped WHERE clause won't match.
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
		CreatedAt: tiedLogTimestamp,
		UpdatedAt: tiedLogTimestamp,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	for _, action := range tiedLogActions {
		entry := &core.LogEntry{
			TaskID:    task.ID,
			Timestamp: tiedLogTimestamp,
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
	newest := tiedLogActions[len(tiedLogActions)-1]
	oldest := tiedLogActions[0]

	t.Run("GetLogsDescNewestFirst", func(t *testing.T) {
		s, ctx, taskID := newTiedLogFixture(t)
		logs, err := s.GetLogs(ctx, taskID, "desc")
		requireLogs(t, "GetLogs", logs, err, len(tiedLogActions))
		// This is the read --amend narrows to with LIMIT 1.
		assertFirstAction(t, logs, newest, "desc: newest row must win the timestamp tie")
		assertIDOrder(t, logs, false)
	})

	t.Run("GetLogsAscOldestFirst", func(t *testing.T) {
		s, ctx, taskID := newTiedLogFixture(t)
		logs, err := s.GetLogs(ctx, taskID, "asc")
		requireLogs(t, "GetLogs", logs, err, len(tiedLogActions))
		assertFirstAction(t, logs, oldest, "asc: oldest row first")
		assertIDOrder(t, logs, true)
	})

	t.Run("ListLogsDescLimit1IsNewest", func(t *testing.T) {
		// The exact query shape amendLatestLogNote issues.
		s, ctx, taskID := newTiedLogFixture(t)
		logs, err := s.ListLogs(ctx, core.LogQuery{
			TaskID:        taskID,
			Limit:         1,
			SortDirection: "desc",
		})
		requireLogs(t, "ListLogs", logs, err, 1)
		assertFirstAction(t, logs, newest, "desc limit 1: --amend would rewrite the wrong row")
	})

	t.Run("ListLogsAscOldestFirst", func(t *testing.T) {
		s, ctx, taskID := newTiedLogFixture(t)
		logs, err := s.ListLogs(ctx, core.LogQuery{
			TaskID:        taskID,
			SortDirection: "asc",
		})
		requireLogs(t, "ListLogs", logs, err, len(tiedLogActions))
		assertFirstAction(t, logs, oldest, "asc: oldest row first")
		assertIDOrder(t, logs, true)
	})
}

// requireLogs fails the test unless the read succeeded and returned
// exactly want rows.
func requireLogs(t *testing.T, op string, logs []*core.LogEntry, err error, want int) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	if len(logs) != want {
		t.Fatalf("%s: got %d logs; want %d", op, len(logs), want)
	}
}

// assertFirstAction checks which row won the ordering.
func assertFirstAction(t *testing.T, logs []*core.LogEntry, want, why string) {
	t.Helper()
	if logs[0].Action != want {
		t.Errorf("logs[0].Action = %q; want %q (%s)", logs[0].Action, want, why)
	}
}

// assertIDOrder checks ids move monotonically in the expected direction,
// proving the tiebreak applied to every row rather than just the first.
func assertIDOrder(t *testing.T, logs []*core.LogEntry, ascending bool) {
	t.Helper()
	ids := make([]int64, 0, len(logs))
	for _, l := range logs {
		ids = append(ids, l.ID)
	}
	for i := 1; i < len(ids); i++ {
		ordered := ids[i] > ids[i-1]
		if !ascending {
			ordered = ids[i] < ids[i-1]
		}
		if !ordered {
			direction := "increasing"
			if !ascending {
				direction = "decreasing"
			}
			t.Errorf("log IDs not monotonically %s: %v", direction, ids)
			return
		}
	}
}
