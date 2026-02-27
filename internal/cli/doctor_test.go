package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

func setupDoctorTest(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tlc-doctor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)

	viper.Reset()

	cleanup := func() {
		os.Chdir(oldWd)
		os.RemoveAll(tmpDir)
	}
	return tmpDir, cleanup
}

// execDoctorDirect calls runDoctor directly, bypassing cobra initialization
// hooks that trigger initConfig/ingestTODO/DB opens.
func execDoctorDirect(t *testing.T, fix bool) (string, error) {
	t.Helper()
	cmd := &cobra.Command{Use: "doctor"}
	buf := new(strings.Builder)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := runDoctor(cmd, fix)
	return buf.String(), err
}

func TestDoctorCmd_AllPass(t *testing.T) {
	tmpDir, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	os.MkdirAll(".tlc", 0o755)
	config := "version: 0.1\nproject:\n  id: org/repo\n"
	os.WriteFile(".tlc/config.yaml", []byte(config), 0o644)

	dbPath := filepath.Join(tmpDir, "db.sqlite")
	viper.Set("storage.db_path", dbPath)
	viper.Set("storage.backend", "sqlite")
	viper.Set("project.id", "org/repo")
	viper.Set("git.track", false)

	output, err := execDoctorDirect(t, false)
	if err != nil {
		t.Logf("output: %s", output)
	}

	if !strings.Contains(output, "checks") {
		t.Errorf("expected summary line, got: %s", output)
	}
	if !strings.Contains(output, "passed") {
		t.Errorf("expected 'passed' in output, got: %s", output)
	}
}

func TestDoctorCmd_MissingTlcDir(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for missing .tlc/")
	}
	if !strings.Contains(output, ".tlc/ directory exists") {
		t.Errorf("expected .tlc dir check in output, got: %s", output)
	}
}

func TestDoctorCmd_MissingTlcDir_Fix(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, _ := execDoctorDirect(t, true)
	if !strings.Contains(output, "fixed") {
		t.Errorf("expected 'fixed' in output, got: %s", output)
	}
	if _, err := os.Stat(".tlc"); os.IsNotExist(err) {
		t.Error(".tlc/ was not created by --fix")
	}
}

func TestDoctorCmd_MissingConfig(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	os.MkdirAll(".tlc", 0o755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for missing config.yaml")
	}
	if !strings.Contains(output, ".tlc/config.yaml exists") {
		t.Errorf("expected config check in output, got: %s", output)
	}
}

func TestDoctorCmd_MissingConfig_Fix(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	os.MkdirAll(".tlc", 0o755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, _ := execDoctorDirect(t, true)
	if !strings.Contains(output, ".tlc/config.yaml exists") {
		t.Errorf("expected config check in output, got: %s", output)
	}
}

func TestDoctorCmd_InvalidYAML(t *testing.T) {
	tmpDir, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	os.MkdirAll(".tlc", 0o755)
	os.WriteFile(".tlc/config.yaml", []byte("{{{invalid yaml"), 0o644)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", filepath.Join(tmpDir, "db.sqlite"))

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
	if !strings.Contains(output, "config YAML is valid") {
		t.Errorf("expected YAML check in output, got: %s", output)
	}
}

func TestDoctorCmd_NoGit(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for no git repo")
	}
	if !strings.Contains(output, "inside git repository") {
		t.Errorf("expected git repo check in output, got: %s", output)
	}
}

func TestDoctorCmd_GitignoreMissing(t *testing.T) {
	tmpDir, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	os.MkdirAll(".tlc", 0o755)
	os.WriteFile(".tlc/config.yaml", []byte("version: 0.1\nproject:\n  id: test\n"), 0o644)
	viper.Set("git.track", true)
	viper.Set("project.id", "test")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", filepath.Join(tmpDir, "db.sqlite"))

	output, err := execDoctorDirect(t, false)
	if err == nil {
		t.Error("expected error for missing .gitignore entry")
	}
	if !strings.Contains(output, ".gitignore") {
		t.Errorf("expected gitignore check in output, got: %s", output)
	}
}

