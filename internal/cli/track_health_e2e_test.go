package cli

// End-to-end tests for track health summary and plan linkage (story 073).
// Exercises: track summary status counts, overcommit warning, --add-plan
// with/without frontmatter tasks, blocked-by resolution.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTrackHealth_E2E_SummaryStatusCounts creates tracks in various
// states and verifies that `track summary` shows correct counts.
func TestTrackHealth_E2E_SummaryStatusCounts(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		tracks := []*core.Track{
			{
				ID: "trk-a", Title: "A", Type: "feature",
				Status:    core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "trk-b", Title: "B", Type: "bug",
				Status:    core.TrackStatusPending,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "trk-c", Title: "C", Type: "feature",
				Status:    core.TrackStatusCompleted,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "trk-d", Title: "D", Type: "refactor",
				Status:    core.TrackStatusAbandoned,
				CreatedAt: now, UpdatedAt: now,
			},
		}
		for _, tr := range tracks {
			seedTrack(t, ctx, s, s, tr)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "summary"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track summary: %v", err)
		}

		out := buf.String()
		for _, want := range []string{
			"Active: 1", "Pending: 1",
			"Completed: 1", "Abandoned: 1",
		} {
			if !contains(out, want) {
				t.Errorf(
					"expected %q in summary output; got:\n%s",
					want, out,
				)
			}
		}
	})
}

// TestTrackHealth_E2E_OvercommitWarning creates >3 active tracks
// (default max_active=3) and verifies the overcommit warning.
func TestTrackHealth_E2E_OvercommitWarning(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		for _, id := range []string{
			"oc-1", "oc-2", "oc-3", "oc-4",
		} {
			seedTrack(t, ctx, s, s, &core.Track{
				ID: id, Title: id, Type: "feature",
				Status:    core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			})
		}

		// Default max_active=3; 4 active tracks -> overcommit.
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "summary"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track summary: %v", err)
		}

		out := buf.String()
		if !contains(out, "overcommitted") {
			t.Errorf(
				"expected overcommit warning; got:\n%s", out,
			)
		}
	})
}

// TestTrackHealth_E2E_NoWarningUnderThreshold sets max_active=5
// and creates only 2 active tracks; verifies no warning.
func TestTrackHealth_E2E_NoWarningUnderThreshold(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		viper.Set("tracks.health.max_active", 5)
		viper.Set("tracks.health.min_progress_to_start", 0)

		now := time.Now().UTC()
		for _, id := range []string{"ok-1", "ok-2"} {
			seedTrack(t, ctx, s, s, &core.Track{
				ID: id, Title: id, Type: "feature",
				Status:    core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			})
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "summary"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track summary: %v", err)
		}

		out := buf.String()
		if contains(out, "overcommitted") {
			t.Errorf(
				"unexpected overcommit warning; got:\n%s", out,
			)
		}
		if !contains(out, "ok") {
			t.Errorf("expected health ok; got:\n%s", out)
		}
	})
}

// TestTrackPlan_E2E_AddPlanWithTasks creates a plan file with
// frontmatter tasks, runs --add-plan, and verifies tasks are created
// with correct TrackID.
func TestTrackPlan_E2E_AddPlanWithTasks(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		track := &core.Track{
			ID: "plan-track", Title: "Plan Track",
			Type: "feature", Status: core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		}
		trackTypeID := seedTrack(t, ctx, s, s, track)

		planContent := `---
title: Test Plan
tracks: [plan-track]
tasks:
  - title: "Task A"
    tags: [phase:1]
  - title: "Task B"
    tags: [phase:1]
---
# Plan body
`
		planPath := filepath.Join(t.TempDir(), "test-plan.md")
		if err := os.WriteFile(
			planPath, []byte(planContent), 0o644,
		); err != nil {
			t.Fatalf("write plan: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "plan-track",
			"--add-plan", planPath,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update --add-plan: %v", err)
		}

		out := buf.String()
		if !contains(out, "created 2 tasks") {
			t.Errorf(
				"expected 'created 2 tasks' in output; got:\n%s",
				out,
			)
		}

		// Verify tasks exist with correct TrackID (now the track's TypeID).
		// Plan ingestion mints TypeID-shaped task IDs; look them up by seq.
		for _, seq := range []int64{1, 2} {
			task, err := s.GetTaskBySeq(ctx, "", seq)
			if err != nil {
				t.Fatalf("GetTaskBySeq seq=%d: %v", seq, err)
			}
			if task == nil {
				t.Fatalf("task seq=%d not found", seq)
			}
			if task.TrackID == nil ||
				*task.TrackID != trackTypeID {
				t.Errorf(
					"task seq=%d: expected TrackID=%s, got %v",
					seq, trackTypeID, task.TrackID,
				)
			}
		}

		// Verify plan is linked in track meta.
		tr, _ := s.GetTrack(ctx, trackTypeID)
		if tr.Meta == nil {
			t.Fatal("expected Meta with plans key")
		}
		plans, ok := tr.Meta["plans"]
		if !ok {
			t.Fatal("expected 'plans' key in Meta")
		}
		planSlice, ok := plans.([]any)
		if !ok {
			t.Fatalf("expected []any for plans, got %T", plans)
		}
		if len(planSlice) != 1 {
			t.Errorf("expected 1 plan linked, got %d", len(planSlice))
		}
	})
}

