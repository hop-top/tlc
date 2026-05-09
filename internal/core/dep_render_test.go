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

// taskWithTypeIDAndSeq mirrors production tasks: durable TypeID as ID,
// display alias derived from Seq. Used to assert renderers prefer the
// alias over the TypeID.
func taskWithTypeIDAndSeq(typeID string, seq int64, title string, deps ...string) *Task {
	t := taskWithDeps(typeID, title, deps...)
	t.Seq = seq
	return t
}

func TestRenderDepTree_HidesTypeID(t *testing.T) {
	tasks := []*Task{
		taskWithTypeIDAndSeq("task_01kr613t07fs3aer4d7gwc15c1", 1, "Root"),
		taskWithTypeIDAndSeq("task_01kr613t07fs3seytf1wb4nt79", 2, "Leaf",
			"task_01kr613t07fs3aer4d7gwc15c1"),
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
	if strings.Contains(tree, "task_01") {
		t.Errorf("dep tree must not leak typeid:\n%s", tree)
	}
	for _, alias := range []string{"T-0001", "T-0002"} {
		if !strings.Contains(tree, alias) {
			t.Errorf("dep tree missing alias %q:\n%s", alias, tree)
		}
	}
}

func TestRenderBatchSummary_HidesTypeID(t *testing.T) {
	tasks := []*Task{
		taskWithTypeIDAndSeq("task_01kr613t07fs3aer4d7gwc15c1", 1, "Root"),
		taskWithTypeIDAndSeq("task_01kr613t07fs3seytf1wb4nt79", 2, "Leaf",
			"task_01kr613t07fs3aer4d7gwc15c1"),
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
	if strings.Contains(summary, "task_01") {
		t.Errorf("batch summary must not leak typeid:\n%s", summary)
	}
	for _, alias := range []string{"T-0001", "T-0002"} {
		if !strings.Contains(summary, alias) {
			t.Errorf("batch summary missing alias %q:\n%s", alias, summary)
		}
	}
}

func TestFormatCriticalPath_HidesTypeID(t *testing.T) {
	tasks := []*Task{
		taskWithTypeIDAndSeq("task_01kr613t07fs3aer4d7gwc15c1", 1, "Root"),
		taskWithTypeIDAndSeq("task_01kr613t07fs3seytf1wb4nt79", 2, "Leaf",
			"task_01kr613t07fs3aer4d7gwc15c1"),
	}

	path := []string{
		"task_01kr613t07fs3aer4d7gwc15c1",
		"task_01kr613t07fs3seytf1wb4nt79",
	}
	got := FormatCriticalPath(path, tasks)
	want := "T-0001 → T-0002"
	if got != want {
		t.Errorf("FormatCriticalPath: got %q want %q", got, want)
	}
}

func TestFormatCriticalPath_UnknownIDFallsBack(t *testing.T) {
	tasks := []*Task{
		taskWithTypeIDAndSeq("task_known", 7, "Known"),
	}
	got := FormatCriticalPath([]string{"task_known", "task_unknown"}, tasks)
	want := "T-0007 → task_unknown"
	if got != want {
		t.Errorf("FormatCriticalPath fallback: got %q want %q", got, want)
	}
}

func TestFormatCriticalPath_Empty(t *testing.T) {
	if got := FormatCriticalPath(nil, nil); got != "" {
		t.Errorf("empty path: got %q", got)
	}
	if got := FormatCriticalPath([]string{}, nil); got != "" {
		t.Errorf("empty path: got %q", got)
	}
}

func TestRenderMermaid_HidesTypeIDInLabels(t *testing.T) {
	tasks := []*Task{
		taskWithTypeIDAndSeq("task_01kr613t07fs3aer4d7gwc15c1", 1, "Root"),
		taskWithTypeIDAndSeq("task_01kr613t07fs3seytf1wb4nt79", 2, "Leaf",
			"task_01kr613t07fs3aer4d7gwc15c1"),
	}

	g, err := NewDepGraph(tasks)
	if err != nil {
		t.Fatalf("NewDepGraph: %v", err)
	}
	strategy, err := g.ComputeStrategy()
	if err != nil {
		t.Fatalf("ComputeStrategy: %v", err)
	}

	out := RenderMermaid(strategy, tasks)
	// Labels must show alias, not typeid.
	for _, alias := range []string{"T-0001", "T-0002"} {
		if !strings.Contains(out, alias) {
			t.Errorf("Mermaid labels missing alias %q:\n%s", alias, out)
		}
	}
	// Mermaid node identifiers (mermaidNodeID) and edges still derive
	// from t.ID — that's an internal graph identifier, not display.
	// We only assert the human-visible label segment doesn't carry
	// the typeid as the display token before the title.
	if strings.Contains(out, "task_01kr613t07fs3aer4d7gwc15c1 Root") {
		t.Errorf("Mermaid label leaked typeid as display token:\n%s", out)
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
