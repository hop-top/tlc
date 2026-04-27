package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

// fakeStorage combines Repository + LogRepository for saveTaskWithLog.
// Tracks calls to verify that local commits happen exactly once and
// before any sync push.
type fakeStorage struct {
	*core.MockRepository
	*core.MockLogRepository
	updateWithLogCalls int
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		MockRepository:    core.NewMockRepository(),
		MockLogRepository: core.NewMockLogRepository(),
	}
}

func (f *fakeStorage) UpdateTaskWithLog(ctx context.Context, task *core.Task, entry *core.LogEntry) error {
	f.updateWithLogCalls++
	if err := f.MockRepository.UpdateTaskWithLog(ctx, task, entry); err != nil {
		return err
	}
	if entry != nil {
		return f.MockLogRepository.AddLog(ctx, entry)
	}
	return nil
}

// withFakeSyncPush swaps syncPushFn for the duration of the test.
func withFakeSyncPush(t *testing.T, fn func(ctx context.Context, system string, task *core.Task) error) {
	t.Helper()
	prev := syncPushFn
	syncPushFn = fn
	t.Cleanup(func() { syncPushFn = prev })
}

func newSyncedTaskFixture(id string) *core.Task {
	originSystem := "github"
	projectID := "hop-top/tlc"
	return &core.Task{
		ID:           id,
		Title:        "test task",
		Description:  "initial body",
		Status:       core.TaskStatus("IN_PROGRESS"),
		OriginSystem: &originSystem,
		ProjectID:    &projectID,
		Meta:         map[string]interface{}{"origin_id": "1"},
	}
}

// TestSaveTaskWithLog_LocalCommitsBeforeSyncPush asserts that the
// audit log entry and the task row update are both written to local
// storage atomically before any sync push is attempted. This is the
// invariant that prevents the "audit advanced, status row didn't"
// drift pattern of T-0750.
func TestSaveTaskWithLog_LocalCommitsBeforeSyncPush(t *testing.T) {
	ctx := context.Background()
	store := newFakeStorage()

	task := newSyncedTaskFixture("GH-1")
	// Seed the row.
	if err := store.MockRepository.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Simulate the in-memory mutations a transition would do.
	task.Status = core.TaskStatus("DONE")
	task.Description = "initial body\n\n---\nCOMPLETED (IN_PROGRESS → DONE)"
	logEntry := &core.LogEntry{
		TaskID:  task.ID,
		Action:  "DONE",
		By:      "tester",
		Note:    "Status changed from IN_PROGRESS to DONE: test",
	}

	pushAttempted := false
	withFakeSyncPush(t, func(_ context.Context, _ string, _ *core.Task) error {
		pushAttempted = true
		// At this moment the local commit MUST already have happened.
		stored, _ := store.MockRepository.GetTask(ctx, task.ID)
		if stored == nil || stored.Status != "DONE" {
			t.Fatalf("local commit must precede sync push; got status=%v", stored.Status)
		}
		return errors.New("simulated remote rejection: 422 Validation Failed")
	})

	cmd := &cobra.Command{}
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	err := saveTaskWithLog(ctx, cmd, task, logEntry, store)
	if err != nil {
		t.Fatalf("saveTaskWithLog returned error %v; sync failures must NOT roll back local truth", err)
	}
	if !pushAttempted {
		t.Fatalf("sync push must have been attempted")
	}

	// Local row reflects the new status.
	stored, _ := store.MockRepository.GetTask(ctx, task.ID)
	if stored == nil {
		t.Fatalf("task vanished from local storage")
	}
	if stored.Status != "DONE" {
		t.Fatalf("status drift: local row = %q, expected DONE", stored.Status)
	}

	// Exactly one audit entry — the new one we wrote.
	logs, _ := store.MockLogRepository.GetLogs(ctx, task.ID, "asc")
	if len(logs) != 1 {
		t.Fatalf("audit log entries: got %d, want 1", len(logs))
	}
	if logs[0].Action != "DONE" {
		t.Fatalf("audit entry action: got %q, want DONE", logs[0].Action)
	}

	// Atomic write was used exactly once.
	if store.updateWithLogCalls != 1 {
		t.Fatalf("UpdateTaskWithLog calls: got %d, want 1", store.updateWithLogCalls)
	}
}

