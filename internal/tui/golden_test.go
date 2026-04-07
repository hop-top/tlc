package tui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"hop.top/tlc/internal/core"
)

// updateGolden controls whether tests overwrite the golden files
// instead of comparing against them. Run with `-update` to refresh.
var updateGolden = flag.Bool("update", false,
	"update golden snapshot files in testdata/")

// goldenModel constructs a deterministic Model with seeded tasks for
// snapshot tests. Width/height are fixed so layout is reproducible.
func goldenModel(t *testing.T) Model {
	t.Helper()

	repo := core.NewMockRepository()
	logRepo := core.NewMockLogRepository()
	svc := core.NewTaskService(repo, logRepo)

	assignee := "alice"
	tasks := []*core.Task{
		{
			ID:         "T-0001",
			Title:      "Implement auth middleware",
			Status:     core.StatusTodo,
			AssignedTo: &assignee,
			Tags:       []string{"auth", "backend"},
			Reference:  "task://hop-top/tlc/T-0001",
		},
		{
			ID:        "T-0002",
			Title:     "Refactor task list",
			Status:    core.StatusInProgress,
			Tags:      []string{"refactor"},
			Reference: "task://hop-top/tlc/T-0002",
		},
		{
			ID:          "T-0003",
			Title:       "Add golden snapshot tests",
			Description: "Capture View() output for parity check.",
			Status:      core.StatusDone,
			Tags:        []string{"test"},
			Reference:   "task://hop-top/tlc/T-0003",
		},
	}

	m := NewModel(svc)
	m.tasks = tasks
	m.width = 80
	m.height = 24
	m.viewport.SetWidth(80)
	m.viewport.SetHeight(18)
	return m
}

// assertGolden compares the rendered output against a golden file at
// testdata/<name>.golden. With -update, the golden file is rewritten
// instead.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
	}
	if string(want) != got {
		t.Errorf("golden %s mismatch\n--- want ---\n%s\n--- got ---\n%s",
			name, string(want), got)
	}
}

func TestGoldenDashboardView(t *testing.T) {
	m := goldenModel(t)
	m.view = viewDashboard
	assertGolden(t, "dashboard", m.View().Content)
}

func TestGoldenDetailView(t *testing.T) {
	m := goldenModel(t)
	m.view = viewDetail
	m.selected = 0
	assertGolden(t, "detail", m.View().Content)
}

func TestGoldenKanbanView(t *testing.T) {
	m := goldenModel(t)
	m.view = viewKanban
	assertGolden(t, "kanban", m.View().Content)
}
