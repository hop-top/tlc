package storage

// CountTasksByStatus exists so aggregate output cannot depend on page size.
// These tests seed more tasks than the CLI's default --limit of 100 and
// assert the counts stay whole no matter what Limit/Offset the query
// carries -- a count that tracks the limit is the defect, not a variant.

import (
	"fmt"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

const (
	countsSeedDone = 90
	countsSeedTodo = 60
	// countsDefaultLimit mirrors `task list`'s --limit default. A count
	// landing here rather than on the seeded totals is the regression.
	countsDefaultLimit = 100
	countsProjectID    = "counts-fixture"
)

// newCountsStorage returns a storage handle over a temp DB, using the
// package's existing isolation pattern: viper reset, detection cache cleared,
// and cwd moved into the tempdir so project detection cannot resolve against
// the repo that invoked the test.
func newCountsStorage(t *testing.T) *SQLiteStorage {
	t.Helper()
	return newDomainRepoTestStorage(t)
}

// seedCounts inserts countsSeedDone DONE + countsSeedTodo TODO tasks,
// summing above countsDefaultLimit on purpose.
func seedCounts(t *testing.T, s *SQLiteStorage) {
	t.Helper()

	ctx := t.Context()
	total := countsSeedDone + countsSeedTodo
	for i := range total {
		status := core.StatusDone
		if i >= countsSeedDone {
			status = core.StatusTodo
		}
		pid := countsProjectID
		task := &core.Task{
			ID:        fmt.Sprintf("T-%04d", i+1),
			Title:     fmt.Sprintf("counts task %d", i+1),
			Status:    status,
			ProjectID: &pid,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %d: %v", i+1, err)
		}
	}
}

func TestCountTasksByStatus_IgnoresLimitAndOffset(t *testing.T) {
	s := newCountsStorage(t)
	seedCounts(t, s)
	ctx := t.Context()

	// Every one of these queries describes the same match set; only the
	// page differs. The counts must not.
	queries := map[string]core.Query{
		"NoPagination":      {AllProjects: true},
		"DefaultLimit":      {AllProjects: true, Limit: countsDefaultLimit},
		"TinyLimit":         {AllProjects: true, Limit: 5},
		"LimitAboveTotal":   {AllProjects: true, Limit: 1000},
		"OffsetOnly":        {AllProjects: true, Offset: 120},
		"LimitAndOffset":    {AllProjects: true, Limit: 10, Offset: 140},
		"SortingIrrelevant": {AllProjects: true, Limit: 7, SortBy: "created_at", SortDirection: "asc"},
	}

	for name, q := range queries {
		t.Run(name, func(t *testing.T) {
			counts, err := s.CountTasksByStatus(ctx, q)
			if err != nil {
				t.Fatalf("CountTasksByStatus: %v", err)
			}

			done := counts[string(core.StatusDone)]
			todo := counts[string(core.StatusTodo)]

			if done != countsSeedDone {
				t.Errorf("DONE = %d, want %d", done, countsSeedDone)
			}
			if todo != countsSeedTodo {
				t.Errorf("TODO = %d, want %d", todo, countsSeedTodo)
			}
			if done+todo == countsDefaultLimit {
				t.Errorf("counts sum to the page size %d, not the match set", countsDefaultLimit)
			}
		})
	}
}

// TestCountTasksByStatus_HonorsFilters proves the count is scoped by the
// same WHERE fragment ListTasks uses. Ignoring pagination must not mean
// ignoring the filters.
func TestCountTasksByStatus_HonorsFilters(t *testing.T) {
	s := newCountsStorage(t)
	seedCounts(t, s)
	ctx := t.Context()

	counts, err := s.CountTasksByStatus(ctx, core.Query{
		AllProjects: true,
		Limit:       countsDefaultLimit,
		Filters: []core.FieldFilter{
			{Field: "status", Value: string(core.StatusDone)},
		},
	})
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}

	if got := counts[string(core.StatusDone)]; got != countsSeedDone {
		t.Errorf("DONE = %d, want %d", got, countsSeedDone)
	}
	if got, ok := counts[string(core.StatusTodo)]; ok {
		t.Errorf("TODO present (%d) despite a DONE-only filter", got)
	}
}

