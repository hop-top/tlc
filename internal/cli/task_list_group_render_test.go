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

// TestFormatTasksGroupedLeavesStructuredFormatsAlone: grouping is a
// table-format concern only. json/yaml/tls/summary/counters must render
// exactly as they do ungrouped.
func TestFormatTasksGroupedLeavesStructuredFormatsAlone(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "alpha", Status: core.StatusTodo, Tags: []string{"api", "urgent"}},
		{ID: "T-0002", Seq: 2, Title: "beta", Status: core.StatusTodo, Tags: []string{"urgent"}},
	}
	for _, format := range []string{formatJSON, formatYAML, "tls", formatSummary, formatCounters} {
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
