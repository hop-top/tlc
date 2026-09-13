package storage

import (
	"context"
	"reflect"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// ledgerRun builds a fully-populated run so a round trip that drops a
// field fails loudly. CreatedAt is set in a non-UTC zone to pin the UTC
// normalisation every other timestamp column already has.
func ledgerRun(id, trackID string, createdAt time.Time) *core.RecipeRun {
	return &core.RecipeRun{
		ID:          id,
		ProjectID:   "proj-a",
		RecipeID:    "code-review",
		Version:     "1.2.0",
		Hash:        "sha256:abc",
		Vars:        map[string]interface{}{"pr": "42", "depth": "standard"},
		SubjectType: "task",
		SubjectID:   "task_subject",
		TrackID:     trackID,
		Selection:   "1-3,7",
		DroppedDeps: []string{"tag->build"},
		ParentRun:   "",
		CreatedBy:   "jad",
		CreatedAt:   createdAt,
	}
}

func TestRecipeRunStore_CreateGetRoundTrip(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()
	edt := time.FixedZone("EDT", -4*3600)
	want := ledgerRun("run_rt", "track_1", time.Date(2026, 9, 12, 10, 0, 0, 0, edt))

	if err := s.CreateRecipeRun(ctx, want); err != nil {
		t.Fatalf("CreateRecipeRun: %v", err)
	}
	got, err := s.GetRecipeRun(ctx, "run_rt")
	if err != nil {
		t.Fatalf("GetRecipeRun: %v", err)
	}
	if got == nil {
		t.Fatal("GetRecipeRun returned nil")
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v (%v); want %v in UTC", got.CreatedAt, got.CreatedAt.Location(), want.CreatedAt)
	}
	got.CreatedAt, want.CreatedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the run:\n got %+v\nwant %+v", got, want)
	}
}

func TestRecipeRunStore_GetMissingIsNil(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	got, err := s.GetRecipeRun(context.Background(), "run_missing")
	if err != nil {
		t.Fatalf("GetRecipeRun: %v", err)
	}
	if got != nil {
		t.Errorf("missing run = %+v; want nil", got)
	}
}

// TestRecipeRunStore_ListFilters pins the ledger queries the CLI and the
// reconciler issue: by recipe, by track, by subject, newest first, paged.
func TestRecipeRunStore_ListFilters(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

	seed := []*core.RecipeRun{
		{ID: "run_1", ProjectID: "proj-a", RecipeID: "code-review", TrackID: "track_1", SubjectType: "task", SubjectID: "task_x", CreatedAt: base},
		{ID: "run_2", ProjectID: "proj-a", RecipeID: "code-review", TrackID: "track_1", CreatedAt: base.Add(1 * time.Minute)},
		{ID: "run_3", ProjectID: "proj-a", RecipeID: "release", TrackID: "track_2", SubjectType: "track", SubjectID: "track_2", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "run_4", ProjectID: "proj-b", RecipeID: "code-review", TrackID: "track_9", CreatedAt: base.Add(3 * time.Minute)},
	}
	for _, r := range seed {
		if err := s.CreateRecipeRun(ctx, r); err != nil {
			t.Fatalf("CreateRecipeRun %s: %v", r.ID, err)
		}
	}

	ids := func(runs []*core.RecipeRun) []string {
		out := make([]string, len(runs))
		for i, r := range runs {
			out[i] = r.ID
		}
		return out
	}
	cases := []struct {
		name  string
		query core.RecipeRunQuery
		want  []string
	}{
		{"all newest first", core.RecipeRunQuery{AllProjects: true}, []string{"run_4", "run_3", "run_2", "run_1"}},
		{"by recipe", core.RecipeRunQuery{RecipeID: "code-review", AllProjects: true}, []string{"run_4", "run_2", "run_1"}},
		{"by track", core.RecipeRunQuery{TrackID: "track_1", AllProjects: true}, []string{"run_2", "run_1"}},
		{"by subject", core.RecipeRunQuery{SubjectType: "task", SubjectID: "task_x", AllProjects: true}, []string{"run_1"}},
		{"by project", core.RecipeRunQuery{ProjectID: "proj-b"}, []string{"run_4"}},
		{"paged", core.RecipeRunQuery{AllProjects: true, Limit: 2, Offset: 1}, []string{"run_3", "run_2"}},
	}
	for _, tc := range cases {
		got, err := s.ListRecipeRuns(ctx, tc.query)
		if err != nil {
			t.Fatalf("%s: ListRecipeRuns: %v", tc.name, err)
		}
		if !reflect.DeepEqual(ids(got), tc.want) {
			t.Errorf("%s: ids = %v; want %v", tc.name, ids(got), tc.want)
		}
	}
}

