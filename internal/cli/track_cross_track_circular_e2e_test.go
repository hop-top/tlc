package cli

// End-to-end tests for two-phase cross-track plan ingestion
// (story 076 scenarios 10-12, task T-0435).
//
// Covers:
//   - circular plan refs ingested in either order resolve
//     symmetrically and both plans end rewritten
//   - a ref to a never-created target track is deferred with a
//     warning and the plan is left untouched

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// writePlan writes a plan.md with the given body and returns path.
func writePlan(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// seedEmptyTrack creates an empty track with the given id (becomes
// the slug). Routes through TrackService so a TypeID is auto-minted
// and the (project_id, slug) UNIQUE constraint stays satisfied.
func seedEmptyTrack(t *testing.T, id string) {
	t.Helper()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	now := time.Now().UTC()
	seedTrack(t, ctxBG(), s, s, &core.Track{
		ID: id, Title: id, Type: "feature",
		Status:    core.TrackStatusActive,
		CreatedAt: now, UpdatedAt: now,
	})
}

// ingestPlan runs `track update <id> --add-plan <path>` and
// returns the combined stdout/stderr. It fails the test on
// non-nil error.
func ingestPlan(t *testing.T, trackID, planPath string) string {
	t.Helper()
	cmd := newTestCmd()
	cmd.AddCommand(TrackCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"track", "update", trackID, "--add-plan", planPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("ingest %s: %v\n%s", trackID, err, buf.String())
	}
	return buf.String()
}

// Circular plans used by scenarios 10 & 11. Each track has 3
// tasks; the third task of each refs the other track's task #2.
const planA = `---
title: A plan
tracks: [alpha]
tasks:
  - title: "A-one"
  - title: "A-two"
  - title: "A-three"
    blocked-by: ["beta#2"]
---
# A
`

const planB = `---
title: B plan
tracks: [beta]
tasks:
  - title: "B-one"
  - title: "B-two"
  - title: "B-three"
    blocked-by: ["alpha#2"]
---
# B
`

// assertFullyResolved verifies the end state after both plans
// are ingested: both third tasks have concrete T-IDs in
// blocked_by and no pending refs; both plan files are rewritten.
func assertFullyResolved(t *testing.T, pathA, pathB string) {
	t.Helper()
	ctx := ctxBG()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	// Resolve slugs → TypeIDs since task.track_id now stores the typeid.
	alphaT, _ := s.GetTrackBySlug(ctx, "", "alpha")
	if alphaT == nil {
		t.Fatalf("alpha track not found by slug")
	}
	betaT, _ := s.GetTrackBySlug(ctx, "", "beta")
	if betaT == nil {
		t.Fatalf("beta track not found by slug")
	}
	alphaID := alphaT.ID
	betaID := betaT.ID

	// A-three and B-three by lookup via track+title.
	aTasks, _ := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "track_id", Operator: core.OpEq, Value: alphaID},
			{Field: "title", Operator: core.OpEq, Value: "A-three"},
		},
		AllProjects: true,
	})
	if len(aTasks) != 1 {
		t.Fatalf("A-three lookup: %d results, want 1", len(aTasks))
	}
	bTasks, _ := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "track_id", Operator: core.OpEq, Value: betaID},
			{Field: "title", Operator: core.OpEq, Value: "B-three"},
		},
		AllProjects: true,
	})
	if len(bTasks) != 1 {
		t.Fatalf("B-three lookup: %d results, want 1", len(bTasks))
	}
	// B-two is the target of A-three's cross-track ref.
	bTwo, _ := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "track_id", Operator: core.OpEq, Value: betaID},
			{Field: "title", Operator: core.OpEq, Value: "B-two"},
		},
		AllProjects: true,
	})
	if len(bTwo) != 1 {
		t.Fatalf("B-two lookup: %d results, want 1", len(bTwo))
	}
	aTwo, _ := s.ListTasks(ctx, core.Query{
		Filters: []core.FieldFilter{
			{Field: "track_id", Operator: core.OpEq, Value: alphaID},
			{Field: "title", Operator: core.OpEq, Value: "A-two"},
		},
		AllProjects: true,
	})
	if len(aTwo) != 1 {
		t.Fatalf("A-two lookup: %d results, want 1", len(aTwo))
	}

	aThree := aTasks[0]
	bThree := bTasks[0]

	// blocked_by promoted to concrete IDs, unresolved cleared.
	gotA := aThree.BlockedBy()
	if len(gotA) != 1 || gotA[0] != bTwo[0].ID {
		t.Errorf("A-three blocked_by = %v, want [%s]", gotA, bTwo[0].ID)
	}
	if unresolved, ok := aThree.Meta["blocked_by_unresolved"]; ok {
		if slice, _ := unresolved.([]any); len(slice) > 0 {
			t.Errorf("A-three still has unresolved refs: %v", slice)
		}
	}
	gotB := bThree.BlockedBy()
	if len(gotB) != 1 || gotB[0] != aTwo[0].ID {
		t.Errorf("B-three blocked_by = %v, want [%s]", gotB, aTwo[0].ID)
	}
	if unresolved, ok := bThree.Meta["blocked_by_unresolved"]; ok {
		if slice, _ := unresolved.([]any); len(slice) > 0 {
			t.Errorf("B-three still has unresolved refs: %v", slice)
		}
	}

	// Both plan.md files rewritten: no more "alpha#2" or "beta#2".
	rawA, _ := os.ReadFile(pathA)
	if strings.Contains(string(rawA), "beta#2") {
		t.Errorf("plan A still contains beta#2 after resolution:\n%s", rawA)
	}
	rawB, _ := os.ReadFile(pathB)
	if strings.Contains(string(rawB), "alpha#2") {
		t.Errorf("plan B still contains alpha#2 after resolution:\n%s", rawB)
	}
}

