package inbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// newTestEnv sets up a temp directory with inbox subdirs, a SQLite
// storage, TaskService, and a FileInboxProcessor. Changes CWD to
// the temp dir so project detection does not find a git repo.
// Cleaned up via t.Cleanup.
func newTestEnv(t *testing.T) (
	*FileInboxProcessor,
	*core.TaskService,
	*storage.SQLiteStorage,
	string, // inboxDir
) {
	t.Helper()
	tmp := t.TempDir()

	// Move CWD out of git repo so DetectProject returns nil.
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	core.ResetDetectionCache()
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
		core.ResetDetectionCache()
	})

	dbPath := filepath.Join(tmp, "test.db")
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	svc := core.NewTaskService(s, s)
	inboxDir := filepath.Join(tmp, "inbox")

	proc := NewFileInboxProcessor(inboxDir, svc, "")
	return proc, svc, s, inboxDir
}

// writeFile is a test helper that writes data to a file inside dir.
func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, name), data, 0o644,
	); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestCreateJSON(t *testing.T) {
	proc, svc, _, inboxDir := newTestEnv(t)
	ctx := context.Background()

	payload, _ := json.Marshal(map[string]interface{}{
		"title":       "Test task from JSON",
		"description": "A task created via inbox",
		"tags":        []string{"inbox", "test"},
		"priority":    "P1",
	})
	writeFile(t, filepath.Join(inboxDir, "create"), "01.json", payload)

	result, err := proc.Process(ctx)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(result.Failed) != 0 {
		for _, f := range result.Failed {
			sidecar := filepath.Join(inboxDir, "failed", f+".error")
			data, _ := os.ReadFile(sidecar)
			t.Logf("failed %s: %s", f, string(data))
		}
		t.Fatalf("expected 0 failed, got %d", len(result.Failed))
	}
	if len(result.Created) != 1 {
		t.Fatalf("expected 1 created, got %d", len(result.Created))
	}

	// Verify task exists in storage.
	tasks, err := svc.ListTasks(ctx, core.Query{})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "Test task from JSON" {
		t.Errorf("title = %q, want %q",
			tasks[0].Title, "Test task from JSON")
	}

	// Verify file moved to processed/.
	processed := filepath.Join(inboxDir, "processed", "01.json")
	if _, err := os.Stat(processed); os.IsNotExist(err) {
		t.Error("expected file in processed/")
	}
}

func TestCreateMarkdown(t *testing.T) {
	proc, svc, _, inboxDir := newTestEnv(t)
	ctx := context.Background()

	md := []byte(`---
title: MD task
priority: P2
---
This is the body.
`)
	writeFile(t, filepath.Join(inboxDir, "create"), "02.md", md)

	result, err := proc.Process(ctx)
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(result.Created) != 1 {
		t.Fatalf("expected 1 created, got %d", len(result.Created))
	}

	tasks, _ := svc.ListTasks(ctx, core.Query{})
	if tasks[0].Description != "This is the body." {
		t.Errorf("description = %q", tasks[0].Description)
	}
}

func TestCreateFailure_InvalidJSON(t *testing.T) {
	proc, _, _, inboxDir := newTestEnv(t)
	ctx := context.Background()

	writeFile(
		t,
		filepath.Join(inboxDir, "create"),
		"bad.json",
		[]byte(`{not valid json`),
	)

	result, err := proc.Process(ctx)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(result.Failed) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(result.Failed))
	}
	if len(result.Created) != 0 {
		t.Fatalf("expected 0 created, got %d", len(result.Created))
	}

	// Verify file in failed/.
	failed := filepath.Join(inboxDir, "failed", "bad.json")
	if _, err := os.Stat(failed); os.IsNotExist(err) {
		t.Error("expected file in failed/")
	}

	// Verify .error sidecar.
	sidecar := filepath.Join(inboxDir, "failed", "bad.json.error")
	if _, err := os.Stat(sidecar); os.IsNotExist(err) {
		t.Error("expected .error sidecar in failed/")
	}
}

func TestTransitionFlow(t *testing.T) {
	proc, svc, s, inboxDir := newTestEnv(t)
	ctx := context.Background()

	// Create a task first.
	task := &core.Task{
		ID:     "T-0099",
		Title:  "Existing task",
		Status: core.StatusTodo,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{
		"id":     "T-0099",
		"status": "IN_PROGRESS",
		"by":     "bot",
		"note":   "auto transition",
	})
	writeFile(
		t,
		filepath.Join(inboxDir, "transition"),
		"move.json",
		payload,
	)

	result, err := proc.Process(ctx)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(result.Transitioned) != 1 {
		t.Fatalf("expected 1 transitioned, got %d",
			len(result.Transitioned))
	}

	// Verify status changed.
	tasks, _ := svc.ListTasks(ctx, core.Query{})
	found := false
	for _, tk := range tasks {
		if tk.ID == "T-0099" {
			found = true
			if tk.Status != core.StatusInProgress {
				t.Errorf("status = %s, want IN_PROGRESS",
					tk.Status)
			}
		}
	}
	if !found {
		t.Error("task T-0099 not found after transition")
	}

	// Verify file moved to processed/.
	processed := filepath.Join(
		inboxDir, "processed", "move.json",
	)
	if _, err := os.Stat(processed); os.IsNotExist(err) {
		t.Error("expected file in processed/")
	}
}

