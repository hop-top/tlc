package cli

// End-to-end tests for track list and show commands.
// Exercises full CLI pipeline: flag parsing -> storage -> query -> render.

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTrackList_E2E_BasicTable creates tracks, lists them, and
// verifies all expected columns appear in the table output.
func TestTrackList_E2E_BasicTable(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		assignee := "dev-1"
		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		for _, tr := range []*core.Track{
			{
				ID: "feat-login", Title: "Login Feature",
				Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
				AssignedTo: &assignee, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "bug-crash", Title: "Fix Crash",
				Type: core.TrackTypeBug, Status: core.TrackStatusPending,
				CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track: %v", err)
			}
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list: %v", err)
		}

		out := buf.String()
		// Verify column headers.
		for _, col := range []string{
			"ID", "Title", "Type", "Status",
			"State", "Progress", "Assignee",
		} {
			if !contains(out, col) {
				t.Errorf("expected column %q in output, got:\n%s", col, out)
			}
		}
		// Verify data rows.
		for _, want := range []string{
			"feat-login", "Login Feature", "feature", "active",
			"bug-crash", "Fix Crash", "bug", "pending",
		} {
			if !contains(out, want) {
				t.Errorf("expected %q in output, got:\n%s", want, out)
			}
		}
	})
}

// TestTrackList_E2E_FilterByStatus creates active + pending tracks,
// filters by --status active, verifies only active tracks appear.
func TestTrackList_E2E_FilterByStatus(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		for _, tr := range []*core.Track{
			{
				ID: "active-track", Title: "Active One",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "pending-track", Title: "Pending One",
				Type: "feature", Status: core.TrackStatusPending,
				CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track: %v", err)
			}
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list", "--status", "active"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --status active: %v", err)
		}

		out := buf.String()
		if !contains(out, "active-track") {
			t.Errorf("expected active-track, got:\n%s", out)
		}
		if contains(out, "pending-track") {
			t.Errorf("unexpected pending-track in output:\n%s", out)
		}
	})
}

// TestTrackList_E2E_FilterByType creates feature + bug tracks,
// filters by --type feature, verifies only feature tracks appear.
func TestTrackList_E2E_FilterByType(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		for _, tr := range []*core.Track{
			{
				ID: "feat-one", Title: "Feature One",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "bug-one", Title: "Bug One",
				Type: "bug", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track: %v", err)
			}
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list", "--type", "feature"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --type feature: %v", err)
		}

		out := buf.String()
		if !contains(out, "feat-one") {
			t.Errorf("expected feat-one, got:\n%s", out)
		}
		if contains(out, "bug-one") {
			t.Errorf("unexpected bug-one in output:\n%s", out)
		}
	})
}

// TestTrackList_E2E_FilterByState creates a stale track (old
// updated_at on linked task) and a healthy track, filters by
// --state stale, verifies only the stale track appears.
func TestTrackList_E2E_FilterByState(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		for _, tr := range []*core.Track{
			{
				ID: "stale-track", Title: "Stale Track",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "healthy-track", Title: "Healthy Track",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track: %v", err)
			}
		}

		// Link tasks: stale track gets a very old task (>7d default
		// threshold), healthy gets a fresh task.
		staleID := "stale-track"
		healthyID := "healthy-track"
		for _, task := range []*core.Task{
			{
				ID: "T-0001", Title: "Old task",
				Status:  core.StatusInProgress,
				Tags:    []string{},
				TrackID: &staleID,
				// 10 days ago — older than 7d default threshold.
				CreatedAt: now.Add(-10 * 24 * time.Hour),
				UpdatedAt: now.Add(-10 * 24 * time.Hour),
			},
			{
				ID: "T-0002", Title: "Fresh task",
				Status:    core.StatusInProgress,
				Tags:      []string{},
				TrackID:   &healthyID,
				CreatedAt: now,
				UpdatedAt: now,
			},
		} {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("create task: %v", err)
			}
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list", "--state", "stale"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --state stale: %v", err)
		}

		out := buf.String()
		if !contains(out, "stale-track") {
			t.Errorf("expected stale-track, got:\n%s", out)
		}
		if contains(out, "healthy-track") {
			t.Errorf("unexpected healthy-track in output:\n%s", out)
		}
	})
}

