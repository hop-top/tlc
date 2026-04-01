package cli

// End-to-end tests for `tlc task graph`.
// Exercises the full CLI pipeline: flag parsing → storage query → graph build → render.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

func TestTaskGraph_E2E_NoBlockers(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	for _, id := range []string{"T-0001", "T-0002"} {
		_ = s.CreateTask(ctx, &core.Task{
			ID:        id,
			Title:     "Task " + id,
			Status:    core.StatusTodo,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "T-0001") {
		t.Errorf("expected T-0001 in output, got:\n%s", out)
	}
	if !strings.Contains(out, "T-0002") {
		t.Errorf("expected T-0002 in output, got:\n%s", out)
	}
	// No indentation — both are roots.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "T-0001") || strings.Contains(line, "T-0002") {
			if strings.HasPrefix(strings.TrimLeft(line, " "), "└") {
				t.Errorf("unexpected indent for root node: %q", line)
			}
		}
	}
}

func TestTaskGraph_E2E_BlockerChain(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	blocker := &core.Task{
		ID:        "T-0010",
		Title:     "Blocker task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	}
	blocked := &core.Task{
		ID:        "T-0011",
		Title:     "Blocked task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	}
	blocked.SetBlockedBy([]string{"T-0010"})

	_ = s.CreateTask(ctx, blocker)
	_ = s.CreateTask(ctx, blocked)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "T-0010") {
		t.Errorf("expected blocker T-0010 in output:\n%s", out)
	}
	if !strings.Contains(out, "T-0011") {
		t.Errorf("expected blocked T-0011 in output:\n%s", out)
	}

	lines := strings.Split(out, "\n")
	blockerIdx, blockedIdx := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "T-0010") {
			blockerIdx = i
		}
		if strings.Contains(l, "T-0011") {
			blockedIdx = i
		}
	}
	if blockerIdx < 0 || blockedIdx < 0 {
		t.Fatalf("could not find both tasks in output")
	}
	if blockerIdx >= blockedIdx {
		t.Errorf("expected blocker (line %d) before blocked (line %d)", blockerIdx, blockedIdx)
	}
	// Blocked task should be indented.
	blockedLine := lines[blockedIdx]
	if !strings.HasPrefix(blockedLine, " ") && !strings.Contains(blockedLine, "└") {
		t.Errorf("expected T-0011 to be indented, got: %q", blockedLine)
	}
}

func TestTaskGraph_E2E_DotFormat(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0020",
		Title:     "Alpha",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})
	blocked := &core.Task{
		ID:        "T-0021",
		Title:     "Beta",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	}
	blocked.SetBlockedBy([]string{"T-0020"})
	_ = s.CreateTask(ctx, blocked)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO", "--format", "dot"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph --format dot: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "digraph") {
		t.Errorf("expected DOT output to start with 'digraph', got:\n%s", out)
	}
	if !strings.Contains(out, "->") {
		t.Errorf("expected directed edge '->' in DOT output:\n%s", out)
	}
	if !strings.Contains(out, "T-0020") || !strings.Contains(out, "T-0021") {
		t.Errorf("expected both task IDs in DOT output:\n%s", out)
	}
}

func TestTaskGraph_E2E_StatusFilter(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0030",
		Title:     "Todo task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0031",
		Title:     "Done task",
		Status:    core.StatusDone,
		CreatedAt: now,
		UpdatedAt: now,
	})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph --status TODO: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "T-0030") {
		t.Errorf("expected T-0030 (TODO) in output:\n%s", out)
	}
	if strings.Contains(out, "T-0031") {
		t.Errorf("unexpected T-0031 (DONE) in TODO-filtered output:\n%s", out)
	}
}

// TestTaskGraph_E2E_Empty verifies "No tasks found." when filter matches nothing.
func TestTaskGraph_E2E_Empty(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph on empty db: %v", err)
	}

	if !strings.Contains(buf.String(), "No tasks found") {
		t.Errorf("expected 'No tasks found' message, got:\n%s", buf.String())
	}
}

// helper: check that SetBlockedBy/BlockedBy round-trips correctly.
// Used only if core.Task doesn't expose these as methods — compile-time checked below.
var _ = (*core.Task).BlockedBy
