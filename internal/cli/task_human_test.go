// Tests for `tlc task approve|reject`: the decisions on human-kind tasks
// that `track execute` waits on.
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

func humanTestTask(id string, kind core.TaskKind) *core.Task {
	now := time.Now().UTC()
	return &core.Task{
		ID: id, Title: "sign-off " + id, Status: core.StatusTodo,
		Kind:      kind,
		Spec:      &core.TaskSpec{Human: &core.HumanSpec{Assignee: "@lead"}},
		RunID:     "run_1",
		StepID:    id,
		CreatedAt: now, UpdatedAt: now,
	}
}

func runTaskHumanCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(append([]string{"task"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

func taskActions(t *testing.T, ctx context.Context, s *storage.SQLiteStorage, id string) []string {
	t.Helper()
	logs, err := s.GetTaskLogs(ctx, id)
	if err != nil {
		t.Fatalf("GetTaskLogs: %v", err)
	}
	out := make([]string, 0, len(logs))
	for _, l := range logs {
		out = append(out, l.Action)
	}
	return out
}

func TestTaskApprove_CompletesHumanTask(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		defer resetTaskFlags()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		if err := s.CreateTask(ctx, humanTestTask("task_h1", core.TaskKindHuman)); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		out, err := runTaskHumanCmd(t, "approve", "task_h1", "--note", "lgtm", "--by", "lead")
		if err != nil {
			t.Fatalf("task approve: %v\n%s", err, out)
		}

		got := mustGetTask(t, ctx, s, "task_h1")
		if got.Status != core.StatusDone {
			t.Errorf("status = %q; want DONE (TODO→DONE is forced for a human decision)", got.Status)
		}
		actions := taskActions(t, ctx, s, "task_h1")
		if !strings.Contains(strings.Join(actions, ","), core.ActionApproved) {
			t.Errorf("log actions = %v; want APPROVED", actions)
		}
		logs, _ := s.GetTaskLogs(ctx, "task_h1")
		var approved *core.LogEntry
		for _, l := range logs {
			if l.Action == core.ActionApproved {
				approved = l
			}
		}
		if approved == nil || approved.By != "lead" || !strings.Contains(approved.Note, "lgtm") {
			t.Errorf("APPROVED entry = %+v; want by lead with the note", approved)
		}
		if !strings.Contains(out, "Approved") {
			t.Errorf("output = %q; want an Approved line", out)
		}
	})
}

func TestTaskApprove_RefusesNonHumanTask(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		defer resetTaskFlags()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		if err := s.CreateTask(ctx, humanTestTask("task_a1", core.TaskKindAgent)); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		out, err := runTaskHumanCmd(t, "approve", "task_a1")
		if err == nil {
			t.Fatalf("approve on an agent task succeeded; want error\n%s", out)
		}
		if !strings.Contains(err.Error(), "agent") || !strings.Contains(err.Error(), "task_a1") {
			t.Errorf("error = %v; want it to name the task and its kind", err)
		}
		if got := mustGetTask(t, ctx, s, "task_a1"); got.Status != core.StatusTodo {
			t.Errorf("status changed to %q", got.Status)
		}
	})
}

func TestTaskReject_BlocksWithReason(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		defer resetTaskFlags()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()
		if err := s.CreateTask(ctx, humanTestTask("task_h2", core.TaskKindHuman)); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		if out, err := runTaskHumanCmd(t, "reject", "task_h2"); err == nil {
			t.Fatalf("reject without --reason succeeded; want error\n%s", out)
		}

		out, err := runTaskHumanCmd(t, "reject", "task_h2", "--reason", "scope creep")
		if err != nil {
			t.Fatalf("task reject: %v\n%s", err, out)
		}
		got := mustGetTask(t, ctx, s, "task_h2")
		if got.Status != core.StatusTodo || got.BlockedReason == nil || !strings.Contains(*got.BlockedReason, "scope creep") {
			t.Errorf("task = %q / %v; want TODO blocked with the reason", got.Status, got.BlockedReason)
		}
		if actions := taskActions(t, ctx, s, "task_h2"); !strings.Contains(strings.Join(actions, ","), core.ActionRejected) {
			t.Errorf("log actions = %v; want REJECTED", actions)
		}
	})
}

// TestTaskApprove_CompletesSubjectWhenLastLeaf: approving the last open
// leaf of a run completes the run's subject, the same rule the executor
// applies when it completes a task itself.
func TestTaskApprove_CompletesSubjectWhenLastLeaf(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		defer resetTaskFlags()
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		subject := &core.Task{ID: "task_subject", Title: "subject", Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now}
		leaf := humanTestTask("task_leaf", core.TaskKindHuman)
		leaf.SetProvenance(core.TaskProvenance{Recipe: "code-review", RecipeVersion: "1.2.0", Subject: "task_subject"})
		for _, task := range []*core.Task{subject, leaf} {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("CreateTask %s: %v", task.ID, err)
			}
		}
		if err := s.CreateRecipeRun(ctx, &core.RecipeRun{ID: "run_1", RecipeID: "code-review", Version: "1.2.0", CreatedAt: now}); err != nil {
			t.Fatalf("CreateRecipeRun: %v", err)
		}
		if err := s.AddRecipeRunTasks(ctx, "run_1", []core.RecipeRunTask{{StepID: "task_leaf", TaskID: "task_leaf"}}); err != nil {
			t.Fatalf("AddRecipeRunTasks: %v", err)
		}

		if out, err := runTaskHumanCmd(t, "approve", "task_leaf", "--note", "ok"); err != nil {
			t.Fatalf("task approve: %v\n%s", err, out)
		}
		if got := mustGetTask(t, ctx, s, "task_subject"); got.Status != core.StatusDone {
			t.Errorf("subject = %q; want DONE after its last leaf was approved", got.Status)
		}
	})
}
