package cli

// Coverage for `--group-limit`: the per-group row cap that keeps every
// group represented when one group would otherwise crowd out the rest.
//
// The knob only exists because `--limit` cannot do this job. `--limit`
// caps the MATCH SET fetched from the store, before grouping; a listing
// of 100 tasks where 95 belong to one track renders four other tracks as
// near-empty footnotes. `--group-limit 5` caps rows WITHIN each group
// instead, so every group gets its five lines.
//
// The two therefore interact, and the interaction is the thing worth
// pinning: once `--limit` has truncated the match set, a per-group total
// is a total of what was fetched, not of what exists. Reporting "5 of 12"
// off a truncated fetch states a number that is not the group's size.
// noteIgnoredPagination in task_list_aggregate.go documents the same
// hazard for counts, and the resolution here follows it: say the totals
// are partial, or say nothing.

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"hop.top/tlc/internal/core"
)

// groupLimitTasks builds n tasks all assigned to owner, ids sequential
// from the given base, so a truncation boundary is easy to assert on.
func groupLimitTasks(owner string, base, n int) []*core.Task {
	out := make([]*core.Task, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("T-%04d", base+i)
		out = append(out, &core.Task{
			ID: id, Seq: int64(base + i), Title: owner + " task",
			Status: core.StatusTodo, AssignedTo: strptr(owner),
		})
	}
	return out
}

// renderGroupLimited runs the table path with both options set.
func renderGroupLimited(t *testing.T, tasks []*core.Task, key string, opts ...listOption) string {
	t.Helper()
	cmd, buf := groupRenderCmd(t)
	all := append([]listOption{withGroupBy(key)}, opts...)
	if err := formatTasks(cmd, tasks, formatTable, false, all...); err != nil {
		t.Fatalf("formatTasks(group-by %q, limited): %v", key, err)
	}
	return buf.String()
}

// TestGroupLimitCapsRowsPerGroup is the core behavior: each group renders
// at most N rows, and the cap is applied per group rather than across the
// whole listing. A cap that leaked across groups would starve the later
// ones, which is the exact failure --group-limit exists to prevent.
func TestGroupLimitCapsRowsPerGroup(t *testing.T) {
	tasks := append(groupLimitTasks("ann", 1, 4), groupLimitTasks("bob", 11, 3)...)

	got := renderGroupLimited(t, tasks, "assignee", withGroupLimit(2))

	// Two rows per group, not two rows total.
	for _, id := range []string{"T-0001", "T-0002", "T-0011", "T-0012"} {
		if !strings.Contains(got, id) {
			t.Errorf("%s must be within the per-group cap; got:\n%s", id, got)
		}
	}
	for _, id := range []string{"T-0003", "T-0004", "T-0013"} {
		if strings.Contains(got, id) {
			t.Errorf("%s is past the cap and must be truncated away; got:\n%s", id, got)
		}
	}
	// Both groups still have a heading: representation is the point.
	if !inOrder(got, []string{"ann", "T-0001", "T-0002", "bob", "T-0011", "T-0012"}) {
		t.Errorf("both groups must render, capped, in order; got:\n%s", got)
	}
}

// TestGroupLimitHeadingCarriesShownOfTotal: a truncated group must say so
// in its own heading. Silently dropping rows under a flag the user typed
// is the same class of defect as a silently truncated count -- the
// output is well-formed and wrong, with nothing to read as a warning.
func TestGroupLimitHeadingCarriesShownOfTotal(t *testing.T) {
	tasks := append(groupLimitTasks("ann", 1, 5), groupLimitTasks("bob", 11, 2)...)

	got := renderGroupLimited(t, tasks, "assignee", withGroupLimit(2))

	if !strings.Contains(got, "ann (2 of 5)") {
		t.Errorf("truncated group heading must carry shown/total; got:\n%s", got)
	}
	// The untruncated group is NOT annotated: "2 of 2" is noise, and the
	// annotation has to mean "there is more" or it means nothing.
	if strings.Contains(got, "bob (2 of 2)") {
		t.Errorf("untruncated group must not be annotated; got:\n%s", got)
	}
}

