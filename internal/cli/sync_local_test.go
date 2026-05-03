package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

func TestIngestTODOWith_ProjectContextSkipsUnknownGlobalTasks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-ingest-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, _ = filepath.EvalSymlinks(tmpDir)
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Create project context
	os.Mkdir(".git", 0o750)
	os.MkdirAll(".tlc", 0o750)
	os.WriteFile(".tlc/config.yaml", []byte("version: 0.1\nproject:\n  id: my-project\n"), 0o600)

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)

	// Load the project config so DetectProject finds it
	viper.SetConfigFile(filepath.Join(tmpDir, ".tlc", "config.yaml"))
	viper.SetConfigType("yaml")
	viper.MergeInConfig()

	// Write a global todo.txt with tasks
	todoFile := filepath.Join(tmpDir, "global-todo.txt")
	os.WriteFile(todoFile, []byte(
		"[ ] T-0001 First task created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z\n"+
			"[x] T-0002 Second task created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z\n",
	), 0o600)
	viper.Set("task.todo_file", todoFile)

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer s.Close()

	if err := ingestTODOWith(s); err != nil {
		t.Fatalf("ingestTODOWith failed: %v", err)
	}

	ctx := context.Background()
	task1, err := s.GetTask(ctx, "T-0001")
	if err != nil {
		t.Fatalf("GetTask T-0001 error: %v", err)
	}
	if task1 != nil {
		t.Fatalf("T-0001 should not be created from shared global TODO in project context")
	}
}

func TestIngestTODOWith_ProjectContextUpdatesExistingTaskOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-ingest-conflict-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, _ = filepath.EvalSymlinks(tmpDir)
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	// Create project context for "project-b"
	os.Mkdir(".git", 0o750)
	os.MkdirAll(".tlc", 0o750)
	os.WriteFile(".tlc/config.yaml", []byte("version: 0.1\nproject:\n  id: project-b\n"), 0o600)

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)

	// Load the project config so DetectProject finds it
	viper.SetConfigFile(filepath.Join(tmpDir, ".tlc", "config.yaml"))
	viper.SetConfigType("yaml")
	viper.MergeInConfig()

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Pre-populate DB with one task under project-a and one under project-b.
	// Tasks are keyed by typeid post-T-0812; the T-NNNN form on TLS lines
	// is a per-project seq alias resolved at ingest time.
	ctx := context.Background()
	projectA := "project-a"
	projectB := "project-b"
	existingTaskID := core.NewTaskID()
	existingTask := &core.Task{
		ID:        existingTaskID,
		Title:     "Existing task in project-a",
		Status:    core.StatusTodo,
		ProjectID: &projectA,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Meta:      map[string]interface{}{},
	}
	if err := s.CreateTask(ctx, existingTask); err != nil {
		t.Fatalf("failed to seed task: %v", err)
	}

	projectTaskID := core.NewTaskID()
	projectTask := &core.Task{
		ID:        projectTaskID,
		Title:     "Existing task in project-b",
		Status:    core.StatusTodo,
		ProjectID: &projectB,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Meta:      map[string]interface{}{},
	}
	if err := s.CreateTask(ctx, projectTask); err != nil {
		t.Fatalf("failed to seed project task: %v", err)
	}

	// Each project's seq counter starts at 1, so both seeded tasks have
	// seq=1. Use that to compose the per-project alias on the TLS lines.
	seededInA, _ := s.GetTaskInProject(ctx, existingTaskID, projectA)
	seededInB, _ := s.GetTaskInProject(ctx, projectTaskID, projectB)
	aliasA := core.FormatTaskAlias(seededInA)
	aliasB := core.FormatTaskAlias(seededInB)

	// Shared global TODO contains a task from another project and an update
	// for the current project's existing task. Both lines use the per-project
	// seq alias as their leading id token (matches what formatTLS emits).
	todoFile := filepath.Join(tmpDir, "global-todo.txt")
	os.WriteFile(todoFile, []byte(
		"[ ] "+aliasA+" Same alias different project project_id=project-a created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z\n"+
			"[~] "+aliasB+" Updated task title project_id=project-b created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z\n",
	), 0o600)
	viper.Set("task.todo_file", todoFile)

	err = ingestTODOWith(s)
	if err != nil {
		t.Fatalf("ingestTODOWith returned error: %v", err)
	}

	// project-a's task must remain unchanged (foreign-project line skipped).
	keptA, _ := s.GetTaskInProject(ctx, existingTaskID, projectA)
	if keptA == nil {
		t.Fatal("project-a task was unexpectedly removed")
	}
	if keptA.Title != "Existing task in project-a" {
		t.Errorf("project-a task title mutated: %q", keptA.Title)
	}

	// project-b's task should have been updated by the matching line.
	updatedTask, _ := s.GetTaskInProject(ctx, projectTaskID, projectB)
	if updatedTask == nil {
		t.Fatal("expected project-b task to remain in DB")
	}
	if updatedTask.Title != "Updated task title" {
		t.Errorf("expected project-b task title to be updated, got %q", updatedTask.Title)
	}
	if updatedTask.Status != core.StatusInProgress {
		t.Errorf("expected project-b task status to be updated, got %q", updatedTask.Status)
	}

	// And no stray T-NNNN-keyed mirror was minted.
	all, _ := s.ListTasks(ctx, core.Query{AllProjects: true})
	for _, row := range all {
		if strings.HasPrefix(row.ID, "T-") {
			t.Errorf("found T-NNNN-keyed mirror after ingest: id=%s title=%q", row.ID, row.Title)
		}
	}
}