// TestTrackList_E2E_JSONOutput verifies JSON format output contains
// expected fields and valid JSON structure.
func TestTrackList_E2E_JSONOutput(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		assignee := "dev-1"
		track := &core.Track{
			ID: "json-e2e-track", Title: "JSON E2E Track",
			Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
			AssignedTo: &assignee, CreatedAt: now, UpdatedAt: now,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		viper.Set("output.format", "json")
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --format json: %v", err)
		}

		out := buf.String()

		// Validate JSON structure.
		var items []trackListOutput
		if err := json.Unmarshal([]byte(out), &items); err != nil {
			t.Fatalf("invalid JSON output: %v\n%s", err, out)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 track in JSON, got %d", len(items))
		}
		item := items[0]
		if item.ID != "json-e2e-track" {
			t.Errorf("expected id json-e2e-track, got %s", item.ID)
		}
		if item.Type != "feature" {
			t.Errorf("expected type feature, got %s", item.Type)
		}
		if item.Status != "active" {
			t.Errorf("expected status active, got %s", item.Status)
		}
		if item.Assignee != "@dev-1" {
			t.Errorf("expected assignee @dev-1, got %s", item.Assignee)
		}
	})
}

// TestTrackShow_E2E_PhaseBreakdown creates a track with phased
// tasks, runs show, verifies phase breakdown with checkmarks
// and per-phase task listings.
func TestTrackShow_E2E_PhaseBreakdown(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "phased-e2e", Title: "Phased E2E",
			Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		trackID := "phased-e2e"
		now := time.Now().UTC()
		tasks := []*core.Task{
			{
				ID: "T-0001", Title: "Schema design",
				Status: core.StatusDone, Tags: []string{"phase:1"},
				TrackID: &trackID, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "T-0002", Title: "API scaffold",
				Status: core.StatusDone, Tags: []string{"phase:1"},
				TrackID: &trackID, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "T-0003", Title: "Build handlers",
				Status: core.StatusInProgress, Tags: []string{"phase:2"},
				TrackID: &trackID, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "T-0004", Title: "Write tests",
				Status: core.StatusTodo, Tags: []string{"phase:2"},
				TrackID: &trackID, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "T-0005", Title: "Deploy infra",
				Status: core.StatusTodo, Tags: []string{"phase:3"},
				TrackID: &trackID, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "T-0006", Title: "Misc cleanup",
				Status: core.StatusTodo, Tags: []string{},
				TrackID: &trackID, CreatedAt: now, UpdatedAt: now,
			},
		}
		for _, task := range tasks {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("create task %s: %v", task.ID, err)
			}
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "phased-e2e"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show: %v", err)
		}

		out := buf.String()

		// Header fields.
		for _, want := range []string{
			"phased-e2e", "Phased E2E", "feature", "active",
		} {
			if !contains(out, want) {
				t.Errorf("expected %q in header, got:\n%s", want, out)
			}
		}

		// Progress line: 2 of 6 done.
		if !contains(out, "2/6") {
			t.Errorf("expected progress 2/6, got:\n%s", out)
		}

		// Phase 1 completed: should have checkmark.
		if !contains(out, "\u2713") {
			t.Errorf("expected checkmark for completed phase, got:\n%s", out)
		}

		// Phase task names.
		for _, want := range []string{
			"Schema design", "API scaffold",
			"Build handlers", "Write tests",
			"Deploy infra",
		} {
			if !contains(out, want) {
				t.Errorf("expected task %q in output, got:\n%s", want, out)
			}
		}

		// Status indicators.
		for _, want := range []string{"DONE", "IN_PROGRESS"} {
			if !contains(out, want) {
				t.Errorf("expected status %q, got:\n%s", want, out)
			}
		}

		// Unphased section with the misc task.
		if !contains(out, "Unphased") {
			t.Errorf("expected Unphased section, got:\n%s", out)
		}
		if !contains(out, "Misc cleanup") {
			t.Errorf("expected unphased task, got:\n%s", out)
		}
	})
}