// TestSaveTaskWithLog_NoForbiddenDriftState exhaustively rejects the
// state of "audit log advanced; status row didn't" — even when sync
// push fails repeatedly, the audit log and status row must always
// agree.
func TestSaveTaskWithLog_NoForbiddenDriftState(t *testing.T) {
	ctx := context.Background()
	store := newFakeStorage()

	task := newSyncedTaskFixture("GH-2")
	if err := store.MockRepository.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed: %v", err)
	}

	withFakeSyncPush(t, func(_ context.Context, _ string, _ *core.Task) error {
		return errors.New("simulated network error")
	})

	cmd := &cobra.Command{}
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	// Three transitions in a row, each with sync failure.
	transitions := []struct {
		status core.TaskStatus
		action string
	}{
		{core.TaskStatus("DONE"), "DONE"},
		{core.TaskStatus("TODO"), "REOPENED"},
		{core.TaskStatus("DONE"), "DONE"},
	}

	for i, tr := range transitions {
		task.Status = tr.status
		entry := &core.LogEntry{TaskID: task.ID, Action: tr.action, By: "tester", Note: "test"}
		if err := saveTaskWithLog(ctx, cmd, task, entry, store); err != nil {
			t.Fatalf("transition %d: unexpected error %v", i, err)
		}
		stored, _ := store.MockRepository.GetTask(ctx, task.ID)
		logs, _ := store.MockLogRepository.GetLogs(ctx, task.ID, "asc")

		// Forbidden state: row status doesn't match the most recent
		// log entry's action.
		if stored.Status != tr.status {
			t.Fatalf("transition %d: row status %q; expected %q",
				i, stored.Status, tr.status)
		}
		if len(logs) != i+1 {
			t.Fatalf("transition %d: log count %d; expected %d",
				i, len(logs), i+1)
		}
		if logs[len(logs)-1].Action != tr.action {
			t.Fatalf("transition %d: last log action %q; expected %q",
				i, logs[len(logs)-1].Action, tr.action)
		}
	}
}

// TestSaveTaskWithLog_NonSyncedTaskNoPush asserts that tasks without an
// origin system never trigger sync push.
func TestSaveTaskWithLog_NonSyncedTaskNoPush(t *testing.T) {
	ctx := context.Background()
	store := newFakeStorage()

	task := &core.Task{
		ID:     "T-9999",
		Title:  "local-only",
		Status: core.TaskStatus("TODO"),
	}
	if err := store.MockRepository.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed: %v", err)
	}

	pushed := false
	withFakeSyncPush(t, func(_ context.Context, _ string, _ *core.Task) error {
		pushed = true
		return nil
	})

	cmd := &cobra.Command{}
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	task.Status = core.TaskStatus("IN_PROGRESS")
	entry := &core.LogEntry{TaskID: task.ID, Action: "CLAIMED", By: "tester"}
	if err := saveTaskWithLog(ctx, cmd, task, entry, store); err != nil {
		t.Fatalf("saveTaskWithLog: %v", err)
	}
	if pushed {
		t.Fatalf("sync push attempted on local-only task")
	}
}

// TestSaveTaskWithLog_SyncSuccessSetsLastSyncAt asserts that on a
// successful push, the LastSyncAt timestamp is updated.
func TestSaveTaskWithLog_SyncSuccessSetsLastSyncAt(t *testing.T) {
	ctx := context.Background()
	store := newFakeStorage()

	task := newSyncedTaskFixture("GH-3")
	if err := store.MockRepository.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed: %v", err)
	}

	withFakeSyncPush(t, func(_ context.Context, _ string, _ *core.Task) error {
		return nil
	})

	cmd := &cobra.Command{}
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})

	task.Status = core.TaskStatus("DONE")
	entry := &core.LogEntry{TaskID: task.ID, Action: "DONE", By: "tester"}
	if err := saveTaskWithLog(ctx, cmd, task, entry, store); err != nil {
		t.Fatalf("saveTaskWithLog: %v", err)
	}

	stored, _ := store.MockRepository.GetTask(ctx, task.ID)
	if stored.LastSyncAt == nil {
		t.Fatalf("LastSyncAt not set after successful sync")
	}
}
