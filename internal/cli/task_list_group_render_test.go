package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

// groupRenderCmd builds a bare cobra command whose output is captured,
// so the rendering tests exercise the real formatTasks chokepoint rather
// than a test-only reimplementation of it.
func groupRenderCmd(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := &cobra.Command{Use: "list"}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	return cmd, &buf
}

// renderGroupedOutput runs the table path of formatTasks with the given
// grouping key and returns the rendered text.
func renderGroupedOutput(t *testing.T, tasks []*core.Task, key string) string {
	t.Helper()
	cmd, buf := groupRenderCmd(t)
	if err := formatTasks(cmd, tasks, formatTable, false, withGroupBy(key)); err != nil {
		t.Fatalf("formatTasks(group-by %q): %v", key, err)
	}
	return buf.String()
}

// TestFormatTasksUngroupedUnchanged is the regression guard for every
// existing user and script: with no grouping key, the table path must be
// byte-for-byte what it renders today. The reference is renderTable
// itself — the one function the ungrouped path has always called.
func TestFormatTasksUngroupedUnchanged(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "first", Status: core.StatusTodo, AssignedTo: strptr("ann")},
		{ID: "T-0002", Seq: 2, Title: "second", Status: core.StatusInProgress, AssignedTo: strptr("bob")},
	}

	var want bytes.Buffer
	renderTable(&want, tasks, nil)

	// No option at all — the shape every pre-existing caller uses.
	cmd, buf := groupRenderCmd(t)
	if err := formatTasks(cmd, tasks, formatTable, false); err != nil {
		t.Fatalf("formatTasks: %v", err)
	}
	if got := buf.String(); got != want.String() {
		t.Errorf("ungrouped output changed.\ngot:\n%q\nwant:\n%q", got, want.String())
	}

	// And with the option present but empty — the `task list` path when
	// --group-by was never set.
	cmd2, buf2 := groupRenderCmd(t)
	if err := formatTasks(cmd2, tasks, formatTable, false, withGroupBy("")); err != nil {
		t.Fatalf("formatTasks(empty group-by): %v", err)
	}
	if got := buf2.String(); got != want.String() {
		t.Errorf("empty --group-by altered output.\ngot:\n%q\nwant:\n%q", got, want.String())
	}
}

// TestFormatTasksGroupedTitledTables pins the rendered shape: one table
// per group, each under its group name, blank line between groups,
// headers repeated.
func TestFormatTasksGroupedTitledTables(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "alpha work", Status: core.StatusTodo, AssignedTo: strptr("ann")},
		{ID: "T-0002", Seq: 2, Title: "bob work", Status: core.StatusTodo, AssignedTo: strptr("bob")},
		{ID: "T-0003", Seq: 3, Title: "more ann", Status: core.StatusTodo, AssignedTo: strptr("ann")},
	}
	got := renderGroupedOutput(t, tasks, "assignee")

	// Group names render as headings, in group order.
	annAt := strings.Index(got, "ann")
	bobAt := strings.Index(got, "bob")
	if annAt < 0 || bobAt < 0 {
		t.Fatalf("both group names must appear; got:\n%s", got)
	}
	if annAt > bobAt {
		t.Errorf("groups must render in group order (ann before bob); got:\n%s", got)
	}

	// Headers repeat per table: two groups means the ID header twice.
	if n := strings.Count(got, "Title"); n != 2 {
		t.Errorf("headers must repeat per group table, got %d occurrences of Title:\n%s", n, got)
	}

	// Each task lands under its own group's table, in order.
	wantSeq := []string{"ann", "T-0001", "T-0003", "bob", "T-0002"}
	if !inOrder(got, wantSeq) {
		t.Errorf("expected sequence %v in output:\n%s", wantSeq, got)
	}

	// A blank line separates the groups.
	if !strings.Contains(got, "\n\n") {
		t.Errorf("groups must be separated by a blank line; got:\n%q", got)
	}
}

// TestFormatTasksGroupedFooterOnlyWhenCountsDiffer is the anti-"looks
// like a bug" rule. Tag grouping duplicates rows; the footer says so.
// When rows == distinct tasks the footer is noise and must be absent.
func TestFormatTasksGroupedFooterOnlyWhenCountsDiffer(t *testing.T) {
	t.Run("duplicated rows print the distinct count", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Seq: 1, Title: "multi", Status: core.StatusTodo, Tags: []string{"api", "urgent"}},
			{ID: "T-0002", Seq: 2, Title: "single", Status: core.StatusTodo, Tags: []string{"urgent"}},
		}
		got := renderGroupedOutput(t, tasks, "tag")
		want := "3 rows, 2 distinct tasks"
		if !strings.Contains(got, want) {
			t.Errorf("expected footer %q in:\n%s", want, got)
		}
	})

	t.Run("one row per task prints no footer", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Seq: 1, Title: "one", Status: core.StatusTodo, Tags: []string{"api"}},
			{ID: "T-0002", Seq: 2, Title: "two", Status: core.StatusTodo, Tags: []string{"urgent"}},
		}
		got := renderGroupedOutput(t, tasks, "tag")
		if strings.Contains(got, "distinct tasks") {
			t.Errorf("footer must be omitted when rows == distinct tasks; got:\n%s", got)
		}
		if strings.Contains(got, " rows,") {
			t.Errorf("no row-count footer expected; got:\n%s", got)
		}
	})

	t.Run("non-tag keys never duplicate, so never footer", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Seq: 1, Title: "a", Status: core.StatusTodo, AssignedTo: strptr("ann")},
			{ID: "T-0002", Seq: 2, Title: "b", Status: core.StatusTodo, AssignedTo: strptr("bob")},
		}
		got := renderGroupedOutput(t, tasks, "assignee")
		if strings.Contains(got, "distinct tasks") {
			t.Errorf("assignee grouping must not print a distinct-count footer; got:\n%s", got)
		}
	})
}

