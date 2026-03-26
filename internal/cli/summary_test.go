package cli

import (
	"bytes"
	"testing"

	"hop.top/tlc/internal/core"
)

func TestRenderSummary(t *testing.T) {
	t.Run("EmptyTasks", func(t *testing.T) {
		var buf bytes.Buffer
		renderSummary(&buf, nil)
		output := buf.String()
		if !contains(output, "No tasks found") {
			t.Errorf("expected 'No tasks found', got: %s", output)
		}
	})

	t.Run("SingleProjectMultipleStatuses", func(t *testing.T) {
		proj := "my-project"
		tasks := []*core.Task{
			{ID: "T-0001", Title: "Task 1", Status: core.StatusTodo, ProjectID: &proj},
			{ID: "T-0002", Title: "Task 2", Status: core.StatusTodo, ProjectID: &proj},
			{ID: "T-0003", Title: "Task 3", Status: core.StatusInProgress, ProjectID: &proj},
			{ID: "T-0004", Title: "Task 4", Status: core.StatusDone, ProjectID: &proj},
		}

		var buf bytes.Buffer
		renderSummary(&buf, tasks)
		output := buf.String()

		if !contains(output, "Project: my-project") {
			t.Errorf("expected project header, got: %s", output)
		}
		if !contains(output, "TODO") {
			t.Errorf("expected TODO status in output, got: %s", output)
		}
		if !contains(output, "IN_PROGRESS") {
			t.Errorf("expected IN_PROGRESS status in output, got: %s", output)
		}
		if !contains(output, "DONE") {
			t.Errorf("expected DONE status in output, got: %s", output)
		}
		if !contains(output, "Total") {
			t.Errorf("expected Total line, got: %s", output)
		}
		if !contains(output, "4") {
			t.Errorf("expected total count of 4, got: %s", output)
		}
	})

	t.Run("NoProjectTasks", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Title: "Orphan task", Status: core.StatusTodo},
		}

		var buf bytes.Buffer
		renderSummary(&buf, tasks)
		output := buf.String()

		if !contains(output, noProject) {
			t.Errorf("expected '%s' group, got: %s", noProject, output)
		}
	})

	t.Run("MultipleProjects", func(t *testing.T) {
		projA := "alpha"
		projB := "beta"
		tasks := []*core.Task{
			{ID: "T-0001", Title: "Alpha 1", Status: core.StatusTodo, ProjectID: &projA},
			{ID: "T-0002", Title: "Beta 1", Status: core.StatusDone, ProjectID: &projB},
			{ID: "T-0003", Title: "Beta 2", Status: core.StatusTodo, ProjectID: &projB},
		}

		var buf bytes.Buffer
		renderSummary(&buf, tasks)
		output := buf.String()

		if !contains(output, "Project: alpha") {
			t.Errorf("expected 'Project: alpha', got: %s", output)
		}
		if !contains(output, "Project: beta") {
			t.Errorf("expected 'Project: beta', got: %s", output)
		}
	})
}

