package core

import (
	"testing"
)

// taskWithDeps creates a Task with blocked-by metadata.
func taskWithDeps(id string, title string, deps ...string) *Task {
	meta := map[string]interface{}{}
	if len(deps) > 0 {
		meta["blocked_by"] = deps
	}
	return &Task{
		ID:    id,
		Title: title,
		Meta:  meta,
	}
}

func TestDepGraph_LinearChain(t *testing.T) {
	// A → B → C: 3 batches of 1
	tasks := []*Task{
		taskWithDeps("T-0001", "Task A"),
		taskWithDeps("T-0002", "Task B", "T-0001"),
		taskWithDeps("T-0003", "Task C", "T-0002"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	if len(strategy.Batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(strategy.Batches))
	}
	for i, batch := range strategy.Batches {
		if len(batch.Tasks) != 1 {
			t.Errorf("batch %d: expected 1 task, got %d", i, len(batch.Tasks))
		}
		if batch.Parallel {
			t.Errorf("batch %d: should not be parallel", i)
		}
	}
	if strategy.MaxParallelism != 1 {
		t.Errorf("expected max parallelism 1, got %d", strategy.MaxParallelism)
	}
}

func TestDepGraph_Diamond(t *testing.T) {
	// A → B, A → C, B → D, C → D: 3 batches: (A), (B,C), (D)
	tasks := []*Task{
		taskWithDeps("T-0001", "Task A"),
		taskWithDeps("T-0002", "Task B", "T-0001"),
		taskWithDeps("T-0003", "Task C", "T-0001"),
		taskWithDeps("T-0004", "Task D", "T-0002", "T-0003"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	if len(strategy.Batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(strategy.Batches))
	}

	// Batch 0: A
	if len(strategy.Batches[0].Tasks) != 1 {
		t.Errorf("batch 0: expected 1 task, got %d", len(strategy.Batches[0].Tasks))
	}
	// Batch 1: B, C (parallel)
	if len(strategy.Batches[1].Tasks) != 2 {
		t.Errorf("batch 1: expected 2 tasks, got %d", len(strategy.Batches[1].Tasks))
	}
	if !strategy.Batches[1].Parallel {
		t.Error("batch 1: should be parallel")
	}
	// Batch 2: D
	if len(strategy.Batches[2].Tasks) != 1 {
		t.Errorf("batch 2: expected 1 task, got %d", len(strategy.Batches[2].Tasks))
	}

	if strategy.MaxParallelism != 2 {
		t.Errorf("expected max parallelism 2, got %d", strategy.MaxParallelism)
	}
}

func TestDepGraph_FullyIndependent(t *testing.T) {
	// A, B, C: 1 batch of 3
	tasks := []*Task{
		taskWithDeps("T-0001", "Task A"),
		taskWithDeps("T-0002", "Task B"),
		taskWithDeps("T-0003", "Task C"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	if len(strategy.Batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(strategy.Batches))
	}
	if len(strategy.Batches[0].Tasks) != 3 {
		t.Errorf("expected 3 tasks in batch, got %d", len(strategy.Batches[0].Tasks))
	}
	if !strategy.Batches[0].Parallel {
		t.Error("batch 0: should be parallel")
	}
	if strategy.MaxParallelism != 3 {
		t.Errorf("expected max parallelism 3, got %d", strategy.MaxParallelism)
	}
}

func TestDepGraph_CycleDetection(t *testing.T) {
	// A → B → A: cycle
	tasks := []*Task{
		taskWithDeps("T-0001", "Task A", "T-0002"),
		taskWithDeps("T-0002", "Task B", "T-0001"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	_, err = g.ComputeStrategy()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}

	_, err = g.TopologicalSort()
	if err == nil {
		t.Fatal("expected cycle error from TopologicalSort, got nil")
	}
}

func TestDepGraph_CrossTrackDeps(t *testing.T) {
	// Cross-project dep should be silently ignored.
	tasks := []*Task{
		taskWithDeps("T-0001", "Task A", "other-project#T-0001"),
		taskWithDeps("T-0002", "Task B", "T-0001"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	if len(strategy.Batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(strategy.Batches))
	}
}

func TestDepGraph_Empty(t *testing.T) {
	g, err := NewDepGraph([]*Task{})
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	if strategy.TotalTasks != 0 {
		t.Errorf("expected 0 total tasks, got %d", strategy.TotalTasks)
	}
	if len(strategy.Batches) != 0 {
		t.Errorf("expected 0 batches, got %d", len(strategy.Batches))
	}
}

func TestDepGraph_SingleTask(t *testing.T) {
	tasks := []*Task{
		taskWithDeps("T-0001", "Sole task"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	if len(strategy.Batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(strategy.Batches))
	}
	if len(strategy.Batches[0].Tasks) != 1 {
		t.Errorf("expected 1 task in batch, got %d", len(strategy.Batches[0].Tasks))
	}
	if strategy.TotalTasks != 1 {
		t.Errorf("expected 1 total task, got %d", strategy.TotalTasks)
	}
}

func TestDepGraph_ComplexTree(t *testing.T) {
	// Simulate kit-adoption-like scenario:
	// Phase 1: A (root)
	// Phase 2: B, C, D (all depend on A)
	// Phase 3: E (depends on B, C), F (depends on D)
	// Phase 4: G (depends on E, F)
	tasks := []*Task{
		taskWithDeps("T-0001", "Foundation setup"),
		taskWithDeps("T-0002", "Module alpha", "T-0001"),
		taskWithDeps("T-0003", "Module beta", "T-0001"),
		taskWithDeps("T-0004", "Module gamma", "T-0001"),
		taskWithDeps("T-0005", "Integration AB", "T-0002", "T-0003"),
		taskWithDeps("T-0006", "Integration D", "T-0004"),
		taskWithDeps("T-0007", "Final assembly", "T-0005", "T-0006"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	if strategy.TotalTasks != 7 {
		t.Errorf("expected 7 total tasks, got %d", strategy.TotalTasks)
	}
	if len(strategy.Batches) != 4 {
		t.Fatalf("expected 4 batches, got %d", len(strategy.Batches))
	}

	// Batch 0: T-0001 (1 task)
	if len(strategy.Batches[0].Tasks) != 1 {
		t.Errorf("batch 0: expected 1 task, got %d", len(strategy.Batches[0].Tasks))
	}
	// Batch 1: T-0002, T-0003, T-0004 (3 parallel)
	if len(strategy.Batches[1].Tasks) != 3 {
		t.Errorf("batch 1: expected 3 tasks, got %d", len(strategy.Batches[1].Tasks))
	}
	if !strategy.Batches[1].Parallel {
		t.Error("batch 1: should be parallel")
	}
	// Batch 2: T-0005, T-0006 (2 parallel)
	if len(strategy.Batches[2].Tasks) != 2 {
		t.Errorf("batch 2: expected 2 tasks, got %d", len(strategy.Batches[2].Tasks))
	}
	// Batch 3: T-0007 (1 task)
	if len(strategy.Batches[3].Tasks) != 1 {
		t.Errorf("batch 3: expected 1 task, got %d", len(strategy.Batches[3].Tasks))
	}

	if strategy.MaxParallelism != 3 {
		t.Errorf("expected max parallelism 3, got %d", strategy.MaxParallelism)
	}

	// Critical path should be length 4 (one from each batch).
	if len(strategy.CriticalPath) != 4 {
		t.Errorf("expected critical path length 4, got %d: %v",
			len(strategy.CriticalPath), strategy.CriticalPath)
	}
}

func TestDepGraph_CriticalPath(t *testing.T) {
	// A → B → D (length 3) vs A → C (length 2)
	tasks := []*Task{
		taskWithDeps("T-0001", "Root"),
		taskWithDeps("T-0002", "Long path", "T-0001"),
		taskWithDeps("T-0003", "Short path", "T-0001"),
		taskWithDeps("T-0004", "End", "T-0002"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	path := g.CriticalPath()
	if len(path) != 3 {
		t.Fatalf("expected critical path length 3, got %d: %v", len(path), path)
	}
	// Path should be T-0001 → T-0002 → T-0004.
	expected := []string{"T-0001", "T-0002", "T-0004"}
	for i, id := range expected {
		if path[i] != id {
			t.Errorf("critical path[%d]: expected %s, got %s", i, id, path[i])
		}
	}
}

func TestDepGraph_HasDeps(t *testing.T) {
	// No deps.
	g1, _ := NewDepGraph([]*Task{
		taskWithDeps("T-0001", "A"),
		taskWithDeps("T-0002", "B"),
	})
	if g1.HasDeps() {
		t.Error("expected HasDeps=false for independent tasks")
	}

	// With deps.
	g2, _ := NewDepGraph([]*Task{
		taskWithDeps("T-0001", "A"),
		taskWithDeps("T-0002", "B", "T-0001"),
	})
	if !g2.HasDeps() {
		t.Error("expected HasDeps=true for dependent tasks")
	}
}

func TestDepGraph_NilTask(t *testing.T) {
	// Nil tasks should be skipped without error.
	tasks := []*Task{
		taskWithDeps("T-0001", "A"),
		nil,
		taskWithDeps("T-0002", "B", "T-0001"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}

	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	if strategy.TotalTasks != 2 {
		t.Errorf("expected 2 total tasks, got %d", strategy.TotalTasks)
	}
}