// TestFormatTasksGroupedHonoursColumns: the grouped path must reuse the
// existing column resolution rather than reimplement it, so a pruned
// column list applies to every group table. statusProvided=true is the
// cheapest column customization the resolver offers — it drops Status.
func TestFormatTasksGroupedHonoursColumns(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "alpha", Status: core.StatusTodo, AssignedTo: strptr("ann")},
		{ID: "T-0002", Seq: 2, Title: "beta", Status: core.StatusTodo, AssignedTo: strptr("bob")},
	}

	cmd, buf := groupRenderCmd(t)
	if err := formatTasks(cmd, tasks, formatTable, true, withGroupBy("assignee")); err != nil {
		t.Fatalf("formatTasks: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "Status") {
		t.Errorf("statusProvided must prune Status from every group table; got:\n%s", got)
	}
	if n := strings.Count(got, "Title"); n != 2 {
		t.Errorf("both group tables must carry the projected header, got %d:\n%s", n, got)
	}
}

// TestFormatTasksGroupedLeavesStructuredFormatsAlone: grouping must not
// reshape the formats that have no notion of a group.
//
// json and yaml are deliberately EXCLUDED: they nest under --group-by so
// a script consumes the same shape a human reads (see
// TestGroupedJSONNestsGroups), and their ungrouped payload is pinned
// unchanged by TestUngroupedJSONUnchanged. The formats listed here are
// the ones with nowhere to put a group name — tls is one line per task,
// and summary/counters are aggregates over the whole set.
func TestFormatTasksGroupedLeavesStructuredFormatsAlone(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "alpha", Status: core.StatusTodo, Tags: []string{"api", "urgent"}},
		{ID: "T-0002", Seq: 2, Title: "beta", Status: core.StatusTodo, Tags: []string{"urgent"}},
	}
	for _, format := range []string{"tls", formatSummary, formatCounters} {
		var plain bytes.Buffer
		cmdPlain, bufPlain := groupRenderCmd(t)
		if err := formatTasks(cmdPlain, tasks, format, false); err != nil {
			t.Fatalf("formatTasks(%s): %v", format, err)
		}
		plain.WriteString(bufPlain.String())

		cmdGrouped, bufGrouped := groupRenderCmd(t)
		if err := formatTasks(cmdGrouped, tasks, format, false, withGroupBy("tag")); err != nil {
			t.Fatalf("formatTasks(%s, grouped): %v", format, err)
		}
		if got := bufGrouped.String(); got != plain.String() {
			t.Errorf("format %s changed under --group-by.\ngot:\n%s\nwant:\n%s", format, got, plain.String())
		}
	}
}

// inOrder reports whether each needle appears in s after the previous one.
func inOrder(s string, needles []string) bool {
	at := 0
	for _, n := range needles {
		i := strings.Index(s[at:], n)
		if i < 0 {
			return false
		}
		at += i + len(n)
	}
	return true
}

// TestGroupTasksNoneIsTrailing: tasks with no value for the grouping
// dimension collect into a single "(none)" group, and that group renders
// LAST regardless of where its name would sort. Populated, named groups
// lead — a reader scanning for their own work should not wade past the
// unassigned pile to reach it.
func TestGroupTasksNoneIsTrailing(t *testing.T) {
	// Names chosen so alphabetical ordering would place "(none)" FIRST:
	// "(" sorts before every letter in ASCII. A grouper that sorted the
	// literal name instead of forcing the section last passes nothing here.
	cases := map[string][]*core.Task{
		"assignee": {
			{ID: "T-0001", AssignedTo: strptr("zoe")},
			{ID: "T-0002"},
			{ID: "T-0003", AssignedTo: strptr("ann")},
		},
		"track": {
			{ID: "T-0001", TrackID: strptr("zeta")},
			{ID: "T-0002"},
			{ID: "T-0003", TrackID: strptr("alpha")},
		},
		"project": {
			{ID: "T-0001", ProjectID: strptr("zzz")},
			{ID: "T-0002"},
			{ID: "T-0003", ProjectID: strptr("aaa")},
		},
		"tag": {
			{ID: "T-0001", Tags: []string{"zulu"}},
			{ID: "T-0002"},
			{ID: "T-0003", Tags: []string{"alpha"}},
		},
	}
	for key, tasks := range cases {
		got := groupNames(groupTasks(tasks, key))
		if len(got) != 3 {
			t.Errorf("groupTasks(_, %q) = %v, want 3 groups", key, got)
			continue
		}
		if got[len(got)-1] != noneGroupName {
			t.Errorf("groupTasks(_, %q) names = %v, want %q last", key, got, noneGroupName)
		}
		// The unset task is IN that group, not dropped.
		if ids := groupIDs(groupTasks(tasks, key), noneGroupName); strings.Join(ids, ",") != "T-0002" {
			t.Errorf("groupTasks(_, %q)[%s] = %v, want [T-0002]", key, noneGroupName, ids)
		}
		// The named groups keep their own ascending order ahead of it.
		if got[0] > got[1] {
			t.Errorf("named groups must stay ascending, got %v for %q", got, key)
		}
	}
}

