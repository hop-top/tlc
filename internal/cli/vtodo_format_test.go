package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskListVtodoFormat verifies `task list --format vtodo` emits a
// VCALENDAR with one VTODO per task.
func TestTaskListVtodoFormat(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		tasks := []*core.Task{
			{ID: "T-0001", Title: "First task", Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now},
			{ID: "T-0002", Title: "Second task", Status: core.StatusInProgress, CreatedAt: now, UpdatedAt: now},
		}
		for _, task := range tasks {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
		}

		viper.Set("output.format", formatVtodo)
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with vtodo format failed: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "BEGIN:VCALENDAR") {
			t.Errorf("expected VCALENDAR envelope, got:\n%s", out)
		}
		if !strings.Contains(out, "END:VCALENDAR") {
			t.Errorf("expected VCALENDAR closer, got:\n%s", out)
		}
		if got := strings.Count(out, "BEGIN:VTODO"); got != 2 {
			t.Errorf("expected 2 VTODO components, got %d:\n%s", got, out)
		}
		if !strings.Contains(out, "First task") {
			t.Errorf("expected task title in output, got:\n%s", out)
		}
		if !strings.Contains(out, "Second task") {
			t.Errorf("expected task title in output, got:\n%s", out)
		}
	})
}

// TestTaskListVtodoOutputFlag verifies that --output redirects the
// VCALENDAR to a file and emits nothing to stdout.
func TestTaskListVtodoOutputFlag(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Output flag task",
			Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		tmp := t.TempDir()
		outPath := filepath.Join(tmp, "tasks.ics")

		viper.Set("output.format", formatVtodo)
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--output", outPath})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --output failed: %v", err)
		}

		data, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		ics := string(data)
		if !strings.Contains(ics, "BEGIN:VCALENDAR") {
			t.Errorf("expected VCALENDAR in output file, got:\n%s", ics)
		}
		if !strings.Contains(ics, "Output flag task") {
			t.Errorf("expected task title in output file, got:\n%s", ics)
		}
	})
}

// TestTaskListVtodo_OutputWriteFailureSurfacesError verifies that a
// failed file write surfaces a non-nil error to the caller. Without
// this, scripts that pipe `tlc task list --format vtodo --output ...`
// can silently believe the export succeeded when nothing was written.
func TestTaskListVtodo_OutputWriteFailureSurfacesError(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Audit me",
			Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		// Point --output at an unwritable path: a non-existent dir.
		// os.WriteFile will return ENOENT.
		tmp := t.TempDir()
		bogus := filepath.Join(tmp, "does-not-exist", "tasks.ics")

		viper.Set("output.format", formatVtodo)
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--output", bogus})

		execErr := cmd.Execute()
		if execErr == nil {
			t.Fatalf("Execute returned nil; want error from failed --output write")
		}
		if !strings.Contains(execErr.Error(), "tasks.ics") &&
			!strings.Contains(execErr.Error(), "no such file") {
			t.Errorf("error %q did not mention the failed write", execErr.Error())
		}
	})
}

// TestTaskShowVtodoFormat verifies `task show <id> --format vtodo` emits
// a single-VTODO VCALENDAR.
func TestTaskShowVtodoFormat(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		if err := s.CreateTask(ctx, &core.Task{
			ID: "T-0001", Title: "Single task",
			Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		viper.Set("output.format", formatVtodo)
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "show", "T-0001"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task show with vtodo format failed: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "BEGIN:VCALENDAR") {
			t.Errorf("expected VCALENDAR envelope, got:\n%s", out)
		}
		if got := strings.Count(out, "BEGIN:VTODO"); got != 1 {
			t.Errorf("expected exactly 1 VTODO, got %d:\n%s", got, out)
		}
		if !strings.Contains(out, "Single task") {
			t.Errorf("expected task title in output, got:\n%s", out)
		}
	})
}

// TestTrackShowVtodoFormat verifies `track show <id> --format vtodo`
// emits a VCALENDAR containing the track and its member tasks.
func TestTrackShowVtodoFormat(t *testing.T) {
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

		track := &core.Track{
			ID:     "alpha",
			Title:  "Alpha track",
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusActive,
		}
		trackID := seedTrack(t, ctx, s, s, track)

		now := time.Now().UTC()
		tasks := []*core.Task{
			{ID: "T-0001", Title: "Task A", Status: core.StatusTodo, TrackID: &trackID, CreatedAt: now, UpdatedAt: now},
			{ID: "T-0002", Title: "Task B", Status: core.StatusInProgress, TrackID: &trackID, CreatedAt: now, UpdatedAt: now},
		}
		for _, task := range tasks {
			if err := s.CreateTask(ctx, task); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
		}

		viper.Set("output.format", formatVtodo)
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", "alpha"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show with vtodo format failed: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "BEGIN:VCALENDAR") {
			t.Errorf("expected VCALENDAR envelope, got:\n%s", out)
		}
		// Track + 2 tasks = 3 VTODOs.
		if got := strings.Count(out, "BEGIN:VTODO"); got != 3 {
			t.Errorf("expected 3 VTODO components (1 track + 2 tasks), got %d:\n%s", got, out)
		}
		if !strings.Contains(out, "Alpha track") {
			t.Errorf("expected track title, got:\n%s", out)
		}
		if !strings.Contains(out, "Task A") || !strings.Contains(out, "Task B") {
			t.Errorf("expected member task titles, got:\n%s", out)
		}
	})
}

// TestTaskListVtodoIncludeLogs verifies that --include-logs adds
// VJOURNAL components for log entries.
func TestTaskListVtodoIncludeLogs(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		task := &core.Task{
			ID: "T-0001", Title: "Logged task",
			Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if err := s.AddLog(ctx, &core.LogEntry{
			TaskID:    "T-0001",
			Action:    "CREATED",
			By:        "tester",
			Note:      "kicked off the task",
			Timestamp: now,
		}); err != nil {
			t.Fatalf("AddLog: %v", err)
		}

		viper.Set("output.format", formatVtodo)
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--include-logs"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --include-logs failed: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "BEGIN:VJOURNAL") {
			t.Errorf("expected VJOURNAL component when --include-logs is set, got:\n%s", out)
		}
	})
}

// TestTaskListVtodoIncludeLogsOffByDefault verifies the default behavior
// of `--format vtodo` excludes VJOURNAL components.
func TestTaskListVtodoIncludeLogsOffByDefault(t *testing.T) {
	withTestLock(func() {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()

		s, err := getStorageRaw()
		if err != nil {
			t.Fatalf("getStorageRaw: %v", err)
		}
		defer s.Close()

		now := time.Now().UTC()
		task := &core.Task{
			ID: "T-0001", Title: "Quiet task",
			Status: core.StatusTodo, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if err := s.AddLog(ctx, &core.LogEntry{
			TaskID:    "T-0001",
			Action:    "CREATED",
			By:        "tester",
			Timestamp: now,
		}); err != nil {
			t.Fatalf("AddLog: %v", err)
		}

		viper.Set("output.format", formatVtodo)
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list failed: %v", err)
		}

		out := buf.String()
		if strings.Contains(out, "BEGIN:VJOURNAL") {
			t.Errorf("did not expect VJOURNAL without --include-logs, got:\n%s", out)
		}
	})
}