func TestRenderCounters(t *testing.T) {
	t.Run("EmptyTasks", func(t *testing.T) {
		var buf bytes.Buffer
		renderCounters(&buf, nil)
		output := buf.String()
		if !contains(output, "No tasks found") {
			t.Errorf("expected 'No tasks found', got: %s", output)
		}
	})

	t.Run("ShowsStatusHeader", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Status: core.StatusTodo},
			{ID: "T-0002", Status: core.StatusDone},
		}

		var buf bytes.Buffer
		renderCounters(&buf, tasks)
		output := buf.String()

		if !contains(output, "Status counts:") {
			t.Errorf("expected 'Status counts:' header, got: %s", output)
		}
	})

	t.Run("FlatCountsAcrossProjects", func(t *testing.T) {
		projA := "alpha"
		projB := "beta"
		tasks := []*core.Task{
			{ID: "T-0001", Status: core.StatusTodo, ProjectID: &projA},
			{ID: "T-0002", Status: core.StatusTodo, ProjectID: &projB},
			{ID: "T-0003", Status: core.StatusInProgress, ProjectID: &projA},
			{ID: "T-0004", Status: core.StatusDone, ProjectID: &projB},
		}

		var buf bytes.Buffer
		renderCounters(&buf, tasks)
		output := buf.String()

		// Should NOT group by project
		if contains(output, "Project:") {
			t.Errorf("expected no project grouping, got: %s", output)
		}
		if !contains(output, "TODO") {
			t.Errorf("expected TODO in output, got: %s", output)
		}
		if !contains(output, "IN_PROGRESS") {
			t.Errorf("expected IN_PROGRESS in output, got: %s", output)
		}
		if !contains(output, "DONE") {
			t.Errorf("expected DONE in output, got: %s", output)
		}
	})

	t.Run("CountsAccurate", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Status: core.StatusTodo},
			{ID: "T-0002", Status: core.StatusTodo},
			{ID: "T-0003", Status: core.StatusTodo},
			{ID: "T-0004", Status: core.StatusDone},
		}

		var buf bytes.Buffer
		renderCounters(&buf, tasks)
		output := buf.String()

		if !contains(output, "3") {
			t.Errorf("expected count of 3 for TODO, got: %s", output)
		}
		if !contains(output, "1") {
			t.Errorf("expected count of 1 for DONE, got: %s", output)
		}
	})
}

func TestTaskListCountersFlag(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	proj := "test-proj"
	tasks := []*core.Task{
		{ID: "T-0001", Title: "Todo 1", Status: core.StatusTodo, ProjectID: &proj},
		{ID: "T-0002", Title: "Todo 2", Status: core.StatusTodo, ProjectID: &proj},
		{ID: "T-0003", Title: "Active", Status: core.StatusInProgress, ProjectID: &proj},
	}
	for _, task := range tasks {
		s.CreateTask(ctx, task)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "TODO", "--status", "IN_PROGRESS", "--counters"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --counters failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "Status counts:") {
		t.Errorf("expected 'Status counts:' header, got: %s", output)
	}
	if !contains(output, "TODO") {
		t.Errorf("expected TODO in output, got: %s", output)
	}
	if !contains(output, "IN_PROGRESS") {
		t.Errorf("expected IN_PROGRESS in output, got: %s", output)
	}
	// Should not show individual task titles
	if contains(output, "Todo 1") {
		t.Errorf("expected no task titles in counter output, got: %s", output)
	}
}

func TestGroupByProject(t *testing.T) {
	projA := "proj-a"
	tasks := []*core.Task{
		{ID: "T-0001", ProjectID: &projA},
		{ID: "T-0002", ProjectID: nil},
		{ID: "T-0003", ProjectID: &projA},
	}

	groups := groupByProject(tasks)

	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
	if len(groups["proj-a"]) != 2 {
		t.Errorf("expected 2 tasks in proj-a, got %d", len(groups["proj-a"]))
	}
	if len(groups[noProject]) != 1 {
		t.Errorf("expected 1 task in %s, got %d", noProject, len(groups[noProject]))
	}
}

func TestStatusCounts(t *testing.T) {
	tasks := []*core.Task{
		{Status: core.StatusTodo},
		{Status: core.StatusTodo},
		{Status: core.StatusDone},
	}

	counts := statusCounts(tasks)
	if counts["TODO"] != 2 {
		t.Errorf("expected 2 TODO, got %d", counts["TODO"])
	}
	if counts["DONE"] != 1 {
		t.Errorf("expected 1 DONE, got %d", counts["DONE"])
	}
}

func TestSummaryLine(t *testing.T) {
	tasks := []*core.Task{
		{Status: core.StatusTodo},
		{Status: core.StatusDone},
		{Status: core.StatusDone},
	}

	line := summaryLine(tasks)
	if !contains(line, "1 TODO") {
		t.Errorf("expected '1 TODO' in summary line, got: %s", line)
	}
	if !contains(line, "2 DONE") {
		t.Errorf("expected '2 DONE' in summary line, got: %s", line)
	}
	if !contains(line, "3 total") {
		t.Errorf("expected '3 total' in summary line, got: %s", line)
	}
}