// TestTrackShow_E2E_EmptyTrack verifies that showing a track with
// no linked tasks displays "no tasks linked" and unlinked state.
func TestTrackShow_E2E_EmptyTrack(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "empty-e2e", Title: "Empty E2E",
			Type: core.TrackTypeFeature, Status: core.TrackStatusPending,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "empty-e2e"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show: %v", err)
		}

		out := buf.String()
		if !contains(out, "no tasks linked") {
			t.Errorf("expected 'no tasks linked', got:\n%s", out)
		}
		if !contains(out, "unlinked") {
			t.Errorf("expected 'unlinked' state, got:\n%s", out)
		}
	})
}

// TestTrackShow_E2E_NotFound verifies that showing a nonexistent
// track returns an error with actionable message.
func TestTrackShow_E2E_NotFound(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "nonexistent-e2e"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for nonexistent track")
		}
		if !contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", err)
		}
	})
}

// TestTrackList_E2E_BlockedWhenAnyTaskBlocked verifies that a track is
// flagged as blocked when at least one non-terminal linked task has a
// blocked_by, even if other tasks are unblocked.
func TestTrackList_E2E_BlockedWhenAnyTaskBlocked(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		for _, tr := range []*core.Track{
			{
				ID: "partial-blocked", Title: "Partially Blocked",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "fully-clear", Title: "No Blockers",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track: %v", err)
			}
		}

		// partial-blocked: one blocked task, one unblocked task.
		trID := "partial-blocked"
		blockedMeta := map[string]interface{}{"blocked_by": []string{"T-9999"}}
		tasks := []*core.Task{
			{
				ID: "T-0010", Title: "Blocked task", Status: core.StatusTodo,
				TrackID: &trID, CreatedAt: now, UpdatedAt: now,
				Meta: blockedMeta,
			},
			{
				ID: "T-0011", Title: "Free task", Status: core.StatusInProgress,
				TrackID: &trID, CreatedAt: now, UpdatedAt: now,
			},
		}
		// fully-clear: one unblocked task only.
		clearID := "fully-clear"
		tasks = append(tasks, &core.Task{
			ID: "T-0012", Title: "Clear task", Status: core.StatusInProgress,
			TrackID: &clearID, CreatedAt: now, UpdatedAt: now,
		})
		for _, task := range tasks {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("create task %s: %v", task.ID, err)
			}
		}

		// Filter by --state blocked: only partial-blocked should appear.
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list", "--state", "blocked"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --state blocked: %v", err)
		}

		out := buf.String()
		if !contains(out, "partial-blocked") {
			t.Errorf("expected partial-blocked in blocked list, got:\n%s", out)
		}
		if contains(out, "fully-clear") {
			t.Errorf("unexpected fully-clear in blocked list, got:\n%s", out)
		}
	})
}

// TestTrackShow_E2E_BlockedState verifies that show renders 'blocked'
// in the state field when any non-terminal task has a blocked_by.
func TestTrackShow_E2E_BlockedState(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID: "show-blocked", Title: "Show Blocked",
			Type: "feature", Status: core.TrackStatusActive,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		trID := "show-blocked"
		blockedMeta := map[string]interface{}{"blocked_by": []string{"T-9999"}}
		blockedTask := &core.Task{
			ID: "T-0020", Title: "Blocked task", Status: core.StatusTodo,
			TrackID: &trID, CreatedAt: now, UpdatedAt: now,
			Meta: blockedMeta,
		}
		freeTask := &core.Task{
			ID: "T-0021", Title: "Free task", Status: core.StatusInProgress,
			TrackID: &trID, CreatedAt: now, UpdatedAt: now,
		}
		for _, task := range []*core.Task{blockedTask, freeTask} {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("create task %s: %v", task.ID, err)
			}
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "show-blocked"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show: %v", err)
		}

		out := buf.String()
		if !contains(out, "blocked") {
			t.Errorf("expected 'blocked' state in show output, got:\n%s", out)
		}
	})
}