// TestCountTasksByStatus_AgreesWithUnpaginatedList ties the SQL count to
// the slice path the CLI falls back to, so the two cannot drift.
func TestCountTasksByStatus_AgreesWithUnpaginatedList(t *testing.T) {
	s := newCountsStorage(t)
	seedCounts(t, s)
	ctx := t.Context()

	q := core.Query{AllProjects: true}

	counts, err := s.CountTasksByStatus(ctx, q)
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}

	tasks, err := s.ListTasks(ctx, q)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	fromSlice := make(map[string]int)
	for _, task := range tasks {
		fromSlice[string(task.Status)]++
	}

	if len(counts) != len(fromSlice) {
		t.Fatalf("status buckets differ: sql=%v slice=%v", counts, fromSlice)
	}
	for status, n := range fromSlice {
		if counts[status] != n {
			t.Errorf("%s: sql=%d slice=%d", status, counts[status], n)
		}
	}
}

// TestCountTasksByStatus_EmptyMatchSet checks the zero case returns an
// empty map rather than an error, so renderers can print "No tasks found".
func TestCountTasksByStatus_EmptyMatchSet(t *testing.T) {
	s := newCountsStorage(t)

	counts, err := s.CountTasksByStatus(t.Context(), core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected no buckets, got %v", counts)
	}
}

