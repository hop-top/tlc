package cli

// End-to-end tests for themed table output (story 074).
// Exercises the full CLI pipeline with 10 seeded tasks and verifies
// four-color rendering: green (primary), pink (blocker), muted (blocked),
// white (default).

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// ansi helpers — match truecolor SGR sequences for kit theme colors.
const (
	// theme.Accent = Neon.Command = #7ED957 (grass green)
	ansiGreen = "\x1b[38;2;126;217;87m"
	// theme.Secondary = Neon.Flag = #FF00FF (neon pink)
	ansiPink = "\x1b[38;2;255;0;255m"
	// theme.Muted = charmtone.Squid = #858392
	ansiMuted = "\x1b[38;2;133;131;146m"
)

// rowLine finds the single rendered table line whose ID column (first
// cell) matches the given task ID. Returns empty string if not found.
func rowLine(output, taskID string) string {
	for _, line := range strings.Split(output, "\n") {
		clean := stripAnsi(line)
		// Split by │ to get cells; first non-empty cell is the ID column.
		cells := strings.Split(clean, "│")
		for _, cell := range cells {
			trimmed := strings.TrimSpace(cell)
			if trimmed == "" {
				continue
			}
			if trimmed == taskID {
				return line
			}
			break // only check first non-empty cell
		}
	}
	return ""
}

// cellColors extracts the ANSI color sequences applied to cell content
// (not borders) in a table row. Borders use muted color for │ chars;
// this function strips those out and returns only cell-content colors.
func cellColors(line string) []string {
	var colors []string
	// Split by the reset sequence to isolate styled segments.
	// Each segment is: <ANSI-color>content
	parts := strings.Split(line, "\x1b[m")
	for _, part := range parts {
		idx := strings.Index(part, "\x1b[")
		if idx < 0 {
			continue
		}
		// Extract the ANSI sequence.
		end := strings.IndexByte(part[idx:], 'm')
		if end < 0 {
			continue
		}
		seq := part[idx : idx+end+1]
		content := part[idx+end+1:]
		// Skip border characters (│).
		trimmed := strings.TrimSpace(content)
		if trimmed == "│" || trimmed == "" {
			continue
		}
		colors = append(colors, seq)
	}
	return colors
}

// assertRowColor checks that the row for taskID has cell content
// colored with expectedANSI and does NOT have cells colored with
// any of the excluded sequences.
func assertRowColor(t *testing.T, output, taskID, expectedANSI string, excludedANSI []string) {
	t.Helper()
	line := rowLine(output, taskID)
	if line == "" {
		t.Errorf("task %s not found in output", taskID)
		return
	}
	cc := cellColors(line)
	hasExpected := false
	for _, c := range cc {
		if c == expectedANSI {
			hasExpected = true
		}
		for _, excl := range excludedANSI {
			if c == excl {
				t.Errorf("task %s: unexpected cell color %q in line:\n  %q",
					taskID, excl, line)
				return
			}
		}
	}
	if !hasExpected {
		t.Errorf("task %s: expected cell color %q not found; got colors %v in line:\n  %q",
			taskID, expectedANSI, cc, line)
	}
}

// assertCellHas verifies that cell colors include expected and exclude others.
func assertCellHas(t *testing.T, taskID, role string, cc []string, expected string, excluded []string) {
	t.Helper()
	hasExpected := false
	for _, c := range cc {
		if c == expected {
			hasExpected = true
		}
		for _, excl := range excluded {
			if c == excl {
				t.Errorf("task %s (%s): unexpected cell color %q", taskID, role, c)
				return
			}
		}
	}
	if !hasExpected {
		t.Errorf("task %s (%s): missing expected color %q; got %v", taskID, role, expected, cc)
	}
}

// seedScenario2 creates 10 tasks matching story 074 Scenario 2.
// T-0003 is TODO (blocker of T-0008), producing all four colors.
func seedScenario2(t *testing.T, s *storage.SQLiteStorage, ctx context.Context) {
	t.Helper()
	reason0006 := "T-0010"
	reason0008 := "T-0003"
	reason0009 := "T-0004"

	tasks := []*core.Task{
		{ID: "T-0001", Title: "Design auth API", Status: core.StatusInProgress},
		{ID: "T-0002", Title: "Implement JWT middleware", Status: core.StatusInProgress},
		{ID: "T-0003", Title: "Write auth unit tests", Status: core.StatusTodo,
			Meta: map[string]interface{}{}},
		{ID: "T-0004", Title: "Integrate OAuth provider", Status: core.StatusInProgress},
		{ID: "T-0005", Title: "Add rate limiting", Status: core.StatusTodo},
		{ID: "T-0006", Title: "Update API docs", Status: core.StatusTodo,
			BlockedReason: &reason0006,
			Meta:          map[string]interface{}{"blocked_by": "T-0010"}},
		{ID: "T-0007", Title: "Migrate session store", Status: core.StatusTodo},
		{ID: "T-0008", Title: "Refactor token refresh", Status: core.StatusTodo,
			BlockedReason: &reason0008,
			Meta:          map[string]interface{}{"blocked_by": "T-0003"}},
		{ID: "T-0009", Title: "Deploy auth service", Status: core.StatusTodo,
			BlockedReason: &reason0009,
			Meta:          map[string]interface{}{"blocked_by": "T-0004"}},
		{ID: "T-0010", Title: "Review security audit", Status: core.StatusInProgress},
	}

	for _, task := range tasks {
		task.CreatedAt = time.Now()
		task.UpdatedAt = time.Now()
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task %s: %v", task.ID, err)
		}
	}
}

