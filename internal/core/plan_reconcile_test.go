package core

import (
	"context"
	"testing"
)

// reconcileTaskRepo is a full in-memory repo for reconciliation tests.
type reconcileTaskRepo struct {
	stubTaskRepo
	tasks map[string]*Task
	seq   int
}

func newReconcileTaskRepo() *reconcileTaskRepo {
	return &reconcileTaskRepo{
		tasks: make(map[string]*Task),
		seq:   100,
	}
}

func (r *reconcileTaskRepo) CreateTask(_ context.Context, task *Task) error {
	cp := *task
	if task.Meta != nil {
		cp.Meta = make(map[string]any, len(task.Meta))
		for k, v := range task.Meta {
			cp.Meta[k] = v
		}
	}
	r.tasks[task.ID] = &cp
	return nil
}

func (r *reconcileTaskRepo) GetTask(_ context.Context, id string) (*Task, error) {
	t, ok := r.tasks[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (r *reconcileTaskRepo) GetTaskBySeq(_ context.Context, projectID string, seq int64) (*Task, error) {
	for _, t := range r.tasks {
		var pid string
		if t.ProjectID != nil {
			pid = *t.ProjectID
		}
		if pid == projectID && t.Seq == seq {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *reconcileTaskRepo) UpdateTask(_ context.Context, task *Task) error {
	cp := *task
	if task.Meta != nil {
		cp.Meta = make(map[string]any, len(task.Meta))
		for k, v := range task.Meta {
			cp.Meta[k] = v
		}
	}
	r.tasks[task.ID] = &cp
	return nil
}

func (r *reconcileTaskRepo) DeleteTask(_ context.Context, id string) error {
	delete(r.tasks, id)
	return nil
}

func (r *reconcileTaskRepo) ListTasks(_ context.Context, query Query) ([]*Task, error) {
	var result []*Task
	for _, t := range r.tasks {
		if matchesFilters(t, query.Filters) {
			cp := *t
			result = append(result, &cp)
		}
	}
	return result, nil
}

func matchesFilters(t *Task, filters []FieldFilter) bool {
	for _, f := range filters {
		if !matchesFilter(t, f) {
			return false
		}
	}
	return true
}

func matchesFilter(t *Task, f FieldFilter) bool {
	switch f.Field {
	case "track_id":
		return t.TrackID != nil && *t.TrackID == f.Value.(string)
	case "title":
		return t.Title == f.Value.(string)
	case "project_id":
		pid := ""
		if t.ProjectID != nil {
			pid = *t.ProjectID
		}
		return pid == f.Value.(string)
	default:
		return true
	}
}

func (r *reconcileTaskRepo) GetNextSequenceID(_ context.Context, _ string) (int, error) {
	r.seq++
	return r.seq, nil
}

// setupTrackWithTasks creates a track and tasks via CreateTasksFromPlan,
// returns the service, task repo, and resulting mapping.
func setupTrackWithTasks(
	t *testing.T, specs []PlanTaskSpec,
) (*TrackService, *reconcileTaskRepo, map[int]string) {
	t.Helper()
	trackRepo := newStubTrackRepo()
	taskRepo := newReconcileTaskRepo()
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()

	track := &Track{
		ID:    "test-track",
		Title: "Test Track",
		Type:  TrackTypeFeature,
	}
	if err := svc.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	result, err := svc.CreateTasksFromPlan(
		ctx, "test-track", specs, "", taskRepo,
	)
	if err != nil {
		t.Fatalf("CreateTasksFromPlan: %v", err)
	}

	// Build mapping from result.
	mapping := make(map[int]string, len(result.CreatedIDs))
	for i, id := range result.CreatedIDs {
		mapping[i] = id
	}
	return svc, taskRepo, mapping
}

func TestReconcile_FirstAddPlanStoresMapping(t *testing.T) {
	trackRepo := newStubTrackRepo()
	taskRepo := newReconcileTaskRepo()
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()

	track := &Track{
		ID:    "test-track",
		Title: "Test Track",
		Type:  TrackTypeFeature,
	}
	if err := svc.CreateTrack(ctx, track); err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	specs := []PlanTaskSpec{
		{Title: "Task A"},
		{Title: "Task B"},
	}

	result, err := svc.CreateTasksFromPlan(
		ctx, "test-track", specs, "", taskRepo,
	)
	if err != nil {
		t.Fatalf("CreateTasksFromPlan: %v", err)
	}
	if len(result.CreatedIDs) != 2 {
		t.Fatalf("expected 2 created, got %d", len(result.CreatedIDs))
	}

	// Verify mapping was stored on track.
	got, _ := svc.GetTrack(ctx, "test-track")
	if len(got.PlanMapping) != 2 {
		t.Fatalf("expected plan_mapping with 2 entries, got %d", len(got.PlanMapping))
	}
	if got.PlanMapping[0] != result.CreatedIDs[0] {
		t.Errorf("mapping[0] = %s, want %s", got.PlanMapping[0], result.CreatedIDs[0])
	}
	if got.PlanMapping[1] != result.CreatedIDs[1] {
		t.Errorf("mapping[1] = %s, want %s", got.PlanMapping[1], result.CreatedIDs[1])
	}
}

func TestReconcile_SamePlanNoChanges(t *testing.T) {
	specs := []PlanTaskSpec{
		{Title: "Task A", Effort: "S", Priority: "P1"},
		{Title: "Task B", Effort: "M", Priority: "P2"},
	}

	svc, taskRepo, mapping := setupTrackWithTasks(t, specs)
	ctx := context.Background()

	// Re-run with same specs.
	rec, err := svc.ReconcileTasksFromPlan(
		ctx, "test-track", specs, "", taskRepo, mapping,
	)
	if err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	if len(rec.Created) != 0 {
		t.Errorf("expected 0 created, got %d", len(rec.Created))
	}
	if len(rec.Updated) != 0 {
		t.Errorf("expected 0 updated, got %d", len(rec.Updated))
	}
	if len(rec.Unchanged) != 2 {
		t.Errorf("expected 2 unchanged, got %d", len(rec.Unchanged))
	}
	if len(rec.Deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d", len(rec.Deleted))
	}
}

func TestReconcile_UpdatedDescription(t *testing.T) {
	specs := []PlanTaskSpec{
		{Title: "Task A", Description: "original"},
	}

	svc, taskRepo, mapping := setupTrackWithTasks(t, specs)
	ctx := context.Background()

	updatedSpecs := []PlanTaskSpec{
		{Title: "Task A", Description: "updated"},
	}

	rec, err := svc.ReconcileTasksFromPlan(
		ctx, "test-track", updatedSpecs, "", taskRepo, mapping,
	)
	if err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	if len(rec.Updated) != 1 {
		t.Errorf("expected 1 updated, got %d", len(rec.Updated))
	}

	task, _ := taskRepo.GetTask(ctx, mapping[0])
	if task.Description != "updated" {
		t.Errorf("description = %q, want 'updated'", task.Description)
	}
}

func TestReconcile_NewTaskAdded(t *testing.T) {
	specs := []PlanTaskSpec{
		{Title: "Task A"},
	}

	svc, taskRepo, mapping := setupTrackWithTasks(t, specs)
	ctx := context.Background()

	updatedSpecs := []PlanTaskSpec{
		{Title: "Task A"},
		{Title: "Task B"},
	}

	rec, err := svc.ReconcileTasksFromPlan(
		ctx, "test-track", updatedSpecs, "", taskRepo, mapping,
	)
	if err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	if len(rec.Created) != 1 {
		t.Errorf("expected 1 created, got %d", len(rec.Created))
	}
	if len(rec.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged, got %d", len(rec.Unchanged))
	}
}

func TestReconcile_RemovedTodoTaskDeleted(t *testing.T) {
	specs := []PlanTaskSpec{
		{Title: "Task A"},
		{Title: "Task B"},
	}

	svc, taskRepo, mapping := setupTrackWithTasks(t, specs)
	ctx := context.Background()

	// Remove Task B.
	updatedSpecs := []PlanTaskSpec{
		{Title: "Task A"},
	}

	rec, err := svc.ReconcileTasksFromPlan(
		ctx, "test-track", updatedSpecs, "", taskRepo, mapping,
	)
	if err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	if len(rec.Deleted) != 1 {
		t.Errorf("expected 1 deleted, got %d", len(rec.Deleted))
	}
	if rec.Deleted[0] != mapping[1] {
		t.Errorf("deleted = %s, want %s", rec.Deleted[0], mapping[1])
	}

	// Verify task is gone.
	task, _ := taskRepo.GetTask(ctx, mapping[1])
	if task != nil {
		t.Error("expected task B to be deleted")
	}
}

func TestReconcile_RemovedInProgressTaskKept(t *testing.T) {
	specs := []PlanTaskSpec{
		{Title: "Task A"},
		{Title: "Task B"},
	}

	svc, taskRepo, mapping := setupTrackWithTasks(t, specs)
	ctx := context.Background()

	// Transition Task B to IN_PROGRESS.
	taskB, _ := taskRepo.GetTask(ctx, mapping[1])
	taskB.Status = StatusInProgress
	_ = taskRepo.UpdateTask(ctx, taskB)

	// Remove Task B from plan.
	updatedSpecs := []PlanTaskSpec{
		{Title: "Task A"},
	}

	rec, err := svc.ReconcileTasksFromPlan(
		ctx, "test-track", updatedSpecs, "", taskRepo, mapping,
	)
	if err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	if len(rec.Kept) != 1 {
		t.Errorf("expected 1 kept, got %d", len(rec.Kept))
	}
	if len(rec.Deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d", len(rec.Deleted))
	}

	// Verify task still exists.
	task, _ := taskRepo.GetTask(ctx, mapping[1])
	if task == nil {
		t.Error("expected task B to still exist")
	}
}

func TestReconcile_ReorderedTasksMatchedByTitle(t *testing.T) {
	specs := []PlanTaskSpec{
		{Title: "Task A", Effort: "S"},
		{Title: "Task B", Effort: "M"},
	}

	svc, taskRepo, mapping := setupTrackWithTasks(t, specs)
	ctx := context.Background()

	// Reverse order.
	reorderedSpecs := []PlanTaskSpec{
		{Title: "Task B", Effort: "M"},
		{Title: "Task A", Effort: "S"},
	}

	rec, err := svc.ReconcileTasksFromPlan(
		ctx, "test-track", reorderedSpecs, "", taskRepo, mapping,
	)
	if err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	if len(rec.Unchanged) != 2 {
		t.Errorf("expected 2 unchanged, got %d", len(rec.Unchanged))
	}
	if len(rec.Created) != 0 {
		t.Errorf("expected 0 created, got %d", len(rec.Created))
	}
}

func TestReconcile_BlockedByUpdatedAfterReconciliation(t *testing.T) {
	specs := []PlanTaskSpec{
		{Title: "Task A"},
		{Title: "Task B", BlockedBy: []BlockedByRef{{Index: 0}}},
	}

	svc, taskRepo, mapping := setupTrackWithTasks(t, specs)
	ctx := context.Background()

	// Re-run same specs to verify blocked-by is re-resolved.
	rec, err := svc.ReconcileTasksFromPlan(
		ctx, "test-track", specs, "", taskRepo, mapping,
	)
	if err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	_ = rec

	// Task B should have blocked_by pointing to Task A.
	taskB, _ := taskRepo.GetTask(ctx, mapping[1])
	if taskB.Meta == nil {
		t.Fatal("task B meta is nil")
	}
	bb, ok := taskB.Meta["blocked_by"].([]string)
	if !ok {
		t.Fatalf("task B blocked_by not []string: %T", taskB.Meta["blocked_by"])
	}
	if len(bb) != 1 || bb[0] != mapping[0] {
		t.Errorf("task B blocked_by = %v, want [%s]", bb, mapping[0])
	}
}
