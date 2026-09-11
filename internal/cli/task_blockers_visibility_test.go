package cli

// Tests for dependency-blocker visibility outside the human `task show`
// view: the `task list` Blocked column and the `task show --format json`
// payload must both surface unmet blockers so dependencies are scriptable.

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// seedBlockedPair creates a blocker task in the given status plus a task
// depending on it, and returns the storage-visible IDs.
func seedBlockedPair(t *testing.T, blockerStatus core.TaskStatus) func() {
	t.Helper()
	ctx, cleanup := setupTestDir(t)
	s, _ := getStorageRaw()

	now := time.Now().UTC()
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0001", Title: "Blocker task", Status: blockerStatus,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	if err := s.CreateTask(ctx, &core.Task{
		ID: "T-0002", Title: "Dependent task", Status: core.StatusTodo,
		CreatedAt: now, UpdatedAt: now,
		Meta: map[string]any{"blocked_by": []string{"T-0001"}},
	}); err != nil {
		t.Fatalf("create dependent: %v", err)
	}
	_ = s.Close()
	return cleanup
}

// TestTaskShowJSON_IncludesResolvedBlockedBy asserts that
// `task show --format json` exposes a top-level blocked_by array with
// resolved, human-meaningful refs — not just opaque IDs buried in meta.
func TestTaskShowJSON_IncludesResolvedBlockedBy(t *testing.T) {
	cleanup := seedBlockedPair(t, core.StatusTodo)
	defer cleanup()

	viper.Set("output.format", "json")
	defer viper.Set("output.format", "table")

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0002"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show --format json: %v", err)
	}

	var payload struct {
		Task struct {
			ID        string `json:"id"`
			BlockedBy []struct {
				Ref    string `json:"ref"`
				Title  string `json:"title"`
				Status string `json:"status"`
				Met    bool   `json:"met"`
			} `json:"blocked_by"`
		} `json:"task"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal show json: %v\nraw: %s", err, buf.String())
	}

	if len(payload.Task.BlockedBy) != 1 {
		t.Fatalf("task.blocked_by = %+v, want 1 entry\nraw: %s",
			payload.Task.BlockedBy, buf.String())
	}
	got := payload.Task.BlockedBy[0]
	if got.Ref != "T-0001" {
		t.Errorf("blocked_by[0].ref = %q, want %q (resolved seq ID)", got.Ref, "T-0001")
	}
	if got.Title != "Blocker task" {
		t.Errorf("blocked_by[0].title = %q, want %q", got.Title, "Blocker task")
	}
	if got.Met {
		t.Errorf("blocked_by[0].met = true, want false (blocker is TODO)")
	}
}

// TestTaskShowJSON_PreservesExistingKeys guards the compatibility
// contract: adding blocked_by must not restructure or drop the keys
// existing tooling already parses.
func TestTaskShowJSON_PreservesExistingKeys(t *testing.T) {
	cleanup := seedBlockedPair(t, core.StatusTodo)
	defer cleanup()

	viper.Set("output.format", "json")
	defer viper.Set("output.format", "table")

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0002"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show --format json: %v", err)
	}

	var payload struct {
		Task map[string]any `json:"task"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal show json: %v", err)
	}
	for _, key := range []string{
		"id", "seq", "title", "status", "assigned_to", "reference",
		"created_at", "updated_at", "archived", "meta",
	} {
		if _, ok := payload.Task[key]; !ok {
			t.Errorf("task.%s missing from JSON output; existing tooling depends on it", key)
		}
	}
	// meta.blocked_by must remain reachable for backwards compatibility.
	meta, _ := payload.Task["meta"].(map[string]any)
	if _, ok := meta["blocked_by"]; !ok {
		t.Errorf("meta.blocked_by disappeared; raw: %s", buf.String())
	}
}

// TestTaskShowJSON_MetBlockerMarkedMet asserts a DONE blocker is
// reported as met, so scripts can distinguish satisfied dependencies.
func TestTaskShowJSON_MetBlockerMarkedMet(t *testing.T) {
	cleanup := seedBlockedPair(t, core.StatusDone)
	defer cleanup()

	viper.Set("output.format", "json")
	defer viper.Set("output.format", "table")

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0002"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show --format json: %v", err)
	}

	var payload struct {
		Task struct {
			BlockedBy []struct {
				Met bool `json:"met"`
			} `json:"blocked_by"`
		} `json:"task"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal show json: %v", err)
	}
	if len(payload.Task.BlockedBy) != 1 {
		t.Fatalf("blocked_by = %+v, want 1 entry", payload.Task.BlockedBy)
	}
	if !payload.Task.BlockedBy[0].Met {
		t.Errorf("blocked_by[0].met = false, want true (blocker is DONE)")
	}
}

// blockedCellFor returns the Blocked-column cell for the table row
// whose first column is taskID. Cells are split on runs of 2+ spaces,
// matching the plain table renderer's padding.
func blockedCellFor(t *testing.T, out, taskID string) string {
	t.Helper()
	var header []string
	for _, line := range strings.Split(out, "\n") {
		fields := splitTableRow(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "ID" {
			header = fields
			continue
		}
		if fields[0] != taskID {
			continue
		}
		for i, h := range header {
			if h == "Blocked" && i < len(fields) {
				return fields[i]
			}
		}
		t.Fatalf("no Blocked column in header %v", header)
	}
	t.Fatalf("no row for %s in output:\n%s", taskID, out)
	return ""
}

func splitTableRow(line string) []string {
	var fields []string
	for _, f := range regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(line), -1) {
		if f != "" {
			fields = append(fields, f)
		}
	}
	return fields
}

// TestTaskList_BlockedColumnShowsUnmetBlockers asserts the Blocked
// column reports a dependency blocker rather than printing "-".
func TestTaskList_BlockedColumnShowsUnmetBlockers(t *testing.T) {
	cleanup := seedBlockedPair(t, core.StatusTodo)
	defer cleanup()

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
	cell := blockedCellFor(t, out, "T-0002")
	if !strings.Contains(cell, "T-0001") {
		t.Errorf("Blocked cell for T-0002 = %q, want it to name unmet blocker T-0001; output:\n%s",
			cell, out)
	}
}

// TestTaskList_BlockedColumnIgnoresMetBlockers asserts a satisfied
// dependency does not light up the Blocked column.
func TestTaskList_BlockedColumnIgnoresMetBlockers(t *testing.T) {
	cleanup := seedBlockedPair(t, core.StatusDone)
	defer cleanup()

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
	cell := blockedCellFor(t, out, "T-0002")
	if strings.Contains(cell, "T-0001") {
		t.Errorf("Blocked cell for T-0002 = %q, want no mention of the met (DONE) blocker; output:\n%s",
			cell, out)
	}
}
