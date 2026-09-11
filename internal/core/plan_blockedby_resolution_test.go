package core

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// resolvingTaskRepo mirrors real storage semantics: GetTask matches the
// durable ID column only, while a "T-NNNN" display alias resolves through
// GetTaskBySeq. Seeded tasks carry their alias's seq in Task.Seq.
type resolvingTaskRepo struct {
	stubTaskRepo
	byRef   map[string]*Task
	bySeq   map[int64]*Task
	created []*Task
	updated []*Task
}

func newResolvingTaskRepo(seed ...*Task) *resolvingTaskRepo {
	r := &resolvingTaskRepo{
		byRef: make(map[string]*Task),
		bySeq: make(map[int64]*Task),
	}
	for _, t := range seed {
		r.add(t)
	}
	return r
}

// add registers a task under its durable ID, plus its seq when set so
// display-alias lookups resolve the way storage does.
func (r *resolvingTaskRepo) add(t *Task) {
	r.byRef[t.ID] = t
	if t.Seq != 0 {
		r.bySeq[t.Seq] = t
	}
	r.tasks = append(r.tasks, t)
}

func (r *resolvingTaskRepo) GetTask(_ context.Context, id string) (*Task, error) {
	if t, ok := r.byRef[id]; ok {
		return t, nil
	}
	return nil, nil
}

func (r *resolvingTaskRepo) GetTaskBySeq(
	_ context.Context, _ string, seq int64,
) (*Task, error) {
	if t, ok := r.bySeq[seq]; ok {
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
	if task.Seq != 0 {
		r.bySeq[task.Seq] = task
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

// seqTaskRepo resolves display aliases the way real storage does: only
// GetTaskBySeq maps a T-NNNN alias to a row; GetTask matches the durable
// ID column alone. A stub that answers GetTask("T-0001") would hide the
// alias-resolution requirement entirely.
type seqTaskRepo struct {
	stubTaskRepo
	byID    map[string]*Task
	bySeq   map[int64]*Task
	created []*Task
}

func newSeqTaskRepo(seed ...*Task) *seqTaskRepo {
	r := &seqTaskRepo{
		byID:  make(map[string]*Task),
		bySeq: make(map[int64]*Task),
	}
	for _, t := range seed {
		r.byID[t.ID] = t
		if t.Seq != 0 {
			r.bySeq[t.Seq] = t
		}
		r.tasks = append(r.tasks, t)
	}
	return r
}

func (r *seqTaskRepo) GetTask(_ context.Context, id string) (*Task, error) {
	return r.byID[id], nil
}

func (r *seqTaskRepo) GetTaskBySeq(_ context.Context, _ string, seq int64) (*Task, error) {
	return r.bySeq[seq], nil
}

func (r *seqTaskRepo) CreateTask(_ context.Context, task *Task) error {
	cp := *task
	if task.Meta != nil {
		cp.Meta = make(map[string]any, len(task.Meta))
		for k, v := range task.Meta {
			cp.Meta[k] = v
		}
	}
	r.created = append(r.created, &cp)
	return nil
}

// TestCreateTasksFromPlan_ResolvesDisplayAlias pins the end-to-end
// contract against realistic storage semantics: a plan ref written as
// the display alias "T-0001" must resolve via seq lookup, exactly as
// `task update --add-blocked-by T-0001` does.
func TestCreateTasksFromPlan_ResolvesDisplayAlias(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const durableID = "task_01m06mv4sqf979kfdpkk76sq53"
	taskRepo := newSeqTaskRepo(&Task{
		ID: durableID, Seq: 1, Title: "Standalone blocker",
		Status: StatusTodo,
	})
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{{
		Title:     "Plan task B",
		BlockedBy: []BlockedByRef{{TaskID: "T-0001"}},
	}}

	if _, err := svc.CreateTasksFromPlan(
		context.Background(), "trk", specs, "", &stubIDGen{next: 2},
	); err != nil {
		t.Fatalf("CreateTasksFromPlan: %v", err)
	}

	if len(taskRepo.created) != 1 {
		t.Fatalf("created %d tasks, want 1", len(taskRepo.created))
	}
	got := NormalizeBlockedBy(taskRepo.created[0].Meta["blocked_by"])
	if len(got) != 1 || got[0] != durableID {
		t.Errorf("blocked_by = %v, want [%s] resolved via seq alias", got, durableID)
	}
}

// blockerTask builds an existing task with a durable typeid-style ID
// whose Seq backs the given "T-NNNN" display alias.
func blockerTask(durableID, alias, title string) *Task {
	seq, err := strconv.ParseInt(strings.TrimPrefix(alias, "T-"), 10, 64)
	if err != nil {
		panic("blockerTask: alias must be T-NNNN, got " + alias)
	}
	return &Task{
		ID:      durableID,
		Seq:     seq,
		Title:   title,
		Status:  StatusTodo,
		TrackID: strPtr("other"),
		Meta:    map[string]any{},
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
		blockerTask(durableID, "T-0968", "Pre-existing blocker"),
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
		blockerTask(durableID, "T-0968", "Pre-existing blocker"),
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
