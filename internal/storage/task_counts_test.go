package storage

// CountTasksByStatus exists so aggregate output cannot depend on page size.
// These tests seed more tasks than the CLI's default --limit of 100 and
// assert the counts stay whole no matter what Limit/Offset the query
// carries -- a count that tracks the limit is the defect, not a variant.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

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

// newCountsStorage returns a storage handle over a temp DB. GIT_* vars are
// cleared for the test's lifetime: project detection consults git, and an
// inherited GIT_DIR would scope the implicit project filter to whatever
// repo invoked the test.
func newCountsStorage(t *testing.T) *SQLiteStorage {
	t.Helper()

	for _, e := range os.Environ() {
		if k, _, _ := splitEnv(e); len(k) > 4 && k[:4] == "GIT_" {
			t.Setenv(k, "")
			_ = os.Unsetenv(k)
		}
	}

	s, err := NewSQLiteStorage(filepath.Join(t.TempDir(), "counts.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func splitEnv(e string) (key, val string, ok bool) {
	for i := range len(e) {
		if e[i] == '=' {
			return e[:i], e[i+1:], true
		}
	}
	return e, "", false
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
