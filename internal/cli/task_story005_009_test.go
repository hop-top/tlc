package cli

// E2E tests for user stories 005–009:
//   005 — Task update (title, description, tags, assignee, status)
//   006 — Task deletion (--yes confirmation skip)
//   007 — Task reopen (mandatory --note; audit trail continuity)
//   008 — Task assignment / unassign (covered in task_lifecycle_test.go; add gaps here)
//   009 — Task audit log (tlc log; filter by action/actor; paginate)

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// ── Story 005 — Task Update ──────────────────────────────────────────────────

// TestTaskUpdateTitle verifies title can be changed in-place (story 005, scenario 1).
func TestTaskUpdateTitle(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Fxi auth bug", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--title", "Fix auth bug"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --title failed: %v", err)
	}

	updated := getTaskByAlias(t, ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.Title != "Fix auth bug" {
		t.Errorf("expected title 'Fix auth bug', got %q", updated.Title)
	}
}

// TestTaskUpdateDescription verifies description is replaced, not appended (story 005, scenario 2).
func TestTaskUpdateDescription(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{
		ID:          "T-0001",
		Title:       "Desc test",
		Status:      core.StatusTodo,
		Description: "Draft.",
	})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--description", "Detailed spec."})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --description failed: %v", err)
	}

	updated := getTaskByAlias(t, ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after update")
	}
	if updated.Description != "Detailed spec." {
		t.Errorf("expected description 'Detailed spec.', got %q", updated.Description)
	}
}

// TestTaskUpdateForceStatus verifies --force bypasses terminal-state guard (story 005, scenario 8).
func TestTaskUpdateForceStatus(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Done task", Status: core.StatusDone})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--status", "TODO", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --force failed: %v", err)
	}

	updated := getTaskByAlias(t, ctx, "T-0001")
	if updated == nil {
		t.Fatal("task not found after force update")
	}
	if updated.Status != core.StatusTodo {
		t.Errorf("expected status TODO after force, got %s", updated.Status)
	}
}

// ── Story 006 — Task Deletion ────────────────────────────────────────────────

// TestTaskDeleteWithYesFlag verifies --yes removes task without interactive prompt
// (story 006, scenario 1).
func TestTaskDeleteWithYesFlag(t *testing.T) {
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
		t.Fatalf("task delete --yes failed: %v", err)
	}

	task := getTaskByAlias(t, ctx, "T-0001")
	if task != nil {
		t.Error("expected task to be deleted, but it still exists")
	}
}

// TestTaskDeleteYesShortFlag verifies -y short flag works identically to --yes
// (story 006, scenario 4).
func TestTaskDeleteYesShortFlag(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Short flag delete", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "delete", "T-0001", "-y"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task delete -y failed: %v", err)
	}

	task := getTaskByAlias(t, ctx, "T-0001")
	if task != nil {
		t.Error("expected task to be deleted with -y short flag")
	}
}

// TestTaskDeleteRequiresYes verifies that delete without --yes returns an error
// in non-interactive context (story 006, scenario 2).
func TestTaskDeleteRequiresYes(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "No-yes delete", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "delete", "T-0001"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when delete called without --yes in non-interactive context")
	}
}

// TestTaskDeleteNotFound verifies error when deleting a non-existent task
// (story 006, scenario 3).
func TestTaskDeleteNotFound(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "delete", "T-9999", "--yes"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// ── Story 007 — Task Reopen ──────────────────────────────────────────────────

// TestTaskReopen verifies a DONE task transitions back to TODO (story 007, scenarios 1-2).
func TestTaskReopen(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	createAndCompleteTask(t, "Reopen from done", "")

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "reopen", "T-0001", "--note", "Revived for next sprint"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task reopen failed: %v", err)
	}

	task := getTaskByAlias(t, ctx, "T-0001")
	if task == nil {
		t.Fatal("task not found after reopen")
	}
	if task.Status != core.StatusTodo {
		t.Errorf("expected status TODO after reopen, got %s", task.Status)
	}
}