// TestCountTasksByProjectAndStatus_GroupsByProject pins the per-project
// grouping a cross-project --summary renders.
//
// A summary built from flat status counts had no project dimension to render,
// so outside a project every project collapsed into one bucket -- a
// plausible-looking number that was the sum of everything. Grouping in SQL
// gives the renderer the dimension it needs without materializing a row.
func TestCountTasksByProjectAndStatus_GroupsByProject(t *testing.T) {
	s := newCountsStorage(t)
	ctx := t.Context()

	seeds := []struct {
		id      string
		project *string
		status  core.TaskStatus
	}{
		{"T-0001", ptr("alpha"), core.StatusTodo},
		{"T-0002", ptr("alpha"), core.StatusTodo},
		{"T-0003", ptr("alpha"), core.StatusDone},
		{"T-0004", ptr("beta"), core.StatusDone},
		{"T-0005", nil, core.StatusTodo},
	}
	for _, seed := range seeds {
		if err := s.CreateTask(ctx, &core.Task{
			ID: seed.id, Title: seed.id, Status: seed.status, ProjectID: seed.project,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}

	byProject, err := s.CountTasksByProjectAndStatus(ctx, core.Query{
		AllProjects: true,
		// A limit that would have truncated, to prove grouping does not
		// reintroduce pagination.
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("CountTasksByProjectAndStatus: %v", err)
	}

	if got := byProject["alpha"][string(core.StatusTodo)]; got != 2 {
		t.Errorf("alpha TODO = %d, want 2", got)
	}
	if got := byProject["alpha"][string(core.StatusDone)]; got != 1 {
		t.Errorf("alpha DONE = %d, want 1", got)
	}
	if got := byProject["beta"][string(core.StatusDone)]; got != 1 {
		t.Errorf("beta DONE = %d, want 1", got)
	}
	// Project-less rows get their own bucket under the empty key, not
	// folded into a named project.
	if got := byProject[""][string(core.StatusTodo)]; got != 1 {
		t.Errorf("no-project TODO = %d, want 1", got)
	}
	// The collapse's signature: every row in a single bucket.
	if len(byProject) != 3 {
		t.Errorf("got %d project buckets, want 3: %v", len(byProject), byProject)
	}
}

// TestCountTasksByProjectAndStatus_AgreesWithByStatus ties the two count
// shapes together: summing the per-project counts must reproduce the flat
// ones, or --counters and --summary would disagree over the same query.
func TestCountTasksByProjectAndStatus_AgreesWithByStatus(t *testing.T) {
	s := newCountsStorage(t)
	seedCounts(t, s)
	ctx := t.Context()

	q := core.Query{AllProjects: true}

	flat, err := s.CountTasksByStatus(ctx, q)
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}
	byProject, err := s.CountTasksByProjectAndStatus(ctx, q)
	if err != nil {
		t.Fatalf("CountTasksByProjectAndStatus: %v", err)
	}

	summed := make(map[string]int)
	for _, counts := range byProject {
		for status, n := range counts {
			summed[status] += n
		}
	}
	if len(summed) != len(flat) {
		t.Fatalf("status buckets differ: flat=%v summed=%v", flat, summed)
	}
	for status, n := range flat {
		if summed[status] != n {
			t.Errorf("%s: flat=%d summed=%d", status, n, summed[status])
		}
	}
}

// TestQueryOverdue_IsOneSQLPredicate pins the overdue definition in the
// WHERE fragment: past due AND still open.
//
// The status exclusion cannot travel through Query.Filters -- those are
// OR-joined per field, so a DONE/SKIPPED exclusion added there would widen
// the match set rather than narrow it. Query.Overdue carries both halves so
// `task list --overdue`, `tlc status`, and `--counters --overdue` cannot mean
// different rows.
func TestQueryOverdue_IsOneSQLPredicate(t *testing.T) {
	s := newCountsStorage(t)
	ctx := t.Context()

	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)

	seeds := []struct {
		id     string
		status core.TaskStatus
		due    *time.Time
	}{
		{"T-0001", core.StatusTodo, &past},       // overdue
		{"T-0002", core.StatusInProgress, &past}, // overdue
		{"T-0003", core.StatusDone, &past},       // finished, so not late
		{"T-0004", core.StatusSkipped, &past},    // skipped, so not late
		{"T-0005", core.StatusTodo, &future},     // not due yet
		{"T-0006", core.StatusTodo, nil},         // no due date at all
	}
	for _, seed := range seeds {
		if err := s.CreateTask(ctx, &core.Task{
			ID: seed.id, Title: seed.id, Status: seed.status, DueAt: seed.due,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}

	now := time.Now().UTC()
	q := core.Query{AllProjects: true, Overdue: &now}

	count, err := s.CountTasks(ctx, q)
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if count != 2 {
		t.Errorf("overdue count = %d, want 2 (open and past due only)", count)
	}

	// The list must select the same rows the count counted.
	tasks, err := s.ListTasks(ctx, q)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != count {
		t.Errorf("ListTasks returned %d rows but CountTasks said %d", len(tasks), count)
	}
	for _, task := range tasks {
		if task.Status == core.StatusDone || task.Status == core.StatusSkipped {
			t.Errorf("%s is %s and must not be overdue", task.ID, task.Status)
		}
	}

	// Per-status counts over the same predicate carry no closed statuses.
	counts, err := s.CountTasksByStatus(ctx, q)
	if err != nil {
		t.Fatalf("CountTasksByStatus: %v", err)
	}
	for _, closed := range []core.TaskStatus{core.StatusDone, core.StatusSkipped} {
		if n, ok := counts[string(closed)]; ok {
			t.Errorf("%s present (%d) in overdue counts", closed, n)
		}
	}
}

// TestQueryBlocked_IsOneSQLPredicate pins --blocked as a column predicate:
// blocked_reason present and non-empty.
func TestQueryBlocked_IsOneSQLPredicate(t *testing.T) {
	s := newCountsStorage(t)
	ctx := t.Context()

	reason := "waiting on review"
	empty := ""
	for _, seed := range []struct {
		id     string
		reason *string
	}{
		{"T-0001", &reason},
		{"T-0002", &reason},
		{"T-0003", &empty}, // set but empty: not blocked
		{"T-0004", nil},    // unset: not blocked
	} {
		if err := s.CreateTask(ctx, &core.Task{
			ID: seed.id, Title: seed.id, Status: core.StatusTodo, BlockedReason: seed.reason,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}

	yes, no := true, false

	blocked, err := s.CountTasks(ctx, core.Query{AllProjects: true, Blocked: &yes})
	if err != nil {
		t.Fatalf("CountTasks(blocked): %v", err)
	}
	if blocked != 2 {
		t.Errorf("blocked count = %d, want 2", blocked)
	}

	unblocked, err := s.CountTasks(ctx, core.Query{AllProjects: true, Blocked: &no})
	if err != nil {
		t.Fatalf("CountTasks(unblocked): %v", err)
	}
	if unblocked != 2 {
		t.Errorf("unblocked count = %d, want 2 (empty string and NULL both count as unblocked)", unblocked)
	}

	// The predicate agrees with the Go-side definition it replaced.
	tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true, Blocked: &yes})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	for _, task := range tasks {
		if !task.IsBlocked() {
			t.Errorf("%s selected by the SQL predicate but IsBlocked() is false", task.ID)
		}
	}
}

// TestCountTasks_HonorsTemporalFilters is the F7 regression.
//
// CountTasks hand-rolled its own filter copy and silently ignored
// DueBefore/DueAfter/HasDue, so a query narrowed by a due date counted every
// row instead. It shares the WHERE fragment now, so every predicate the list
// honors narrows the count too.
func TestCountTasks_HonorsTemporalFilters(t *testing.T) {
	s := newCountsStorage(t)
	ctx := t.Context()

	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)
	now := time.Now().UTC()

	for _, seed := range []struct {
		id  string
		due *time.Time
	}{
		{"T-0001", &past},
		{"T-0002", &past},
		{"T-0003", &future},
		{"T-0004", nil},
		{"T-0005", nil},
	} {
		if err := s.CreateTask(ctx, &core.Task{
			ID: seed.id, Title: seed.id, Status: core.StatusTodo, DueAt: seed.due,
		}); err != nil {
			t.Fatalf("seed %s: %v", seed.id, err)
		}
	}

	hasDue, noDue := true, false
	cases := map[string]struct {
		query core.Query
		want  int
	}{
		"NoFilter":  {core.Query{AllProjects: true}, 5},
		"DueBefore": {core.Query{AllProjects: true, DueBefore: &now}, 2},
		"DueAfter":  {core.Query{AllProjects: true, DueAfter: &now}, 1},
		"HasDue":    {core.Query{AllProjects: true, HasDue: &hasDue}, 3},
		"NoDue":     {core.Query{AllProjects: true, HasDue: &noDue}, 2},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := s.CountTasks(ctx, tc.query)
			if err != nil {
				t.Fatalf("CountTasks: %v", err)
			}
			if got != tc.want {
				t.Errorf("count = %d, want %d -- the predicate was ignored", got, tc.want)
			}
			// A count that ignores its filter reports the whole table.
			if tc.want != 5 && got == 5 {
				t.Errorf("count equals the full table; the filter had no effect")
			}
		})
	}
}