func TestIngestTODOWith_ProjectContextIgnoresForeignSameID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-ingest-sameid-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, _ = filepath.EvalSymlinks(tmpDir)
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	os.Mkdir(".git", 0o750)
	os.MkdirAll(".tlc", 0o750)
	os.WriteFile(".tlc/config.yaml", []byte("version: 0.1\nproject:\n  id: project-b\n"), 0o600)

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)
	viper.SetConfigFile(filepath.Join(tmpDir, ".tlc", "config.yaml"))
	viper.SetConfigType("yaml")
	viper.MergeInConfig()

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ctx := context.Background()
	projectB := "project-b"
	task := &core.Task{
		ID:        "T-0001",
		Title:     "APS task to preserve",
		Status:    core.StatusTodo,
		ProjectID: &projectB,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Meta:      map[string]interface{}{},
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("failed to seed project task: %v", err)
	}

	todoFile := filepath.Join(tmpDir, "global-todo.txt")
	os.WriteFile(todoFile, []byte(
		"[~] T-0001 TLC task with same ID project_id=project-a created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z\n",
	), 0o600)
	viper.Set("task.todo_file", todoFile)

	if err := ingestTODOWith(s); err != nil {
		t.Fatalf("ingestTODOWith returned error: %v", err)
	}

	retrieved, err := s.GetTaskInProject(ctx, "T-0001", "project-b")
	if err != nil {
		t.Fatalf("GetTaskInProject failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected project-b task to remain")
	}
	if retrieved.Title != "APS task to preserve" {
		t.Fatalf("expected foreign same-ID line to be ignored, got %q", retrieved.Title)
	}
	if retrieved.Status != core.StatusTodo {
		t.Fatalf("expected status TODO to remain, got %q", retrieved.Status)
	}
}

func TestParseTLS_QuotedTitlePreservesSpecialChars(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantTitle string
		wantMeta  map[string]interface{}
	}{
		{
			name:      "quoted title with equals sign",
			line:      `[ ] T-0001 "Set HOP_ENTRY=1 for delegated calls"`,
			wantTitle: "Set HOP_ENTRY=1 for delegated calls",
			wantMeta:  map[string]interface{}{},
		},
		{
			name:      "quoted title with at-sign",
			line:      `[ ] T-0002 "Deploy @staging environment"`,
			wantTitle: "Deploy @staging environment",
			wantMeta:  map[string]interface{}{},
		},
		{
			name:      "quoted title with hash",
			line:      `[ ] T-0003 "Fix issue #42 in parser"`,
			wantTitle: "Fix issue #42 in parser",
			wantMeta:  map[string]interface{}{},
		},
		{
			name:      "quoted title with meta after",
			line:      `[ ] T-0004 "Set X=1" @alice #feat`,
			wantTitle: "Set X=1",
			wantMeta:  map[string]interface{}{},
		},
		{
			name:      "unquoted title without special chars",
			line:      `[ ] T-0005 Simple task title`,
			wantTitle: "Simple task title",
			wantMeta:  map[string]interface{}{},
		},
		{
			name:      "unquoted title with kv (legacy compat)",
			line:      `[ ] T-0006 Simple title FOO=bar`,
			wantTitle: "Simple title",
			wantMeta:  map[string]interface{}{"FOO": "bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := parseTLS(tt.line)
			if err != nil {
				t.Fatalf("parseTLS failed: %v", err)
			}
			if task.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", task.Title, tt.wantTitle)
			}
			for k, v := range tt.wantMeta {
				got, ok := task.Meta[k]
				if !ok {
					t.Errorf("missing meta key %q", k)
				} else if got != v {
					t.Errorf("meta[%q] = %v, want %v", k, got, v)
				}
			}
		})
	}
}

func TestFormatTLS_QuotesTitleForRoundtrip(t *testing.T) {
	task := &core.Task{
		ID:     "T-0001",
		Title:  "Set HOP_ENTRY=1 for delegated calls",
		Status: core.StatusTodo,
		Meta:   map[string]interface{}{},
	}

	line := formatTLS(task)
	if !strings.Contains(line, `"Set HOP_ENTRY=1 for delegated calls"`) {
		t.Fatalf("expected quoted title in TLS, got: %s", line)
	}

	// Roundtrip: parse the formatted line back.
	parsed, err := parseTLS(line)
	if err != nil {
		t.Fatalf("parseTLS roundtrip failed: %v", err)
	}
	if parsed.Title != task.Title {
		t.Fatalf("roundtrip title = %q, want %q", parsed.Title, task.Title)
	}
	if _, hasKey := parsed.Meta["HOP_ENTRY"]; hasKey {
		t.Fatal("HOP_ENTRY should not be extracted into meta on roundtrip")
	}
}

