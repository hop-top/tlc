package cli

// End-to-end tests for case-insensitive + alias + fuzzy field normalization
// on `tlc task list --status` and `--priority`.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestFieldNorm_E2E_StatusLowercase(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0001",
		Title:     "Done task",
		Status:    core.StatusDone,
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0002",
		Title:     "Todo task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "done"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --status done: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "T-0001") {
		t.Errorf("expected T-0001 (DONE) in output:\n%s", out)
	}
	if strings.Contains(out, "T-0002") {
		t.Errorf("unexpected T-0002 (TODO) with --status done:\n%s", out)
	}
}

func TestFieldNorm_E2E_StatusAlias(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0010",
		Title:     "Done task",
		Status:    core.StatusDone,
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0011",
		Title:     "Todo task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})

	for _, alias := range []string{"complete", "completed", "finish"} {
		t.Run(alias, func(t *testing.T) {
			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{"task", "list", "--status", alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task list --status %s: %v", alias, err)
			}
			out := buf.String()
			if !strings.Contains(out, "T-0010") {
				t.Errorf("alias %q: expected T-0010 (DONE) in output:\n%s", alias, out)
			}
			if strings.Contains(out, "T-0011") {
				t.Errorf("alias %q: unexpected T-0011 (TODO):\n%s", alias, out)
			}
		})
	}
}

func TestFieldNorm_E2E_PriorityAlias(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0020",
		Title:     "High priority task",
		Status:    core.StatusTodo,
		Priority:  "P1",
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0021",
		Title:     "Low priority task",
		Status:    core.StatusTodo,
		Priority:  "P3",
		CreatedAt: now,
		UpdatedAt: now,
	})

	for _, alias := range []string{"high", "1", "p1"} {
		t.Run(alias, func(t *testing.T) {
			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{"task", "list", "--status", "TODO", "--priority", alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task list --priority %s: %v", alias, err)
			}
			out := buf.String()
			if !strings.Contains(out, "T-0020") {
				t.Errorf("alias %q: expected T-0020 (P1) in output:\n%s", alias, out)
			}
			if strings.Contains(out, "T-0021") {
				t.Errorf("alias %q: unexpected T-0021 (P3):\n%s", alias, out)
			}
		})
	}
}

func TestFieldNorm_E2E_UnknownStatus(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	cmd.SetArgs([]string{"task", "list", "--status", "xyz"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown status, got nil")
	}
	if !strings.Contains(err.Error(), "xyz") {
		t.Errorf("expected error to mention 'xyz', got: %v", err)
	}
}
