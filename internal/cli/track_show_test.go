package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

func TestTrackShow_FullDetail(t *testing.T) {
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

		assignee := "me"
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID:         "browser-rendering",
			Title:      "Browser rendering",
			Type:       core.TrackTypeFeature,
			Status:     core.TrackStatusActive,
			AssignedTo: &assignee,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "browser-rendering"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show failed: %v", err)
		}

		out := buf.String()
		for _, want := range []string{
			"browser-rendering",
			"Browser rendering",
			"feature",
			"active",
			"@me",
			"Progress",
		} {
			if !contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})
}

func TestTrackShow_PhaseBreakdown(t *testing.T) {
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
			ID:     "phased-track",
			Title:  "Phased Track",
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		trackID := "phased-track"
		now := time.Now().UTC()
		tasks := []*core.Task{
			{
				ID: "T-0001", Title: "Parse HTML", Status: core.StatusDone,
				Tags: []string{"phase:1"}, TrackID: &trackID,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "T-0002", Title: "Render DOM", Status: core.StatusDone,
				Tags: []string{"phase:1"}, TrackID: &trackID,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "T-0003", Title: "CSS engine", Status: core.StatusInProgress,
				Tags: []string{"phase:2"}, TrackID: &trackID,
				CreatedAt: now, UpdatedAt: now,
			},
			{
				ID: "T-0004", Title: "Layout engine", Status: core.StatusTodo,
				Tags: []string{"phase:2"}, TrackID: &trackID,
				CreatedAt: now, UpdatedAt: now,
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
		cmd.SetArgs([]string{"track", "show", "phased-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show failed: %v", err)
		}

		out := buf.String()
		for _, want := range []string{
			"Phase",
			"Parse HTML",
			"CSS engine",
			"DONE",
			"IN_PROGRESS",
		} {
			if !contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})
}

func TestTrackShow_UnphasedTasks(t *testing.T) {
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
			ID:     "unphased-track",
			Title:  "Unphased Track",
			Type:   core.TrackTypeBug,
			Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		trackID := "unphased-track"
		now := time.Now().UTC()
		task := &core.Task{
			ID: "T-0010", Title: "Fix something", Status: core.StatusTodo,
			Tags: []string{}, TrackID: &trackID,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "unphased-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "Unphased") {
			t.Errorf("expected 'Unphased' section, got:\n%s", out)
		}
		if !contains(out, "Fix something") {
			t.Errorf("expected unphased task in output, got:\n%s", out)
		}
	})
}

func TestTrackShow_NoPhasedTasksShowsNone(t *testing.T) {
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
			ID:     "empty-track",
			Title:  "Empty Track",
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusPending,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "empty-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "(none)") {
			t.Errorf("expected '(none)' for empty unphased section, got:\n%s", out)
		}
		if !contains(out, "no tasks linked") {
			t.Errorf("expected 'no tasks linked', got:\n%s", out)
		}
	})
}

func TestTrackShow_NotFound(t *testing.T) {
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
		cmd.SetArgs([]string{"track", "show", "nonexistent"})

		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for nonexistent track, got nil")
		}
		if !contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", err)
		}
	})
}

// TestTrackShow_DoesNotLeakTasksFromOtherProjects is a regression test for
// cross-project task pollution in `tlc track show`. The pre-fix linked-task
// queries in core.TrackService (linkedTasks/linkedTaskStats) and the local
// fetch in runTrackShow filtered tasks by track_id only with AllProjects:true,
// so tasks from a different project that happened to share the same track_id
// string got merged into the display — polluting progress counts and the
// unphased task list. Linked tasks must be scoped by (track_id, project_id)
// jointly, where project_id comes from the track's owning project.
//
// Repro: a track "shared-id" exists in project "proj-a" with one legitimate
// task. A second task in project "proj-b" is linked to track_id="shared-id"
// (which represents either a different track with the same id, or stale data).
// `tlc track show shared-id` from proj-a context should show only the
// proj-a task, never the proj-b one.
func TestTrackShow_DoesNotLeakTasksFromOtherProjects(t *testing.T) {
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

		projA := "proj-a"
		projB := "proj-b"

		// Track lives in proj-a.
		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID:        "shared-id",
			Title:     "Shared ID Track",
			Type:      core.TrackTypeRefactor,
			Status:    core.TrackStatusActive,
			ProjectID: &projA,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		trackID := "shared-id"
		now := time.Now().UTC()

		// Legitimate task: same project as the track.
		ownTask := &core.Task{
			ID:        "T-0001",
			Title:     "Own project task",
			Status:    core.StatusTodo,
			TrackID:   &trackID,
			ProjectID: &projA,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.CreateTask(ctx, ownTask); err != nil {
			t.Fatalf("create own task: %v", err)
		}

		// Contaminating task: different project, same track_id string.
		// This is the row that should NOT appear in `track show` output.
		foreignTask := &core.Task{
			ID:        "T-9999",
			Title:     "FOREIGN PROJECT LEAK",
			Status:    core.StatusDone,
			TrackID:   &trackID,
			ProjectID: &projB,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.CreateTask(ctx, foreignTask); err != nil {
			t.Fatalf("create foreign task: %v", err)
		}

		// Run `tlc track show shared-id`. Detection has no project context
		// (setupTestDir uses a bare tmpdir), so resolveTrackID will find the
		// only "shared-id" track row and runTrackShow lists its tasks.
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "shared-id"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show failed: %v", err)
		}

		out := buf.String()

		// Sanity: the legitimate task must be present.
		if !contains(out, "Own project task") {
			t.Errorf("expected own task in output, got:\n%s", out)
		}

		// Bug: the foreign-project task must NOT appear.
		if contains(out, "FOREIGN PROJECT LEAK") {
			t.Errorf("BUG: track show leaked task from another project.\n"+
				"Track 'shared-id' lives in %q, but task T-9999 from %q\n"+
				"appears in the output. runTrackShow filters by track_id only\n"+
				"with AllProjects:true; it must also scope by project_id.\n\nOutput:\n%s",
				projA, projB, out)
		}
		if contains(out, "T-9999") {
			t.Errorf("BUG: foreign task ID T-9999 leaked into track show output:\n%s", out)
		}
	})
}

func TestTrackShow_JSONFormat(t *testing.T) {
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
			ID:     "json-show-track",
			Title:  "JSON Show Track",
			Type:   core.TrackTypeRefactor,
			Status: core.TrackStatusActive,
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
		cmd.SetArgs([]string{"track", "show", "json-show-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show failed: %v", err)
		}

		out := buf.String()
		if !contains(out, `"id"`) || !contains(out, "json-show-track") {
			t.Errorf("expected JSON output, got:\n%s", out)
		}
	})
}
