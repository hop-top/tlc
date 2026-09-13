package storage

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// TestSQLiteStorage_Query_RunIDAndClaimedBefore pins the two predicates
// the executor's readiness and reclaim queries push down to SQL: strict
// mode keeps only recipe-born tasks (run_id set), and reclaim selects
// tasks claimed before a cutoff.
func TestSQLiteStorage_Query_RunIDAndClaimedBefore(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	recent := now.Add(-5 * time.Minute)

	seed := []*core.Task{
		{ID: "task_recipe_old", Title: "a", Status: core.StatusInProgress, RunID: "run_1", ClaimedAt: &old},
		{ID: "task_recipe_new", Title: "b", Status: core.StatusInProgress, RunID: "run_1", ClaimedAt: &recent},
		{ID: "task_plain", Title: "c", Status: core.StatusTodo},
		{ID: "task_plain_claimed", Title: "d", Status: core.StatusInProgress, ClaimedAt: &old},
	}
	for _, task := range seed {
		task.CreatedAt, task.UpdatedAt = now, now
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	ids := func(tasks []*core.Task) []string {
		out := make([]string, len(tasks))
		for i, task := range tasks {
			out[i] = task.ID
		}
		sort.Strings(out)
		return out
	}
	yes, no := true, false
	cutoff := now.Add(-time.Hour)
	cases := []struct {
		name  string
		query core.Query
		want  []string
	}{
		{"has run id", core.Query{HasRunID: &yes, AllProjects: true}, []string{"task_recipe_new", "task_recipe_old"}},
		{"no run id", core.Query{HasRunID: &no, AllProjects: true}, []string{"task_plain", "task_plain_claimed"}},
		{"claimed before cutoff", core.Query{ClaimedBefore: &cutoff, AllProjects: true}, []string{"task_plain_claimed", "task_recipe_old"}},
		{"stale recipe claims", core.Query{HasRunID: &yes, ClaimedBefore: &cutoff, AllProjects: true}, []string{"task_recipe_old"}},
	}
	for _, tc := range cases {
		got, err := s.ListTasks(ctx, tc.query)
		if err != nil {
			t.Fatalf("%s: ListTasks: %v", tc.name, err)
		}
		if !reflect.DeepEqual(ids(got), tc.want) {
			t.Errorf("%s: ids = %v; want %v", tc.name, ids(got), tc.want)
		}
	}

	// The same predicates reach the counts, which share the WHERE builder.
	n, err := s.CountTasks(ctx, core.Query{HasRunID: &yes, AllProjects: true})
	if err != nil {
		t.Fatalf("CountTasks: %v", err)
	}
	if n != 2 {
		t.Errorf("CountTasks(HasRunID) = %d; want 2", n)
	}
}
