package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// writeInboxFile writes data to inboxDir/sub/name.
func writeInboxFile(
	t *testing.T, inboxDir, sub, name string, data []byte,
) {
	t.Helper()
	dir := filepath.Join(inboxDir, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, name), data, 0o644,
	); err != nil {
		t.Fatalf("write %s/%s: %v", sub, name, err)
	}
}

func TestInboxProcessE2E_CreateJSON(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cwd, _ := os.Getwd()
	inboxDir := filepath.Join(cwd, ".tlc", "inbox")

	payload, _ := json.Marshal(map[string]interface{}{
		"title":    "E2E inbox task",
		"tags":     []string{"inbox"},
		"priority": "P1",
	})
	writeInboxFile(t, inboxDir, "create", "01.json", payload)

	cmd := newTestCmd()
	cmd.AddCommand(InboxCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"inbox", "process"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("inbox process: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "1 created") {
		t.Errorf("expected '1 created' in output, got %q", out)
	}

	// Verify task exists.
	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(
		context.Background(), core.Query{Limit: 100},
	)
	found := false
	for _, tk := range tasks {
		if tk.Title == "E2E inbox task" {
			found = true
			if tk.Priority != "P1" {
				t.Errorf("priority = %s, want P1", tk.Priority)
			}
		}
	}
	if !found {
		t.Error("task 'E2E inbox task' not found")
	}

	// Verify file moved to processed/.
	processed := filepath.Join(inboxDir, "processed", "01.json")
	if _, err := os.Stat(processed); os.IsNotExist(err) {
		t.Error("expected file in processed/")
	}
}

func TestInboxProcessE2E_CreateMarkdown(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cwd, _ := os.Getwd()
	inboxDir := filepath.Join(cwd, ".tlc", "inbox")

	md := []byte("---\ntitle: MD inbox task\npriority: P2\n---\nBody text.\n")
	writeInboxFile(t, inboxDir, "create", "02.md", md)

	cmd := newTestCmd()
	cmd.AddCommand(InboxCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"inbox", "process"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("inbox process: %v", err)
	}

	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(
		context.Background(), core.Query{Limit: 100},
	)
	for _, tk := range tasks {
		if tk.Title == "MD inbox task" {
			if tk.Description != "Body text." {
				t.Errorf(
					"description = %q, want 'Body text.'",
					tk.Description,
				)
			}
			return
		}
	}
	t.Error("task 'MD inbox task' not found")
}

func TestInboxProcessE2E_Transition(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	// Create a task directly.
	s, _ := getStorageRaw()
	defer s.Close()
	svc := core.NewTaskService(s, s)
	id, _ := svc.NextTaskID(ctx, "")
	task := &core.Task{
		ID:     id,
		Title:  "To transition",
		Status: core.StatusTodo,
	}
	if err := svc.CreateTask(ctx, task, "test", ""); err != nil {
		t.Fatalf("create task: %v", err)
	}

	cwd, _ := os.Getwd()
	inboxDir := filepath.Join(cwd, ".tlc", "inbox")

	payload, _ := json.Marshal(map[string]string{
		"id":     id,
		"status": "IN_PROGRESS",
		"by":     "e2e",
		"note":   "auto claim",
	})
	writeInboxFile(t, inboxDir, "transition", "t.json", payload)

	cmd := newTestCmd()
	cmd.AddCommand(InboxCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"inbox", "process"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("inbox process: %v", err)
	}

	if !strings.Contains(buf.String(), "1 transitioned") {
		t.Errorf("expected '1 transitioned', got %q", buf.String())
	}

	// Verify status changed.
	tasks, _ := s.ListTasks(ctx, core.Query{Limit: 100})
	for _, tk := range tasks {
		if tk.ID == id && tk.Status != core.StatusInProgress {
			t.Errorf("status = %s, want IN_PROGRESS", tk.Status)
		}
	}
}

func TestInboxProcessE2E_FailedFiles(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cwd, _ := os.Getwd()
	inboxDir := filepath.Join(cwd, ".tlc", "inbox")

	// Invalid JSON — no title.
	writeInboxFile(
		t, inboxDir, "create", "bad.json",
		[]byte(`{"description": "no title"}`),
	)

	cmd := newTestCmd()
	cmd.AddCommand(InboxCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"inbox", "process"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("inbox process: %v", err)
	}

	if !strings.Contains(buf.String(), "1 failed") {
		t.Errorf("expected '1 failed', got %q", buf.String())
	}

	// Verify file in failed/.
	failed := filepath.Join(inboxDir, "failed", "bad.json")
	if _, err := os.Stat(failed); os.IsNotExist(err) {
		t.Error("expected file in failed/")
	}

	// Verify .error sidecar.
	sidecar := filepath.Join(inboxDir, "failed", "bad.json.error")
	if _, err := os.Stat(sidecar); os.IsNotExist(err) {
		t.Error("expected .error sidecar")
	}
}

func TestInboxProcessE2E_FullLifecycle(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cwd, _ := os.Getwd()
	inboxDir := filepath.Join(cwd, ".tlc", "inbox")

	// Step 1: Create via inbox.
	payload, _ := json.Marshal(map[string]interface{}{
		"title": "Lifecycle task",
	})
	writeInboxFile(t, inboxDir, "create", "a.json", payload)

	cmd := newTestCmd()
	cmd.AddCommand(InboxCmd)
	cmd.SetArgs([]string{"inbox", "process"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("first process: %v", err)
	}

	// Step 2: Find the created task's ID.
	s, _ := getStorageRaw()
	defer s.Close()
	tasks, _ := s.ListTasks(
		context.Background(), core.Query{Limit: 100},
	)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	taskID := tasks[0].ID

	// Step 3: Transition via inbox.
	tr, _ := json.Marshal(map[string]string{
		"id":     taskID,
		"status": "IN_PROGRESS",
		"by":     "lifecycle-test",
	})
	writeInboxFile(
		t, inboxDir, "transition", "b.json", tr,
	)

	cmd2 := newTestCmd()
	cmd2.AddCommand(InboxCmd)
	buf := new(bytes.Buffer)
	cmd2.SetOut(buf)
	cmd2.SetArgs([]string{"inbox", "process"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("second process: %v", err)
	}

	if !strings.Contains(buf.String(), "1 transitioned") {
		t.Errorf("expected '1 transitioned', got %q", buf.String())
	}

	// Verify final state.
	tasks, _ = s.ListTasks(
		context.Background(), core.Query{Limit: 100},
	)
	if tasks[0].Status != core.StatusInProgress {
		t.Errorf(
			"final status = %s, want IN_PROGRESS",
			tasks[0].Status,
		)
	}
}
