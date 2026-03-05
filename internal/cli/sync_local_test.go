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

func TestIngestTODOWith_AssignsCurrentProjectID(t *testing.T) {
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

	// Verify tasks were created with the current project ID
	ctx := context.Background()
	task1, err := s.GetTask(ctx, "T-0001")
	if err != nil {
		t.Fatalf("GetTask T-0001 error: %v", err)
	}
	if task1 == nil {
		t.Fatal("T-0001 not found in database")
	}
	if task1.ProjectID == nil || *task1.ProjectID != "my-project" {
		got := "<nil>"
		if task1.ProjectID != nil {
			got = *task1.ProjectID
		}
		t.Errorf("T-0001 project_id = %q, want %q", got, "my-project")
	}
}

func TestIngestTODOWith_NoConflictWithOtherProjects(t *testing.T) {
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

	// Pre-populate DB with a task under "project-a"
	ctx := context.Background()
	projectA := "project-a"
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

	// Write global todo.txt that includes the same task ID
	todoFile := filepath.Join(tmpDir, "global-todo.txt")
	os.WriteFile(todoFile, []byte(
		"[ ] T-0001 Same ID different project created_at=2026-01-01T00:00:00Z updated_at=2026-01-01T00:00:00Z\n",
	), 0o600)
	viper.Set("task.todo_file", todoFile)

	// This should NOT produce UNIQUE constraint errors.
	// It should create T-0001 under project-b (the current project).
	err = ingestTODOWith(s)
	if err != nil {
		t.Fatalf("ingestTODOWith returned error: %v", err)
	}

	// Verify: T-0001 should exist under both project-a and project-b
	allTasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	projectCounts := make(map[string]int)
	for _, task := range allTasks {
		if task.ID == "T-0001" {
			pid := ""
			if task.ProjectID != nil {
				pid = *task.ProjectID
			}
			projectCounts[pid]++
		}
	}

	if projectCounts["project-a"] != 1 {
		t.Errorf("expected 1 T-0001 under project-a, got %d", projectCounts["project-a"])
	}
	if projectCounts["project-b"] != 1 {
		t.Errorf("expected 1 T-0001 under project-b, got %d", projectCounts["project-b"])
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
	initCmd := newTestInitCmd()
	cmd.AddCommand(initCmd)

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
