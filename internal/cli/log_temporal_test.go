package cli

// End-to-end tests for `tlc log` temporal filters (T-1381).
//
// Covers --since / --until: flag parsing → util.ParseSince → SQL pushdown
// in storage.ListLogs → JSON output. Before T-1381, both flags were wired
// as cobra StringVars but the values never reached the query layer, so
// `tlc log --since '2 days ago'` silently returned every log row.

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// seedTemporalLogs creates a task plus four canonical log entries:
//
//	old      now - 10d  CREATED
//	mid      now - 3d   CLAIMED
//	recent   now - 12h  UPDATED
//	fresh    now - 5m   DONE
//
// Returned cleanup is a no-op (setupTestDir already registers t.Cleanup
// for the tmp dir) — kept so callers can defer-call uniformly.
func seedTemporalLogs(t *testing.T) (taskID string, cleanup func()) {
	t.Helper()
	ctx, cleanup := setupTestDir(t)
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	now := time.Now().UTC()
	task := &core.Task{
		ID:        "T-0001",
		Title:     "temporal log fixture",
		Status:    core.StatusInProgress,
		CreatedAt: now.Add(-10 * 24 * time.Hour),
		UpdatedAt: now,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	entries := []*core.LogEntry{
		{TaskID: task.ID, Timestamp: now.Add(-10 * 24 * time.Hour), By: "alice", Action: "CREATED", Note: "old"},
		{TaskID: task.ID, Timestamp: now.Add(-3 * 24 * time.Hour), By: "alice", Action: "CLAIMED", Note: "mid"},
		{TaskID: task.ID, Timestamp: now.Add(-12 * time.Hour), By: "bob", Action: "UPDATED", Note: "recent"},
		{TaskID: task.ID, Timestamp: now.Add(-5 * time.Minute), By: "bob", Action: "DONE", Note: "fresh"},
	}
	for _, e := range entries {
		if err := s.AddLog(ctx, e); err != nil {
			t.Fatalf("AddLog %q: %v", e.Note, err)
		}
	}
	return task.ID, cleanup
}

// runLogJSON executes `tlc log <args>` with JSON output and returns the
// decoded LogEntry slice + raw stderr/stdout for diagnosis.
func runLogJSON(t *testing.T, args ...string) ([]*core.LogEntry, string, error) {
	t.Helper()
	viper.Set("output.format", "json")
	cmd := newTestCmd()
	cmd.AddCommand(logCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	full := append([]string{"log"}, args...)
	full = append(full, "--format", "json")
	cmd.SetArgs(full)

	err := cmd.Execute()
	out := buf.String()
	if err != nil {
		return nil, out, err
	}
	var logs []*core.LogEntry
	if jsonErr := json.Unmarshal([]byte(out), &logs); jsonErr != nil {
		// Pinpoint format drift early; JSON is contract-tested.
		t.Fatalf("decode json: %v; raw output:\n%s", jsonErr, out)
	}
	return logs, out, nil
}

func TestLog_TemporalFilters_E2E(t *testing.T) {
	t.Run("SinceTwoDaysAgo_ReturnsRecentAndFreshOnly", func(t *testing.T) {
		taskID, _ := seedTemporalLogs(t)
		defer resetTaskFlags()

		logs, out, err := runLogJSON(t, taskID, "--since", "2 days ago")
		if err != nil {
			t.Fatalf("log --since: %v; out:\n%s", err, out)
		}
		if len(logs) != 2 {
			t.Fatalf("want 2 logs (recent+fresh); got %d:\n%s", len(logs), out)
		}
		// SortDirection defaults to DESC, so fresh comes first.
		if logs[0].Note != "fresh" || logs[1].Note != "recent" {
			t.Errorf("want [fresh, recent]; got [%s, %s]", logs[0].Note, logs[1].Note)
		}
	})

	t.Run("UntilYesterday_ReturnsOldAndMidOnly", func(t *testing.T) {
		taskID, _ := seedTemporalLogs(t)
		defer resetTaskFlags()

		logs, out, err := runLogJSON(t, taskID, "--until", "yesterday")
		if err != nil {
			t.Fatalf("log --until: %v; out:\n%s", err, out)
		}
		if len(logs) != 2 {
			t.Fatalf("want 2 logs (old+mid); got %d:\n%s", len(logs), out)
		}
		// DESC: mid (-3d) before old (-10d).
		if logs[0].Note != "mid" || logs[1].Note != "old" {
			t.Errorf("want [mid, old]; got [%s, %s]", logs[0].Note, logs[1].Note)
		}
	})

	t.Run("SinceAndUntil_ReturnsIntersection", func(t *testing.T) {
		taskID, _ := seedTemporalLogs(t)
		defer resetTaskFlags()

		// 4d-ago → 1h-ago window: mid (-3d) + recent (-12h). Excludes
		// old (-10d, before since) and fresh (-5m, after until).
		logs, out, err := runLogJSON(t, taskID,
			"--since", "4 days ago",
			"--until", "1 hour ago",
		)
		if err != nil {
			t.Fatalf("log --since --until: %v; out:\n%s", err, out)
		}
		if len(logs) != 2 {
			t.Fatalf("want 2 logs (mid+recent); got %d:\n%s", len(logs), out)
		}
		got := map[string]bool{logs[0].Note: true, logs[1].Note: true}
		if !got["mid"] || !got["recent"] {
			t.Errorf("want {mid, recent}; got %v", got)
		}
	})

	t.Run("SinceFutureDate_ReturnsEmpty", func(t *testing.T) {
		taskID, _ := seedTemporalLogs(t)
		defer resetTaskFlags()

		// ParseSince accepts ISO dates as well; an absolute future
		// boundary returns no rows because every seeded entry is in
		// the past. Use a far-future literal so the test is stable
		// across years.
		futureISO := time.Now().UTC().AddDate(1, 0, 0).Format("2006-01-02")
		logs, out, err := runLogJSON(t, taskID, "--since", futureISO)
		if err != nil {
			t.Fatalf("log --since %s: %v; out:\n%s", futureISO, err, out)
		}
		if len(logs) != 0 {
			t.Fatalf("want 0 logs; got %d:\n%s", len(logs), out)
		}
	})

	t.Run("SinceAfterUntil_Errors", func(t *testing.T) {
		taskID, _ := seedTemporalLogs(t)
		defer resetTaskFlags()

		viper.Set("output.format", "json")
		cmd := newTestCmd()
		cmd.AddCommand(logCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{
			"log", taskID,
			"--since", "1 hour ago",
			"--until", "1 day ago",
			"--format", "json",
		})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error when --since is after --until; got nil, out:\n%s", buf.String())
		}
		if !contains(err.Error(), "after") {
			t.Errorf("expected 'after' in error; got: %v", err)
		}
	})

	t.Run("InvalidSinceValue_Errors", func(t *testing.T) {
		taskID, _ := seedTemporalLogs(t)
		defer resetTaskFlags()

		cmd := newTestCmd()
		cmd.AddCommand(logCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"log", taskID, "--since", "totally-not-a-time"})

		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected parse error on --since garbage; got nil; out:\n%s", buf.String())
		}
		if !contains(err.Error(), "--since") {
			t.Errorf("expected error to mention --since; got: %v", err)
		}
	})
}
