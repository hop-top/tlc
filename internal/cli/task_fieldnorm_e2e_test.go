package cli

// End-to-end tests for case-insensitive + alias + fuzzy field normalization
// on `tlc task list --status` and `--priority`.

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

func TestFieldNorm_E2E_StatusLowercase(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0001",
		Title:     "Done task",
		Status:    core.StatusDone,
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0002",
		Title:     "Todo task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "done"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --status done: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "T-0001") {
		t.Errorf("expected T-0001 (DONE) in output:\n%s", out)
	}
	if strings.Contains(out, "T-0002") {
		t.Errorf("unexpected T-0002 (TODO) with --status done:\n%s", out)
	}
}

func TestFieldNorm_E2E_StatusAlias(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0010",
		Title:     "Done task",
		Status:    core.StatusDone,
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0011",
		Title:     "Todo task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})

	for _, alias := range []string{"complete", "completed", "finish"} {
		t.Run(alias, func(t *testing.T) {
			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{"task", "list", "--status", alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task list --status %s: %v", alias, err)
			}
			out := buf.String()
			if !strings.Contains(out, "T-0010") {
				t.Errorf("alias %q: expected T-0010 (DONE) in output:\n%s", alias, out)
			}
			if strings.Contains(out, "T-0011") {
				t.Errorf("alias %q: unexpected T-0011 (TODO):\n%s", alias, out)
			}
		})
	}
}

func TestFieldNorm_E2E_PriorityAlias(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0020",
		Title:     "High priority task",
		Status:    core.StatusTodo,
		Priority:  "P1",
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0021",
		Title:     "Low priority task",
		Status:    core.StatusTodo,
		Priority:  "P3",
		CreatedAt: now,
		UpdatedAt: now,
	})

	for _, alias := range []string{"high", "1", "p1"} {
		t.Run(alias, func(t *testing.T) {
			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{"task", "list", "--status", "TODO", "--priority", alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task list --priority %s: %v", alias, err)
			}
			out := buf.String()
			if !strings.Contains(out, "T-0020") {
				t.Errorf("alias %q: expected T-0020 (P1) in output:\n%s", alias, out)
			}
			if strings.Contains(out, "T-0021") {
				t.Errorf("alias %q: unexpected T-0021 (P3):\n%s", alias, out)
			}
		})
	}
}