// TestThemedTable_FourColors_E2E verifies that `tlc task list` renders
// the four distinct row colors described in story 074 Scenario 2.
func TestThemedTable_FourColors_E2E(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	resetTaskFlags()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	seedScenario2(t, s, ctx)

	// Expected color per task — purely status + state driven.
	// IN_PROGRESS → green, blocker (not IN_PROGRESS) → pink,
	// blocked → muted, rest → white.
	type rowExpect struct {
		color    string
		exclude  []string
		role     string
	}
	taskExpect := map[string]rowExpect{
		"T-0001": {ansiGreen, []string{ansiPink, ansiMuted}, "in-progress"},
		"T-0002": {ansiGreen, []string{ansiPink, ansiMuted}, "in-progress"},
		"T-0003": {ansiPink, []string{ansiGreen, ansiMuted}, "blocker"},
		"T-0004": {ansiGreen, []string{ansiPink, ansiMuted}, "in-progress"},
		"T-0005": {"", nil, "white"},
		"T-0006": {ansiMuted, []string{ansiGreen, ansiPink}, "blocked"},
		"T-0007": {"", nil, "white"},
		"T-0008": {ansiMuted, []string{ansiGreen, ansiPink}, "blocked"},
		"T-0009": {ansiMuted, []string{ansiGreen, ansiPink}, "blocked"},
		"T-0010": {ansiGreen, []string{ansiPink, ansiMuted}, "in-progress"},
	}

	// blockerOf maps blocker → the task it blocks. A blocker only
	// renders pink if the blocked task is also in the result set.
	blockerOf := map[string]string{
		"T-0003": "T-0008",
	}

	assertVisibleColors := func(t *testing.T, out string) {
		t.Helper()
		for id, exp := range taskExpect {
			line := rowLine(out, id)
			if line == "" {
				continue // not in result set
			}
			// Blocker role depends on blocked task being visible.
			if dep, ok := blockerOf[id]; ok && rowLine(out, dep) == "" {
				// Blocked task not visible → blocker falls to white.
				cc := cellColors(line)
				for _, c := range cc {
					if c == ansiGreen || c == ansiPink || c == ansiMuted {
						t.Errorf("task %s (white/no-dep): unexpected color %q", id, c)
					}
				}
				continue
			}
			cc := cellColors(line)
			if exp.color == "" {
				for _, c := range cc {
					if c == ansiGreen || c == ansiPink || c == ansiMuted {
						t.Errorf("task %s (%s): unexpected color %q", id, exp.role, c)
					}
				}
			} else {
				assertCellHas(t, id, exp.role, cc, exp.color, exp.exclude)
			}
		}
	}

	t.Run("full list", func(t *testing.T) {
		resetTaskFlags()
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

		// All 10 tasks must be present.
		for _, id := range []string{
			"T-0001", "T-0002", "T-0003", "T-0004", "T-0005",
			"T-0006", "T-0007", "T-0008", "T-0009", "T-0010",
		} {
			if !strings.Contains(stripAnsi(out), id) {
				t.Errorf("missing task %s in output", id)
			}
		}

		assertVisibleColors(t, out)
	})

	t.Run("with --limit 7", func(t *testing.T) {
		resetTaskFlags()
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--limit", "7"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list --limit 7: %v", err)
		}

		out := buf.String()

		// Count visible rows — must not exceed limit.
		var count int
		for _, id := range []string{
			"T-0001", "T-0002", "T-0003", "T-0004", "T-0005",
			"T-0006", "T-0007", "T-0008", "T-0009", "T-0010",
		} {
			if rowLine(out, id) != "" {
				count++
			}
		}
		if count > 7 {
			t.Errorf("expected at most 7 rows; got %d", count)
		}
		if count == 0 {
			t.Fatal("no task rows found")
		}

		assertVisibleColors(t, out)
	})
}

// TestThemedTable_SingleFilter_E2E verifies that --status TODO applies
// status+state coloring: blockers pink, blocked muted, rest white
// (no IN_PROGRESS tasks → no green).
func TestThemedTable_SingleFilter_E2E(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	resetTaskFlags()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	seedScenario2(t, s, ctx)

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

	// PINK: T-0003 is a blocker (T-0008 depends on it).
	assertRowColor(t, out, "T-0003", ansiPink,
		[]string{ansiGreen, ansiMuted})

	// MUTED: blocked tasks.
	for _, id := range []string{"T-0006", "T-0008", "T-0009"} {
		assertRowColor(t, out, id, ansiMuted,
			[]string{ansiGreen, ansiPink})
	}

	// WHITE: TODO, not blocker, not blocked.
	for _, id := range []string{"T-0005", "T-0007"} {
		line := rowLine(out, id)
		if line == "" {
			t.Errorf("task %s not found", id)
			continue
		}
		cc := cellColors(line)
		for _, c := range cc {
			if c == ansiGreen || c == ansiPink || c == ansiMuted {
				t.Errorf("task %s (white): unexpected color %q", id, c)
			}
		}
	}
}

// TestThemedTable_HeadersInMuted_E2E verifies that table headers render
// in the muted color.
func TestThemedTable_HeadersInMuted_E2E(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	resetTaskFlags()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	task := &core.Task{
		ID:        "T-0001",
		Title:     "Header color test",
		Status:    core.StatusTodo,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "TODO"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list: %v", err)
	}

	out := buf.String()

	// Find the header line containing "ID" and "Title".
	for _, line := range strings.Split(out, "\n") {
		clean := stripAnsi(line)
		if strings.Contains(clean, "ID") && strings.Contains(clean, "Title") {
			if !strings.Contains(line, ansiMuted) {
				t.Errorf("header line missing muted color:\n  %q", line)
			}
			return
		}
	}
	t.Error("header line with ID and Title not found")
}

