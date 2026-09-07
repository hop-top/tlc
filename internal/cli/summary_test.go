package cli

import (
	"bytes"
	"strings"
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

// TestRenderFromCounts covers the counts-map renderers the aggregate paths
// use. These take counts computed in the store, so a slice is never walked
// and the totals cannot depend on page size.
func TestRenderFromCounts(t *testing.T) {
	t.Run("CountersSumsAndSorts", func(t *testing.T) {
		var buf bytes.Buffer
		renderCountersFromCounts(&buf, map[string]int{"TODO": 60, "DONE": 90})
		out := buf.String()

		if !contains(out, "Status counts:") {
			t.Errorf("missing header, got: %s", out)
		}
		// Statuses render alphabetically, so DONE precedes TODO.
		if strings.Index(out, "DONE") > strings.Index(out, "TODO") {
			t.Errorf("expected DONE before TODO, got: %s", out)
		}
		for _, want := range []string{"90", "60"} {
			if !contains(out, want) {
				t.Errorf("expected count %s, got: %s", want, out)
			}
		}
	})

	t.Run("CountersEmpty", func(t *testing.T) {
		var buf bytes.Buffer
		renderCountersFromCounts(&buf, map[string]int{})
		if !contains(buf.String(), "No tasks found") {
			t.Errorf("expected 'No tasks found', got: %s", buf.String())
		}
	})

	t.Run("SummaryTotalIsSumOfCounts", func(t *testing.T) {
		var buf bytes.Buffer
		renderSummaryFromCounts(&buf, map[string]map[string]int{
			"agg-fixture": {"DONE": 90, "TODO": 60},
		})
		out := buf.String()

		if !contains(out, "Project: agg-fixture") {
			t.Errorf("missing project header, got: %s", out)
		}
		// 150 is the sum, and deliberately above the default --limit of
		// 100 so a page-scoped total would be visible here.
		if !contains(out, "150") {
			t.Errorf("expected Total of 150, got: %s", out)
		}
	})

	t.Run("SummaryPerProjectTotals", func(t *testing.T) {
		var buf bytes.Buffer
		renderSummaryFromCounts(&buf, map[string]map[string]int{
			"alpha": {"TODO": 2},
			"beta":  {"DONE": 3},
		})
		out := buf.String()

		if strings.Index(out, "Project: alpha") > strings.Index(out, "Project: beta") {
			t.Errorf("expected alpha before beta, got: %s", out)
		}
		// Each project totals independently; neither sees the other's rows.
		if strings.Count(out, "Total") != 2 {
			t.Errorf("expected one Total per project, got: %s", out)
		}
	})

	t.Run("SummaryEmpty", func(t *testing.T) {
		var buf bytes.Buffer
		renderSummaryFromCounts(&buf, map[string]map[string]int{})
		if !contains(buf.String(), "No tasks found") {
			t.Errorf("expected 'No tasks found', got: %s", buf.String())
		}
	})
}

// TestCanCountInStore pins which aggregate queries may be served by a
// single SQL COUNT. Getting this wrong does not merely slow things down:
// counting in the store while a Go-side filter is pending would count rows
// that filter is about to discard.
func TestCanCountInStore(t *testing.T) {
	t.Cleanup(resetTaskFlags)

	t.Run("PlainCountersQualifies", func(t *testing.T) {
		resetTaskFlags()
		if !canCountInStore(formatCounters, core.Query{}, nil) {
			t.Error("plain --counters should count in the store")
		}
	})

	t.Run("PostQueryFiltersDisqualify", func(t *testing.T) {
		for name, set := range map[string]func(){
			"stale":     func() { taskListStale = true },
			"blocked":   func() { taskListBlocked = true },
			"overdue":   func() { taskListOverdue = true },
			"blockedBy": func() { taskListBlockedBy = []string{"T-0001"} },
		} {
			resetTaskFlags()
			set()
			if canCountInStore(formatCounters, core.Query{}, nil) {
				t.Errorf("--%s is filtered in Go; store count would over-count", name)
			}
		}
	})

	t.Run("QualifiedTracksDisqualify", func(t *testing.T) {
		resetTaskFlags()
		qids := []core.QualifiedTrackID{{TrackID: "some-track"}}
		if canCountInStore(formatCounters, core.Query{}, qids) {
			t.Error("qualified track IDs are filtered in Go; store count would over-count")
		}
	})

	t.Run("SummaryAllProjectsDisqualifies", func(t *testing.T) {
		resetTaskFlags()
		// Flat status counts carry no project dimension, so the grouped
		// summary cannot split them across projects.
		if canCountInStore(formatSummary, core.Query{AllProjects: true}, nil) {
			t.Error("--summary --all-projects needs per-project counts")
		}
		// Counters are flat, so the same query is fine for them.
		if !canCountInStore(formatCounters, core.Query{AllProjects: true}, nil) {
			t.Error("--counters is flat; --all-projects should still count in the store")
		}
	})
}

// TestResolveAggregateFormat pins the flag precedence the pre-query
// resolution now depends on.
func TestResolveAggregateFormat(t *testing.T) {
	t.Cleanup(resetTaskFlags)

	resetTaskFlags()
	if got := resolveAggregateFormat(); got != "" {
		t.Errorf("no flags: got %q, want \"\"", got)
	}

	resetTaskFlags()
	taskListSummary = true
	if got := resolveAggregateFormat(); got != formatSummary {
		t.Errorf("--summary: got %q, want %q", got, formatSummary)
	}

	resetTaskFlags()
	taskListCounters = true
	if got := resolveAggregateFormat(); got != formatCounters {
		t.Errorf("--counters: got %q, want %q", got, formatCounters)
	}

	// Both: --counters wins, matching the pre-existing tail-end order.
	resetTaskFlags()
	taskListSummary = true
	taskListCounters = true
	if got := resolveAggregateFormat(); got != formatCounters {
		t.Errorf("both flags: got %q, want %q", got, formatCounters)
	}
}
