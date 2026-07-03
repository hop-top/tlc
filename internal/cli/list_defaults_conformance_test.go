package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// headerFields returns the column names from the first non-empty line of output.
func headerFields(out string) []string {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return strings.Fields(trimmed)
		}
	}
	return nil
}

// headerContains reports whether the first non-empty output line contains col
// as one of its whitespace-delimited fields.
func headerContains(out, col string) bool {
	for _, f := range headerFields(out) {
		if strings.EqualFold(f, col) {
			return true
		}
	}
	return false
}

// TestListDefaultsConformance exercises the full config-driven defaults ladder
// across task list and track list, catching cross-cutting regressions that
// per-command unit tests miss.
func TestListDefaultsConformance(t *testing.T) {
	// Scenario 1: defaults.list.columns reaches BOTH commands when no
	// domain-specific override is set.
	t.Run("CrossDomainBothCommands", func(t *testing.T) {
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

			if err := s.CreateTask(ctx, &core.Task{
				ID: "T-0001", Title: "Cross domain task", Status: core.StatusTodo,
			}); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}

			svc := core.NewTrackService(s, s)
			if err := svc.CreateTrack(ctx, &core.Track{
				ID: "cross-track", Title: "Cross Domain Track", Type: core.TrackTypeFeature,
				Status: core.TrackStatusActive,
			}); err != nil {
				t.Fatalf("CreateTrack: %v", err)
			}

			viper.Set("defaults.list.columns", []string{"id", "title"})
			t.Cleanup(func() { viper.Set("defaults.list.columns", nil) })

			// task list
			{
				cmd := newTestCmd()
				cmd.AddCommand(TaskCmd)
				buf := new(bytes.Buffer)
				cmd.SetOut(buf)
				cmd.SetErr(buf)
				cmd.SetArgs([]string{"task", "list"})
				if err := cmd.Execute(); err != nil {
					t.Fatalf("task list: %v", err)
				}
				out := buf.String()
				t.Logf("task list output: %q", out)
				hdr := headerFields(out)
				if len(hdr) != 2 {
					t.Errorf("task list: expected exactly [ID Title] in header, got %v", hdr)
				} else {
					if !strings.EqualFold(hdr[0], "ID") || !strings.EqualFold(hdr[1], "Title") {
						t.Errorf("task list: expected header [ID Title], got %v", hdr)
					}
				}
				if headerContains(out, "Status") {
					t.Errorf("task list: header should not contain Status when defaults.list.columns=[id,title]; got:\n%s", out)
				}
			}

			resetTrackListFlags()
			resetTrackFlags()

			// track list
			{
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
				t.Logf("track list output: %q", out)
				hdr := headerFields(out)
				if len(hdr) != 2 {
					t.Errorf("track list: expected exactly [ID Title] in header, got %v", hdr)
				} else {
					if !strings.EqualFold(hdr[0], "ID") || !strings.EqualFold(hdr[1], "Title") {
						t.Errorf("track list: expected header [ID Title], got %v", hdr)
					}
				}
				if headerContains(out, "Status") {
					t.Errorf("track list: header should not contain Status when defaults.list.columns=[id,title]; got:\n%s", out)
				}
			}
		})
	})

	// Scenario 2: domain-specific key wins over defaults; other domain still
	// falls back to defaults.
	t.Run("DomainOverrideWinsOverDefaults", func(t *testing.T) {
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

			if err := s.CreateTask(ctx, &core.Task{
				ID: "T-0001", Title: "Override task", Status: core.StatusTodo,
			}); err != nil {
				t.Fatalf("CreateTask: %v", err)
			}

			svc := core.NewTrackService(s, s)
			if err := svc.CreateTrack(ctx, &core.Track{
				ID: "override-track", Title: "Override Track", Type: core.TrackTypeFeature,
				Status: core.TrackStatusActive,
			}); err != nil {
				t.Fatalf("CreateTrack: %v", err)
			}

			viper.Set("defaults.list.columns", []string{"id", "title"})
			viper.Set("task.list.columns", []string{"id", "status"})
			t.Cleanup(func() {
				viper.Set("defaults.list.columns", nil)
				viper.Set("task.list.columns", nil)
			})

			// task list — domain key wins: expect id + status, NOT title.
			// task.list.columns=[id,status] so Title does not appear in header.
			// The task row shows its ID; title column is absent.
			{
				cmd := newTestCmd()
				cmd.AddCommand(TaskCmd)
				buf := new(bytes.Buffer)
				cmd.SetOut(buf)
				cmd.SetErr(buf)
				cmd.SetArgs([]string{"task", "list"})
				if err := cmd.Execute(); err != nil {
					t.Fatalf("task list: %v", err)
				}
				out := buf.String()
				t.Logf("task list (domain override) output: %q", out)
				if !headerContains(out, "ID") {
					t.Errorf("task list: expected ID header; got:\n%s", out)
				}
				// task.list.columns=[id,status] — Title must NOT appear in header
				if headerContains(out, "Title") {
					t.Errorf("task list: should NOT have Title header (task.list.columns=[id,status]); got:\n%s", out)
				}
				// The task's ID should appear in output (data row present)
				if !strings.Contains(out, "T-0001") {
					t.Errorf("task list: expected task row T-0001 in output; got:\n%s", out)
				}
			}

			resetTrackListFlags()
			resetTrackFlags()

			// track list — falls back to defaults: expect id + title
			{
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
				t.Logf("track list (fallback to defaults) output: %q", out)
				if !headerContains(out, "ID") {
					t.Errorf("track list: expected ID header; got:\n%s", out)
				}
				if !headerContains(out, "Title") {
					t.Errorf("track list: expected Title header (from defaults.list.columns); got:\n%s", out)
				}
				if headerContains(out, "Type") {
					t.Errorf("track list: should NOT have Type header (not in defaults.list.columns); got:\n%s", out)
				}
			}
		})
	})

	// Scenario 3: config task.list.status filters rows AND prunes Status column
	// (uses IN_PROGRESS, not DONE — pre-existing DONE list-filter bug is out of scope).
	t.Run("ConfigStatusFiltersAndPrunesTask", func(t *testing.T) {
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

			if err := s.CreateTask(ctx, &core.Task{
				ID: "T-0001", Title: "InProgress task", Status: core.StatusInProgress,
			}); err != nil {
				t.Fatalf("CreateTask T-0001: %v", err)
			}
			if err := s.CreateTask(ctx, &core.Task{
				ID: "T-0002", Title: "Todo task", Status: core.StatusTodo,
			}); err != nil {
				t.Fatalf("CreateTask T-0002: %v", err)
			}

			viper.Set("task.list.status", []string{"IN_PROGRESS"})
			t.Cleanup(func() { viper.Set("task.list.status", nil) })

			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "list"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task list: %v", err)
			}
			out := buf.String()
			t.Logf("output: %q", out)

			if !strings.Contains(out, "InProgress task") {
				t.Errorf("expected 'InProgress task' in output; got:\n%s", out)
			}
			if strings.Contains(out, "Todo task") {
				t.Errorf("did not expect 'Todo task' in output when status=IN_PROGRESS; got:\n%s", out)
			}
			if headerContains(out, "Status") {
				t.Errorf("expected Status column to be pruned when config status is set; got:\n%s", out)
			}
		})
	})

	// Scenario 4: columns config with status included + --status TODO still
	// prunes the Status column ("always prune when status filter is provided"
	// beats explicit column request). Uses viper.Set("task.list.columns") since
	// --cols is a kit persistent flag not available on the bare test cmd root.
	t.Run("ExplicitColsStillPrunesStatus", func(t *testing.T) {
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

			if err := s.CreateTask(ctx, &core.Task{
				ID: "T-0001", Title: "Prune cols TODO", Status: core.StatusTodo,
			}); err != nil {
				t.Fatalf("CreateTask T-0001: %v", err)
			}
			if err := s.CreateTask(ctx, &core.Task{
				ID: "T-0002", Title: "Prune cols InProgress", Status: core.StatusInProgress,
			}); err != nil {
				t.Fatalf("CreateTask T-0002: %v", err)
			}

			// Request id+title+status columns via config; apply --status TODO filter.
			// Status pruning rule fires because a status filter is active, even
			// though status is in the column list.
			viper.Set("task.list.columns", []string{"id", "title", "status"})
			t.Cleanup(func() { viper.Set("task.list.columns", nil) })

			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "list", "--status", "TODO"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task list --status TODO: %v", err)
			}
			out := buf.String()
			t.Logf("output: %q", out)

			// Status column pruned despite being requested in task.list.columns
			if headerContains(out, "Status") {
				t.Errorf("expected Status column to be pruned when --status filter active; got:\n%s", out)
			}
			// Only the TODO task appears
			if !strings.Contains(out, "Prune cols TODO") {
				t.Errorf("expected TODO task in output; got:\n%s", out)
			}
			if strings.Contains(out, "Prune cols InProgress") {
				t.Errorf("did not expect InProgress task in --status TODO output; got:\n%s", out)
			}
		})
	})

	// Scenario 5: non-column flag default (limit) via config trims rows.
	// Seeds 3 tasks with distinct titles; sets limit=1; asserts exactly 1 shows.
	t.Run("GenericFlagDefaultLimitOneRow", func(t *testing.T) {
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

			titles := []string{"Alpha limit task", "Beta limit task", "Gamma limit task"}
			for i, title := range titles {
				id := "T-000" + string(rune('1'+i))
				if err := s.CreateTask(ctx, &core.Task{
					ID: id, Title: title, Status: core.StatusTodo,
				}); err != nil {
					t.Fatalf("CreateTask %s: %v", id, err)
				}
			}

			viper.Set("task.list.limit", 1)
			t.Cleanup(func() { viper.Set("task.list.limit", nil) })

			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "list"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task list: %v", err)
			}
			out := buf.String()
			t.Logf("output: %q", out)

			// Count how many of the 3 distinct titles appear.
			present := 0
			for _, title := range titles {
				if strings.Contains(out, title) {
					present++
				}
			}
			if present != 1 {
				t.Errorf("expected exactly 1 task title in output with limit=1, found %d; output:\n%s", present, out)
			}
		})
	})
}
