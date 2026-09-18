package storage

// A nil context reaching database/sql is not a clean error: sql.(*DB).conn
// takes db.mu and THEN evaluates `<-ctx.Done()`, so a nil ctx panics with
// the connection mutex still held. Any deferred Close() then blocks on that
// mutex forever, and the panic never surfaces — the process hangs until the
// test binary's timeout kills it, and the goroutine dump names whichever
// test happened to be holding the suite's global lock rather than the
// caller that passed nil.
//
// That is a caller mistake, but the failure mode turns a one-line bug into
// an unkillable hang with a misdirected stack trace. It cost a real
// diagnosis: the two tests that did it were skipped in CI for months as an
// unexplained "deadlock" rather than fixed.
//
// These tests run the call in a goroutine with a deadline, because a
// regression here HANGS. A plain call would wedge the whole package and
// report as a timeout in an unrelated test — exactly the failure being
// fixed.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// deadlockGrace is how long a call gets before it counts as wedged.
// Generous: these operations are sub-second, so anything near it is a
// deadlock rather than a slow machine.
const deadlockGrace = 10 * time.Second

// withDeadline runs fn and fails if it has not returned in time, rather
// than letting a regression wedge the package.
func withDeadline(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(deadlockGrace):
		t.Fatalf("%s did not return within %s — nil context deadlocked the pool", what, deadlockGrace)
	}
}

// TestNilContextWriteDoesNotDeadlock is the headline regression: a write
// handed a nil context must come back rather than panic under db.mu and
// wedge every later Close.
//
// It asserts the call SUCCEEDS rather than errors. Coercing nil to
// Background is the conservative choice: refusing would turn today's
// hang into a new failure for any caller that currently gets away with
// it, while go vet already catches nil at compile time (SA1012). The
// contract being pinned is "a caller bug stays a caller bug", not "nil
// is a supported argument".
func TestNilContextWriteDoesNotDeadlock(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStorage(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer func() { _ = s.Close() }()

	task := &core.Task{ID: "T-0001", Title: "nil ctx probe", Status: core.StatusTodo}

	withDeadline(t, "CreateTask(nil)", func() {
		//nolint:staticcheck // SA1012: passing nil is the defect under test
		if err := s.CreateTask(nil, task); err != nil {
			t.Errorf("CreateTask(nil): %v", err)
		}
	})

	// The row must actually be there: a guard that swallowed the write
	// and returned nil would satisfy the deadline check above.
	//
	// TaskIDExists rather than GetTask, deliberately. GetTask scopes its
	// read through core.DetectProject(), which finds the ambient repo
	// when the suite runs inside one, so it misses a row written to the
	// global bucket — with a real context too. That is project scoping,
	// not the defect under test, and asserting through it would make
	// this test fail for a reason it is not about.
	exists, err := s.TaskIDExists(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("TaskIDExists after nil-context write: %v", err)
	}
	if !exists {
		t.Errorf("nil-context write did not persist task %s", task.ID)
	}
}

// TestNilContextWriteLeavesPoolUsable pins the second half. Refusing the
// call is not enough on its own: the original defect's damage was that the
// panic escaped while db.mu was held, so everything touching the pool
// afterwards — including Close — blocked forever. The store must still
// work after a nil-context call is refused.
func TestNilContextWriteLeavesPoolUsable(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStorage(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer func() { _ = s.Close() }()

	withDeadline(t, "nil-context call", func() {
		//nolint:staticcheck // SA1012: passing nil is the defect under test
		_ = s.CreateTask(nil, &core.Task{ID: "T-0001", Title: "poisons pool?", Status: core.StatusTodo})
	})

	withDeadline(t, "subsequent CreateTask", func() {
		task := &core.Task{ID: "T-0002", Title: "after nil ctx", Status: core.StatusTodo}
		if err := s.CreateTask(context.Background(), task); err != nil {
			t.Errorf("store unusable after a nil-context call: %v", err)
		}
	})

	withDeadline(t, "Close", func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close after a nil-context call: %v", err)
		}
	})
}

// TestNilContextReadDoesNotDeadlock covers the read path, which reaches
// database/sql through QueryContext rather than BeginTx. Fixing only the
// write transaction would leave the same hang one call away.
func TestNilContextReadDoesNotDeadlock(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSQLiteStorage(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer func() { _ = s.Close() }()

	if err := s.CreateTask(context.Background(), &core.Task{
		ID: "T-0001", Title: "read probe", Status: core.StatusTodo,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Asserts only that the read RETURNS. Whether it finds the row
	// depends on core.DetectProject() and the ambient directory, which
	// is project scoping rather than context handling; pinning it here
	// would couple this regression to where the suite happens to run.
	withDeadline(t, "GetTask(nil)", func() {
		//nolint:staticcheck // SA1012: passing nil is the defect under test
		if _, err := s.GetTask(nil, "T-0001"); err != nil {
			t.Errorf("GetTask(nil): %v", err)
		}
	})
}