func TestFieldNorm_E2E_UnknownStatus(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	cmd.SetArgs([]string{"task", "list", "--status", "xyz"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown status, got nil")
	}
	if !strings.Contains(err.Error(), "xyz") {
		t.Errorf("expected error to mention 'xyz', got: %v", err)
	}
}

func TestFieldNorm_E2E_UpdateStatusAlias(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0030",
		Title:     "Alias update task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})

	for _, tc := range []struct {
		alias  string
		expect core.TaskStatus
	}{
		{"in-progress", core.StatusInProgress},
		{"wip", core.StatusInProgress},
		{"IN_PROGRESS", core.StatusInProgress},
	} {
		t.Run(tc.alias, func(t *testing.T) {
			// Reset to TODO first
			task := mustGetTask(t, ctx, s, "T-0030")
			task.Status = core.StatusTodo
			_ = s.UpdateTask(ctx, task)

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs([]string{"task", "update", "T-0030", "--status", tc.alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task update --status %s: %v", tc.alias, err)
			}

			updated := mustGetTask(t, ctx, s, "T-0030")
			if updated.Status != tc.expect {
				t.Errorf("alias %q: got status %s, want %s", tc.alias, updated.Status, tc.expect)
			}
		})
	}
}

func TestFieldNorm_E2E_UpdateStatusUnknown(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	now := time.Now()
	_ = s.CreateTask(ctx, &core.Task{
		ID:        "T-0031",
		Title:     "Unknown status task",
		Status:    core.StatusTodo,
		CreatedAt: now,
		UpdatedAt: now,
	})

	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	cmd.SetArgs([]string{"task", "update", "T-0031", "--status", "xyz"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown status, got nil")
	}
	if !strings.Contains(err.Error(), "xyz") {
		t.Errorf("expected error to mention 'xyz', got: %v", err)
	}
}

// firstTask returns the single task that should exist after a fresh
// setupTestDir + one create. Fails the test if zero or more than one is
// present.
func firstTask(t *testing.T, ctx context.Context, s *storage.SQLiteStorage) *core.Task {
	t.Helper()
	tasks, err := s.ListTasks(ctx, core.Query{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected exactly 1 task, got %d", len(tasks))
	}
	return tasks[0]
}

// TestFieldNorm_E2E_CreateStatusLowercase verifies `tlc task create x
// -s todo` stores canonical "TODO", not the literal lowercase input.
// This is the regression test for the corruption bug fixed in T-1352
// where the create path bypassed normalization entirely and the row
// became invisible to canonical filters.
func TestFieldNorm_E2E_CreateStatusLowercase(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "lowercase status", "--status", "todo"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create --status todo: %v", err)
	}

	got := firstTask(t, ctx, s)
	if got.Status != core.StatusTodo {
		t.Errorf("expected status %q, got %q (literal-not-canonical regression)", core.StatusTodo, got.Status)
	}
}

func TestFieldNorm_E2E_CreateStatusAlias(t *testing.T) {
	for _, tc := range []struct {
		alias  string
		expect core.TaskStatus
	}{
		{"wip", core.StatusInProgress},
		{"in-progress", core.StatusInProgress},
		{"complete", core.StatusDone},
		{"skip", core.StatusSkipped},
	} {
		t.Run(tc.alias, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "create", "alias status", "--status", tc.alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task create --status %s: %v", tc.alias, err)
			}

			got := firstTask(t, ctx, s)
			if got.Status != tc.expect {
				t.Errorf("alias %q: got status %q, want %q", tc.alias, got.Status, tc.expect)
			}
		})
	}
}

func TestFieldNorm_E2E_CreatePriorityLowercase(t *testing.T) {
	for _, tc := range []struct {
		input  string
		expect core.Priority
	}{
		{"p0", core.PriorityP0},
		{"p1", core.PriorityP1},
		{"p2", core.PriorityP2},
		{"p3", core.PriorityP3},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "create", "lowercase priority", "--priority", tc.input})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task create --priority %s: %v", tc.input, err)
			}

			got := firstTask(t, ctx, s)
			if got.Priority != tc.expect {
				t.Errorf("input %q: got priority %q, want %q", tc.input, got.Priority, tc.expect)
			}
		})
	}
}

func TestFieldNorm_E2E_CreatePriorityAlias(t *testing.T) {
	for _, tc := range []struct {
		alias  string
		expect core.Priority
	}{
		{"critical", core.PriorityP0},
		{"high", core.PriorityP1},
		{"medium", core.PriorityP2},
		{"med", core.PriorityP2},
		{"low", core.PriorityP3},
	} {
		t.Run(tc.alias, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "create", "alias priority", "--priority", tc.alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task create --priority %s: %v", tc.alias, err)
			}

			got := firstTask(t, ctx, s)
			if got.Priority != tc.expect {
				t.Errorf("alias %q: got priority %q, want %q", tc.alias, got.Priority, tc.expect)
			}
		})
	}
}

func TestFieldNorm_E2E_CreateEffortLowercase(t *testing.T) {
	for _, tc := range []struct {
		input  string
		expect core.Effort
	}{
		{"xs", core.EffortXS},
		{"s", core.EffortS},
		{"m", core.EffortM},
		{"l", core.EffortL},
		{"xl", core.EffortXL},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "create", "lowercase effort", "--effort", tc.input})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task create --effort %s: %v", tc.input, err)
			}

			got := firstTask(t, ctx, s)
			if got.Effort != tc.expect {
				t.Errorf("input %q: got effort %q, want %q", tc.input, got.Effort, tc.expect)
			}
		})
	}
}

func TestFieldNorm_E2E_CreateEffortAlias(t *testing.T) {
	for _, tc := range []struct {
		alias  string
		expect core.Effort
	}{
		{"tiny", core.EffortXS},
		{"small", core.EffortS},
		{"medium", core.EffortM},
		{"med", core.EffortM},
		{"large", core.EffortL},
		{"huge", core.EffortXL},
	} {
		t.Run(tc.alias, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "create", "alias effort", "--effort", tc.alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task create --effort %s: %v", tc.alias, err)
			}

			got := firstTask(t, ctx, s)
			if got.Effort != tc.expect {
				t.Errorf("alias %q: got effort %q, want %q", tc.alias, got.Effort, tc.expect)
			}
		})
	}
}

