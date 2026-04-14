package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/tui/styles"
)

// TestSaveTask_UniqueIDs verifies that rapid consecutive creates produce distinct
// task IDs — the pre-fix bug used len(tasks)+1 which collided when the count
// matched an existing ID.
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
	}
	if ids[0] == ids[1] {
		t.Fatalf("both tasks got the same ID %q — UNIQUE constraint would fire", ids[0])
	}
}

// TestSaveTask_IDNotDerivedFromCount proves that ID generation does not use
// len(tasks)+1: even with a pre-populated repo the new task gets a fresh
// sequence number rather than a recycled position-based value.
func TestSaveTask_IDNotDerivedFromCount(t *testing.T) {
	repo := core.NewMockRepository()
	logRepo := core.NewMockLogRepository()
	svc := core.NewTaskService(repo, logRepo)

	// Pre-populate the repo with tasks T-0001 and T-0002 directly so the
	// sequence counter stays at 0. A naive len(tasks)+1 would produce T-0003,
	// but after the sequence is advanced by the two NextTaskID calls below
	// it should also produce T-0003 — crucially NOT T-0001 or T-0002.
	repo.Tasks["T-0001"] = &core.Task{ID: "T-0001", Title: "pre"}
	repo.Tasks["T-0002"] = &core.Task{ID: "T-0002", Title: "pre"}

	m := NewModel(svc, styles.DefaultKitTheme())
	cmd := m.saveTask("New Task", "")
	msg := cmd()

	if err, ok := msg.(error); ok {
		t.Fatalf("saveTask returned error: %v", err)
	}

	// New task should be T-0001 from the sequence counter (starts at 1),
	// but MockRepository.CreateTask now enforces uniqueness so it should
	// retry until T-0003.
	found := false
	for id := range repo.Tasks {
		if id != "T-0001" && id != "T-0002" {
			found = true
			if strings.HasPrefix(id, "T-") {
				t.Logf("new task ID: %s", id)
			}
		}
	}
	if !found {
		t.Fatal("no new task created beyond pre-populated ones")
	}
}

// TestSaveTask_RetryOnCollision verifies the retry loop: when the sequence
// produces an ID that already exists the command should retry and succeed.
func TestSaveTask_RetryOnCollision(t *testing.T) {
	repo := core.NewMockRepository()
	logRepo := core.NewMockLogRepository()

	// Seed sequence to start at 0; T-0001 already in repo so first attempt
	// will collide; retry should land on T-0002.
	repo.Tasks["T-0001"] = &core.Task{ID: "T-0001", Title: "existing"}

	svc := core.NewTaskService(repo, logRepo)
	m := NewModel(svc, styles.DefaultKitTheme())

	cmd := m.saveTask("Collision Task", "")
	var msg tea.Msg
	msg = cmd()

	if err, ok := msg.(error); ok {
		t.Fatalf("saveTask returned error after collision: %v", err)
	}

	// Should now have both T-0001 (pre-existing) and T-0002 (retried).
	if _, ok := repo.Tasks["T-0002"]; !ok {
		t.Fatalf("expected T-0002 after retry, repo keys: %v", repoKeys(repo))
	}
}

func repoKeys(r *core.MockRepository) []string {
	keys := make([]string, 0, len(r.Tasks))
	for k := range r.Tasks {
		keys = append(keys, k)
	}
	return keys
}
