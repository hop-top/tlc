package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// helpers ----------------------------------------------------------------

// createGraphTasks populates a fresh test DB with a standard dependency chain
// and a standalone task:
//
//	T-0001 (DONE)  ← T-0002 (IN_PROGRESS) ← T-0003 (TODO)
//	T-0004 (TODO)  standalone
func createGraphTasks(t *testing.T) {
	t.Helper()
	ctx, _ := setupTestDir(t)
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	tasks := []*core.Task{
		{ID: "T-0001", Title: "Foundation task", Status: core.StatusDone, CreatedAt: now, UpdatedAt: now},
		{
			ID:        "T-0002",
			Title:     "Middle task",
			Status:    core.StatusInProgress,
			CreatedAt: now, UpdatedAt: now,
			Meta: map[string]interface{}{"blocked_by": []string{"T-0001"}},
		},
		{
			ID:        "T-0003",
			Title:     "Leaf task",
			Status:    core.StatusTodo,
			CreatedAt: now, UpdatedAt: now,
			Meta: map[string]interface{}{"blocked_by": []string{"T-0002"}},
		},
		{ID: "T-0004", Title: "Standalone task", Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now},
	}
	for _, task := range tasks {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}
}

// unit: buildGraph -------------------------------------------------------

func TestBuildGraph_Linear(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Title: "A", Status: core.StatusDone},
		{
			ID:     "T-0002",
			Title:  "B",
			Status: core.StatusTodo,
			Meta:   map[string]interface{}{"blocked_by": []string{"T-0001"}},
		},
		{
			ID:     "T-0003",
			Title:  "C",
			Status: core.StatusTodo,
			Meta:   map[string]interface{}{"blocked_by": []string{"T-0002"}},
		},
	}

	g := buildGraph(tasks)

	if len(g.roots) != 1 || g.roots[0] != "T-0001" {
		t.Errorf("expected single root T-0001, got %v", g.roots)
	}
	if got := g.blockedBy["T-0002"]; len(got) != 1 || got[0] != "T-0001" {
		t.Errorf("T-0002 blockedBy expected [T-0001], got %v", got)
	}
	if got := g.blocks["T-0001"]; len(got) != 1 || got[0] != "T-0002" {
		t.Errorf("T-0001 blocks expected [T-0002], got %v", got)
	}
}

func TestBuildGraph_CrossProjectEdgeStripped(t *testing.T) {
	// blocker from another project should not appear as an in-scope edge
	tasks := []*core.Task{
		{ID: "T-0001", Title: "A", Status: core.StatusTodo},
		{
			ID:     "T-0002",
			Title:  "B",
			Status: core.StatusTodo,
			Meta:   map[string]interface{}{"blocked_by": []string{"other-project/T-9999"}},
		},
	}
	g := buildGraph(tasks)
	if len(g.blockedBy["T-0002"]) != 0 {
		t.Errorf("expected no in-scope blockedBy edges for T-0002, got %v", g.blockedBy["T-0002"])
	}
	if len(g.roots) != 2 {
		t.Errorf("expected 2 roots (both tasks are free), got %v", g.roots)
	}
}

func TestBuildGraph_Standalone(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Title: "solo", Status: core.StatusTodo},
	}
	g := buildGraph(tasks)
	if len(g.roots) != 1 {
		t.Errorf("expected 1 root, got %d", len(g.roots))
	}
	if len(g.blockedBy["T-0001"]) != 0 {
		t.Errorf("expected no blockers, got %v", g.blockedBy["T-0001"])
	}
}

func TestBuildGraph_MultipleRoots(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Title: "Root A", Status: core.StatusTodo},
		{ID: "T-0002", Title: "Root B", Status: core.StatusTodo},
		{
			ID:     "T-0003",
			Title:  "Depends on both",
			Status: core.StatusTodo,
			Meta:   map[string]interface{}{"blocked_by": []string{"T-0001", "T-0002"}},
		},
	}
	g := buildGraph(tasks)
	if len(g.roots) != 2 {
		t.Errorf("expected 2 roots, got %v", g.roots)
	}
	if len(g.blockedBy["T-0003"]) != 2 {
		t.Errorf("T-0003 should have 2 blockers, got %v", g.blockedBy["T-0003"])
	}
}

// CLI integration --------------------------------------------------------

func TestTaskGraph_ASCIIDefault(t *testing.T) {
	createGraphTasks(t)
	resetTaskFlags()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO,IN_PROGRESS,DONE"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph failed: %v", err)
	}

	out := buf.String()
	for _, id := range []string{"T-0001", "T-0002", "T-0003", "T-0004"} {
		if !contains(out, id) {
			t.Errorf("expected %s in graph output", id)
		}
	}
	if !contains(out, "Task Dependency Graph") {
		t.Error("expected header 'Task Dependency Graph'")
	}
}

func TestTaskGraph_DOTFormat(t *testing.T) {
	createGraphTasks(t)
	resetTaskFlags()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO,IN_PROGRESS,DONE", "--format", "dot"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph --format dot failed: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "digraph tasks {") {
		t.Errorf("expected DOT output starting with 'digraph tasks {', got: %.80s", out)
	}
	if !contains(out, "T-0001") {
		t.Error("expected T-0001 in DOT output")
	}
	// Edge: T-0001 blocks T-0002 (direction: dep -> blocked)
	if !contains(out, `"T-0001" -> "T-0002"`) {
		t.Errorf("expected edge T-0001 -> T-0002 in DOT output, got:\n%s", out)
	}
}

func TestTaskGraph_FilterByStatus(t *testing.T) {
	createGraphTasks(t)
	resetTaskFlags()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	// Only TODO tasks
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph --status TODO failed: %v", err)
	}

	out := buf.String()
	// T-0003 and T-0004 are TODO; T-0001 is DONE, T-0002 is IN_PROGRESS
	if contains(out, "Foundation task") {
		t.Error("did not expect DONE task in TODO-only graph")
	}
	if contains(out, "Middle task") {
		t.Error("did not expect IN_PROGRESS task in TODO-only graph")
	}
	if !contains(out, "T-0003") {
		t.Error("expected TODO task T-0003 in output")
	}
}

func TestTaskGraph_EmptyResult(t *testing.T) {
	createGraphTasks(t)
	resetTaskFlags()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "SKIPPED"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph with empty result failed: %v", err)
	}

	out := buf.String()
	if !contains(out, "No tasks found") {
		t.Errorf("expected 'No tasks found' message, got: %s", out)
	}
}

func TestTaskGraph_ASCIIIndentation(t *testing.T) {
	// Validates that deeper nodes are indented relative to their parents.
	createGraphTasks(t)
	resetTaskFlags()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "graph", "--status", "TODO,IN_PROGRESS,DONE"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task graph failed: %v", err)
	}

	out := buf.String()
	lines := strings.Split(out, "\n")

	var rootLine, childLine string
	for _, line := range lines {
		if strings.Contains(line, "T-0001") {
			rootLine = line
		}
		if strings.Contains(line, "T-0002") {
			childLine = line
		}
	}

	if rootLine == "" || childLine == "" {
		t.Fatalf("could not find T-0001 or T-0002 in output:\n%s", out)
	}

	rootIndent := len(rootLine) - len(strings.TrimLeft(rootLine, " "))
	childIndent := len(childLine) - len(strings.TrimLeft(childLine, " "))
	if childIndent <= rootIndent {
		t.Errorf("child T-0002 (indent=%d) should be indented more than root T-0001 (indent=%d)", childIndent, rootIndent)
	}
}
