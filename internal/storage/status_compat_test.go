package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

func withVocab(t *testing.T, names []string, terminal map[string]bool) {
	t.Helper()
	defs := make([]config.StatusDefinition, 0, len(names))
	for _, n := range names {
		defs = append(defs, config.StatusDefinition{Name: n, Label: n, IsTerminal: terminal[n]})
	}
	core.SetTaskConfigProvider(func() *config.TaskConfig {
		return &config.TaskConfig{Statuses: defs}
	})
	core.ResetDefaultWorkflow()
	t.Cleanup(func() {
		core.SetTaskConfigProvider(nil)
		core.ResetDefaultWorkflow()
	})
}

// withDefaultVocab declares the seven default statuses.
func withDefaultVocab(t *testing.T) {
	t.Helper()
	withVocab(t, core.TaskStatusStrings(), map[string]bool{"done": true, string(core.StatusCancelled): true})
}

// withLegacyVocab declares a custom vocabulary that spells its names the
// way old rows do; reads must leave it alone.
func withLegacyVocab(t *testing.T) {
	t.Helper()
	withVocab(t, []string{"TODO", "IN_PROGRESS", "DONE", "SKIPPED"},
		map[string]bool{"DONE": true, "SKIPPED": true})
}

var legacySeed = map[string]string{
	"T-1": "TODO",
	"T-2": "IN_PROGRESS",
	"T-3": "DONE",
	"T-4": "SKIPPED",
	"T-5": "BACKLOG",
	"T-6": "Todo",
	"T-7": "todo",
	"T-8": "in_review",
}

var legacyWant = map[string]string{
	"T-1": "todo",
	"T-2": "in_progress",
	"T-3": "done",
	"T-4": string(core.StatusCancelled),
	"T-5": "backlog",
	"T-6": "todo",
	"T-7": "todo",
	"T-8": "in_review",
}