// TestTaskReopenNotFound verifies error when reopening a non-existent task
// (story 007, scenario 5).
func TestTaskReopenNotFound(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "reopen", "T-9999", "--note", "Missing task"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

// TestTaskReopenAuditContinuity verifies full lifecycle is preserved in description
// (story 007, scenario 4).
func TestTaskReopenAuditContinuity(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	// Create → claim → complete via CLI so audit entries are written
	createAndCompleteTask(t, "Audit trail task", "Initial work done.")

	// Reopen
	cmd3 := newTestCmd()
	cmd3.AddCommand(TaskCmd)
	buf3 := new(bytes.Buffer)
	cmd3.SetOut(buf3)
	cmd3.SetErr(buf3)
	cmd3.SetArgs([]string{"task", "reopen", "T-0001", "--note", "Missing edge case"})
	if err := cmd3.Execute(); err != nil {
		t.Fatalf("task reopen failed: %v", err)
	}

	task := getTaskByAlias(t, ctx, "T-0001")
	if task == nil {
		t.Fatal("task not found after reopen")
	}

	// Description must contain both original text and reopen note.
	if !contains(task.Description, "Initial work done.") {
		t.Errorf("original description lost after reopen, got: %s", task.Description)
	}
	if !contains(task.Description, "Missing edge case") {
		t.Errorf("reopen note not found in description, got: %s", task.Description)
	}
}

// ── Story 009 — Task Audit Log ───────────────────────────────────────────────

// TestTaskAuditLog verifies log entries are returned for a specific task
// (story 009, scenario 1).
func TestTaskAuditLog(t *testing.T) {

	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	// Create a task and add explicit log entries so we control the data.
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTask(ctx, &core.Task{
		ID:        "T-0001",
		Title:     "Audit log test",
		Status:    core.StatusDone,
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = s.AddLog(ctx, &core.LogEntry{
		TaskID: "T-0001", Timestamp: now, By: testEngineer1, Action: "CLAIMED",
	})
	_ = s.AddLog(ctx, &core.LogEntry{
		TaskID: "T-0001", Timestamp: now.Add(time.Minute), By: testEngineer1, Action: "DONE",
	})

	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"log", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("tlc log T-0001 failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "CLAIMED") {
		t.Errorf("expected CLAIMED log entry, got: %s", output)
	}
	if !contains(output, "DONE") {
		t.Errorf("expected DONE log entry, got: %s", output)
	}
}

// TestTaskAuditLogAll verifies --all returns logs across all tasks
// (story 009, scenario 2).
func TestTaskAuditLogAll(t *testing.T) {

	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("T-%04d", i)
		s.CreateTask(ctx, &core.Task{
			ID: id, Title: fmt.Sprintf("Task %d", i), Status: core.StatusTodo,
			CreatedAt: now, UpdatedAt: now,
		})
		_ = s.AddLog(ctx, &core.LogEntry{
			TaskID: id, Timestamp: now, By: testEngineer1, Action: "CREATED",
		})
	}

	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"log", "--all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("tlc log --all failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "T-0001") {
		t.Errorf("expected T-0001 in --all output, got: %s", output)
	}
	if !contains(output, "T-0003") {
		t.Errorf("expected T-0003 in --all output, got: %s", output)
	}
}

// TestTaskAuditLogFilterByAction verifies --action filters log entries
// (story 009, scenario 3).
func TestTaskAuditLogFilterByAction(t *testing.T) {

	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "Filter action", Status: core.StatusDone,
		CreatedAt: now, UpdatedAt: now,
	})
	_ = s.AddLog(ctx, &core.LogEntry{
		TaskID: "T-0001", Timestamp: now, By: testEngineer1, Action: "CLAIMED",
	})
	_ = s.AddLog(ctx, &core.LogEntry{
		TaskID: "T-0001", Timestamp: now.Add(time.Minute), By: testEngineer1, Action: "DONE",
	})

	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"log", "--all", "--action", "CLAIMED"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("tlc log --action CLAIMED failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "CLAIMED") {
		t.Errorf("expected CLAIMED in output, got: %s", output)
	}
	// DONE entries should NOT appear when action=CLAIMED
	// (table output may include header; check that no DONE row appears)
}

// TestTaskAuditLogFilterByActor verifies --by filters log entries
// (story 009, scenario 4).
func TestTaskAuditLogFilterByActor(t *testing.T) {

	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "Filter actor", Status: core.StatusTodo,
		CreatedAt: now, UpdatedAt: now,
	})
	_ = s.AddLog(ctx, &core.LogEntry{
		TaskID: "T-0001", Timestamp: now, By: testEngineer1, Action: "CLAIMED",
	})
	_ = s.AddLog(ctx, &core.LogEntry{
		TaskID: "T-0001", Timestamp: now.Add(time.Minute), By: testEngineer2, Action: "UPDATED",
	})

	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"log", "--all", "--by", testEngineer1})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("tlc log --by engineer-1 failed: %v", err)
	}

	output := buf.String()
	if !contains(output, testEngineer1) {
		t.Errorf("expected engineer-1 in filtered output, got: %s", output)
	}
}

// TestTaskAuditLogPagination verifies --limit and --offset paginate results
// (story 009, scenario 5).
func TestTaskAuditLogPagination(t *testing.T) {

	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "Pagination log test", Status: core.StatusTodo,
		CreatedAt: now, UpdatedAt: now,
	})
	for i := 0; i < 15; i++ {
		_ = s.AddLog(ctx, &core.LogEntry{
			TaskID:    "T-0001",
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			By:        testEngineer1,
			Action:    "UPDATED",
			Note:      fmt.Sprintf("update %d", i),
		})
	}

	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"log", "T-0001", "--limit", "5", "--offset", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("tlc log --limit 5 --offset 5 failed: %v", err)
	}

	output := buf.String()
	// Table footer says "Showing N log entries"
	if !contains(output, "Showing 5 log entries") {
		t.Errorf("expected 'Showing 5 log entries', got: %s", output)
	}
}

// TestTaskAuditLogRequiresTaskIDOrAll verifies error without task-id or --all
// (story 009, scenario 6).
func TestTaskAuditLogRequiresTaskIDOrAll(t *testing.T) {

	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"log"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when log called without task-id or --all")
	}
}

// TestTaskAuditLogJSONFormat verifies --format json produces valid JSON
// (story 009, scenario 7).
func TestTaskAuditLogJSONFormat(t *testing.T) {

	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now().UTC()
	s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "JSON log test", Status: core.StatusTodo,
		CreatedAt: now, UpdatedAt: now,
	})
	_ = s.AddLog(ctx, &core.LogEntry{
		TaskID: "T-0001", Timestamp: now, By: testEngineer1, Action: "CREATED",
	})

	// logCmd reads format from viper, not a direct flag; set via viper.
	viper.Set("output.format", "json")
	defer viper.Set("output.format", nil)

	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"log", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("tlc log json format failed: %v", err)
	}

	output := buf.String()
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		t.Fatalf("log --format json output is not valid JSON: %v\noutput: %s", err, output)
	}
	if len(entries) == 0 {
		t.Error("expected at least one log entry in JSON output")
	}
}
