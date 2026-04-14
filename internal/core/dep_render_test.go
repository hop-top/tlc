package core

import (
	"strings"
	"testing"
)

func TestRenderDepTree_LinearChain(t *testing.T) {
	tasks := []*Task{
		taskWithDeps("T-0001", "Root"),
		taskWithDeps("T-0002", "Middle", "T-0001"),
		taskWithDeps("T-0003", "Leaf", "T-0002"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}
	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	tree := RenderDepTree(strategy, tasks)
	if tree == "" {
		t.Fatal("expected non-empty tree output")
	}

	// All task IDs should appear.
	for _, id := range []string{"T-0001", "T-0002", "T-0003"} {
		if !strings.Contains(tree, id) {
			t.Errorf("tree missing task %s:\n%s", id, tree)
		}
	}

	// Titles should appear.
	for _, title := range []string{"Root", "Middle", "Leaf"} {
		if !strings.Contains(tree, title) {
			t.Errorf("tree missing title %q:\n%s", title, tree)
		}
	}
}

func TestRenderDepTree_Empty(t *testing.T) {
	tree := RenderDepTree(nil, nil)
	if tree != "" {
		t.Errorf("expected empty string for nil strategy, got %q", tree)
	}

	tree = RenderDepTree(&ExecutionStrategy{}, nil)
	if tree != "" {
		t.Errorf("expected empty string for empty strategy, got %q", tree)
	}
}

func TestRenderDepTree_DiamondAnnotation(t *testing.T) {
	// D depends on B and C → should show multi-parent annotation.
	tasks := []*Task{
		taskWithDeps("T-0001", "Root"),
		taskWithDeps("T-0002", "Left", "T-0001"),
		taskWithDeps("T-0003", "Right", "T-0001"),
		taskWithDeps("T-0004", "Join", "T-0002", "T-0003"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}
	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	tree := RenderDepTree(strategy, tasks)
	// T-0004 has two parents; should show [depends: ...] annotation.
	if !strings.Contains(tree, "[depends:") {
		t.Errorf("expected multi-parent annotation for T-0004:\n%s", tree)
	}
}

func TestRenderBatchSummary_Format(t *testing.T) {
	tasks := []*Task{
		taskWithDeps("T-0001", "Root"),
		taskWithDeps("T-0002", "Left", "T-0001"),
		taskWithDeps("T-0003", "Right", "T-0001"),
		taskWithDeps("T-0004", "End", "T-0002", "T-0003"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}
	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	summary := RenderBatchSummary(strategy)
	if summary == "" {
		t.Fatal("expected non-empty batch summary")
	}

	// Verify header format.
	if !strings.Contains(summary, "Execution Strategy") {
		t.Errorf("missing 'Execution Strategy' header:\n%s", summary)
	}
	if !strings.Contains(summary, "max parallelism: 2") {
		t.Errorf("missing parallelism info:\n%s", summary)
	}

	// Verify batch lines.
	if !strings.Contains(summary, "Batch 1") {
		t.Errorf("missing Batch 1:\n%s", summary)
	}
	if !strings.Contains(summary, "Batch 2 (parallel, 2 agents)") {
		t.Errorf("missing parallel batch info:\n%s", summary)
	}
	if !strings.Contains(summary, "Batch 3") {
		t.Errorf("missing Batch 3:\n%s", summary)
	}
}

func TestRenderBatchSummary_Empty(t *testing.T) {
	summary := RenderBatchSummary(nil)
	if summary != "" {
		t.Errorf("expected empty string for nil strategy, got %q", summary)
	}

	summary = RenderBatchSummary(&ExecutionStrategy{})
	if summary != "" {
		t.Errorf("expected empty string for empty strategy, got %q", summary)
	}
}

func TestRenderBatchSummary_Sequential(t *testing.T) {
	tasks := []*Task{
		taskWithDeps("T-0001", "A"),
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

	summary := RenderBatchSummary(strategy)
	// Each batch has 1 task → sequential.
	if !strings.Contains(summary, "sequential, 1 agent") {
		t.Errorf("expected sequential batch info:\n%s", summary)
	}
}
