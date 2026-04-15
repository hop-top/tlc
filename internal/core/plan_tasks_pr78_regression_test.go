package core

// Regression tests for PR #78 review fixes:
//   1. Cross-project refs stored in meta["blocked_by_cross_project"],
//      not meta["blocked_by_unresolved"].
//   2. Error message for invalid tlc:// URI mentions both URI forms.

import (
	"context"
	"strings"
	"testing"
)

// TestCreateTasksFromPlan_CrossProjectRefMetaKey verifies that
// cross-project blocked-by refs (e.g. "hop-top/other#T-0001")
// are stored under meta["blocked_by_cross_project"] and NOT under
// meta["blocked_by_unresolved"]. This prevents
// ResolvePendingCrossTrackRefs from treating them as corrupted
// cross-track entries.
func TestCreateTasksFromPlan_CrossProjectRefMetaKey(t *testing.T) {
	trackRepo := newStubTrackRepo()
	ensureTrack(t, trackRepo, "my-trk")
	taskRepo := &creatingTaskRepo{}
	svc := NewTrackService(trackRepo, taskRepo)
	ctx := context.Background()
	idGen := &stubIDGen{next: 200}

	specs := []PlanTaskSpec{
		{Title: "Upstream task"},
		{
			Title: "Depends on external project",
			BlockedBy: []BlockedByRef{{
				CrossProject: &CrossProjectRef{
					ProjectID: "hop-top/other",
					TaskID:    "T-0001",
				},
			}},
		},
	}

	result, err := svc.CreateTasksFromPlan(
		ctx, "my-trk", specs, "proj-x", idGen,
	)
	if err != nil {
		t.Fatalf("CreateTasksFromPlan failed: %v", err)
	}

	// The cross-project ref should be reported as unresolved.
	if len(result.UnresolvedRefs) != 1 {
		t.Fatalf("UnresolvedRefs = %v, want 1 entry", result.UnresolvedRefs)
	}
	if result.UnresolvedRefs[0] != "hop-top/other#T-0001" {
		t.Errorf("UnresolvedRefs[0] = %q, want %q",
			result.UnresolvedRefs[0], "hop-top/other#T-0001")
	}

	// Inspect the second created task's meta.
	if len(taskRepo.created) < 2 {
		t.Fatalf("expected 2 created tasks, got %d", len(taskRepo.created))
	}
	meta := taskRepo.created[1].Meta

	// Must be in blocked_by_cross_project.
	crossProj, ok := meta[metaKeyCrossProject]
	if !ok {
		t.Fatal("meta missing blocked_by_cross_project key")
	}
	cpSlice, ok := crossProj.([]string)
	if !ok {
		t.Fatalf("blocked_by_cross_project type = %T, want []string",
			crossProj)
	}
	if len(cpSlice) != 1 || cpSlice[0] != "hop-top/other#T-0001" {
		t.Errorf("blocked_by_cross_project = %v, want [hop-top/other#T-0001]",
			cpSlice)
	}

	// Must NOT be in blocked_by_unresolved.
	if _, exists := meta[metaKeyUnresolved]; exists {
		t.Errorf("cross-project ref leaked into blocked_by_unresolved: %v",
			meta[metaKeyUnresolved])
	}
}

// TestResolvePendingCrossTrackRefs_IgnoresCrossProjectRefs
// verifies that ResolvePendingCrossTrackRefs does not process
// entries stored under meta["blocked_by_cross_project"]. Those
// refs must remain untouched — they belong to a separate
// cross-project resolver.
func TestResolvePendingCrossTrackRefs_IgnoresCrossProjectRefs(
	t *testing.T,
) {
	ctx := context.Background()

	trackRepo := newStubTrackRepo()
	taskRepo := &stubTaskRepo{}

	// Task with a cross-project ref in the dedicated key.
	// It must NOT appear in StillUnresolved and must NOT be
	// treated as corrupted.
	task := &Task{
		ID: "T-0300", Title: "cross-proj dep",
		TrackID: strPtr("alpha"),
		Meta: map[string]any{
			metaKeyCrossProject: []string{"hop-top/other#T-0001"},
		},
	}
	taskRepo.tasks = []*Task{task}
	svc := NewTrackService(trackRepo, taskRepo)

	res, err := svc.ResolvePendingCrossTrackRefs(ctx, "", nil)
	if err != nil {
		t.Fatalf("phase 2: %v", err)
	}

	// No promotions should have happened.
	if res.PromotedTasks != 0 {
		t.Errorf("PromotedTasks = %d, want 0", res.PromotedTasks)
	}

	// The cross-project ref must NOT appear in StillUnresolved.
	for _, u := range res.StillUnresolved {
		if strings.Contains(u.Ref, "hop-top/other") {
			t.Errorf("cross-project ref leaked into StillUnresolved: %+v", u)
		}
	}

	// The cross-project key must remain intact on the task.
	cp, ok := task.Meta[metaKeyCrossProject]
	if !ok {
		t.Error("blocked_by_cross_project was removed from task meta")
	}
	cpSlice := toStringSlice(cp)
	if len(cpSlice) != 1 || cpSlice[0] != "hop-top/other#T-0001" {
		t.Errorf("blocked_by_cross_project mutated: %v", cpSlice)
	}
}

// TestParseTLCURI_InvalidMentionsBothForms verifies that parsing
// an invalid tlc:// URI produces an error message containing both
// the full form (tlc://<org>/<project>/<T-NNNN>) and the local
// form (tlc:///T-NNNN).
func TestParseTLCURI_InvalidMentionsBothForms(t *testing.T) {
	invalids := []string{
		"tlc://incomplete",
		"tlc://org/project/bad-id",
		"tlc:///not-a-task",
	}
	for _, s := range invalids {
		_, err := parseBlockedByRef(s)
		if err == nil {
			t.Errorf("parseBlockedByRef(%q): expected error", s)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, "tlc://<org>/<project>/<T-NNNN>") {
			t.Errorf("parseBlockedByRef(%q): error %q missing full URI form",
				s, msg)
		}
		if !strings.Contains(msg, "tlc:///T-NNNN") {
			t.Errorf("parseBlockedByRef(%q): error %q missing local URI form",
				s, msg)
		}
	}
}
