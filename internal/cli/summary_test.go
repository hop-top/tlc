package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
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

// TestAggregateFormat pins every spelling of an aggregate to the same
// answer. The bool flags were the only spelling recognized before, so
// `--format summary`, `-f counters` and a config `output.format` slipped
// past the pagination clearing and reported the page size as the count —
// silently, at exit 0. A spelling missing from this table is a spelling
// that can regress the same way.
func TestAggregateFormat(t *testing.T) {
	t.Cleanup(func() {
		resetTaskFlags()
		viper.Reset()
	})

	cases := []struct {
		name         string
		flags        func()
		outputFormat string
		want         string
	}{
		{"NoFlags", func() {}, "table", ""},
		{"SummaryFlag", func() { taskListSummary = true }, "table", formatSummary},
		{"CountersFlag", func() { taskListCounters = true }, "table", formatCounters},
		// Both flags: --counters wins, matching the pre-existing
		// tail-end format order.
		{"BothFlags", func() {
			taskListSummary = true
			taskListCounters = true
		}, "table", formatCounters},
		// The format spellings. --format summary and -f summary are the
		// same flag, so output.format covers both.
		{"FormatSummary", func() {}, formatSummary, formatSummary},
		{"FormatCounters", func() {}, formatCounters, formatCounters},
		// A config output.format reaches the renderers by the same
		// route as the flag, so it must resolve the same way.
		{"ConfigSummary", func() {}, formatSummary, formatSummary},
		// Non-aggregate formats stay off the counting path.
		{"FormatJSON", func() {}, formatJSON, ""},
		{"FormatYAML", func() {}, formatYAML, ""},
		{"FormatTLS", func() {}, "tls", ""},
		{"FormatVtodo", func() {}, formatVtodo, ""},
		{"FormatEmpty", func() {}, "", ""},
		// A flag alongside a conflicting output.format: the flag wins,
		// as it always has.
		{"SummaryFlagOverJSON", func() { taskListSummary = true }, formatJSON, formatSummary},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetTaskFlags()
			viper.Reset()
			viper.Set("output.format", tc.outputFormat)
			tc.flags()

			if got := aggregateFormat(); got != tc.want {
				t.Errorf("aggregateFormat() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestAggregateFormatValue pins the output.format mapping on its own, since
// `task stale` consults it directly to decide whether to lift its scan cap.
func TestAggregateFormatValue(t *testing.T) {
	for value, want := range map[string]string{
		formatSummary:  formatSummary,
		formatCounters: formatCounters,
		formatJSON:     "",
		formatYAML:     "",
		formatVtodo:    "",
		"table":        "",
		"tls":          "",
		"":             "",
		"SUMMARY":      "",
	} {
		if got := aggregateFormatValue(value); got != want {
			t.Errorf("aggregateFormatValue(%q) = %q, want %q", value, got, want)
		}
	}
}

// TestTaskListPostFilters pins the gate for the store-count fast path.
//
// It is the same list the filters are applied from, deliberately: an
// allowlist of flag names has to mirror the filter block by hand, and the
// next Go-side filter added would be missing from the copy — over-counting
// silently, the exact defect class the counting path exists to fix. So the
// assertion here is that every Go-side filter appears, and that the flags
// now expressed in SQL do not.
func TestTaskListPostFilters(t *testing.T) {
	t.Cleanup(resetTaskFlags)

	t.Run("NoneWhenUnfiltered", func(t *testing.T) {
		resetTaskFlags()
		if got := taskListPostFilters(nil); len(got) != 0 {
			t.Errorf("unfiltered run has %d post-filters, want 0", len(got))
		}
	})

	t.Run("GoSideFiltersAppear", func(t *testing.T) {
		for name, set := range map[string]func(){
			"stale":     func() { taskListStale = true },
			"blockedBy": func() { taskListBlockedBy = []string{"T-0001"} },
		} {
			resetTaskFlags()
			set()
			if got := taskListPostFilters(nil); len(got) == 0 {
				t.Errorf("--%s is filtered in Go but contributes no post-filter; store count would over-count", name)
			}
		}
	})

	t.Run("QualifiedTracksAppear", func(t *testing.T) {
		resetTaskFlags()
		qids := []core.QualifiedTrackID{{TrackID: "some-track"}}
		if got := taskListPostFilters(qids); len(got) == 0 {
			t.Error("qualified track IDs are filtered in Go but contribute no post-filter")
		}
	})

	t.Run("SQLFiltersDoNotAppear", func(t *testing.T) {
		// --blocked and --overdue are column predicates now, pushed
		// into the WHERE fragment. If either reappears here the
		// pushdown regressed and the fast path lost cases it can serve.
		for name, set := range map[string]func(){
			"blocked": func() { taskListBlocked = true },
			"overdue": func() { taskListOverdue = true },
		} {
			resetTaskFlags()
			set()
			if got := taskListPostFilters(nil); len(got) != 0 {
				t.Errorf("--%s is expressed in SQL but still adds %d post-filter(s)", name, len(got))
			}
		}
	})

	t.Run("SummaryAllProjectsStillQualifies", func(t *testing.T) {
		// Counts group by project in SQL, so a cross-project summary no
		// longer has to materialize every row to bucket it.
		resetTaskFlags()
		taskListSummary = true
		taskListAllProjects = true
		if got := taskListPostFilters(nil); len(got) != 0 {
			t.Errorf("--summary --all-projects should stay on the counting path, got %d post-filter(s)", len(got))
		}
	})
}

// TestApplyPostFilters checks the conjunction: a task survives only when
// every filter accepts it.
func TestApplyPostFilters(t *testing.T) {
	blocked := "waiting on review"
	tasks := []*core.Task{
		{ID: "T-0001", Status: core.StatusTodo},
		{ID: "T-0002", Status: core.StatusTodo, BlockedReason: &blocked},
		{ID: "T-0003", Status: core.StatusDone, BlockedReason: &blocked},
	}

	t.Run("NoFiltersPassesEverything", func(t *testing.T) {
		got := applyPostFilters(append([]*core.Task(nil), tasks...), nil)
		if len(got) != len(tasks) {
			t.Errorf("got %d tasks, want %d", len(got), len(tasks))
		}
	})

	t.Run("AllFiltersMustAccept", func(t *testing.T) {
		got := applyPostFilters(append([]*core.Task(nil), tasks...), []taskPostFilter{
			func(task *core.Task) bool { return task.IsBlocked() },
			func(task *core.Task) bool { return task.Status == core.StatusTodo },
		})
		if len(got) != 1 || got[0].ID != "T-0002" {
			t.Errorf("got %v, want only T-0002", got)
		}
	})
}

// TestRenderAggregateCounts pins how per-project counts reach each renderer.
func TestRenderAggregateCounts(t *testing.T) {
	byProject := map[string]map[string]int{
		"alpha": {"TODO": 2, "DONE": 1},
		"beta":  {"TODO": 3},
		"":      {"DONE": 4},
	}

	t.Run("SummaryKeepsProjectGrouping", func(t *testing.T) {
		cmd := newTestCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		if err := renderAggregateCounts(cmd, formatSummary, byProject); err != nil {
			t.Fatalf("renderAggregateCounts: %v", err)
		}
		out := buf.String()

		// One block per project, with the store's empty project key
		// rendered under the same label the slice path uses.
		if n := strings.Count(out, "Project: "); n != 3 {
			t.Errorf("got %d project blocks, want 3\n%s", n, out)
		}
		if !contains(out, "Project: "+noProject) {
			t.Errorf("missing %q block\n%s", noProject, out)
		}
	})

	t.Run("CountersFlattensAcrossProjects", func(t *testing.T) {
		cmd := newTestCmd()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		if err := renderAggregateCounts(cmd, formatCounters, byProject); err != nil {
			t.Fatalf("renderAggregateCounts: %v", err)
		}
		out := buf.String()

		if contains(out, "Project:") {
			t.Errorf("counters must not group by project\n%s", out)
		}
		// TODO sums 2+3, DONE sums 1+4 -- flattened, not per-project.
		for _, want := range []string{"TODO", "5", "DONE"} {
			if !contains(out, want) {
				t.Errorf("missing %q\n%s", want, out)
			}
		}
	})

	t.Run("UnknownFormatErrors", func(t *testing.T) {
		err := renderAggregateCounts(newTestCmd(), "bogus", byProject)
		if err == nil {
			t.Fatal("expected an error for an unknown aggregate format")
		}
		if !contains(err.Error(), "summary") || !contains(err.Error(), "counters") {
			t.Errorf("error should name the valid values, got: %v", err)
		}
	})
}

// TestFlattenAndLabelProjectCounts pins the two count reshapers.
func TestFlattenAndLabelProjectCounts(t *testing.T) {
	byProject := map[string]map[string]int{
		"alpha": {"TODO": 2},
		"beta":  {"TODO": 3, "DONE": 1},
	}

	flat := flattenProjectCounts(byProject)
	if flat["TODO"] != 5 || flat["DONE"] != 1 {
		t.Errorf("flatten = %v, want TODO=5 DONE=1", flat)
	}

	labeled := labelProjectCounts(map[string]map[string]int{"": {"TODO": 1}})
	if _, ok := labeled[noProject]; !ok {
		t.Errorf("empty project key not relabelled: %v", labeled)
	}
	if _, ok := labeled[""]; ok {
		t.Errorf("empty project key survived relabelling: %v", labeled)
	}
}

// TestSumCounts pins the fold that replaced four hand-rolled copies.
func TestSumCounts(t *testing.T) {
	if got := sumCounts(nil); got != 0 {
		t.Errorf("sumCounts(nil) = %d, want 0", got)
	}
	if got := sumCounts(map[string]int{"TODO": 60, "DONE": 90}); got != 150 {
		t.Errorf("sumCounts = %d, want 150", got)
	}
}
