package core

import (
	"context"
	"testing"
)

// resolvingTaskRepo is a task repo stub that can resolve a display
// alias ("T-0968") to a stored task whose durable ID is a typeid
// ("task_01..."), mirroring real storage semantics. Created tasks are
// accumulated and become resolvable by their durable ID.
type resolvingTaskRepo struct {
	stubTaskRepo
	// byRef maps any accepted lookup key (display alias or durable ID)
	// to the stored task.
	byRef   map[string]*Task
	created []*Task
	updated []*Task
}

func newResolvingTaskRepo(seed ...*Task) *resolvingTaskRepo {
	r := &resolvingTaskRepo{byRef: make(map[string]*Task)}
	for _, t := range seed {
		r.add(t)
	}
	return r
}

// add registers a task under its durable ID and, when set, its display
// alias stored in Meta["alias"].
func (r *resolvingTaskRepo) add(t *Task) {
	r.byRef[t.ID] = t
	if alias, ok := t.Meta["alias"].(string); ok && alias != "" {
		r.byRef[alias] = t
	}
	r.tasks = append(r.tasks, t)
}

func (r *resolvingTaskRepo) GetTask(_ context.Context, id string) (*Task, error) {
	if t, ok := r.byRef[id]; ok {
		return t, nil
	}
	return nil, nil
}

func (r *resolvingTaskRepo) CreateTask(_ context.Context, task *Task) error {
	cp := *task
	if task.Meta != nil {
		cp.Meta = make(map[string]any, len(task.Meta))
		for k, v := range task.Meta {
			cp.Meta[k] = v
		}
	}
	r.created = append(r.created, &cp)
	r.add(&cp)
	return nil
}

func (r *resolvingTaskRepo) UpdateTask(_ context.Context, task *Task) error {
	r.updated = append(r.updated, task)
	r.byRef[task.ID] = task
	if alias, ok := task.Meta["alias"].(string); ok && alias != "" {
		r.byRef[alias] = task
	}
	for i, t := range r.tasks {
		if t.ID == task.ID {
			r.tasks[i] = task
			return nil
		}
	}
	r.tasks = append(r.tasks, task)
	return nil
}

// blockerTask builds an existing task with a durable typeid-style ID and
// a display alias.
func blockerTask(durableID, alias, title, trackID string) *Task {
	return &Task{
		ID:      durableID,
		Title:   title,
		Status:  StatusTodo,
		TrackID: strPtr(trackID),
		Meta:    map[string]any{"alias": alias},
	}
}

// TestCreateTasksFromPlan_TaskIDRefStoresResolvedID asserts that a
// same-project "T-NNNN" blocked-by ref is persisted as the resolved
// task's durable ID, not as the raw alias string. Storing the raw alias
// leaves a dangling edge that renders as "(missing)".
func TestCreateTasksFromPlan_TaskIDRefStoresResolvedID(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const durableID = "task_01m06j5p9cf3f9pff2mh4w8tjk"
	taskRepo := newResolvingTaskRepo(
		blockerTask(durableID, "T-0968", "Pre-existing blocker", "other"),
	)
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{{
		Title:     "Depends on existing task",
		BlockedBy: []BlockedByRef{{TaskID: "T-0968"}},
	}}

	if _, err := svc.CreateTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
	); err != nil {
		t.Fatalf("CreateTasksFromPlan: %v", err)
	}

	if len(taskRepo.created) != 1 {
		t.Fatalf("created %d tasks, want 1", len(taskRepo.created))
	}
	got := NormalizeBlockedBy(taskRepo.created[0].Meta["blocked_by"])
	if len(got) != 1 || got[0] != durableID {
		t.Errorf("blocked_by = %v, want [%s] (resolved durable ID)", got, durableID)
	}
}

// TestReconcile_TaskIDRefStoresResolvedID is the reconcile-path twin of
// the above: resolveRefList must also store the resolved durable ID.
func TestReconcile_TaskIDRefStoresResolvedID(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const durableID = "task_01m06j5p9cf3f9pff2mh4w8tjk"
	const mappedID = "task_01existingplanrow00000000"
	taskRepo := newResolvingTaskRepo(
		blockerTask(durableID, "T-0968", "Pre-existing blocker", "other"),
		&Task{
			ID: mappedID, Title: "Depends on existing task",
			Status: StatusTodo, TrackID: strPtr("trk"),
			Meta: map[string]any{},
		},
	)
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{{
		Title:     "Depends on existing task",
		BlockedBy: []BlockedByRef{{TaskID: "T-0968"}},
	}}

	if _, err := svc.ReconcileTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
		map[int]string{0: mappedID},
	); err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	stored, _ := taskRepo.GetTask(context.Background(), mappedID)
	if stored == nil {
		t.Fatal("mapped task disappeared")
	}
	got := stored.BlockedBy()
	if len(got) != 1 || got[0] != durableID {
		t.Errorf("blocked_by = %v, want [%s] (resolved durable ID)", got, durableID)
	}
}