// TestRecipeRunStore_RunTasks covers the step→task map: insertion order
// preserved, one task per step per run, lookup by run and by track, and
// deletion of a run taking its rows with it regardless of the
// connection's foreign_keys pragma.
func TestRecipeRunStore_RunTasks(t *testing.T) {
	s := newDomainRepoTestStorage(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)

	for _, r := range []*core.RecipeRun{
		{ID: "run_a", RecipeID: "code-review", TrackID: "track_1", CreatedAt: now},
		{ID: "run_b", RecipeID: "code-review", TrackID: "track_1", ParentRun: "run_a", CreatedAt: now.Add(time.Minute)},
		{ID: "run_c", RecipeID: "code-review", TrackID: "track_2", CreatedAt: now.Add(2 * time.Minute)},
	} {
		if err := s.CreateRecipeRun(ctx, r); err != nil {
			t.Fatalf("CreateRecipeRun %s: %v", r.ID, err)
		}
	}

	wantA := []core.RecipeRunTask{
		{RunID: "run_a", StepID: "lint", TaskID: "task_1"},
		{RunID: "run_a", StepID: "review", TaskID: "task_2"},
		{RunID: "run_a", StepID: "sign-off", TaskID: "task_3"},
	}
	if err := s.AddRecipeRunTasks(ctx, "run_a", wantA); err != nil {
		t.Fatalf("AddRecipeRunTasks run_a: %v", err)
	}
	if err := s.AddRecipeRunTasks(ctx, "run_b", []core.RecipeRunTask{{StepID: "security-scan", TaskID: "task_4"}}); err != nil {
		t.Fatalf("AddRecipeRunTasks run_b: %v", err)
	}
	if err := s.AddRecipeRunTasks(ctx, "run_c", []core.RecipeRunTask{{StepID: "lint", TaskID: "task_9"}}); err != nil {
		t.Fatalf("AddRecipeRunTasks run_c: %v", err)
	}

	// One task per step per run.
	if err := s.AddRecipeRunTasks(ctx, "run_a", []core.RecipeRunTask{{StepID: "lint", TaskID: "task_dup"}}); err == nil {
		t.Error("duplicate (run, step) accepted; want error")
	}

	got, err := s.ListRecipeRunTasks(ctx, "run_a")
	if err != nil {
		t.Fatalf("ListRecipeRunTasks: %v", err)
	}
	if !reflect.DeepEqual(got, wantA) {
		t.Errorf("ListRecipeRunTasks(run_a) = %+v; want %+v", got, wantA)
	}

	// By track: both runs on track_1, run order then insertion order, and
	// the run id filled in on every row.
	byTrack, err := s.ListRecipeRunTasksByTrack(ctx, "track_1")
	if err != nil {
		t.Fatalf("ListRecipeRunTasksByTrack: %v", err)
	}
	wantTrack := append(append([]core.RecipeRunTask{}, wantA...), core.RecipeRunTask{RunID: "run_b", StepID: "security-scan", TaskID: "task_4"})
	if !reflect.DeepEqual(byTrack, wantTrack) {
		t.Errorf("ListRecipeRunTasksByTrack(track_1) = %+v; want %+v", byTrack, wantTrack)
	}

	assertRunDeleted(t, s, "run_a", "run_b")
}

// assertRunDeleted deletes victim and checks its run and rows are gone
// while survivor's rows are untouched; deleting a missing run errors.
func assertRunDeleted(t *testing.T, s *SQLiteStorage, victim, survivor string) {
	t.Helper()
	ctx := context.Background()
	if err := s.DeleteRecipeRun(ctx, victim); err != nil {
		t.Fatalf("DeleteRecipeRun: %v", err)
	}
	if run, _ := s.GetRecipeRun(ctx, victim); run != nil {
		t.Errorf("%s still present after delete", victim)
	}
	if rows, _ := s.ListRecipeRunTasks(ctx, victim); len(rows) != 0 {
		t.Errorf("%s tasks survived delete: %+v", victim, rows)
	}
	if rows, _ := s.ListRecipeRunTasks(ctx, survivor); len(rows) != 1 {
		t.Errorf("%s tasks damaged by deleting %s: %+v", survivor, victim, rows)
	}
	if err := s.DeleteRecipeRun(ctx, "run_missing"); err == nil {
		t.Error("deleting a missing run succeeded; want error")
	}
}

// TestRecipeRunStore_ImplementsInterface pins the narrow store contract
// the executor and the CLI depend on.
func TestRecipeRunStore_ImplementsInterface(t *testing.T) {
	var _ core.RecipeRunStore = (*SQLiteStorage)(nil)
}