// TestTrackPlan_E2E_Circular_AThenB ingests two mutually-
// referencing plans in A-then-B order and asserts both sides
// end fully resolved.
func TestTrackPlan_E2E_Circular_AThenB(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		dir := t.TempDir()
		pathA := writePlan(t, dir, "alpha.md", planA)
		pathB := writePlan(t, dir, "beta.md", planB)
		seedEmptyTrack(t, "alpha")
		seedEmptyTrack(t, "beta")

		// Phase 1 of A: deferred (beta not ingested yet).
		outA := ingestPlan(t, "alpha", pathA)
		if !strings.Contains(outA, "deferred") &&
			!strings.Contains(outA, "unresolved") &&
			!strings.Contains(outA, "pending") {
			t.Errorf(
				"expected A's ingest to mention deferred refs; got:\n%s",
				outA,
			)
		}
		// A's plan.md should NOT yet be rewritten.
		rawA1, _ := os.ReadFile(pathA)
		if !strings.Contains(string(rawA1), "beta#2") {
			t.Errorf(
				"A's plan was rewritten prematurely:\n%s", rawA1,
			)
		}

		// Phase 1 of B: B refs alpha#2 which is present now,
		// so B's ref resolves immediately. Phase 2 runs
		// project-wide and picks up A's pending ref.
		_ = ingestPlan(t, "beta", pathB)

		assertFullyResolved(t, pathA, pathB)
	})
}

// TestTrackPlan_E2E_Circular_BThenA ingests the same circular
// plans in the opposite order and asserts the end state is
// identical (two-phase symmetry).
func TestTrackPlan_E2E_Circular_BThenA(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		dir := t.TempDir()
		pathA := writePlan(t, dir, "alpha.md", planA)
		pathB := writePlan(t, dir, "beta.md", planB)
		seedEmptyTrack(t, "alpha")
		seedEmptyTrack(t, "beta")

		_ = ingestPlan(t, "beta", pathB)
		// B's plan.md should NOT yet be rewritten.
		rawB1, _ := os.ReadFile(pathB)
		if !strings.Contains(string(rawB1), "alpha#2") {
			t.Errorf(
				"B's plan was rewritten prematurely:\n%s", rawB1,
			)
		}

		_ = ingestPlan(t, "alpha", pathA)

		assertFullyResolved(t, pathA, pathB)
	})
}

// TestTrackPlan_E2E_DeferredMissingTrackNoRewrite verifies that
// a ref to a never-created track is deferred (not a hard error),
// a warning is logged, and the plan.md is left untouched.
func TestTrackPlan_E2E_DeferredMissingTrackNoRewrite(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		dir := t.TempDir()
		body := `---
title: Lonely
tracks: [lonely]
tasks:
  - title: "only-task"
    blocked-by: ["never-created#1"]
---
# Lonely
`
		path := writePlan(t, dir, "lonely.md", body)
		seedEmptyTrack(t, "lonely")

		out := ingestPlan(t, "lonely", path)
		if !strings.Contains(out, "never-created") {
			t.Errorf(
				"expected warning mentioning 'never-created'; got:\n%s",
				out,
			)
		}

		// Task exists with unresolved ref. Resolve slug → TypeID first.
		s, _ := getStorageRaw()
		defer s.Close()
		lonelyT, _ := s.GetTrackBySlug(ctxBG(), "", "lonely")
		if lonelyT == nil {
			t.Fatalf("lonely track not found by slug")
		}
		tasks, _ := s.ListTasks(ctxBG(), core.Query{
			Filters: []core.FieldFilter{
				{Field: "track_id", Operator: core.OpEq, Value: lonelyT.ID},
			},
			AllProjects: true,
		})
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		task := tasks[0]
		if len(task.BlockedBy()) != 0 {
			t.Errorf(
				"unexpected blocked_by promotion: %v", task.BlockedBy(),
			)
		}
		unresolved, ok := task.Meta["blocked_by_unresolved"]
		if !ok {
			t.Fatalf(
				"expected blocked_by_unresolved in meta; got %+v", task.Meta,
			)
		}
		// Accept either []string or []any storage shape.
		gotRef := ""
		switch v := unresolved.(type) {
		case []string:
			if len(v) == 1 {
				gotRef = v[0]
			}
		case []any:
			if len(v) == 1 {
				if s, ok := v[0].(string); ok {
					gotRef = s
				}
			}
		}
		if gotRef != "never-created#1" {
			t.Errorf(
				"unresolved ref = %q, want %q",
				gotRef, "never-created#1",
			)
		}

		// Plan.md untouched.
		raw, _ := os.ReadFile(path)
		if !strings.Contains(string(raw), `"never-created#1"`) {
			t.Errorf("plan was rewritten prematurely:\n%s", raw)
		}
	})
}