// TestTrackSummary_E2E_StatusCounts creates tracks in various
// statuses, runs summary, and verifies the status counts.
func TestTrackSummary_E2E_StatusCounts(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		for _, tr := range []*core.Track{
			{
				ID: "sum-active-1", Title: "Active 1",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "sum-active-2", Title: "Active 2",
				Type: "bug", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "sum-active-3", Title: "Active 3",
				Type: "feature", Status: core.TrackStatusActive,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "sum-completed", Title: "Completed One",
				Type: "feature", Status: core.TrackStatusCompleted,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "sum-pending", Title: "Pending One",
				Type: "refactor", Status: core.TrackStatusPending,
				CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track %s: %v", tr.ID, err)
			}
		}

		// Link a task to one active track so progress is non-zero.
		trackID := "sum-active-1"
		task := &core.Task{
			ID: "T-0001", Title: "Some work",
			Status: core.StatusDone, Tags: []string{},
			TrackID: &trackID, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task: %v", err)
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
		if !contains(out, "Active: 3") {
			t.Errorf("expected Active: 3, got:\n%s", out)
		}
		if !contains(out, "Completed: 1") {
			t.Errorf("expected Completed: 1, got:\n%s", out)
		}
		if !contains(out, "Pending: 1") {
			t.Errorf("expected Pending: 1, got:\n%s", out)
		}
	})
}

// TestTrackList_E2E_DefaultScopeCurrentProject verifies that when
// running `tlc track list` (no --all-projects flag) inside a
// .tlc/-configured project, only tracks belonging to the current
// detected project are returned. Tracks from other projects must
// be filtered out by storage.ListTracks via core.DetectProject().
func TestTrackList_E2E_DefaultScopeCurrentProject(t *testing.T) {
	withTestLock(func() {
		// Establish project context: cwd has .tlc/config.yaml with
		// project.id=hop-top/tlc, so DetectProject() returns InProject=true.
		setupProjectScopedTestDir(t, "tlc-track-list-default-scope-", "hop-top/tlc")
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()
		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		inProj := "hop-top/tlc"
		otherProj := "other-org/foo"
		for _, tr := range []*core.Track{
			{
				ID: "in-proj-track", Title: "In Project",
				Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
				ProjectID: &inProj, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "other-proj-track", Title: "Other Project",
				Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
				ProjectID: &otherProj, CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track %s: %v", tr.ID, err)
			}
		}

		// Sanity: detection must place us in the in-project context.
		if det := core.DetectProject(); det == nil || !det.InProject ||
			det.ProjectID != inProj {
			t.Fatalf("expected DetectProject to return InProject=true with id=%s, got %+v",
				inProj, det)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list: %v", err)
		}

		out := buf.String()
		if !contains(out, "in-proj-track") {
			t.Errorf("expected in-proj-track in default-scope output, got:\n%s", out)
		}
		if contains(out, "other-proj-track") {
			t.Errorf("unexpected other-proj-track in default-scope output, got:\n%s", out)
		}
	})
}