func TestTransitionFailure_InvalidStatus(t *testing.T) {
	proc, _, s, inboxDir := newTestEnv(t)
	ctx := context.Background()

	// Create a task in DONE status.
	task := &core.Task{
		ID:     "T-0050",
		Title:  "Done task",
		Status: core.StatusDone,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Try invalid transition: DONE -> TODO (not allowed by state
	// machine).
	payload, _ := json.Marshal(map[string]string{
		"id":     "T-0050",
		"status": "TODO",
		"by":     "bot",
	})
	writeFile(
		t,
		filepath.Join(inboxDir, "transition"),
		"bad-transition.json",
		payload,
	)

	result, err := proc.Process(ctx)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(result.Failed) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(result.Failed))
	}
	if len(result.Transitioned) != 0 {
		t.Fatalf("expected 0 transitioned, got %d",
			len(result.Transitioned))
	}

	// Verify failed/ and .error sidecar.
	failed := filepath.Join(
		inboxDir, "failed", "bad-transition.json",
	)
	if _, err := os.Stat(failed); os.IsNotExist(err) {
		t.Error("expected file in failed/")
	}
	sidecar := filepath.Join(
		inboxDir, "failed", "bad-transition.json.error",
	)
	if _, err := os.Stat(sidecar); os.IsNotExist(err) {
		t.Error("expected .error sidecar")
	}
}

func TestOrdering_CreatesBeforeTransitions(t *testing.T) {
	proc, svc, _, inboxDir := newTestEnv(t)
	ctx := context.Background()

	// Write two create files in reverse-alpha order.
	p1, _ := json.Marshal(map[string]string{
		"title": "Second task",
	})
	p2, _ := json.Marshal(map[string]string{
		"title": "First task",
	})
	writeFile(
		t,
		filepath.Join(inboxDir, "create"), "b.json", p1,
	)
	writeFile(
		t,
		filepath.Join(inboxDir, "create"), "a.json", p2,
	)

	// Write a transition that targets the first-created task.
	// The create must run before the transition for this to work.
	tr, _ := json.Marshal(map[string]string{
		"id":     "T-0001",
		"status": "IN_PROGRESS",
		"by":     "bot",
	})
	writeFile(
		t,
		filepath.Join(inboxDir, "transition"), "t.json", tr,
	)

	result, err := proc.Process(ctx)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(result.Created) != 2 {
		t.Fatalf("expected 2 created, got %d", len(result.Created))
	}
	if len(result.Transitioned) != 1 {
		t.Fatalf("expected 1 transitioned, got %d",
			len(result.Transitioned))
	}

	// a.json should produce T-0001, b.json T-0002 (lex order).
	tasks, _ := svc.ListTasks(ctx, core.Query{})
	for _, tk := range tasks {
		if tk.ID == "T-0001" && tk.Title != "First task" {
			t.Errorf("T-0001 title = %q, want First task",
				tk.Title)
		}
	}
}

func TestEmptyInbox(t *testing.T) {
	proc, _, _, _ := newTestEnv(t)
	ctx := context.Background()

	result, err := proc.Process(ctx)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(result.Created) != 0 {
		t.Errorf("expected 0 created, got %d", len(result.Created))
	}
	if len(result.Transitioned) != 0 {
		t.Errorf("expected 0 transitioned, got %d",
			len(result.Transitioned))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

// TestTagPolicyRejectsInboxCreate covers the inbox write path, which has
// no CLI surface of its own.
//
// The inbox already has exactly the right shape of failure for a policy
// violation: the offending file goes to failed/ with the reason in its
// sidecar, so the user gets the message naming the tag and the allowed
// set, and the other files in the batch still process. That last part is
// what this asserts alongside the rejection.
func TestTagPolicyRejectsInboxCreate(t *testing.T) {
	proc, _, store, inboxDir := newTestEnv(t)
	ctx := context.Background()

	// Restored to nil rather than to a captured previous value: core
	// exposes a setter and no getter, and this package registers no
	// provider of its own, so nil IS the state to return to.
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{
			Statuses:     config.GetDefaultStatuses(),
			StateMachine: config.GetDefaultStateMachine(),
			Tags: config.TagsConfig{
				Policy:  config.TagPolicyClosed,
				Allowed: []string{"storage"},
			},
		}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})

	bad, _ := json.Marshal(map[string]any{
		"title": "rejected", "tags": []string{"bogustag"},
	})
	writeFile(t, filepath.Join(inboxDir, "create"), "01-bad.json", bad)

	good, _ := json.Marshal(map[string]any{
		"title": "accepted", "tags": []string{"storage", "type:feat"},
	})
	writeFile(t, filepath.Join(inboxDir, "create"), "02-good.json", good)

	result, err := proc.Process(ctx)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(result.Failed) != 1 || result.Failed[0] != "01-bad.json" {
		t.Fatalf("failed = %v, want [01-bad.json]", result.Failed)
	}
	// The other file in the batch must still have processed: one bad tag
	// is a bad FILE, not a bad run.
	if len(result.Created) != 1 {
		t.Fatalf("created = %v, want exactly the good file's task", result.Created)
	}

	sidecar := filepath.Join(inboxDir, "failed", "01-bad.json.error")
	data, readErr := os.ReadFile(sidecar)
	if readErr != nil {
		t.Fatalf("read failure sidecar: %v", readErr)
	}
	for _, want := range []string{"bogustag", "storage"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("sidecar should name %q, got: %s", want, data)
		}
	}

	// And the accepted task really carries its tags.
	task, getErr := store.GetTask(ctx, result.Created[0])
	if getErr != nil || task == nil {
		t.Fatalf("get created task: %v", getErr)
	}
	if len(task.Tags) != 2 {
		t.Errorf("created task tags = %v, want both allowed tags", task.Tags)
	}
}