// TestGroupTasksNoneFoldsUnsetStatusAndPriority: status and priority
// route the empty value through the SAME "(none)" treatment as the
// free-text keys. One notion of "unset" across every dimension — not a
// nameless trailing bucket for two of them.
func TestGroupTasksNoneFoldsUnsetStatusAndPriority(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Status: core.StatusDone},
			{ID: "T-0002", Status: ""},
			{ID: "T-0003", Status: core.StatusTodo},
		}
		got := groupNames(groupTasks(tasks, "status"))
		want := []string{"TODO", "DONE", noneGroupName}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("status groups = %v, want %v", got, want)
		}
	})

	t.Run("priority", func(t *testing.T) {
		tasks := []*core.Task{
			{ID: "T-0001", Priority: core.PriorityP2},
			{ID: "T-0002", Priority: ""},
			{ID: "T-0003", Priority: core.PriorityP0},
		}
		got := groupNames(groupTasks(tasks, "priority"))
		want := []string{"P0", "P2", noneGroupName}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("priority groups = %v, want %v", got, want)
		}
	})

	t.Run("unset still trails a value outside the vocabulary", func(t *testing.T) {
		// A stale row carrying a status the configured vocabulary no
		// longer knows must still render — and "(none)" must sit after
		// it, because "no status" is the weakest thing to say about a row.
		tasks := []*core.Task{
			{ID: "T-0001", Status: ""},
			{ID: "T-0002", Status: core.TaskStatus("ZZ_RETIRED")},
			{ID: "T-0003", Status: core.StatusTodo},
		}
		got := groupNames(groupTasks(tasks, "status"))
		want := []string{"TODO", "ZZ_RETIRED", noneGroupName}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("status groups = %v, want %v", got, want)
		}
	})
}

// TestFormatTasksAllNoneStillRendersTitledTable pins the decision for a
// listing where NOTHING carries the grouping value: it renders a single
// "(none)"-titled table, not a bare ungrouped one.
//
// Reasoning: the user explicitly typed --group-by. Degrading to a plain
// table produces output byte-identical to `tlc task list`, leaving them
// unable to tell whether the flag was ignored, mistyped, or correctly
// reported that nothing here has a track. The heading answers the
// question the command asked.
func TestFormatTasksAllNoneStillRendersTitledTable(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "orphan one", Status: core.StatusTodo},
		{ID: "T-0002", Seq: 2, Title: "orphan two", Status: core.StatusTodo},
	}
	got := renderGroupedOutput(t, tasks, "track")

	if !strings.Contains(got, noneGroupName) {
		t.Errorf("all-unset listing must still carry the %q heading; got:\n%s", noneGroupName, got)
	}
	// Exactly one section: one header row, both tasks under it.
	if n := strings.Count(got, "Title"); n != 1 {
		t.Errorf("all-unset listing must render ONE table, got %d headers:\n%s", n, got)
	}
	if !inOrder(got, []string{noneGroupName, "T-0001", "T-0002"}) {
		t.Errorf("both tasks must render under the heading:\n%s", got)
	}
	// And it must NOT be indistinguishable from the ungrouped listing.
	var plain bytes.Buffer
	renderTable(&plain, tasks, nil)
	if got == plain.String() {
		t.Error("all-unset grouping degraded to a plain ungrouped table; the flag would look ignored")
	}
}

// TestFormatTasksNoneHeadingRenders: the "(none)" group gets a real
// heading in rendered output, not the blank line an empty group name
// produced before.
func TestFormatTasksNoneHeadingRenders(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "owned", Status: core.StatusTodo, AssignedTo: strptr("ann")},
		{ID: "T-0002", Seq: 2, Title: "orphan", Status: core.StatusTodo},
	}
	got := renderGroupedOutput(t, tasks, "assignee")
	if !strings.Contains(got, noneGroupName) {
		t.Errorf("expected %q heading; got:\n%s", noneGroupName, got)
	}
	// Named group leads, "(none)" trails.
	if !inOrder(got, []string{"ann", "T-0001", noneGroupName, "T-0002"}) {
		t.Errorf("named group must lead and %q must trail; got:\n%s", noneGroupName, got)
	}
}