// TestTrackPlan_E2E_AddPlanLinkOnly verifies that --add-plan with
// a plan that has no tasks field links the plan without creating
// tasks.
func TestTrackPlan_E2E_AddPlanLinkOnly(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		trackTypeID := seedTrack(t, ctx, s, s, &core.Track{
			ID: "link-only", Title: "Link Only",
			Type: "feature", Status: core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		})

		planContent := `---
title: Design Notes
tracks: [link-only]
---
# Design notes without tasks
`
		planPath := filepath.Join(t.TempDir(), "design.md")
		if err := os.WriteFile(
			planPath, []byte(planContent), 0o644,
		); err != nil {
			t.Fatalf("write plan: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "link-only",
			"--add-plan", planPath,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update --add-plan: %v", err)
		}

		out := buf.String()
		if !contains(out, "no tasks extracted") {
			t.Errorf(
				"expected 'no tasks extracted' in output; got:\n%s",
				out,
			)
		}

		// Verify plan is linked but no tasks created.
		tr, _ := s.GetTrack(ctx, trackTypeID)
		if tr.Meta == nil || tr.Meta["plans"] == nil {
			t.Error("expected plan linked in Meta")
		}

		tasks, _ := s.ListTasks(ctx, core.Query{AllProjects: true})
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}
	})
}

// TestTrackPlan_E2E_BlockedByResolution verifies that plan tasks
// with blocked-by index references are resolved to real task IDs.
func TestTrackPlan_E2E_BlockedByResolution(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		seedTrack(t, ctx, s, s, &core.Track{
			ID: "dep-track", Title: "Dep Track",
			Type: "feature", Status: core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		})

		planContent := `---
title: Dependency Plan
tracks: [dep-track]
tasks:
  - title: "Task A"
    tags: [phase:1]
  - title: "Task B"
    tags: [phase:1]
    blocked-by: [0]
  - title: "Task C"
    tags: [phase:2]
    blocked-by: [0, 1]
---
# Plan with blocked-by
`
		planPath := filepath.Join(t.TempDir(), "dep-plan.md")
		if err := os.WriteFile(
			planPath, []byte(planContent), 0o644,
		); err != nil {
			t.Fatalf("write plan: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"track", "update", "dep-track",
			"--add-plan", planPath,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track update --add-plan: %v", err)
		}

		out := buf.String()
		if !contains(out, "created 3 tasks") {
			t.Errorf(
				"expected 'created 3 tasks'; got:\n%s", out,
			)
		}

		// Verify blocked_by resolved to real task IDs (now TypeIDs).
		// Plan ingestion mints TypeIDs for created tasks; look up by seq.
		taskA, err := s.GetTaskBySeq(ctx, "", 1)
		if err != nil || taskA == nil {
			t.Fatalf("GetTaskBySeq seq=1: %v", err)
		}
		taskB, err := s.GetTaskBySeq(ctx, "", 2)
		if err != nil || taskB == nil {
			t.Fatalf("GetTaskBySeq seq=2: %v", err)
		}
		blockedBy, ok := taskB.Meta["blocked_by"]
		if !ok {
			t.Fatal("seq=2 missing blocked_by in Meta")
		}
		blockedSlice, ok := blockedBy.([]any)
		if !ok {
			t.Fatalf(
				"expected []any for blocked_by, got %T",
				blockedBy,
			)
		}
		if len(blockedSlice) != 1 {
			t.Fatalf(
				"seq=2 expected 1 blocked_by, got %d",
				len(blockedSlice),
			)
		}
		if blockedSlice[0] != taskA.ID {
			t.Errorf(
				"seq=2 blocked_by[0] = %v, want %s",
				blockedSlice[0], taskA.ID,
			)
		}

		// Task C (seq=3) blocked by Task A (seq=1) and Task B (seq=2).
		taskC, err := s.GetTaskBySeq(ctx, "", 3)
		if err != nil || taskC == nil {
			t.Fatalf("GetTaskBySeq seq=3: %v", err)
		}
		blockedBy, ok = taskC.Meta["blocked_by"]
		if !ok {
			t.Fatal("seq=3 missing blocked_by in Meta")
		}
		blockedSlice, ok = blockedBy.([]any)
		if !ok {
			t.Fatalf(
				"expected []any for blocked_by, got %T",
				blockedBy,
			)
		}
		if len(blockedSlice) != 2 {
			t.Fatalf(
				"seq=3 expected 2 blocked_by, got %d",
				len(blockedSlice),
			)
		}
		if blockedSlice[0] != taskA.ID ||
			blockedSlice[1] != taskB.ID {
			t.Errorf(
				"seq=3 blocked_by = %v, want [%s %s]",
				blockedSlice, taskA.ID, taskB.ID,
			)
		}
	})
}