func TestParseTLS_BlockedByList(t *testing.T) {
	task, err := parseTLS(`[ ] T-0001 "Blocked task" blocked_by=T-0002,T-0003`)
	if err != nil {
		t.Fatalf("parseTLS failed: %v", err)
	}

	blockedBy := task.BlockedBy()
	if len(blockedBy) != 2 {
		t.Fatalf("expected 2 blockers, got %d (%v)", len(blockedBy), blockedBy)
	}
	if blockedBy[0] != "T-0002" || blockedBy[1] != "T-0003" {
		t.Fatalf("blocked_by = %v, want [T-0002 T-0003]", blockedBy)
	}
}

func TestFormatTLS_BlockedByRoundtrip(t *testing.T) {
	task := &core.Task{
		ID:     "T-0001",
		Title:  "Blocked task",
		Status: core.StatusTodo,
		Meta: map[string]interface{}{
			"blocked_by": []string{"T-0002", "T-0003"},
		},
	}

	line := formatTLS(task)
	if !strings.Contains(line, "blocked_by=T-0002,T-0003") {
		t.Fatalf("expected blocked_by token in TLS, got: %s", line)
	}

	parsed, err := parseTLS(line)
	if err != nil {
		t.Fatalf("parseTLS roundtrip failed: %v", err)
	}
	blockedBy := parsed.BlockedBy()
	if len(blockedBy) != 2 || blockedBy[0] != "T-0002" || blockedBy[1] != "T-0003" {
		t.Fatalf("roundtrip blocked_by = %v, want [T-0002 T-0003]", blockedBy)
	}
}

func TestParseQuotedString(t *testing.T) {
	tests := []struct {
		input    string
		wantStr  string
		wantRest string
		wantErr  bool
	}{
		{`"hello world" rest`, "hello world", "rest", false},
		{`"escaped \"quote\"" rest`, `escaped "quote"`, "rest", false},
		{`"newline\\n" rest`, `newline\n`, "rest", false},
		{`"unterminated`, "", "", true},
		{`not quoted`, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, rest, err := parseQuotedString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				if got != tt.wantStr {
					t.Errorf("string = %q, want %q", got, tt.wantStr)
				}
				if rest != tt.wantRest {
					t.Errorf("rest = %q, want %q", rest, tt.wantRest)
				}
			}
		})
	}
}

func TestTrimMatchingQuotes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"foo"`, "foo"},
		{`'bar'`, "bar"},
		{`"mismatched'`, `"mismatched'`},
		{`foo`, "foo"},
		{`""`, `""`},
		{`''`, `''`},
		{`"hello world"`, "hello world"},
		{`'it works'`, "it works"},
		{`"only open`, `"only open`},
		{`only close"`, `only close"`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := trimMatchingQuotes(tt.input)
			if got != tt.want {
				t.Errorf("trimMatchingQuotes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInitDoesNotTriggerGlobalIngestion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tlc-init-noingest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDir, _ = filepath.EvalSymlinks(tmpDir)
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	os.Mkdir(".git", 0o750)

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)

	// Write a global todo.txt with tasks from another project
	todoFile := filepath.Join(tmpDir, "global-todo.txt")
	os.WriteFile(todoFile, []byte(
		"[ ] T-0001 Task from other project\n"+
			"[ ] T-0002 Another task from other project\n",
	), 0o600)
	viper.Set("task.todo_file", todoFile)

	// Pre-populate DB with these tasks under "other-project"
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	otherProject := "other-project"
	ctx := context.Background()
	for _, id := range []string{"T-0001", "T-0002"} {
		s.CreateTask(ctx, &core.Task{
			ID:        id,
			Title:     "Task from other project",
			Status:    core.StatusTodo,
			ProjectID: &otherProject,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Meta:      map[string]interface{}{},
		})
	}
	s.Close()

	// Run init — should NOT produce UNIQUE constraint errors
	cmd := newTestCmd()
	cmd.AddCommand(newTestInitCmd())

	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"init", "--no-track"})

	err = cmd.Execute()
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// The init command's getStorage() triggers global todo ingestion which
	// would print "Warning: failed to create task" to stdout for each
	// task that conflicts. Since fmt.Printf goes to os.Stdout (not cmd output),
	// we verify by checking the DB doesn't have tasks under empty project_id.
	s2, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	allTasks, _ := s2.ListTasks(ctx, core.Query{AllProjects: true})
	for _, task := range allTasks {
		pid := ""
		if task.ProjectID != nil {
			pid = *task.ProjectID
		}
		if pid == "" {
			t.Errorf("task %s has empty project_id — init should not create tasks with empty project_id", task.ID)
		}
	}
}
