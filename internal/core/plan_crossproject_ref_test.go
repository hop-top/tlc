package core

import (
	"context"
	"strings"
	"testing"
)

// Cross-project blocked-by refs must never vanish: a ref that parses is
// either resolved, or deferred with a diagnostic. Silently dropping one
// leaves the plan asserting a dependency that does not exist.

// TestReconcile_CrossProjectRefNotSilentlyDropped asserts that a
// cross-project ref surviving into reconcile is recorded as deferred
// rather than falling through the switch and disappearing.
func TestReconcile_CrossProjectRefNotSilentlyDropped(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const mappedID = "task_01planrow000000000000000"
	taskRepo := newResolvingTaskRepo(&Task{
		ID: mappedID, Title: "Plan task", Status: StatusTodo,
		TrackID: strPtr("trk"), Meta: map[string]any{},
	})
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{{
		Title: "Plan task",
		BlockedBy: []BlockedByRef{{
			CrossProject: &CrossProjectRef{
				ProjectID: "hop-top/kit", TaskID: "T-0968",
			},
		}},
	}}

	if _, err := svc.ReconcileTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
		map[int]string{0: mappedID},
	); err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	stored, _ := taskRepo.GetTask(context.Background(), mappedID)
	cross := NormalizeStringSliceMeta(stored.Meta[metaKeyCrossProject])
	if len(cross) != 1 {
		t.Fatalf("%s = %v, want the cross-project ref recorded, not dropped",
			metaKeyCrossProject, cross)
	}
	if !strings.Contains(cross[0], "T-0968") {
		t.Errorf("recorded ref %q does not name the target task", cross[0])
	}
}

// TestReconcile_ReportsDeferredCrossProjectRefs asserts the reconcile
// result carries deferred cross-project refs so the CLI can print a
// diagnostic instead of reporting an unqualified "Unchanged".
func TestReconcile_ReportsDeferredCrossProjectRefs(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "trk")

	const mappedID = "task_01planrow000000000000000"
	taskRepo := newResolvingTaskRepo(&Task{
		ID: mappedID, Title: "Plan task", Status: StatusTodo,
		TrackID: strPtr("trk"), Meta: map[string]any{},
	})
	svc := NewTrackService(trackRepo, taskRepo)

	specs := []PlanTaskSpec{{
		Title: "Plan task",
		BlockedBy: []BlockedByRef{{
			CrossProject: &CrossProjectRef{
				ProjectID: "hop-top/kit", TaskID: "T-0968",
			},
		}},
	}}

	res, err := svc.ReconcileTasksFromPlan(
		context.Background(), "trk", specs, "proj", &stubIDGen{next: 1},
		map[int]string{0: mappedID},
	)
	if err != nil {
		t.Fatalf("ReconcileTasksFromPlan: %v", err)
	}

	if len(res.DeferredCrossProject) != 1 {
		t.Fatalf("DeferredCrossProject = %v, want 1 entry so the CLI can warn",
			res.DeferredCrossProject)
	}
	if !strings.Contains(res.DeferredCrossProject[0].Ref, "T-0968") {
		t.Errorf("deferred entry %+v does not name the target task",
			res.DeferredCrossProject[0])
	}
}

// TestParseBlockedByRef_SlashFormAccepted pins the canonical
// cross-project plan syntax: "<org>/<project>/<T-NNNN>", matching how
// refs are stored and how the CLI's --blocked-by accepts them.
func TestParseBlockedByRef_SlashFormAccepted(t *testing.T) {
	ref, err := parseBlockedByRef("hop-top/kit/T-0968")
	if err != nil {
		t.Fatalf("parseBlockedByRef(slash form): unexpected error: %v", err)
	}
	if ref.CrossProject == nil {
		t.Fatalf("slash form did not parse as cross-project: %+v", ref)
	}
	if ref.CrossProject.ProjectID != "hop-top/kit" {
		t.Errorf("ProjectID = %q, want %q", ref.CrossProject.ProjectID, "hop-top/kit")
	}
	if ref.CrossProject.TaskID != "T-0968" {
		t.Errorf("TaskID = %q, want %q", ref.CrossProject.TaskID, "T-0968")
	}
}

// TestParseBlockedByRef_ErrorNamesCanonicalForm asserts the parser's
// rejection message advertises the canonical slash form, so the error
// text agrees with the documented syntax.
func TestParseBlockedByRef_ErrorNamesCanonicalForm(t *testing.T) {
	_, err := parseBlockedByRef("totally bogus ref")
	if err == nil {
		t.Fatal("expected an error for a bogus ref")
	}
	if !strings.Contains(err.Error(), "<org>/<project>/<T-NNNN>") {
		t.Errorf("error %q does not advertise the canonical slash form", err)
	}
}

// TestParseBlockedByRef_HashFormStillAccepted keeps the legacy
// "<org/project>#<T-NNNN>" spelling working so existing plans do not
// break; it is deprecated, not removed.
func TestParseBlockedByRef_HashFormStillAccepted(t *testing.T) {
	ref, err := parseBlockedByRef("hop-top/kit#T-0968")
	if err != nil {
		t.Fatalf("legacy hash form rejected: %v", err)
	}
	if ref.CrossProject == nil ||
		ref.CrossProject.ProjectID != "hop-top/kit" ||
		ref.CrossProject.TaskID != "T-0968" {
		t.Errorf("hash form parsed incorrectly: %+v", ref.CrossProject)
	}
}
