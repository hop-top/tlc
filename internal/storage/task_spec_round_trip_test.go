package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// newSpecTestStorage opens a file-backed store in a temp cwd with project
// detection reset, so GetTask resolves through the global bucket exactly
// as the CRUD tests do.
func newSpecTestStorage(t *testing.T) (*SQLiteStorage, string) {
	t.Helper()
	resetProjectDetection()
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		resetProjectDetection()
	})

	dbPath := filepath.Join(tmpDir, "test_task_spec.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dbPath
}

// specTestTask builds a task with every recipe-era field populated, plus
// an origin so the two origin-driven readers can find it.
func specTestTask(id string) *core.Task {
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	claimed := now.Add(5 * time.Minute)
	origin := "github"
	return &core.Task{
		ID:           id,
		Title:        "lint " + id,
		Status:       core.StatusInProgress,
		CreatedAt:    now,
		UpdatedAt:    now,
		OriginSystem: &origin,
		Meta:         map[string]interface{}{"origin_id": id},
		Kind:         core.TaskKindExec,
		Spec: &core.TaskSpec{
			Agent: "claude",
			When:  "results.build.exit_code == 0",
			Exec: &core.ExecSpec{
				Argv:      []string{"golangci-lint", "run"},
				Cwd:       ".",
				Env:       map[string]string{"CI": "1"},
				Timeout:   "5m",
				StdoutMax: 4096,
			},
			Human: &core.HumanSpec{Assignee: "@lead", Timeout: "48h", OnTimeout: "reject"},
			Retry: &core.RetrySpec{MaxAttempts: 3, Backoff: "30s"},
			Gate:  &core.StepGate{Contract: "lint-clean", EvaURL: "http://eva.local"},
		},
		// JSON numbers decode to float64; the fixture uses float64 so
		// DeepEqual compares like with like after the round trip.
		Result:      map[string]interface{}{"exit_code": float64(0), "stdout": "ok"},
		Attempts:    2,
		ClaimedAt:   &claimed,
		RunID:       "run_01",
		StepID:      "lint",
		StepOrdinal: 3,
	}
}

func assertTimePtr(t *testing.T, label string, got, want *time.Time) {
	t.Helper()
	switch {
	case want == nil && got == nil:
	case want == nil:
		t.Errorf("%s = %v; want nil", label, got)
	case got == nil:
		t.Errorf("%s = nil; want %v", label, want)
	case !got.Equal(*want):
		t.Errorf("%s = %v; want %v", label, got, want)
	case got.Location() != time.UTC:
		t.Errorf("%s location = %v; want UTC", label, got.Location())
	}
}

func assertSpecFields(t *testing.T, label string, got, want *core.Task) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: returned nil task", label)
	}
	if got.Kind != want.Kind {
		t.Errorf("%s: Kind = %q; want %q", label, got.Kind, want.Kind)
	}
	if !reflect.DeepEqual(got.Spec, want.Spec) {
		t.Errorf("%s: Spec = %+v; want %+v", label, got.Spec, want.Spec)
	}
	if !reflect.DeepEqual(got.Result, want.Result) {
		t.Errorf("%s: Result = %v; want %v", label, got.Result, want.Result)
	}
	if got.Attempts != want.Attempts {
		t.Errorf("%s: Attempts = %d; want %d", label, got.Attempts, want.Attempts)
	}
	assertTimePtr(t, label+": ClaimedAt", got.ClaimedAt, want.ClaimedAt)
	if got.RunID != want.RunID || got.StepID != want.StepID || got.StepOrdinal != want.StepOrdinal {
		t.Errorf("%s: run/step = (%q, %q, %d); want (%q, %q, %d)", label,
			got.RunID, got.StepID, got.StepOrdinal, want.RunID, want.StepID, want.StepOrdinal)
	}
}

// firstTask adapts the list-returning readers to the single-task shape.
func firstTask(tasks []*core.Task, err error) (*core.Task, error) {
	if err != nil || len(tasks) != 1 {
		return nil, err
	}
	return tasks[0], nil
}

