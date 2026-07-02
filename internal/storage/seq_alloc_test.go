package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestCreateTask_AssignsAtomicSeq verifies that CreateTask allocates a
// sequence number transactionally and persists it on the task row.
func TestCreateTask_AssignsAtomicSeq(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	pid := "proj-a"

	for i := 1; i <= 5; i++ {
		task := &core.Task{
			ID:        core.NewTaskID(),
			Title:     "task",
			Status:    core.StatusTodo,
			ProjectID: &pid,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if int(task.Seq) != i {
			t.Errorf("task[%d].Seq = %d; want %d", i, task.Seq, i)
		}
	}
}

// TestCreateTask_ConcurrentAllocationsAreUnique fires N concurrent inserts
// against the same project and asserts every assigned seq is distinct
// and forms the contiguous range 1..N.
func TestCreateTask_ConcurrentAllocationsAreUnique(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	const n = 50
	pid := "proj-a"
	ctx := context.Background()

	var wg sync.WaitGroup
	seqs := make([]int64, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := &core.Task{
				ID:        core.NewTaskID(),
				Title:     "concurrent",
				Status:    core.StatusTodo,
				ProjectID: &pid,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			if err := s.CreateTask(ctx, task); err != nil {
				errs[idx] = err
				return
			}
			seqs[idx] = task.Seq
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent CreateTask[%d] failed: %v", i, err)
		}
	}

	seen := make(map[int64]bool, n)
	var minSeq, maxSeq int64 = 1 << 62, 0
	for _, s := range seqs {
		if seen[s] {
			t.Fatalf("duplicate seq assigned: %d (slice: %v)", s, seqs)
		}
		seen[s] = true
		if s < minSeq {
			minSeq = s
		}
		if s > maxSeq {
			maxSeq = s
		}
	}
	if minSeq != 1 || maxSeq != int64(n) {
		t.Errorf("seq range = [%d..%d]; want [1..%d]", minSeq, maxSeq, n)
	}
}

// TestCreateTask_PerProjectSequencesAreIndependent verifies seq counters
// scope per project_id (T-0001 in proj-a does not collide with T-0001 in
// proj-b).
func TestCreateTask_PerProjectSequencesAreIndependent(t *testing.T) {
	s, err := NewSQLiteStorage(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	pa := "proj-a"
	pb := "proj-b"

	taskA := &core.Task{
		ID: core.NewTaskID(), Title: "a", Status: core.StatusTodo,
		ProjectID: &pa, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	taskB := &core.Task{
		ID: core.NewTaskID(), Title: "b", Status: core.StatusTodo,
		ProjectID: &pb, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}

	if err := s.CreateTask(ctx, taskA); err != nil {
		t.Fatalf("CreateTask A: %v", err)
	}
	if err := s.CreateTask(ctx, taskB); err != nil {
		t.Fatalf("CreateTask B: %v", err)
	}

	if taskA.Seq != 1 {
		t.Errorf("taskA.Seq = %d; want 1", taskA.Seq)
	}
	if taskB.Seq != 1 {
		t.Errorf("taskB.Seq = %d; want 1 (different project)", taskB.Seq)
	}
}
