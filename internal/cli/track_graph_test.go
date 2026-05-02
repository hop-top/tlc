package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// createTrackWithDepTasks sets up a track with tasks that have blocked-by deps.
func createTrackWithDepTasks(t *testing.T) {
	t.Helper()

	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	_ = ctx

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	track := &core.Track{
		ID:     "dep-track",
		Title:  "Dependency Track",
		Type:   core.TrackTypeFeature,
		Status: core.TrackStatusActive,
	}
	trackTypeID := seedTrack(t, ctx, s, s, track)

	tasks := []*core.Task{
		{
			ID:      "T-0001",
			Title:   "Root task",
			Status:  core.StatusTodo,
			TrackID: &trackTypeID,
		},
		{
			ID:      "T-0002",
			Title:   "Child A",
			Status:  core.StatusTodo,
			TrackID: &trackTypeID,
			Meta:    map[string]interface{}{"blocked_by": []string{"T-0001"}},
		},
		{
			ID:      "T-0003",
			Title:   "Child B",
			Status:  core.StatusTodo,
			TrackID: &trackTypeID,
			Meta:    map[string]interface{}{"blocked_by": []string{"T-0001"}},
		},
		{
			ID:      "T-0004",
			Title:   "Final",
			Status:  core.StatusTodo,
			TrackID: &trackTypeID,
			Meta:    map[string]interface{}{"blocked_by": []string{"T-0002", "T-0003"}},
		},
	}

	for _, task := range tasks {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}
}

func TestTrackGraph_TableFormat(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		resetTrackGraphFlags()
		createTrackWithDepTasks(t)

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "graph", "dep-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track graph failed: %v", err)
		}

		out := buf.String()
		// Should show dependency tree.
		for _, want := range []string{
			"Dependency Tree",
			"T-0001",
			"T-0004",
			"Execution Strategy",
			"Batch",
		} {
			if !contains(out, want) {
				t.Errorf("expected output to contain %q, got:\n%s", want, out)
			}
		}
	})
}

func TestTrackGraph_BatchesOnly(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		resetTrackGraphFlags()
		createTrackWithDepTasks(t)

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "graph", "dep-track", "--batches-only"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track graph --batches-only failed: %v", err)
		}

		out := buf.String()
		// Should NOT show dependency tree header.
		if contains(out, "Dependency Tree") {
			t.Errorf("--batches-only should not show tree:\n%s", out)
		}
		// Should show batch summary.
		if !contains(out, "Execution Strategy") {
			t.Errorf("expected batch summary:\n%s", out)
		}
	})
}

func TestTrackGraph_JSONFormat(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		resetTrackGraphFlags()
		createTrackWithDepTasks(t)

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "graph", "dep-track", "--format", "json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track graph --format json failed: %v", err)
		}

		out := buf.String()
		// JSON should contain structured fields.
		for _, want := range []string{
			"batches",
			"critical_path",
			"total_tasks",
			"max_parallelism",
		} {
			if !contains(out, want) {
				t.Errorf("JSON output missing %q:\n%s", want, out)
			}
		}
	})
}

func TestTrackGraph_CriticalPath(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		resetTrackGraphFlags()
		createTrackWithDepTasks(t)

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "graph", "dep-track", "--critical-path"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track graph --critical-path failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "Critical Path") {
			t.Errorf("expected Critical Path section:\n%s", out)
		}
	})
}

func TestTrackGraph_NoTasks(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		resetTrackGraphFlags()

		ctx, cleanup := setupTestDir(t)
		defer cleanup()

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
		cmd.SetArgs([]string{"track", "graph", "empty-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track graph on empty track failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "No tasks linked") {
			t.Errorf("expected 'No tasks linked' message:\n%s", out)
		}
	})
}

func TestTrackShow_WithDepsShowsStrategy(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		resetTrackGraphFlags()
		createTrackWithDepTasks(t)

		viper.Set("output.format", "json")
		defer viper.Set("output.format", "")

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "dep-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "execution_strategy") {
			t.Errorf("track show with deps should include execution_strategy:\n%s", out)
		}
	})
}

func TestTrackShow_WithoutDepsOmitsStrategy(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		resetTrackGraphFlags()

		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		svc := core.NewTrackService(s, s)
		track := &core.Track{
			ID:     "no-dep-track",
			Title:  "No Dep Track",
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusActive,
		}
		if err := svc.CreateTrack(ctx, track); err != nil {
			t.Fatalf("create track: %v", err)
		}

		// Tasks with no blocked-by.
		trackID := "no-dep-track"
		for _, task := range []*core.Task{
			{ID: "T-0010", Title: "Task X", Status: core.StatusTodo, TrackID: &trackID},
			{ID: "T-0011", Title: "Task Y", Status: core.StatusTodo, TrackID: &trackID},
		} {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
		}

		viper.Set("output.format", "json")
		defer viper.Set("output.format", "")

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "no-dep-track"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show failed: %v", err)
		}

		out := buf.String()
		// Without deps, execution_strategy should be null/omitted.
		if contains(out, "\"execution_strategy\"") {
			t.Errorf("track show without deps should omit execution_strategy:\n%s", out)
		}
	})
}

func TestTrackGraph_MermaidFormat(t *testing.T) {
	withTestLock(func() {
		resetTaskFlags()
		resetTrackGraphFlags()
		createTrackWithDepTasks(t)

		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "graph", "dep-track", "--format", "mermaid"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track graph --format mermaid failed: %v", err)
		}

		out := buf.String()
		if !contains(out, "flowchart TB") {
			t.Errorf("expected mermaid flowchart output, got:\n%s", out)
		}
		if !contains(out, "subgraph") {
			t.Errorf("expected mermaid subgraph sections, got:\n%s", out)
		}
	})
}