func TestDoctorCmd_GitignoreFix(t *testing.T) {
	tmpDir, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	os.MkdirAll(".tlc", 0o755)
	os.WriteFile(".tlc/config.yaml", []byte("version: 0.1\nproject:\n  id: test\n"), 0o644)
	os.WriteFile(".gitignore", []byte("*.log\n"), 0o644)
	viper.Set("git.track", true)
	viper.Set("project.id", "test")
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", filepath.Join(tmpDir, "db.sqlite"))

	output, _ := execDoctorDirect(t, true)
	if !strings.Contains(output, "fixed") {
		t.Logf("output: %s", output)
	}

	content, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if !strings.Contains(string(content), ".tlc/") {
		t.Error(".gitignore should contain .tlc/ after fix")
	}
}

func TestDoctorCmd_OutputFormat(t *testing.T) {
	_, cleanup := setupDoctorTest(t)
	defer cleanup()

	os.Mkdir(".git", 0o755)
	viper.Set("git.track", false)
	viper.Set("storage.backend", "sqlite")

	output, _ := execDoctorDirect(t, false)

	if !strings.Contains(output, "Git") {
		t.Errorf("expected Git category in output, got: %s", output)
	}
	if !strings.Contains(output, "Config") {
		t.Errorf("expected Config category in output, got: %s", output)
	}
	if !strings.Contains(output, "Storage") {
		t.Errorf("expected Storage category in output, got: %s", output)
	}
	if !strings.Contains(output, "checks") {
		t.Errorf("expected summary with 'checks' in output, got: %s", output)
	}
}

// setupDoctorProjectTest creates a temp dir with .git, .tlc/config.yaml, and a
// writable SQLite DB, suitable for testing project-scoped doctor checks.
func setupDoctorProjectTest(t *testing.T) (dbPath string, cleanup func()) {
	t.Helper()
	tmpDir, baseCleanup := setupDoctorTest(t)

	os.Mkdir(".git", 0o755)
	os.MkdirAll(".tlc", 0o755)
	os.WriteFile(".tlc/config.yaml", []byte("version: 0.1\nproject:\n  id: test/project\n"), 0o644)

	dbPath = filepath.Join(tmpDir, "db.sqlite")
	viper.Set("storage.db_path", dbPath)
	viper.Set("storage.backend", "sqlite")
	viper.Set("project.id", "test/project")
	viper.Set("git.track", false)

	// Point viper at the config file so DetectProject() finds it
	// instead of falling back to auto-detection from the directory name.
	configPath := filepath.Join(tmpDir, ".tlc", "config.yaml")
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")
	_ = viper.MergeInConfig()

	core.ResetDetectionCache()

	cleanup = func() {
		core.ResetDetectionCache()
		baseCleanup()
	}
	return dbPath, cleanup
}

func TestDoctorCmd_ProjectTodoSynced_NoFile(t *testing.T) {
	_, cleanup := setupDoctorProjectTest(t)
	defer cleanup()

	// No .tlc/todo.txt — should pass.
	r := checkProjectTodoSynced(false)
	if r.status != "pass" {
		t.Errorf("expected pass when no todo.txt, got %s: %s", r.status, r.message)
	}
}

func TestDoctorCmd_ProjectTodoSynced_EmptyFile(t *testing.T) {
	_, cleanup := setupDoctorProjectTest(t)
	defer cleanup()

	os.WriteFile(".tlc/todo.txt", []byte("  \n\n"), 0o644)

	r := checkProjectTodoSynced(false)
	if r.status != "pass" {
		t.Errorf("expected pass for empty todo.txt, got %s: %s", r.status, r.message)
	}
}

