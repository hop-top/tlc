package core

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"hop.top/kit/go/runtime/domain"
)

func TestRecipeRun_IsDomainEntity(t *testing.T) {
	var _ domain.Entity = (*RecipeRun)(nil)
	if got := (RecipeRun{ID: "run_1"}).GetID(); got != "run_1" {
		t.Errorf("GetID() = %q; want run_1", got)
	}
}

// TestLogActions_RecipeEra pins the audit vocabulary the executor and the
// human-gate commands write, so the strings cannot drift between writer
// and any reader that greps the log.
func TestLogActions_RecipeEra(t *testing.T) {
	for name, got := range map[string]string{
		"APPROVED":  ActionApproved,
		"REJECTED":  ActionRejected,
		"RECLAIMED": ActionReclaimed,
		"RETRY":     ActionRetry,
	} {
		if got != name {
			t.Errorf("action %s = %q", name, got)
		}
	}
}

func mockIDs(tasks []*Task) []string {
	out := make([]string, len(tasks))
	for i, task := range tasks {
		out[i] = task.ID
	}
	sort.Strings(out)
	return out
}

// TestMockRepository_ListTasks_QueryFlags brings the mock in line with the
// SQL store for the flags the executor queries on, so executor unit tests
// on the mock exercise the same predicates as the real store.
func TestMockRepository_ListTasks_QueryFlags(t *testing.T) {
	m := NewMockRepository()
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	old, recent := now.Add(-2*time.Hour), now.Add(-5*time.Minute)
	reason := "waiting on infra"
	projA := "proj-a"

	seed := []*Task{
		{ID: "task_recipe_old", Status: StatusInProgress, RunID: "run_1", ClaimedAt: &old, ProjectID: &projA},
		{ID: "task_recipe_new", Status: StatusInProgress, RunID: "run_1", ClaimedAt: &recent, ProjectID: &projA},
		{ID: "task_plain", Status: StatusTodo, ProjectID: &projA},
		{ID: "task_blocked", Status: StatusTodo, BlockedReason: &reason, ProjectID: &projA},
		{ID: "task_archived", Status: StatusDone, Archived: true, ProjectID: &projA},
		{ID: "task_exec", Status: StatusTodo, Kind: TaskKindExec, RunID: "run_2"},
	}
	for _, task := range seed {
		if err := m.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}

	yes, no := true, false
	cutoff := now.Add(-time.Hour)
	cases := []struct {
		name  string
		query Query
		want  []string
	}{
		{"default excludes archived", Query{}, []string{"task_blocked", "task_exec", "task_plain", "task_recipe_new", "task_recipe_old"}},
		{"include archived", Query{IncludeArchived: true}, []string{"task_archived", "task_blocked", "task_exec", "task_plain", "task_recipe_new", "task_recipe_old"}},
		{"has run id", Query{HasRunID: &yes}, []string{"task_exec", "task_recipe_new", "task_recipe_old"}},
		{"no run id", Query{HasRunID: &no}, []string{"task_blocked", "task_plain"}},
		{"blocked", Query{Blocked: &yes}, []string{"task_blocked"}},
		{"not blocked", Query{Blocked: &no}, []string{"task_exec", "task_plain", "task_recipe_new", "task_recipe_old"}},
		{"claimed before", Query{ClaimedBefore: &cutoff}, []string{"task_recipe_old"}},
		{"kind filter", Query{Filters: []FieldFilter{{Field: "kind", Operator: OpEq, Value: "exec"}}}, []string{"task_exec"}},
		{"kind filter default agent", Query{Filters: []FieldFilter{{Field: "kind", Operator: OpEq, Value: "agent"}}}, []string{"task_blocked", "task_plain", "task_recipe_new", "task_recipe_old"}},
		{"run_id filter", Query{Filters: []FieldFilter{{Field: "run_id", Operator: OpEq, Value: "run_1"}}}, []string{"task_recipe_new", "task_recipe_old"}},
		{"project_id filter", Query{Filters: []FieldFilter{{Field: "project_id", Operator: OpEq, Value: "proj-a"}}}, []string{"task_blocked", "task_plain", "task_recipe_new", "task_recipe_old"}},
		{"project_id empty matches unscoped", Query{Filters: []FieldFilter{{Field: "project_id", Operator: OpEq, Value: ""}}}, []string{"task_exec"}},
	}
	for _, tc := range cases {
		got, err := m.ListTasks(ctx, tc.query)
		if err != nil {
			t.Fatalf("%s: ListTasks: %v", tc.name, err)
		}
		if !reflect.DeepEqual(mockIDs(got), tc.want) {
			t.Errorf("%s: ids = %v; want %v", tc.name, mockIDs(got), tc.want)
		}
	}
}

// TestMockRepository_ClaimTask mirrors the SQL compare-and-set so executor
// unit tests can exercise claim races on the mock.
func TestMockRepository_ClaimTask(t *testing.T) {
	m := NewMockRepository()
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 11, 0, 0, 0, time.UTC)
	projA := "proj-a"
	other := "someone-else"

	for _, task := range []*Task{
		{ID: "task_free", Status: StatusTodo, ProjectID: &projA},
		{ID: "task_owned", Status: StatusTodo, ProjectID: &projA, AssignedTo: &other},
	} {
		if err := m.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
	}
	var _ TaskClaimer = m

	won, err := m.ClaimTask(ctx, "task_free", "proj-a", StatusTodo, StatusInProgress, "executor-1", now)
	if err != nil || !won {
		t.Fatalf("first claim = (%v, %v); want (true, nil)", won, err)
	}
	got, _ := m.GetTask(ctx, "task_free")
	if got.Status != StatusInProgress || got.AssignedTo == nil || *got.AssignedTo != "executor-1" || got.ClaimedAt == nil || !got.ClaimedAt.Equal(now) {
		t.Errorf("claimed task = %+v; want IN_PROGRESS by executor-1 at %v", got, now)
	}

	for _, tc := range []struct {
		name, id, project, actor string
	}{
		{"second claimant", "task_free", "proj-a", "executor-2"},
		{"owned by another", "task_owned", "proj-a", "executor-1"},
		{"wrong project", "task_free", "proj-b", "executor-1"},
		{"missing", "task_nope", "proj-a", "executor-1"},
	} {
		won, err := m.ClaimTask(ctx, tc.id, tc.project, StatusTodo, StatusInProgress, tc.actor, now)
		if err != nil || won {
			t.Errorf("%s: claim = (%v, %v); want (false, nil)", tc.name, won, err)
		}
	}
}
