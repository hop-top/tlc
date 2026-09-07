package cli

// `tlc status` prints task and overdue counts as bare facts. They used to
// be measured by taking len() of a capped slice (--limit 50 for the
// assigned counts, 1000 for overdue), so once a project outgrew the cap
// the numbers silently drifted low with no warning and exit 0.
//
// Observed on real data before the fix: 1077 active tasks, of which 4 were
// overdue, but only 3 fell inside the first 1000 rows by created_at DESC --
// status reported "Overdue: 3". The fixtures below reproduce that shape at
// smaller scale, seeding past both caps.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

const (
	// statusSeedInProgress and statusSeedTodo both exceed the old
	// per-status cap of 50.
	statusSeedInProgress = 70
	statusSeedTodo       = 80
	statusOldAssignedCap = 50
	statusSeedUser       = "counts-owner"
	statusProjectID      = "status-fixture"
)

// seedStatusDB seeds assigned active tasks past the old cap, plus one
// overdue task placed last by created_at so any scan cap would miss it.
func seedStatusDB(t *testing.T, dbPath string) {
	t.Helper()

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("open seed storage: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := t.Context()
	user := statusSeedUser
	base := time.Now().UTC().Add(-72 * time.Hour)

	total := statusSeedInProgress + statusSeedTodo
	for i := range total {
		status := core.StatusInProgress
		if i >= statusSeedInProgress {
			status = core.StatusTodo
		}
		pid := statusProjectID
		created := base.Add(time.Duration(i) * time.Minute)
		task := &core.Task{
			ID:         fmt.Sprintf("T-%04d", i+1),
			Title:      fmt.Sprintf("status task %d", i+1),
			Status:     status,
			ProjectID:  &pid,
			AssignedTo: &user,
			CreatedAt:  created,
			UpdatedAt:  created,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %d: %v", i+1, err)
		}
	}

	// One overdue task, created oldest so it sorts last under
	// created_at DESC -- the position a scan cap would drop first.
	pid := statusProjectID
	due := time.Now().UTC().Add(-48 * time.Hour)
	oldest := base.Add(-24 * time.Hour)
	overdue := &core.Task{
		ID:         "T-9999",
		Title:      "overdue straggler",
		Status:     core.StatusTodo,
		ProjectID:  &pid,
		AssignedTo: &user,
		DueAt:      &due,
		CreatedAt:  oldest,
		UpdatedAt:  oldest,
	}
	if err := s.CreateTask(ctx, overdue); err != nil {
		t.Fatalf("seed overdue task: %v", err)
	}
}

// TestStatus_CountsAreNotCapped asserts the printed counts describe the
// whole match set, not the slice a cap happened to return.
func TestStatus_CountsAreNotCapped(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "status.db")

	seedStatusDB(t, dbPath)
	env := append(aggEnv(t, home, dbPath), "TLC_USER="+statusSeedUser)

	out, code := runAgg(t, bin, cwd, env, "status")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	var inProgress, todo int
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "in progress") {
			continue
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(line),
			"Tasks: %d in progress, %d todo (yours)", &inProgress, &todo); err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		break
	}

	if inProgress != statusSeedInProgress {
		t.Errorf("in progress = %d, want %d\n%s", inProgress, statusSeedInProgress, out)
	}
	// The overdue straggler is TODO too, so it counts here.
	if wantTodo := statusSeedTodo + 1; todo != wantTodo {
		t.Errorf("todo = %d, want %d\n%s", todo, wantTodo, out)
	}
	// The defect's signature: a count pinned to the old cap.
	if inProgress == statusOldAssignedCap || todo == statusOldAssignedCap {
		t.Errorf("a count equals the old cap of %d -- slice measured, not match set\n%s",
			statusOldAssignedCap, out)
	}
}

// TestStatus_OverdueCountsWholeMatchSet pins the overdue line, the count
// that was demonstrably wrong on real data.
func TestStatus_OverdueCountsWholeMatchSet(t *testing.T) {
	bin := buildTLCBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(home, "status.db")

	seedStatusDB(t, dbPath)
	env := append(aggEnv(t, home, dbPath), "TLC_USER="+statusSeedUser)

	out, code := runAgg(t, bin, cwd, env, "status")
	if code != 0 {
		t.Fatalf("exit %d, want 0\n%s", code, out)
	}

	// Exactly one task is overdue, and it is the one a scan cap would
	// have dropped, so its absence is the regression.
	if !strings.Contains(out, "Overdue:  1 task(s)") {
		t.Errorf("expected 'Overdue:  1 task(s)', got:\n%s", out)
	}
}
