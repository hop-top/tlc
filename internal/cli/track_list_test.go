package cli

import (
	"bytes"
	"strings"
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
			{
				ID: "tr-pending", Title: "Pending", Type: "feature",
				Status: core.TrackStatusPending,
			},
			{
				ID: "tr-active", Title: "Active", Type: "feature",
				Status: core.TrackStatusActive,
			},
			{
				ID: "tr-completed", Title: "Completed", Type: "feature",
				Status: core.TrackStatusCompleted,
			},
			{
				ID: "tr-abandoned", Title: "Abandoned", Type: "feature",
				Status: core.TrackStatusAbandoned,
			},
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
			{
				ID: "track-a", Title: "Track A", Type: "feature",
				Status: core.TrackStatusActive, ProjectID: &projA,
			},
			{
				ID: "track-b", Title: "Track B", Type: "bug",
				Status: core.TrackStatusActive, ProjectID: &projB,
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

// TestTrackListDefaultColumns_AllResolve guards that every key in
// trackListDefaultColumns has a matching entry in trackColumnHeaders.
func TestTrackListDefaultColumns_AllResolve(t *testing.T) {
	for _, k := range trackListDefaultColumns {
		if _, ok := trackColumnHeaders[k]; !ok {
			t.Errorf("trackListDefaultColumns key %q not in trackColumnHeaders", k)
		}
	}
}

// TestTrackList_DefaultColumnsIncludeStatus verifies the default table output
// includes Status, ID, and Title header columns.
func TestTrackList_DefaultColumnsIncludeStatus(t *testing.T) {
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
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: "tr-status-default", Title: "Status Default", Type: "feature",
			Status: core.TrackStatusActive,
		}); err != nil {
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
		t.Logf("output: %q", out)
		for _, hdr := range []string{"Status", "ID", "Title"} {
			if !strings.Contains(out, hdr) {
				t.Errorf("expected %q column header in default output; got:\n%s", hdr, out)
			}
		}
	})
}

// TestTrackList_StatusFlagPrunesStatusColumn verifies that --status prunes
// the Status column from table output.
func TestTrackList_StatusFlagPrunesStatusColumn(t *testing.T) {
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
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: "tr-prune-status", Title: "Prune Status", Type: "feature",
			Status: core.TrackStatusActive,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list", "--status", "active"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list --status active failed: %v", err)
		}

		out := buf.String()
		t.Logf("output: %q", out)
		// The header row should not contain "Status" as a standalone column header.
		// Split on newlines to check only the header line.
		lines := strings.SplitN(out, "\n", 2)
		if len(lines) > 0 && strings.Contains(lines[0], "Status") {
			t.Errorf("expected Status column to be pruned when --status is set; header:\n%s", lines[0])
		}
		if !strings.Contains(out, "Prune Status") {
			t.Errorf("expected track title in output; got:\n%s", out)
		}
	})
}

// TestTrackList_ConfigColumnsOverride verifies that tracks.list.columns in
// config overrides the default column set.
func TestTrackList_ConfigColumnsOverride(t *testing.T) {
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
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: "tr-col-override", Title: "Col Override", Type: "feature",
			Status: core.TrackStatusActive,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		viper.Set("tracks.list.columns", []string{"id", "title", "progress"})
		t.Cleanup(func() { viper.Set("tracks.list.columns", nil) })

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list with config columns failed: %v", err)
		}

		out := buf.String()
		t.Logf("output: %q", out)
		for _, hdr := range []string{"ID", "Title", "Progress"} {
			if !strings.Contains(out, hdr) {
				t.Errorf("expected %q header; got:\n%s", hdr, out)
			}
		}
		for _, hdr := range []string{"Status", "Type", "State", "Assignee"} {
			if strings.Contains(out, hdr) {
				t.Errorf("did not expect %q when columns=[id,title,progress]; got:\n%s", hdr, out)
			}
		}
	})
}

// TestTrackList_CrossDomainFallback verifies that defaults.list.columns
// applies to track list when no domain-specific override is set.
func TestTrackList_CrossDomainFallback(t *testing.T) {
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
		if err := svc.CreateTrack(ctx, &core.Track{
			ID: "tr-cross-domain", Title: "Cross Domain", Type: "feature",
			Status: core.TrackStatusActive,
		}); err != nil {
			t.Fatalf("create track: %v", err)
		}

		viper.Set("defaults.list.columns", []string{"id", "title"})
		t.Cleanup(func() { viper.Set("defaults.list.columns", nil) })

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track list with defaults.list.columns failed: %v", err)
		}

		out := buf.String()
		t.Logf("output: %q", out)
		if !strings.Contains(out, "ID") {
			t.Errorf("expected 'ID' header; got:\n%s", out)
		}
		if !strings.Contains(out, "Title") {
			t.Errorf("expected 'Title' header; got:\n%s", out)
		}
		for _, hdr := range []string{"Status", "Type", "State", "Progress", "Assignee"} {
			if strings.Contains(out, hdr) {
				t.Errorf("did not expect %q when defaults.list.columns=[id,title]; got:\n%s", hdr, out)
			}
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
