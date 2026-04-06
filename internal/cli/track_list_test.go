package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

func TestTrackList_Empty(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		resetTrackListFlags()

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list failed: %v", err)
		}

		if !contains(buf.String(), "No tracks found") {
			t.Errorf("expected 'No tracks found', got: %s", buf.String())
		}
	})
}

func TestTrackList_ShowsTracks(t *testing.T) {
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
			ID:     "browser-rendering",
			Title:  "Browser rendering",
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list failed: %v", err)
		}

		out := buf.String()
		for _, want := range []string{"browser-rendering", "Browser rendering", "feature", "active"} {
			if !contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})
}

func TestTrackList_FilterByStatus(t *testing.T) {
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
		for _, tr := range []*core.Track{
			{ID: "track-active", Title: "Active track", Type: "feature", Status: core.TrackStatusActive},
			{ID: "track-completed", Title: "Completed track", Type: "feature", Status: core.TrackStatusCompleted},
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
			t.Fatalf("track list failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "track-active") {
			t.Errorf("expected active track in output, got:\n%s", out)
		}
		if contains(out, "track-completed") {
			t.Errorf("did not expect completed track in output, got:\n%s", out)
		}
	})
}

func TestTrackList_DefaultFilterExcludesTerminal(t *testing.T) {
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
		for _, tr := range []*core.Track{
			{ID: "tr-pending", Title: "Pending", Type: "feature",
				Status: core.TrackStatusPending},
			{ID: "tr-active", Title: "Active", Type: "feature",
				Status: core.TrackStatusActive},
			{ID: "tr-completed", Title: "Completed", Type: "feature",
				Status: core.TrackStatusCompleted},
			{ID: "tr-abandoned", Title: "Abandoned", Type: "feature",
				Status: core.TrackStatusAbandoned},
		} {
			if err := svc.CreateTrack(ctx, tr); err != nil {
				t.Fatalf("create track: %v", err)
			}
		}

		// Default: no --status flag → should show pending + active only.
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "tr-pending") {
			t.Errorf("expected pending track in default output, got:\n%s", out)
		}
		if !contains(out, "tr-active") {
			t.Errorf("expected active track in default output, got:\n%s", out)
		}
		if contains(out, "tr-completed") {
			t.Errorf("did not expect completed track in default output, got:\n%s", out)
		}
		if contains(out, "tr-abandoned") {
			t.Errorf("did not expect abandoned track in default output, got:\n%s", out)
		}
	})
}

func TestTrackList_FilterByType(t *testing.T) {
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
		for _, tr := range []*core.Track{
			{ID: "feat-track", Title: "Feature track", Type: "feature", Status: core.TrackStatusActive},
			{ID: "bug-track", Title: "Bug track", Type: "bug", Status: core.TrackStatusActive},
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
		cmd.SetArgs([]string{"track", "list", "--type", "bug"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "bug-track") {
			t.Errorf("expected bug track in output, got:\n%s", out)
		}
		if contains(out, "feat-track") {
			t.Errorf("did not expect feature track in output, got:\n%s", out)
		}
	})
}

func TestTrackList_TableColumns(t *testing.T) {
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
			ID:     "test-track",
			Title:  "Test Track",
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list failed: %v", err)
		}

		out := buf.String()
		for _, col := range []string{"ID", "Title", "Type", "Status", "State", "Progress", "Assignee"} {
			if !contains(out, col) {
				t.Errorf("expected column header %q in output, got:\n%s", col, out)
			}
		}
	})
}

func TestTrackList_JSONFormat(t *testing.T) {
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
			ID:     "json-track",
			Title:  "JSON Track",
			Type:   core.TrackTypeFeature,
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
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list failed: %v", err)
		}

		out := buf.String()
		if !contains(out, `"id"`) || !contains(out, "json-track") {
			t.Errorf("expected JSON output with track, got:\n%s", out)
		}
	})
}

func TestTrackList_YAMLFormat(t *testing.T) {
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
			ID:     "yaml-track",
			Title:  "YAML Track",
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		viper.Set("output.format", "yaml")
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "id:") || !contains(out, "yaml-track") {
			t.Errorf("expected YAML output with track, got:\n%s", out)
		}
	})
}

func TestTrackList_AllProjects(t *testing.T) {
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
		projA := "hop-top/tlc"
		projB := "hop-top/aps"
		for _, tr := range []*core.Track{
			{ID: "track-a", Title: "Track A", Type: "feature",
				Status: core.TrackStatusActive, ProjectID: &projA},
			{ID: "track-b", Title: "Track B", Type: "bug",
				Status: core.TrackStatusActive, ProjectID: &projB},
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
		cmd.SetArgs([]string{"track", "list", "--all-projects"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --all-projects failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "track-a") {
			t.Errorf("expected track-a in output, got:\n%s", out)
		}
		if !contains(out, "track-b") {
			t.Errorf("expected track-b in output, got:\n%s", out)
		}
		// Project column should appear.
		if !contains(out, "Project") {
			t.Errorf("expected 'Project' column header, got:\n%s", out)
		}
	})
}

func TestTrackList_AllProjects_JSONIncludesProject(t *testing.T) {
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
		projID := "hop-top/tlc"
		track := &core.Track{
			ID:        "proj-track",
			Title:     "Project Track",
			Type:      core.TrackTypeFeature,
			Status:    core.TrackStatusActive,
			ProjectID: &projID,
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
		cmd.SetArgs([]string{"track", "list", "--all-projects"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --all-projects --format json failed: %v", err)
		}

		out := buf.String()
		if !contains(out, `"project"`) {
			t.Errorf("expected 'project' field in JSON output, got:\n%s", out)
		}
		if !contains(out, "hop-top/tlc") {
			t.Errorf("expected project ID in JSON output, got:\n%s", out)
		}
	})
}
