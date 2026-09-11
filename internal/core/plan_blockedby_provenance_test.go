package core

import (
	"context"
	"testing"
)

// These tests pin the provenance contract for plan-managed blocked-by
// edges: an --add-plan run owns exactly the edges it declared, and must
// leave every other edge alone.

// TestReconcile_PreservesOutOfBandBlockedBy asserts that reconciling a
// plan whose frontmatter declares no blocked-by for a task does not
// destroy an edge added out-of-band (task update --add-blocked-by).
func TestReconcile_PreservesOutOfBandBlockedBy(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const manualBlocker = "task_01manualblocker0000000000"
	const mappedID = "task_01planrow000000000000000"
	taskRepo := newResolvingTaskRepo(
		blockerTask(manualBlocker, "T-0968", "Manual blocker"),
		&Task{
			ID: mappedID, Title: "Plan task", Status: StatusTodo,
			TrackID: strPtr("trk"),
			// Edge set out-of-band; plan never mentioned it.
			Meta: map[string]any{"blocked_by": []string{manualBlocker}},
		},
	)
	svc := NewTrackService(trackRepo, taskRepo)

	// Plan frontmatter declares NO blocked-by for this task.
	specs := []PlanTaskSpec{{Title: "Plan task"}}

	if _, err := svc.ReconcileTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
		map[int]string{0: mappedID},
	); err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	stored, _ := taskRepo.GetTask(context.Background(), mappedID)
	got := stored.BlockedBy()
	if len(got) != 1 || got[0] != manualBlocker {
		t.Errorf("blocked_by = %v, want [%s] preserved (plan did not declare it)",
			got, manualBlocker)
	}
}

// TestReconcile_PlanCanRemoveEdgeItDeclared asserts the other half of
// the contract: an edge a previous ingest of this plan authored IS
// removed when the plan stops declaring it. Provenance must not
// degenerate into "never clear anything".
func TestReconcile_PlanCanRemoveEdgeItDeclared(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const planBlocker = "task_01planblocker00000000000"
	const mappedID = "task_01planrow000000000000000"
	taskRepo := newResolvingTaskRepo(
		blockerTask(planBlocker, "T-0968", "Plan-declared blocker"),
		&Task{
			ID: mappedID, Title: "Plan task", Status: StatusTodo,
			TrackID: strPtr("trk"),
			// Edge present AND marked as authored by the plan.
			Meta: map[string]any{
				"blocked_by":     []string{planBlocker},
				metaKeyPlanOwned: []string{planBlocker},
			},
		},
	)
	svc := NewTrackService(trackRepo, taskRepo)

	// Plan no longer declares the edge.
	specs := []PlanTaskSpec{{Title: "Plan task"}}

	if _, err := svc.ReconcileTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
		map[int]string{0: mappedID},
	); err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	stored, _ := taskRepo.GetTask(context.Background(), mappedID)
	if got := stored.BlockedBy(); len(got) != 0 {
		t.Errorf("blocked_by = %v, want empty (plan retracted the edge it authored)", got)
	}
	if _, ok := stored.Meta[metaKeyPlanOwned]; ok {
		t.Errorf("%s should be cleared alongside the edge, got %v",
			metaKeyPlanOwned, stored.Meta[metaKeyPlanOwned])
	}
}

// TestReconcile_MixedManualAndPlanEdges covers both provenances on one
// task: the plan retracts its own edge while the manual one survives.
func TestReconcile_MixedManualAndPlanEdges(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const planBlocker = "task_01planblocker00000000000"
	const manualBlocker = "task_01manualblocker0000000000"
	const mappedID = "task_01planrow000000000000000"
	taskRepo := newResolvingTaskRepo(
		blockerTask(planBlocker, "T-0001", "Plan blocker"),
		blockerTask(manualBlocker, "T-0002", "Manual blocker"),
		&Task{
			ID: mappedID, Title: "Plan task", Status: StatusTodo,
			TrackID: strPtr("trk"),
			Meta: map[string]any{
				"blocked_by":     []string{planBlocker, manualBlocker},
				metaKeyPlanOwned: []string{planBlocker},
			},
		},
	)
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{{Title: "Plan task"}}

	if _, err := svc.ReconcileTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
		map[int]string{0: mappedID},
	); err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	stored, _ := taskRepo.GetTask(context.Background(), mappedID)
	got := stored.BlockedBy()
	if len(got) != 1 || got[0] != manualBlocker {
		t.Errorf("blocked_by = %v, want only manual edge [%s]", got, manualBlocker)
	}
}

// TestReconcile_RecordsPlanProvenance asserts that a plan-declared edge
// is marked as plan-owned so a later run can retract it.
func TestReconcile_RecordsPlanProvenance(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const blockerID = "task_01blocker00000000000000"
	const mappedID = "task_01planrow000000000000000"
	taskRepo := newResolvingTaskRepo(
		blockerTask(blockerID, "T-0968", "Blocker"),
		&Task{
			ID: mappedID, Title: "Plan task", Status: StatusTodo,
			TrackID: strPtr("trk"), Meta: map[string]any{},
		},
	)
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{{
		Title:     "Plan task",
		BlockedBy: []BlockedByRef{{TaskID: "T-0968"}},
	}}

	if _, err := svc.ReconcileTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
		map[int]string{0: mappedID},
	); err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	stored, _ := taskRepo.GetTask(context.Background(), mappedID)
	owned := NormalizeStringSliceMeta(stored.Meta[metaKeyPlanOwned])
	if len(owned) != 1 || owned[0] != blockerID {
		t.Errorf("%s = %v, want [%s]", metaKeyPlanOwned, owned, blockerID)
	}
}

// TestCreateTasksFromPlan_RecordsPlanProvenance pins the same marking on
// the first-ingest path, so the first reconcile can retract correctly.
func TestCreateTasksFromPlan_RecordsPlanProvenance(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const blockerID = "task_01blocker00000000000000"
	taskRepo := newResolvingTaskRepo(
		blockerTask(blockerID, "T-0968", "Blocker"),
	)
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{{
		Title:     "Plan task",
		BlockedBy: []BlockedByRef{{TaskID: "T-0968"}},
	}}

	if _, err := svc.CreateTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
	); err != nil {
		t.Fatalf("CreateTasksFromPlan: %v", err)
	}

	owned := NormalizeStringSliceMeta(taskRepo.created[0].Meta[metaKeyPlanOwned])
	if len(owned) != 1 || owned[0] != blockerID {
		t.Errorf("%s = %v, want [%s]", metaKeyPlanOwned, owned, blockerID)
	}
}