// TestCountTasks_AgreesWithListTasks ties the scalar count to the list over
// the same query, so the shared fragment cannot drift between them.
func TestCountTasks_AgreesWithListTasks(t *testing.T) {
	s := newCountsStorage(t)
	seedCounts(t, s)
	ctx := t.Context()

	for name, q := range map[string]core.Query{
		"All":       {AllProjects: true},
		"DoneOnly":  {AllProjects: true, Filters: []core.FieldFilter{{Field: "status", Value: string(core.StatusDone)}}},
		"Search":    {AllProjects: true, Search: "counts task 1"},
		"WithLimit": {AllProjects: true, Limit: 5},
	} {
		t.Run(name, func(t *testing.T) {
			count, err := s.CountTasks(ctx, q)
			if err != nil {
				t.Fatalf("CountTasks: %v", err)
			}

			// The list is compared without pagination: the count
			// deliberately ignores Limit, so a paginated list would be
			// the wrong comparand.
			unpaged := q
			unpaged.Limit = 0
			unpaged.Offset = 0
			tasks, err := s.ListTasks(ctx, unpaged)
			if err != nil {
				t.Fatalf("ListTasks: %v", err)
			}
			if count != len(tasks) {
				t.Errorf("CountTasks=%d, unpaginated ListTasks=%d", count, len(tasks))
			}
		})
	}
}

func ptr(s string) *string { return &s }