func TestDoctorCmd_ProjectTodoSynced_InSync(t *testing.T) {
	dbPath, cleanup := setupDoctorProjectTest(t)
	defer cleanup()

	// Create a task in the database.
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	projectID := "test/project"
	err = s.CreateTask(context.Background(), &core.Task{
		ID:        "T-0001",
		Title:     "Existing task",
		Status:    core.StatusTodo,
		ProjectID: &projectID,
		Meta:      map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	_ = s.Close()

	// Write matching content to todo.txt.
	os.WriteFile(".tlc/todo.txt", []byte("[ ] T-0001 Existing task\n"), 0o644)

	r := checkProjectTodoSynced(false)
	if r.status != "pass" {
		t.Errorf("expected pass for in-sync todo.txt, got %s: %s", r.status, r.message)
	}
}

func TestDoctorCmd_ProjectTodoSynced_OutOfSync(t *testing.T) {
	dbPath, cleanup := setupDoctorProjectTest(t)
	defer cleanup()

	// Create a task in the database with different status.
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	projectID := "test/project"
	err = s.CreateTask(context.Background(), &core.Task{
		ID:        "T-0001",
		Title:     "Existing task",
		Status:    core.StatusTodo,
		ProjectID: &projectID,
		Meta:      map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	_ = s.Close()

	// Write todo.txt with the task marked DONE — out of sync.
	os.WriteFile(".tlc/todo.txt", []byte("[x] T-0001 Existing task\n"), 0o644)

	r := checkProjectTodoSynced(false)
	if r.status != "warn" {
		t.Errorf("expected warn for out-of-sync todo.txt, got %s: %s", r.status, r.message)
	}
	if !strings.Contains(r.message, "out of sync") {
		t.Errorf("expected 'out of sync' in message, got: %s", r.message)
	}
}

func TestDoctorCmd_ProjectTodoSynced_FixCreatesNewTask(t *testing.T) {
	dbPath, cleanup := setupDoctorProjectTest(t)
	defer cleanup()

	// Write todo.txt with a task that doesn't exist in the database.
	os.WriteFile(".tlc/todo.txt", []byte("[>] T-0001 New task from file @dev #urgent\n"), 0o644)

	r := checkProjectTodoSynced(true)
	if r.status != "pass" || !r.fixed {
		t.Errorf("expected pass+fixed, got status=%s fixed=%v msg=%s", r.status, r.fixed, r.fixMsg)
	}
	if !strings.Contains(r.fixMsg, "1 created") {
		t.Errorf("expected '1 created' in fixMsg, got: %s", r.fixMsg)
	}

	// Verify the task exists in the database with correct project ID.
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = s.Close() }()

	task, err := s.GetTask(context.Background(), "T-0001")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if task.Status != core.StatusInProgress {
		t.Errorf("expected IN_PROGRESS, got %s", task.Status)
	}
	if task.ProjectID == nil || *task.ProjectID != "test/project" {
		t.Errorf("expected project_id=test/project, got %v", task.ProjectID)
	}
}

func TestDoctorCmd_ProjectTodoSynced_FixUpdatesExisting(t *testing.T) {
	dbPath, cleanup := setupDoctorProjectTest(t)
	defer cleanup()

	// Create a task in the database.
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	projectID := "test/project"
	err = s.CreateTask(context.Background(), &core.Task{
		ID:        "T-0001",
		Title:     "Old title",
		Status:    core.StatusTodo,
		ProjectID: &projectID,
		Meta:      map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	_ = s.Close()

	// Write todo.txt with updated status and title.
	os.WriteFile(".tlc/todo.txt", []byte("[x] T-0001 Updated title\n"), 0o644)

	r := checkProjectTodoSynced(true)
	if r.status != "pass" || !r.fixed {
		t.Errorf("expected pass+fixed, got status=%s fixed=%v msg=%s", r.status, r.fixed, r.fixMsg)
	}
	if !strings.Contains(r.fixMsg, "1 updated") {
		t.Errorf("expected '1 updated' in fixMsg, got: %s", r.fixMsg)
	}

	// Verify the task was updated.
	s2, _ := storage.NewSQLiteStorage(dbPath)
	defer func() { _ = s2.Close() }()

	task, _ := s2.GetTask(context.Background(), "T-0001")
	if task.Status != core.StatusDone {
		t.Errorf("expected DONE, got %s", task.Status)
	}
	if task.Title != "Updated title" {
		t.Errorf("expected 'Updated title', got '%s'", task.Title)
	}
}
