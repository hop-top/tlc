package core

// Regression tests for T-0703: plan ingest rejects forward blocked-by
// references (indices pointing to tasks defined later in the array).

import (
	"context"
	"testing"
)

// TestCreateTasksFromPlan_ForwardBlockedByRef verifies that a task
// can reference a task defined later in the specs array via
// blocked-by index (forward reference). This is the core bug:
// pre-flight validated against loop index i instead of len(specs).
func TestCreateTasksFromPlan_ForwardBlockedByRef(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")
	taskRepo := &creatingTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()
	idGen := &stubIDGen{next: 300}

	specs := []PlanTaskSpec{
		{Title: "A", BlockedBy: []BlockedByRef{{Index: 1}}}, // forward ref to B
		{Title: "B"},
	}

	result, err := svc.CreateTasksFromPlan(ctx, "trk", specs, "", idGen)
	if err != nil {
		t.Fatalf("forward blocked-by should succeed: %v", err)
	}

	ids := result.CreatedIDs
	if len(ids) != 2 {
		t.Fatalf("ids len = %d, want 2", len(ids))
	}

	// Task A must have blocked_by = [task B's ID].
	taskA := taskRepo.created[0]
	blockedBy, ok := taskA.Meta["blocked_by"].([]string)
	if !ok {
		t.Fatalf("task A meta[blocked_by] = %T, want []string",
			taskA.Meta["blocked_by"])
	}
	if len(blockedBy) != 1 || blockedBy[0] != ids[1] {
		t.Errorf("task A blocked_by = %v, want [%s]", blockedBy, ids[1])
	}
}

// TestCreateTasksFromPlan_ForwardRefChain tests multiple forward
// references across the array — task 0 blocked by task 2, task 1
// blocked by task 2.
func TestCreateTasksFromPlan_ForwardRefChain(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")
	taskRepo := &creatingTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()
	idGen := &stubIDGen{next: 400}

	specs := []PlanTaskSpec{
		{Title: "A", BlockedBy: []BlockedByRef{{Index: 2}}},
		{Title: "B", BlockedBy: []BlockedByRef{{Index: 2}}},
		{Title: "C"},
	}

	result, err := svc.CreateTasksFromPlan(ctx, "trk", specs, "", idGen)
	if err != nil {
		t.Fatalf("forward ref chain should succeed: %v", err)
	}

	ids := result.CreatedIDs
	if len(ids) != 3 {
		t.Fatalf("ids len = %d, want 3", len(ids))
	}

	// Both A and B must have blocked_by = [task C's ID].
	for i, name := range []string{"A", "B"} {
		task := taskRepo.created[i]
		blockedBy, ok := task.Meta["blocked_by"].([]string)
		if !ok {
			t.Fatalf("task %s meta[blocked_by] = %T, want []string",
				name, task.Meta["blocked_by"])
		}
		if len(blockedBy) != 1 || blockedBy[0] != ids[2] {
			t.Errorf("task %s blocked_by = %v, want [%s]",
				name, blockedBy, ids[2])
		}
	}
}

// TestCreateTasksFromPlan_MixedForwardAndBackwardRefs tests a mix
// of forward and backward references in the same plan.
func TestCreateTasksFromPlan_MixedForwardAndBackwardRefs(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")
	taskRepo := &creatingTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()
	idGen := &stubIDGen{next: 500}

	specs := []PlanTaskSpec{
		{Title: "A", BlockedBy: []BlockedByRef{{Index: 1}}}, // forward
		{Title: "B"},
		{Title: "C", BlockedBy: []BlockedByRef{{Index: 0}, {Index: 1}}}, // backward
	}

	result, err := svc.CreateTasksFromPlan(ctx, "trk", specs, "", idGen)
	if err != nil {
		t.Fatalf("mixed refs should succeed: %v", err)
	}

	ids := result.CreatedIDs
	if len(ids) != 3 {
		t.Fatalf("ids len = %d, want 3", len(ids))
	}

	// Task A blocked by B (forward).
	taskA := taskRepo.created[0]
	bbA, _ := taskA.Meta["blocked_by"].([]string)
	if len(bbA) != 1 || bbA[0] != ids[1] {
		t.Errorf("task A blocked_by = %v, want [%s]", bbA, ids[1])
	}

	// Task C blocked by A and B (backward).
	taskC := taskRepo.created[2]
	bbC, _ := taskC.Meta["blocked_by"].([]string)
	if len(bbC) != 2 || bbC[0] != ids[0] || bbC[1] != ids[1] {
		t.Errorf("task C blocked_by = %v, want [%s, %s]",
			bbC, ids[0], ids[1])
	}
}