// TestFieldNorm_E2E_CreateStatusCorruptionRegression locks in the most
// safety-critical fix from T-1352: a row created with non-canonical
// status must not be invisible to the canonical filter. Before the
// fix, `create -s todo` stored "todo" literal and `list --status TODO`
// (which normalizes to TODO) never matched it.
func TestFieldNorm_E2E_CreateStatusCorruptionRegression(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	resetTaskFlags()
	createCmd := newTestCmd()
	createCmd.AddCommand(TaskCmd)
	createBuf := new(bytes.Buffer)
	createCmd.SetOut(createBuf)
	createCmd.SetErr(createBuf)
	createCmd.SetArgs([]string{"task", "create", "regression-row", "--status", "todo"})
	if err := createCmd.Execute(); err != nil {
		t.Fatalf("create: %v", err)
	}

	resetTaskFlags()
	listCmd := newTestCmd()
	listCmd.AddCommand(TaskCmd)
	listBuf := new(bytes.Buffer)
	listCmd.SetOut(listBuf)
	listCmd.SetArgs([]string{"task", "list", "--status", "TODO"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("list: %v", err)
	}

	out := listBuf.String()
	if !strings.Contains(out, "regression-row") {
		t.Errorf("created with --status todo, list --status TODO did not find it:\n%s", out)
	}
}

func TestFieldNorm_E2E_CreateUnknownPriority(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "garbage priority", "--priority", "garbage"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unresolvable priority, got nil")
	}
	if !strings.Contains(err.Error(), "garbage") {
		t.Errorf("expected error to mention 'garbage', got: %v", err)
	}
}

func TestFieldNorm_E2E_UpdatePriorityLowercase(t *testing.T) {
	for _, tc := range []struct {
		input  string
		expect core.Priority
	}{
		{"p0", core.PriorityP0},
		{"p1", core.PriorityP1},
		{"p2", core.PriorityP2},
		{"p3", core.PriorityP3},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			_ = s.CreateTask(ctx, &core.Task{
				ID:     "T-0001",
				Title:  "update lowercase priority",
				Status: core.StatusTodo,
			})

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "update", "T-0001", "--priority", tc.input})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task update --priority %s: %v", tc.input, err)
			}

			got := mustGetTask(t, ctx, s, "T-0001")
			if got.Priority != tc.expect {
				t.Errorf("input %q: got priority %q, want %q", tc.input, got.Priority, tc.expect)
			}
		})
	}
}

func TestFieldNorm_E2E_UpdatePriorityAlias(t *testing.T) {
	for _, tc := range []struct {
		alias  string
		expect core.Priority
	}{
		{"critical", core.PriorityP0},
		{"high", core.PriorityP1},
		{"medium", core.PriorityP2},
		{"low", core.PriorityP3},
	} {
		t.Run(tc.alias, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			_ = s.CreateTask(ctx, &core.Task{
				ID:     "T-0001",
				Title:  "update alias priority",
				Status: core.StatusTodo,
			})

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "update", "T-0001", "--priority", tc.alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task update --priority %s: %v", tc.alias, err)
			}

			got := mustGetTask(t, ctx, s, "T-0001")
			if got.Priority != tc.expect {
				t.Errorf("alias %q: got priority %q, want %q", tc.alias, got.Priority, tc.expect)
			}
		})
	}
}

func TestFieldNorm_E2E_UpdateEffortLowercase(t *testing.T) {
	for _, tc := range []struct {
		input  string
		expect core.Effort
	}{
		{"xs", core.EffortXS},
		{"s", core.EffortS},
		{"m", core.EffortM},
		{"l", core.EffortL},
		{"xl", core.EffortXL},
	} {
		t.Run(tc.input, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			_ = s.CreateTask(ctx, &core.Task{
				ID:     "T-0001",
				Title:  "update lowercase effort",
				Status: core.StatusTodo,
			})

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "update", "T-0001", "--effort", tc.input})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task update --effort %s: %v", tc.input, err)
			}

			got := mustGetTask(t, ctx, s, "T-0001")
			if got.Effort != tc.expect {
				t.Errorf("input %q: got effort %q, want %q", tc.input, got.Effort, tc.expect)
			}
		})
	}
}