// openSeededLegacy builds a file DB holding legacy rows, closes it and
// reopens it, as a user's existing DB would be met by a new binary.
func openSeededLegacy(t *testing.T) *SQLiteStorage {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tlc.db")
	s, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	now := "2026-04-27T00:00:00Z"
	seq := int64(1)
	for id, status := range legacySeed {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO tasks (project_id, id, seq, title, description, status, reference, created_at, updated_at)
			VALUES ('', ?, ?, ?, '', ?, ?, ?, ?)
		`, id, seq, "title-"+id, status, "ref:"+id, now, now); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		seq++
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	s, err = NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func rawStatus(t *testing.T, s *SQLiteStorage, id string) string {
	t.Helper()
	var got string
	if err := s.db.QueryRowContext(context.Background(), `SELECT status FROM tasks WHERE id = ?`, id).Scan(&got); err != nil {
		t.Fatalf("raw status %s: %v", id, err)
	}
	return got
}

func TestRead_LegacyLabelsNormaliseUnderDefault(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)
	ctx := context.Background()

	for id, want := range legacyWant {
		got, err := s.GetTaskInProject(ctx, id, "")
		if err != nil || got == nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if string(got.Status) != want {
			t.Errorf("get %s: %q, want %q", id, got.Status, want)
		}
		if raw := rawStatus(t, s, id); raw != legacySeed[id] {
			t.Errorf("get %s rewrote stored row: %q, was %q", id, raw, legacySeed[id])
		}
	}

	tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != len(legacySeed) {
		t.Fatalf("list: %d tasks, want %d", len(tasks), len(legacySeed))
	}
	for _, task := range tasks {
		if string(task.Status) != legacyWant[task.ID] {
			t.Errorf("list %s: %q, want %q", task.ID, task.Status, legacyWant[task.ID])
		}
	}
}

func TestRead_CustomVocabularyUntouched(t *testing.T) {
	withLegacyVocab(t)
	s := openSeededLegacy(t)
	ctx := context.Background()
	for _, id := range []string{"T-1", "T-2", "T-3", "T-4"} {
		got, err := s.GetTaskInProject(ctx, id, "")
		if err != nil || got == nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if string(got.Status) != legacySeed[id] {
			t.Errorf("%s: %q, want %q", id, got.Status, legacySeed[id])
		}
	}
}

func queryIDs(t *testing.T, s *SQLiteStorage, q core.Query) []string {
	t.Helper()
	q.AllProjects = true
	tasks, err := s.ListTasks(context.Background(), q)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func sameSet(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	m := map[string]bool{}
	for _, g := range got {
		m[g] = true
	}
	for _, w := range want {
		if !m[w] {
			return false
		}
	}
	return true
}

func TestQuery_StatusFilterMatchesLegacyRows(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)

	cases := []struct {
		name string
		f    core.FieldFilter
		want []string
	}{
		{"eq todo", core.FieldFilter{Field: "status", Operator: core.OpEq, Value: "todo"}, []string{"T-1", "T-6", "T-7"}},
		{"eq legacy spelling", core.FieldFilter{Field: "status", Operator: core.OpEq, Value: "TODO"}, []string{"T-1", "T-6", "T-7"}},
		{"eq cancelled", core.FieldFilter{Field: "status", Operator: core.OpEq, Value: core.StatusCancelled}, []string{"T-4"}},
		{"in", core.FieldFilter{Field: "status", Operator: core.OpIn, Value: "done,backlog"}, []string{"T-3", "T-5"}},
		{"noteq todo", core.FieldFilter{Field: "status", Operator: core.OpNotEq, Value: "todo"}, []string{"T-2", "T-3", "T-4", "T-5", "T-8"}},
	}
	for _, c := range cases {
		got := queryIDs(t, s, core.Query{Filters: []core.FieldFilter{c.f}})
		if !sameSet(got, c.want...) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	counts, err := s.CountTasksByStatus(context.Background(), core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts["todo"] != 3 || counts[string(core.StatusCancelled)] != 1 || counts["TODO"] != 0 || counts["SKIPPED"] != 0 {
		t.Errorf("counts not normalised: %v", counts)
	}
	byProj, err := s.CountTasksByProjectAndStatus(context.Background(), core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("counts by project: %v", err)
	}
	if byProj[""]["todo"] != 3 || byProj[""]["done"] != 1 || byProj[""]["DONE"] != 0 {
		t.Errorf("project counts not normalised: %v", byProj)
	}
}

func TestQuery_OverdueExcludesLegacyTerminalRows(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)
	past := "2026-01-01T00:00:00Z"
	if _, err := s.db.ExecContext(context.Background(), `UPDATE tasks SET due_at = ?`, past); err != nil {
		t.Fatalf("set due: %v", err)
	}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	got := queryIDs(t, s, core.Query{Overdue: &now})
	if !sameSet(got, "T-1", "T-2", "T-5", "T-6", "T-7", "T-8") {
		t.Errorf("overdue: %v (DONE/SKIPPED rows must not count)", got)
	}
}

func TestArchive_LegacyTerminalRows(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)
	n, err := s.ArchiveTasks(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if n != 2 {
		t.Errorf("archived %d, want 2 (legacy DONE and SKIPPED)", n)
	}
}

func TestClaim_LegacyFromStatus(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)
	ok, err := s.ClaimTask(context.Background(), "T-1", "", "todo", "in_progress", "me", time.Now())
	if err != nil || !ok {
		t.Fatalf("claim legacy TODO row: ok=%v err=%v", ok, err)
	}
	if raw := rawStatus(t, s, "T-1"); raw != "in_progress" {
		t.Errorf("stored %q, want in_progress", raw)
	}
}

func TestWrite_RejectsUndeclaredStatus(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)
	ctx := context.Background()

	for _, bad := range []string{"TODO", "IN_PROGRESS", "SKIPPED", "Done", "shipped"} {
		task, err := s.GetTaskInProject(ctx, "T-7", "")
		if err != nil || task == nil {
			t.Fatalf("get: %v", err)
		}
		task.Status = core.TaskStatus(bad)
		if err := s.UpdateTask(ctx, task); err == nil || !strings.Contains(err.Error(), "invalid status") {
			t.Errorf("UpdateTask %q: err %v, want invalid status", bad, err)
		}
		entry := &core.LogEntry{TaskID: "T-7", By: "x", Action: "status_change", Timestamp: time.Now()}
		if err := s.UpdateTaskWithLog(ctx, task, entry); err == nil {
			t.Errorf("UpdateTaskWithLog %q: want error", bad)
		}
		fresh := &core.Task{ID: "N-" + bad, Title: "n", Status: core.TaskStatus(bad)}
		if err := s.CreateTask(ctx, fresh); err == nil {
			t.Errorf("CreateTask %q: want error", bad)
		}
		if ok, err := s.ClaimTask(ctx, "T-7", "", "todo", core.TaskStatus(bad), "me", time.Now()); err == nil || ok {
			t.Errorf("ClaimTask to %q: ok=%v err=%v, want error", bad, ok, err)
		}
	}
	if raw := rawStatus(t, s, "T-7"); raw != "todo" {
		t.Errorf("rejected writes changed the row: %q", raw)
	}
}

func TestWrite_DeclaredStatusAcceptedAndNormalisesRow(t *testing.T) {
	withDefaultVocab(t)
	s := openSeededLegacy(t)
	ctx := context.Background()
	task, err := s.GetTaskInProject(ctx, "T-1", "")
	if err != nil || task == nil {
		t.Fatalf("get: %v", err)
	}
	task.Title = "renamed"
	if err := s.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update: %v", err)
	}
	if raw := rawStatus(t, s, "T-1"); raw != "todo" {
		t.Errorf("row after write: %q, want todo (normalised on write)", raw)
	}
}

func TestWrite_CustomVocabularyDeclaredNamesAccepted(t *testing.T) {
	withLegacyVocab(t)
	s := openSeededLegacy(t)
	ctx := context.Background()
	task, err := s.GetTaskInProject(ctx, "T-1", "")
	if err != nil || task == nil {
		t.Fatalf("get: %v", err)
	}
	task.Status = "IN_PROGRESS"
	if err := s.UpdateTask(ctx, task); err != nil {
		t.Fatalf("declared custom name rejected: %v", err)
	}
	task.Status = "in_progress"
	if err := s.UpdateTask(ctx, task); err == nil {
		t.Error("undeclared lowercase name accepted under custom vocabulary")
	}
}
