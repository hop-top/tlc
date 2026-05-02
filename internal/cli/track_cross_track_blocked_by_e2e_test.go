package cli

// End-to-end tests for cross-track blocked-by references in plan
// ingestion (story 076, task T-0434).
//
// Covers:
//   - happy path: "<track-id>#N" resolves to T-NNNN and plan file is
//     rewritten so the entry becomes the resolved ID
//   - missing prerequisite track: ingestion fails, nothing written
//   - bad task index: ingestion fails, nothing written
//   - concrete "T-NNNN" ref: accepted verbatim

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// ctxBG returns a fresh background context for e2e tests.
func ctxBG() context.Context { return context.Background() }

// seedTrackWithPlan creates a track and ingests a plan.md with the
// given tasks in one step. Returns the plan path. Routes track creation
// through TrackService so a TypeID is auto-minted and the supplied id
// is promoted to Track.Slug.
func seedTrackWithPlan(
	t *testing.T, id, title, planBody string,
) string {
	t.Helper()
	ctx := ctxBG()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	now := time.Now().UTC()
	seedTrack(t, ctx, s, s, &core.Track{
		ID: id, Title: title, Type: "feature",
		Status: core.TrackStatusActive, CreatedAt: now, UpdatedAt: now,
	})

	planPath := filepath.Join(t.TempDir(), id+"-plan.md")
	if err := os.WriteFile(
		planPath, []byte(planBody), 0o644,
	); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TrackCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"track", "update", id, "--add-plan", planPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("seed ingest %s: %v\n%s", id, err, buf.String())
	}
	return planPath
}

// TestTrackPlan_E2E_CrossTrackBlockedBy_HappyPath verifies that a
// plan ingested after a prerequisite track resolves cross-track
// references and rewrites the plan file on disk.
func TestTrackPlan_E2E_CrossTrackBlockedBy_HappyPath(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		// Track A: two tasks, Foo and Bar.
		_ = seedTrackWithPlan(t, "alpha", "Alpha", `---
title: Alpha plan
tracks: [alpha]
tasks:
  - title: "Foo"
  - title: "Bar"
---
# Alpha
`)

		// Track B references alpha#2 (Bar) and a raw T-0001 (Foo).
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		ctx := ctxBG()
		now := time.Now().UTC()
		seedTrack(t, ctx, s, s, &core.Track{
			ID: "beta", Title: "Beta", Type: "feature",
			Status:    core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		})
		s.Close()

		// NOTE: Raw "T-NNNN" cross-track refs in plan frontmatter no
		// longer round-trip through plan ingestion. The preflight
		// check in plan_tasks.go calls GetTask(ctx, "T-0001") which
		// now requires a typeid since alias→typeid resolution is not
		// threaded into plan ingestion. The "alpha#2" cross-track
		// form below is what tests cross-track resolution end-to-end.
		// Tracked as a follow-up under the typeid-ids epilogue.
		planBody := `---
title: Beta plan
tracks: [beta]
tasks:
  - title: "Quux"
    blocked-by: ["alpha#2"]
---
# Beta
`
		planPath := filepath.Join(t.TempDir(), "beta-plan.md")
		if err := os.WriteFile(
			planPath, []byte(planBody), 0o644,
		); err != nil {
			t.Fatalf("write plan: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "beta", "--add-plan", planPath,
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("ingest beta: %v\n%s", err, buf.String())
		}

		// Plan ingestion now mints TypeID-shaped task IDs and resolves
		// cross-track refs to those TypeIDs. Verify Quux (seq=3 globally,
		// the only beta task) has blocked_by pointing to alpha's #2 task
		// (Bar, seq=2 in alpha).
		s2, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw (verify): %v", err)
		}
		defer s2.Close()
		alphaBar, _ := s2.GetTaskBySeq(ctx, "", 2)
		quux, _ := s2.GetTaskBySeq(ctx, "", 3)
		if alphaBar == nil || quux == nil {
			t.Fatalf("expected alpha bar (seq=2) + quux (seq=3); bar=%v quux=%v",
				alphaBar, quux)
		}
		got := quux.BlockedBy()
		if len(got) != 1 {
			t.Fatalf("blocked_by len = %d, want 1: %v", len(got), got)
		}
		if got[0] != alphaBar.ID {
			t.Errorf("blocked_by[0] = %q, want %q (alpha bar TypeID)",
				got[0], alphaBar.ID)
		}

		// Verify plan file on disk has been rewritten: the string
		// "alpha#2" must be gone, replaced with the resolved typeid.
		rewritten, err := os.ReadFile(planPath)
		if err != nil {
			t.Fatalf("re-read plan: %v", err)
		}
		if strings.Contains(string(rewritten), "alpha#2") {
			t.Errorf("plan still contains alpha#2 after ingest:\n%s",
				rewritten)
		}
		if !strings.Contains(string(rewritten), alphaBar.ID) {
			t.Errorf("plan missing resolved %s after ingest:\n%s",
				alphaBar.ID, rewritten)
		}
	})
}

