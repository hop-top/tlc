package core

// Regression guards for PR #47 review points (T-0435).
// Each test pins a behaviour flagged by the reviewer so the fix
// cannot silently regress.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ambiguityTaskRepo returns two tasks for ANY ListTasks query,
// simulating a target track with multiple rows sharing the same
// (track_id, title) — an ambiguity that must be a hard error.
type ambiguityTaskRepo struct {
	stubTaskRepo
	created []*Task
}

func (r *ambiguityTaskRepo) CreateTask(
	_ context.Context, task *Task,
) error {
	cp := *task
	r.created = append(r.created, &cp)
	return nil
}

func (r *ambiguityTaskRepo) ListTasks(
	_ context.Context, _ Query,
) ([]*Task, error) {
	projA := "proj-a"
	return []*Task{
		{ID: "T-0001", Title: "dup", TrackID: strPtr("beta"), ProjectID: &projA},
		{ID: "T-0002", Title: "dup", TrackID: strPtr("beta"), ProjectID: &projA},
	}, nil
}

// TestCreateTasksFromPlan_AmbiguityFailsBeforeAnyCreate is a
// regression guard for PR #47 review item 1: when a cross-track
// ref resolves to multiple DB rows, the whole ingest must fail
// BEFORE any tasks are created. Previously preflight only
// validated OOR and let ambiguity leak into the per-task loop,
// so earlier tasks would already be committed.
func TestCreateTasksFromPlan_AmbiguityFailsBeforeAnyCreate(t *testing.T) {
	ctx := context.Background()

	// Target track 'beta' has a plan with one task titled "dup".
	// The ambiguityTaskRepo then returns two rows for any lookup,
	// so preflight must catch it.
	dir := t.TempDir()
	betaPlan := filepath.Join(dir, "beta.md")
	if err := os.WriteFile(
		betaPlan,
		[]byte("---\ntitle: beta\ntasks:\n  - title: \"dup\"\n---\n"),
		0o644,
	); err != nil {
		t.Fatalf("write beta plan: %v", err)
	}
	projA := "proj-a"

	trackRepo := newStubTrackRepo()
	beta := &Track{
		ID: "beta", Title: "Beta", Type: TrackTypeFeature,
		Status:    TrackStatusActive,
		ProjectID: &projA,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Meta:      map[string]any{"plans": []string{betaPlan}},
	}
	if err := trackRepo.CreateTrack(ctx, beta); err != nil {
		t.Fatalf("CreateTrack beta: %v", err)
	}

	taskRepo := &ambiguityTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)

	// Two specs: the first is a plain task that would otherwise
	// be created, the second references beta#1 (which is
	// ambiguous in the DB). We expect ZERO tasks to be created
	// because preflight should fail first.
	specs := []PlanTaskSpec{
		{Title: "Would-be first"},
		{
			Title: "References ambiguous target",
			BlockedBy: []BlockedByRef{{
				CrossTrack: &CrossTrackRef{TrackID: "beta", TaskNum: 1},
			}},
		},
	}

	_, err := svc.CreateTasksFromPlan(
		ctx, "alpha", specs, projA, &stubIDGen{next: 1},
	)
	if err == nil {
		t.Fatal("expected ambiguity hard error; got nil")
	}
	if len(taskRepo.created) != 0 {
		t.Errorf(
			"contract violated: %d tasks created despite ambiguity; "+
				"preflight should fail before any CreateTask",
			len(taskRepo.created),
		)
	}
}

// projectIsolationTaskRepo records queries and stores tasks
// grouped by project_id so we can verify phase 2 never touches
// the wrong bucket.
type projectIsolationTaskRepo struct {
	stubTaskRepo
	queries []Query
}

func (r *projectIsolationTaskRepo) ListTasks(
	_ context.Context, q Query,
) ([]*Task, error) {
	r.queries = append(r.queries, q)
	// Filter by project_id if present, otherwise return all.
	var wantPID string
	havePID := false
	for _, f := range q.Filters {
		if f.Field == "project_id" {
			wantPID, _ = f.Value.(string)
			havePID = true
		}
	}
	if !havePID {
		return r.tasks, nil
	}
	var out []*Task
	for _, t := range r.tasks {
		gotPID := ""
		if t.ProjectID != nil {
			gotPID = *t.ProjectID
		}
		if gotPID == wantPID {
			out = append(out, t)
		}
	}
	return out, nil
}

// TestResolvePendingCrossTrackRefs_ScopesByProjectIDEvenWhenEmpty
// is a regression guard for PR #47 review item 2: phase 2 must
// always filter by project_id, even when projectID == "". A
// default-project phase 2 pass must not touch or rewrite tasks
// belonging to named projects.
func TestResolvePendingCrossTrackRefs_ScopesByProjectIDEvenWhenEmpty(
	t *testing.T,
) {
	ctx := context.Background()

	trackRepo := newStubTrackRepo()
	taskRepo := &projectIsolationTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)

	// Run phase 2 with empty projectID.
	if _, err := svc.ResolvePendingCrossTrackRefs(ctx, "", nil); err != nil {
		t.Fatalf("phase 2: %v", err)
	}

	if len(taskRepo.queries) == 0 {
		t.Fatal("ListTasks never called")
	}
	q := taskRepo.queries[0]
	var sawPIDFilter bool
	var gotValue interface{}
	for _, f := range q.Filters {
		if f.Field == "project_id" {
			sawPIDFilter = true
			gotValue = f.Value
		}
	}
	if !sawPIDFilter {
		t.Errorf(
			"phase 2 with projectID=\"\" has no project_id "+
				"filter; would touch all projects. Filters: %+v",
			q.Filters,
		)
	}
	if gotValue != "" {
		t.Errorf(
			"phase 2 project_id filter = %v, want empty string",
			gotValue,
		)
	}
}

// TestResolvePendingCrossTrackRefs_UnparseableRefKeepsTaskDirty
// is a regression guard for PR #47 review item 4: a corrupted
// entry in blocked_by_unresolved must keep the owning plan
// "dirty" so it isn't prematurely rewritten. The fix records
// unparseable entries in res.StillUnresolved.
func TestResolvePendingCrossTrackRefs_UnparseableRefKeepsTaskDirty(
	t *testing.T,
) {
	ctx := context.Background()

	trackRepo := newStubTrackRepo()
	taskRepo := &stubTaskRepo{}
	// Task with a corrupted unresolved entry.
	badTask := &Task{
		ID: "T-0001", Title: "bad", TrackID: strPtr("alpha"),
		Meta: map[string]any{
			metaKeyUnresolved: []string{"this is not a ref"},
		},
	}
	taskRepo.tasks = []*Task{badTask}
	svc := NewTrackService(trackRepo, taskRepo)

	res, err := svc.ResolvePendingCrossTrackRefs(ctx, "", nil)
	if err != nil {
		t.Fatalf("phase 2: %v", err)
	}
	if len(res.StillUnresolved) != 1 {
		t.Errorf(
			"expected 1 StillUnresolved entry for corrupted ref; "+
				"got %d: %+v",
			len(res.StillUnresolved), res.StillUnresolved,
		)
	}
	if len(res.StillUnresolved) > 0 {
		got := res.StillUnresolved[0]
		if got.TaskID != "T-0001" || got.Ref != "this is not a ref" {
			t.Errorf(
				"StillUnresolved entry = %+v; want "+
					"{T-0001, \"this is not a ref\"}",
				got,
			)
		}
	}
}