// TestTaskSpecFields_RoundTrip drives one fully-populated task through
// every reader the store exposes. Each reader had its own scan site
// before the codec; this keeps them from drifting apart again.
func TestTaskSpecFields_RoundTrip(t *testing.T) {
	s, dbPath := newSpecTestStorage(t)
	ctx := context.Background()
	want := specTestTask("task_spec_rt")

	if err := s.CreateTask(ctx, want); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	readers := []struct {
		name string
		read func() (*core.Task, error)
	}{
		{"GetTask", func() (*core.Task, error) { return s.GetTask(ctx, want.ID) }},
		{"GetTaskInProject", func() (*core.Task, error) { return s.GetTaskInProject(ctx, want.ID, "") }},
		{"ListTasks", func() (*core.Task, error) { return firstTask(s.ListTasks(ctx, core.Query{AllProjects: true})) }},
		{"FindTaskByOrigin", func() (*core.Task, error) { return s.FindTaskByOrigin(ctx, "github", want.ID) }},
		{"GetTasksNeedingPush", func() (*core.Task, error) { return firstTask(s.GetTasksNeedingPush(ctx)) }},
	}
	for _, r := range readers {
		got, err := r.read()
		if err != nil {
			t.Fatalf("%s: %v", r.name, err)
		}
		assertSpecFields(t, r.name, got, want)
	}

	assertRawSpecColumns(t, dbPath, want.ID)
}

// assertRawSpecColumns checks the stored shape: claimed_at is UTC RFC3339
// like every other timestamp, and the blobs are JSON so SQL can
// json_extract them later.
func assertRawSpecColumns(t *testing.T, dbPath, id string) {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer raw.Close()

	cols := readRecipeCols(t, raw, id)
	if cols.kind != "exec" || cols.attempts != 2 || cols.ordinal.Int64 != 3 {
		t.Errorf("raw kind/attempts/ordinal = %q/%d/%d; want exec/2/3", cols.kind, cols.attempts, cols.ordinal.Int64)
	}
	if !strings.HasSuffix(cols.nullable["claimed_at"].String, "Z") {
		t.Errorf("claimed_at stored as %q; want UTC RFC3339 with Z suffix", cols.nullable["claimed_at"].String)
	}
	if !strings.Contains(cols.nullable["spec"].String, `"max_attempts":3`) {
		t.Errorf("spec column is not the JSON spec: %s", cols.nullable["spec"].String)
	}
	if !strings.Contains(cols.nullable["result"].String, `"stdout":"ok"`) {
		t.Errorf("result column is not the JSON result: %s", cols.nullable["result"].String)
	}
}

// TestTaskSpecFields_UpdatePersists covers the two write paths that are
// not CreateTask: a plain update and the update+log transaction the
// state machine uses.
func TestTaskSpecFields_UpdatePersists(t *testing.T) {
	s, _ := newSpecTestStorage(t)
	ctx := context.Background()

	task := &core.Task{
		ID:        "task_spec_upd",
		Title:     "plain",
		Status:    core.StatusTodo,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	want := specTestTask(task.ID)
	want.Title = task.Title
	want.Status = task.Status
	want.OriginSystem = nil
	want.Meta = nil
	want.CreatedAt = task.CreatedAt
	want.UpdatedAt = time.Now().UTC()
	if err := s.UpdateTask(ctx, want); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	got, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after UpdateTask: %v", err)
	}
	assertSpecFields(t, "UpdateTask", got, want)

	got.Attempts = 5
	got.Result = map[string]interface{}{"exit_code": float64(1)}
	got.ClaimedAt = nil
	entry := &core.LogEntry{
		TaskID:    task.ID,
		Timestamp: time.Now().UTC(),
		By:        "agent",
		Action:    "RETRY",
		Note:      "attempt 5",
	}
	if err := s.UpdateTaskWithLog(ctx, got, entry); err != nil {
		t.Fatalf("UpdateTaskWithLog: %v", err)
	}
	again, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after UpdateTaskWithLog: %v", err)
	}
	assertSpecFields(t, "UpdateTaskWithLog", again, got)

	logs, err := s.GetTaskLogs(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTaskLogs: %v", err)
	}
	if len(logs) != 1 || logs[0].Action != "RETRY" {
		t.Errorf("logs = %+v; want one RETRY entry", logs)
	}
}

// TestTaskSpecFields_DefaultsForPlainTask pins what a task that never
// touched recipes looks like after a round trip: kind is made explicit
// on persist (the column is NOT NULL), everything else stays empty and
// the nullable columns stay NULL rather than "" or 0.
func TestTaskSpecFields_DefaultsForPlainTask(t *testing.T) {
	s, dbPath := newSpecTestStorage(t)
	ctx := context.Background()

	task := &core.Task{
		ID:        "task_spec_plain",
		Title:     "plain",
		Status:    core.StatusTodo,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	got, err := s.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	want := &core.Task{Kind: core.TaskKindAgent}
	assertSpecFields(t, "plain task", got, want)

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer raw.Close()
	assertRecipeDefaults(t, "plain task", readRecipeCols(t, raw, task.ID))
}