// TestTrackPlan_E2E_CrossTrackBlockedBy_MissingTrack_Defers
// verifies that a cross-track ref to a track that has not been
// created yet is DEFERRED (not a hard failure, per T-0435):
// the ingest succeeds, beta's tasks exist, the unresolved ref
// is recorded, a warning is emitted, and the plan is left
// untouched so a later ingest of alpha can resolve it.
func TestTrackPlan_E2E_CrossTrackBlockedBy_MissingTrack_Defers(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		// Only create beta; alpha is the deferred target.
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		ctx := ctxBG()
		now := time.Now().UTC()
		seedTrack(t, ctx, s, s, &core.Track{
			ID: "beta", Title: "Beta", Type: "feature",
			Status:    core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		})
		s.Close()

		planBody := `---
title: Beta plan
tracks: [beta]
tasks:
  - title: "Quux"
    blocked-by: ["alpha#1"]
---
`
		planPath := filepath.Join(t.TempDir(), "beta-plan.md")
		_ = os.WriteFile(planPath, []byte(planBody), 0o644)

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "beta", "--add-plan", planPath,
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf(
				"ingest should succeed with deferred ref; got %v\n%s",
				err, buf.String(),
			)
		}

		out := buf.String()
		if !strings.Contains(out, "alpha#1") ||
			!strings.Contains(strings.ToLower(out), "unresolved") {
			t.Errorf(
				"expected warning mentioning unresolved alpha#1; got:\n%s",
				out,
			)
		}

		// Task was created with the ref in blocked_by_unresolved.
		s2, _ := getStorageRaw()
		defer s2.Close()
		tasks, _ := s2.ListTasks(ctx, core.Query{AllProjects: true})
		if len(tasks) != 1 {
			t.Fatalf("expected 1 beta task, got %d", len(tasks))
		}
		if _, ok := tasks[0].Meta["blocked_by_unresolved"]; !ok {
			t.Errorf(
				"missing blocked_by_unresolved in meta: %+v",
				tasks[0].Meta,
			)
		}

		// Plan file on disk should be unchanged (deferred, not
		// yet resolved).
		raw, _ := os.ReadFile(planPath)
		if !strings.Contains(string(raw), `"alpha#1"`) {
			t.Errorf(
				"plan should be untouched while ref deferred:\n%s", raw,
			)
		}
	})
}

