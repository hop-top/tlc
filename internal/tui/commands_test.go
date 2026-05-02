package tui

import (
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
)

// TestSaveTask_UniqueIDs verifies that rapid consecutive creates produce
// distinct durable IDs (TypeIDs) — TypeIDs are uuidv7-backed so collisions
// are vanishingly rare, but the command must still produce two rows.
func TestSaveTask_UniqueIDs(t *testing.T) {
	repo := core.NewMockRepository()
	logRepo := core.NewMockLogRepository()
	svc := core.NewTaskService(repo, logRepo)
	m := NewModel(svc, styles.DefaultKitTheme())

	// Simulate creating two tasks back-to-back via the TUI saveTask command.
	cmd1 := m.saveTask("First Task", "desc1")
	msg1 := cmd1()

	// First create must succeed (not an error).
	if err, ok := msg1.(error); ok {
		t.Fatalf("first saveTask returned error: %v", err)
	}

	cmd2 := m.saveTask("Second Task", "desc2")
	msg2 := cmd2()

	if err, ok := msg2.(error); ok {
		t.Fatalf("second saveTask returned error: %v", err)
	}

	// Both tasks must be in the repo with distinct IDs.
	if len(repo.Tasks) != 2 {
		t.Fatalf("expected 2 tasks in repo, got %d", len(repo.Tasks))
	}

	ids := make([]string, 0, 2)
	for id := range repo.Tasks {
		ids = append(ids, id)
		if !core.IsTaskID(id) {
			t.Errorf("task id %q is not a TypeID (expected task_<26>)", id)
		}
	}
	if ids[0] == ids[1] {
		t.Fatalf("both tasks got the same ID %q — UNIQUE constraint would fire", ids[0])
	}
}

// TestSaveTask_DurableIDIsTypeID proves that new tasks created via the
// TUI use the typeid form for their durable ID (so URIs survive renames),
// not the T-NNNN display alias.
func TestSaveTask_DurableIDIsTypeID(t *testing.T) {
	repo := core.NewMockRepository()
	logRepo := core.NewMockLogRepository()
	svc := core.NewTaskService(repo, logRepo)

	m := NewModel(svc, styles.DefaultKitTheme())
	cmd := m.saveTask("New Task", "")
	msg := cmd()

	if err, ok := msg.(error); ok {
		t.Fatalf("saveTask returned error: %v", err)
	}

	if len(repo.Tasks) != 1 {
		t.Fatalf("expected 1 task in repo, got %d", len(repo.Tasks))
	}
	for id, task := range repo.Tasks {
		if !core.IsTaskID(id) {
			t.Errorf("task id %q is not a TypeID; want task_<26>", id)
		}
		if !strings.Contains(task.Reference, id) {
			t.Errorf("task.Reference %q must embed durable typeid %q",
				task.Reference, id)
		}
	}
}