// TestGroupLimitZeroAndNegativeAreNoCap: the flag's zero value must mean
// "unset", not "show nothing". A zero cap rendering empty tables would
// make a config-supplied default silently blank the listing.
func TestGroupLimitZeroAndNegativeAreNoCap(t *testing.T) {
	tasks := groupLimitTasks("ann", 1, 3)
	want := renderGroupedOutput(t, tasks, "assignee")
	for _, n := range []int{0, -1} {
		if got := renderGroupLimited(t, tasks, "assignee", withGroupLimit(n)); got != want {
			t.Errorf("withGroupLimit(%d) must not cap.\ngot:\n%s\nwant:\n%s", n, got, want)
		}
	}
}

// TestGroupLimitNeverTouchesUngroupedPath is the mutation guard named in
// the brief: --group-limit is meaningless without --group-by, and the
// ungrouped table must be byte-for-byte unchanged even if the option is
// somehow present. The command rejects the combination up front
// (TestGroupLimitWithoutGroupByIsRejected); this pins the renderer too,
// so the reject and the render cannot disagree.
func TestGroupLimitNeverTouchesUngroupedPath(t *testing.T) {
	tasks := groupLimitTasks("ann", 1, 5)

	var want bytes.Buffer
	renderTable(&want, tasks, nil)

	cmd, buf := groupRenderCmd(t)
	if err := formatTasks(cmd, tasks, formatTable, false, withGroupLimit(2)); err != nil {
		t.Fatalf("formatTasks: %v", err)
	}
	if got := buf.String(); got != want.String() {
		t.Errorf("--group-limit leaked into the ungrouped path.\ngot:\n%s\nwant:\n%s", got, want.String())
	}
}

// TestGroupLimitFooterCountsRenderedRows: the existing distinct-count
// footer reports what was RENDERED. Under a cap the rendered row count
// drops, and the footer must drop with it rather than reporting rows the
// reader cannot see.
func TestGroupLimitFooterCountsRenderedRows(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "multi", Status: core.StatusTodo, Tags: []string{"api", "urgent"}},
		{ID: "T-0002", Seq: 2, Title: "two", Status: core.StatusTodo, Tags: []string{"api", "urgent"}},
		{ID: "T-0003", Seq: 3, Title: "three", Status: core.StatusTodo, Tags: []string{"api"}},
	}
	// Uncapped: api has 3, urgent has 2 -> 5 rows over 3 distinct tasks.
	if got := renderGroupedOutput(t, tasks, "tag"); !strings.Contains(got, "5 rows, 3 distinct tasks") {
		t.Fatalf("fixture assumption broken; got:\n%s", got)
	}
	// Capped at 1 per group: each group keeps its first row, and that is
	// T-0001 in BOTH (it carries both tags). Two rows, one distinct task
	// — so the footer must report 2/1, the post-cap truth, never the
	// pre-cap 5/3 the reader can no longer see on screen.
	got := renderGroupLimited(t, tasks, "tag", withGroupLimit(1))
	if strings.Contains(got, "5 rows") {
		t.Errorf("footer must count RENDERED rows, not pre-cap rows; got:\n%s", got)
	}
	if !strings.Contains(got, "2 rows, 1 distinct tasks") {
		t.Errorf("footer must report the post-cap counts; got:\n%s", got)
	}
}