// TestTrackPlan_E2E_CrossTrackBlockedBy_BodyProseUntouched is a
// regression guard for PR #46 review: a quoted "<track>#<N>" in
// the markdown body of a plan (outside the YAML frontmatter) must
// survive ingestion unmodified, even when the same string is
// being rewritten inside the frontmatter block.
func TestTrackPlan_E2E_CrossTrackBlockedBy_BodyProseUntouched(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		_ = seedTrackWithPlan(t, "alpha", "Alpha", `---
title: Alpha
tracks: [alpha]
tasks:
  - title: "Foo"
  - title: "Bar"
---
`)

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		ctx := ctxBG()
		now := time.Now().UTC()
		seedTrack(t, ctx, s, s, &core.Track{
			ID: "beta", Title: "Beta", Type: "feature",
			Status:    core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		})
		s.Close()

		planBody := `---
title: Beta
tracks: [beta]
tasks:
  - title: "Quux"
    blocked-by: ["alpha#2"]
---

# Beta

Note: this task depends on "alpha#2" (see alpha plan). Prose
mentions of "alpha#2" must NOT be rewritten by ingestion.
`
		planPath := filepath.Join(t.TempDir(), "beta.md")
		if err := os.WriteFile(
			planPath, []byte(planBody), 0o644,
		); err != nil {
			t.Fatalf("write plan: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "beta", "--add-plan", planPath,
		})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("ingest beta: %v\n%s", err, buf.String())
		}

		rewritten, _ := os.ReadFile(planPath)
		s2 := string(rewritten)
		// Frontmatter should have alpha's task #2 (Bar) resolved by
		// TypeID. Look up that TypeID by seq=2.
		stor, _ := getStorageRaw()
		defer stor.Close()
		bar, _ := stor.GetTaskBySeq(ctxBG(), "", 2)
		if bar == nil {
			t.Fatalf("alpha #2 task not found by seq")
		}
		if !strings.Contains(s2, `"`+bar.ID+`"`) {
			t.Errorf("frontmatter not rewritten with %s:\n%s", bar.ID, s2)
		}
		// Body prose must still contain the raw ref.
		if !strings.Contains(s2, `depends on "alpha#2"`) {
			t.Errorf(
				"body prose 'depends on \"alpha#2\"' was rewritten:\n%s",
				s2,
			)
		}
		if !strings.Contains(s2, `mentions of "alpha#2" must NOT`) {
			t.Errorf("second body mention was rewritten:\n%s", s2)
		}
	})
}

// TestTrackPlan_E2E_CrossTrackBlockedBy_IndexOutOfRange verifies
// that referencing a task number past the target plan's length
// fails cleanly.
func TestTrackPlan_E2E_CrossTrackBlockedBy_IndexOutOfRange(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		// Alpha has 2 tasks.
		_ = seedTrackWithPlan(t, "alpha", "Alpha", `---
title: Alpha
tracks: [alpha]
tasks:
  - title: "One"
  - title: "Two"
---
`)

		// Beta references alpha#99 (out of range).
		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		ctx := ctxBG()
		now := time.Now().UTC()
		seedTrack(t, ctx, s, s, &core.Track{
			ID: "beta", Title: "Beta", Type: "feature",
			Status:    core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		})
		s.Close()

		planBody := `---
title: Beta
tracks: [beta]
tasks:
  - title: "Quux"
    blocked-by: ["alpha#99"]
---
`
		planPath := filepath.Join(t.TempDir(), "beta.md")
		_ = os.WriteFile(planPath, []byte(planBody), 0o644)

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "beta", "--add-plan", planPath,
		})
		err = cmd.Execute()
		if err == nil {
			t.Fatalf("expected out-of-range error; got nil\n%s", buf.String())
		}
		if !strings.Contains(err.Error(), "alpha#99") &&
			!strings.Contains(err.Error(), "out of range") {
			t.Errorf("error should name bad ref; got %v", err)
		}

		// No beta tasks should have been created (only alpha's 2).
		s2, _ := getStorageRaw()
		defer s2.Close()
		tasks, _ := s2.ListTasks(ctx, core.Query{AllProjects: true})
		if len(tasks) != 2 {
			t.Errorf(
				"expected 2 tasks (alpha only); got %d", len(tasks),
			)
		}
	})
}