func TestFieldNorm_E2E_UpdateEffortAlias(t *testing.T) {
	for _, tc := range []struct {
		alias  string
		expect core.Effort
	}{
		{"tiny", core.EffortXS},
		{"small", core.EffortS},
		{"medium", core.EffortM},
		{"large", core.EffortL},
		{"huge", core.EffortXL},
	} {
		t.Run(tc.alias, func(t *testing.T) {
			ctx, cleanup := setupTestDir(t)
			defer cleanup()
			s, _ := getStorageRaw()
			defer s.Close()

			_ = s.CreateTask(ctx, &core.Task{
				ID:     "T-0001",
				Title:  "update alias effort",
				Status: core.StatusTodo,
			})

			resetTaskFlags()
			cmd := newTestCmd()
			cmd.AddCommand(TaskCmd)
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs([]string{"task", "update", "T-0001", "--effort", tc.alias})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("task update --effort %s: %v", tc.alias, err)
			}

			got := mustGetTask(t, ctx, s, "T-0001")
			if got.Effort != tc.expect {
				t.Errorf("alias %q: got effort %q, want %q", tc.alias, got.Effort, tc.expect)
			}
		})
	}
}

// TestFieldNorm_E2E_UpdateEffortClearEmpty locks in that an explicitly
// empty effort value on update clears the field, matching the
// pre-T-1353 contract where core.ValidEffort accepted "". Repo
// convention (see also --due, --remind-at, --rrule) accepts "" as a
// clear sentinel.
func TestFieldNorm_E2E_UpdateEffortClearEmpty(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	_ = s.CreateTask(ctx, &core.Task{
		ID:     "T-0001",
		Title:  "task with effort",
		Status: core.StatusTodo,
		Effort: core.EffortL,
	})

	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--effort", ""})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --effort '': %v", err)
	}

	got := mustGetTask(t, ctx, s, "T-0001")
	if got.Effort != "" {
		t.Errorf("expected effort cleared, got %q", got.Effort)
	}
}

// TestFieldNorm_E2E_UpdateEffortClearDash locks in the "-" clear
// sentinel for effort, matching the existing pattern for --due,
// --remind-at, --rrule, --assigned-to.
func TestFieldNorm_E2E_UpdateEffortClearDash(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	_ = s.CreateTask(ctx, &core.Task{
		ID:     "T-0001",
		Title:  "task with effort",
		Status: core.StatusTodo,
		Effort: core.EffortL,
	})

	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--effort", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --effort -: %v", err)
	}

	got := mustGetTask(t, ctx, s, "T-0001")
	if got.Effort != "" {
		t.Errorf("expected effort cleared, got %q", got.Effort)
	}
}

// TestFieldNorm_E2E_UpdatePriorityClearEmpty — symmetric to the effort
// clear-empty test. Pre-T-1353, core.ValidPriority accepted "" and the
// field would be cleared. The normalizer-on-empty path must preserve
// that contract.
func TestFieldNorm_E2E_UpdatePriorityClearEmpty(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	_ = s.CreateTask(ctx, &core.Task{
		ID:       "T-0001",
		Title:    "task with priority",
		Status:   core.StatusTodo,
		Priority: core.PriorityP1,
	})

	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--priority", ""})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --priority '': %v", err)
	}

	got := mustGetTask(t, ctx, s, "T-0001")
	if got.Priority != "" {
		t.Errorf("expected priority cleared, got %q", got.Priority)
	}
}

func TestFieldNorm_E2E_UpdatePriorityClearDash(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	_ = s.CreateTask(ctx, &core.Task{
		ID:       "T-0001",
		Title:    "task with priority",
		Status:   core.StatusTodo,
		Priority: core.PriorityP1,
	})

	resetTaskFlags()
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--priority", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update --priority -: %v", err)
	}

	got := mustGetTask(t, ctx, s, "T-0001")
	if got.Priority != "" {
		t.Errorf("expected priority cleared, got %q", got.Priority)
	}
}