// TestTrackList_E2E_AllProjectsFromInsideProject verifies that running
// `tlc track list --all-projects` from inside a .tlc/-configured project
// returns tracks from ALL projects (current + others) without erroring
// when re-fetching cross-project tracks for state computation.
//
// Regression test for T-0765: prior to the fix, the post-query
// state-compute step in runTrackList re-fetched each track via
// GetTrack which auto-scopes to the current project, causing
// "track <id> not found" for any track from a different project.
func TestTrackList_E2E_AllProjectsFromInsideProject(t *testing.T) {
	withTestLock(func() {
		setupProjectScopedTestDir(t, "tlc-track-list-all-projects-inside-", "hop-top/tlc")
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()
		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		inProj := "hop-top/tlc"
		otherProj := "other-org/foo"
		for _, tr := range []*core.Track{
			{
				ID: "in-proj-track", Title: "In Project",
				Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
				ProjectID: &inProj, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "other-proj-track", Title: "Other Project",
				Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
				ProjectID: &otherProj, CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track %s: %v", tr.ID, err)
			}
		}

		// Sanity: detection must place us in the in-project context.
		if det := core.DetectProject(); det == nil || !det.InProject ||
			det.ProjectID != inProj {
			t.Fatalf("expected DetectProject to return InProject=true with id=%s, got %+v",
				inProj, det)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list", "--all-projects"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --all-projects: %v", err)
		}

		out := buf.String()
		if !contains(out, "in-proj-track") {
			t.Errorf("expected in-proj-track in --all-projects output, got:\n%s", out)
		}
		if !contains(out, "other-proj-track") {
			t.Errorf("expected other-proj-track in --all-projects output, got:\n%s", out)
		}
		// Project column should appear when --all-projects is set.
		if !contains(out, "Project") {
			t.Errorf("expected 'Project' column header, got:\n%s", out)
		}
		// Both project IDs should appear in the rendered table.
		if !contains(out, otherProj) {
			t.Errorf("expected project id %q in output, got:\n%s", otherProj, out)
		}
	})
}

// TestTrackList_E2E_DefaultScopeOtherProjectTrackPersisted is the
// sanity-check sibling of the default-scope test. Same two-project
// fixture, but instead of going through the CLI it queries the
// storage layer directly with AllProjects=true to prove the
// other-project track was actually persisted. This isolates the
// default-scope assertion: its failure mode would be "other track
// silently missing from the DB" — this test rules that out.
//
// Direct storage call is used (not `tlc track list --all-projects`)
// because, when run inside a project context, the post-query
// state-compute step in runTrackList re-fetches each track via
// GetTrack which scopes to current project (sqlite_track.go:51-66)
// and would fail on the cross-project row. That re-fetch path is
// out of scope for this test.
func TestTrackList_E2E_DefaultScopeOtherProjectTrackPersisted(t *testing.T) {
	withTestLock(func() {
		setupProjectScopedTestDir(t, "tlc-track-list-persisted-", "hop-top/tlc")
		resetTrackListFlags()
		resetTrackFlags()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		ctx := context.Background()
		now := time.Now().UTC()
		svc := core.NewTrackService(s, s)
		inProj := "hop-top/tlc"
		otherProj := "other-org/foo"
		for _, tr := range []*core.Track{
			{
				ID: "in-proj-track", Title: "In Project",
				Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
				ProjectID: &inProj, CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "other-proj-track", Title: "Other Project",
				Type: core.TrackTypeFeature, Status: core.TrackStatusActive,
				ProjectID: &otherProj, CreatedAt: now, UpdatedAt: now,
			},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track %s: %v", tr.ID, err)
			}
		}

		// AllProjects=true bypasses the auto project filter and returns
		// every track regardless of owning project.
		all, err := s.ListTracks(ctx, core.TrackQuery{AllProjects: true})
		if err != nil {
			t.Fatalf("ListTracks AllProjects=true: %v", err)
		}
		seen := map[string]bool{}
		for _, tr := range all {
			seen[tr.ID] = true
		}
		if !seen["in-proj-track"] {
			t.Errorf("expected in-proj-track persisted, got tracks: %v", seen)
		}
		if !seen["other-proj-track"] {
			t.Errorf("expected other-proj-track persisted, got tracks: %v", seen)
		}

		// Default-scoped storage query (no AllProjects, no ProjectID
		// override) must also exclude the cross-project row — this is
		// the core behavior the parent test asserts via the CLI.
		scoped, err := s.ListTracks(ctx, core.TrackQuery{})
		if err != nil {
			t.Fatalf("ListTracks default scope: %v", err)
		}
		scopedIDs := map[string]bool{}
		for _, tr := range scoped {
			scopedIDs[tr.ID] = true
		}
		if !scopedIDs["in-proj-track"] {
			t.Errorf("expected in-proj-track in default-scope storage list, got: %v", scopedIDs)
		}
		if scopedIDs["other-proj-track"] {
			t.Errorf("unexpected other-proj-track in default-scope storage list, got: %v", scopedIDs)
		}
	})
}
