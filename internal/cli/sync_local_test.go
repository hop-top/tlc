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
	ctx := context.Background()
	projectA := "project-a"
	projectB := "project-b"
	existingTask := &core.Task{
		ID:        "T-0001",
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

	projectTask := &core.Task{
		ID:        "T-0002",
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

	// Shared global TODO contains a task from another project and an update
	// for the current project's existing task.
	todoFile := filepath.Join(tmpDir, "global-todo.txt")
	os.WriteFile(todoFile, []byte(
		"[ ] T-0001 Same ID different project project_id=project-a created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z\n"+
			"[~] T-0002 Updated task title project_id=project-b created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z\n",
	), 0o600)
	viper.Set("task.todo_file", todoFile)

	err = ingestTODOWith(s)
	if err != nil {
		t.Fatalf("ingestTODOWith returned error: %v", err)
	}

	// Verify: T-0001 should still exist only under project-a and T-0002 should
	// be updated in project-b.
	allTasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	projectCounts := make(map[string]int)
	var updatedTask *core.Task
	for _, task := range allTasks {
		if task.ID == "T-0001" {
			pid := ""
			if task.ProjectID != nil {
				pid = *task.ProjectID
			}
			projectCounts[pid]++
		}
		if task.ID == "T-0002" && task.ProjectID != nil && *task.ProjectID == "project-b" {
			updatedTask = task
		}
	}

	if projectCounts["project-a"] != 1 {
		t.Errorf("expected 1 T-0001 under project-a, got %d", projectCounts["project-a"])
	}
	if projectCounts["project-b"] != 0 {
		t.Errorf("expected no T-0001 cloned into project-b, got %d", projectCounts["project-b"])
	}
	if updatedTask == nil {
		t.Fatal("expected T-0002 to remain in project-b")
	}
	if updatedTask.Title != "Updated task title" {
		t.Errorf("expected T-0002 title to be updated, got %q", updatedTask.Title)
	}
	if updatedTask.Status != core.StatusInProgress {
		t.Errorf("expected T-0002 status to be updated, got %q", updatedTask.Status)
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

func TestParseQuotedString(t *testing.T) {
	tests := []struct {
		input     string
		wantStr   string
		wantRest  string
		wantErr   bool
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
