package cli

// End-to-end tests for track list and show commands.
// Exercises full CLI pipeline: flag parsing -> storage -> query -> render.

import (
	"bytes"
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