// TestGroupLimitFooterOmittedWhenCapRemovesDuplication: capping can make
// the duplication disappear entirely. Then the footer has nothing to
// explain and must be omitted rather than printed as "2 rows, 2 tasks".
func TestGroupLimitFooterOmittedWhenCapRemovesDuplication(t *testing.T) {
	tasks := []*core.Task{
		{ID: "T-0001", Seq: 1, Title: "multi", Status: core.StatusTodo, Tags: []string{"api"}},
		{ID: "T-0002", Seq: 2, Title: "two", Status: core.StatusTodo, Tags: []string{"api", "urgent"}},
	}
	// Uncapped: api has 2, urgent has 1 -> 3 rows over 2 distinct.
	if got := renderGroupedOutput(t, tasks, "tag"); !strings.Contains(got, "3 rows, 2 distinct tasks") {
		t.Fatalf("fixture assumption broken; got:\n%s", got)
	}
	// Capped at 1: api keeps T-0001, urgent keeps T-0002 -> 2 rows, 2
	// distinct, no duplication left to report.
	got := renderGroupLimited(t, tasks, "tag", withGroupLimit(1))
	if strings.Contains(got, "distinct tasks") {
		t.Errorf("rendered rows == distinct tasks, so no footer; got:\n%s", got)
	}
}

// TestGroupLimitPartialTotalsNote: when the match set itself was
// truncated by --limit, a per-group total counts only what was fetched.
// The renderer must say so instead of printing a number that reads as
// the group's real size.
func TestGroupLimitPartialTotalsNote(t *testing.T) {
	tasks := append(groupLimitTasks("ann", 1, 5), groupLimitTasks("bob", 11, 2)...)

	t.Run("partial match set is disclosed", func(t *testing.T) {
		got := renderGroupLimited(t, tasks, "assignee",
			withGroupLimit(2), withPartialMatchSet(true))
		if !strings.Contains(got, "partial") {
			t.Errorf("a --limit-truncated match set must be disclosed; got:\n%s", got)
		}
		if !strings.Contains(got, "--limit") {
			t.Errorf("the note must name --limit as the cause; got:\n%s", got)
		}
	})

	t.Run("complete match set prints no note", func(t *testing.T) {
		got := renderGroupLimited(t, tasks, "assignee", withGroupLimit(2))
		if strings.Contains(got, "partial") {
			t.Errorf("complete match set must print no partial-totals note; got:\n%s", got)
		}
	})

	t.Run("no note without a cap to qualify", func(t *testing.T) {
		// Nothing was truncated per group, so there is no per-group
		// total on screen for the note to qualify.
		got := renderGroupLimited(t, tasks, "assignee", withPartialMatchSet(true))
		if strings.Contains(got, "partial") {
			t.Errorf("no per-group totals rendered, so no note; got:\n%s", got)
		}
	})
}

// TestGroupLimitWithoutGroupByIsRejected: the flag caps rows within a
// group, so with no grouping there is nothing to cap. Accepting it
// silently would leave the user believing their listing was capped when
// it was not -- the flag would appear to work and do nothing.
//
// Driven through the real command so the config-supplied spelling is
// covered by the same gate as the typed flag.
func TestGroupLimitWithoutGroupByIsRejected(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = s.Close() }()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--group-limit", "3"})

	err = cmd.Execute()
	if err == nil {
		t.Fatalf("--group-limit without --group-by must fail; got success:\n%s", buf.String())
	}
	msg := err.Error()
	for _, want := range []string{"--group-limit", "--group-by"} {
		if !strings.Contains(msg, want) {
			t.Errorf("rejection must name %s; got: %s", want, msg)
		}
	}
}

// TestGroupLimitWithGroupByIsAccepted is the other half of the gate: the
// combination the flag exists for must run clean. Without this, a gate
// that rejected --group-limit unconditionally would pass the test above.
func TestGroupLimitWithGroupByIsAccepted(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = s.Close() }()

	for i, owner := range []string{"ann", "ann", "ann", "bob"} {
		task := &core.Task{
			ID: fmt.Sprintf("T-%04d", i+1), Seq: int64(i + 1),
			Title: "seeded", Status: core.StatusTodo, AssignedTo: strptr(owner),
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--group-by", "assignee", "--group-limit", "2"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--group-by with --group-limit must succeed: %v\n%s", err, buf.String())
	}
	got := buf.String()
	if !strings.Contains(got, "ann (2 of 3)") {
		t.Errorf("capped group must report shown/total; got:\n%s", got)
	}
}
