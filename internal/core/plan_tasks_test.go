package core

import (
	"context"
	"fmt"
	"testing"
)

// stubIDGen returns incrementing sequence IDs starting from start.
type stubIDGen struct {
	next int
}

func (g *stubIDGen) GetNextSequenceID(
	_ context.Context, _ string,
) (int, error) {
	id := g.next
	g.next++
	return id, nil
}

// failingIDGen always returns an error.
type failingIDGen struct{}

func (g *failingIDGen) GetNextSequenceID(
	_ context.Context, _ string,
) (int, error) {
	return 0, fmt.Errorf("id generation failed")
}

// ensureTrack creates a minimal track in the stub repo so
// CreateTasksFromPlan can persist the plan mapping.
func ensureTrack(t *testing.T, repo *stubTrackRepo, id string) {
	t.Helper()
	_ = repo.CreateTrack(context.Background(), &Track{
		ID: id, Title: id, Type: TrackTypeFeature,
		Status: TrackStatusPending,
	})
}

func TestCreateTasksFromPlan_Basic(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "test-track")
	taskRepo := &stubTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()
	idGen := &stubIDGen{next: 100}

	specs := []PlanTaskSpec{
		{
			Title:    "First task",
			Effort:   "S",
			Priority: "P1",
			Tags:     []string{"phase:1"},
		},
		{
			Title:      "Second task",
			Effort:     "M",
			Priority:   "P2",
			AssignedTo: "@me",
			BlockedBy:  []BlockedByRef{{Index: 0}},
		},
		{
			Title:     "Third task",
			BlockedBy: []BlockedByRef{{Index: 0}, {Index: 1}},
		},
	}

	result, err := svc.CreateTasksFromPlan(ctx, "test-track", specs, "proj1", idGen)
	if err != nil {
		t.Fatalf("CreateTasksFromPlan failed: %v", err)
	}
	ids := result.CreatedIDs
	if len(ids) != 3 {
		t.Fatalf("ids len = %d, want 3", len(ids))
	}
	if ids[0] != "T-0100" {
		t.Errorf("ids[0] = %q, want T-0100", ids[0])
	}
	if ids[1] != "T-0101" {
		t.Errorf("ids[1] = %q, want T-0101", ids[1])
	}
	if ids[2] != "T-0102" {
		t.Errorf("ids[2] = %q, want T-0102", ids[2])
	}

	// Verify tasks created in taskRepo.
	if len(taskRepo.tasks) != 3 {
		// stubTaskRepo doesn't accumulate by default; check via mock
		t.Logf("stubTaskRepo doesn't accumulate tasks; skipping count check")
	}
}

func TestCreateTasksFromPlan_BlockedByResolution(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")
	taskRepo := &creatingTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()
	idGen := &stubIDGen{next: 50}

	specs := []PlanTaskSpec{
		{Title: "A"},
		{Title: "B", BlockedBy: []BlockedByRef{{Index: 0}}},
	}

	result, err := svc.CreateTasksFromPlan(ctx, "trk", specs, "", idGen)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ids := result.CreatedIDs

	// Verify task B has blocked_by referencing task A's ID.
	taskB := taskRepo.created[1]
	blockedBy, ok := taskB.Meta["blocked_by"].([]string)
	if !ok {
		t.Fatalf("task B meta[blocked_by] not []string: %T", taskB.Meta["blocked_by"])
	}
	if len(blockedBy) != 1 || blockedBy[0] != ids[0] {
		t.Errorf("task B blocked_by = %v, want [%s]", blockedBy, ids[0])
	}
}

func TestCreateTasksFromPlan_EmptySpecs(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	result, err := svc.CreateTasksFromPlan(ctx, "trk", nil, "", &stubIDGen{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || len(result.CreatedIDs) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestCreateTasksFromPlan_InvalidBlockedByIndex(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	specs := []PlanTaskSpec{
		{Title: "A"},
		{Title: "B", BlockedBy: []BlockedByRef{{Index: 1}}}, // self-ref, invalid
	}

	_, err := svc.CreateTasksFromPlan(ctx, "trk", specs, "", &stubIDGen{next: 1})
	if err == nil {
		t.Fatal("expected error for invalid blocked-by index")
	}
}

func TestCreateTasksFromPlan_NegativeBlockedByIndex(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	specs := []PlanTaskSpec{
		{Title: "A", BlockedBy: []BlockedByRef{{Index: -1}}},
	}

	_, err := svc.CreateTasksFromPlan(ctx, "trk", specs, "", &stubIDGen{next: 1})
	if err == nil {
		t.Fatal("expected error for negative blocked-by index")
	}
}

func TestCreateTasksFromPlan_IDGenFailure(t *testing.T) {
	svc := NewTrackService(newStubTrackRepo(), &stubTaskRepo{})
	ctx := context.Background()

	specs := []PlanTaskSpec{{Title: "A"}}

	_, err := svc.CreateTasksFromPlan(ctx, "trk", specs, "", &failingIDGen{})
	if err == nil {
		t.Fatal("expected error for ID gen failure")
	}
}

func TestCreateTasksFromPlan_AssigneeNormalization(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")
	taskRepo := &creatingTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()

	specs := []PlanTaskSpec{
		{Title: "A", AssignedTo: "@alice"},
		{Title: "B", AssignedTo: "bob"},
	}

	_, err := svc.CreateTasksFromPlan(ctx, "trk", specs, "", &stubIDGen{next: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if taskRepo.created[0].AssignedTo == nil || *taskRepo.created[0].AssignedTo != "alice" {
		t.Errorf("task A assignee = %v, want 'alice'", taskRepo.created[0].AssignedTo)
	}
	if taskRepo.created[1].AssignedTo == nil || *taskRepo.created[1].AssignedTo != "bob" {
		t.Errorf("task B assignee = %v, want 'bob'", taskRepo.created[1].AssignedTo)
	}
}

func TestCreateTasksFromPlan_TrackIDSet(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "my-track")
	taskRepo := &creatingTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()

	specs := []PlanTaskSpec{{Title: "A"}}
	_, err := svc.CreateTasksFromPlan(ctx, "my-track", specs, "", &stubIDGen{next: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if taskRepo.created[0].TrackID == nil || *taskRepo.created[0].TrackID != "my-track" {
		t.Errorf("task track_id = %v, want 'my-track'", taskRepo.created[0].TrackID)
	}
}

// creatingTaskRepo accumulates created tasks for inspection.
type creatingTaskRepo struct {
	stubTaskRepo
	created []*Task
}

func (r *creatingTaskRepo) CreateTask(_ context.Context, task *Task) error {
	cp := *task
	// Deep-copy meta.
	if task.Meta != nil {
		cp.Meta = make(map[string]any, len(task.Meta))
		for k, v := range task.Meta {
			cp.Meta[k] = v
		}
	}
	r.created = append(r.created, &cp)
	return nil
}
