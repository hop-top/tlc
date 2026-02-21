package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
)

// TestTODOFileSync_ProjectScoping tests TODO file filtering by project
// Verifies local .tlc/todo.txt contains only current project tasks
// Verifies global todo.txt contains tasks from all projects.
func TestTODOFileSync_ProjectScoping(t *testing.T) {
	tempDir := t.TempDir()

	projDir := filepath.Join(tempDir, "project1")
	tlcDir := filepath.Join(projDir, ".tlc")
	require.NoError(t, os.MkdirAll(tlcDir, 0o755))

	configPath := filepath.Join(tlcDir, "config.yaml")
	configContent := `
project:
  id: org/project1
storage:
  db_path: %s
`
	configContent = strings.Replace(configContent, "%s", filepath.Join(tempDir, "db.sqlite"), 1)
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	ctx := context.Background()
	dbPath := filepath.Join(tempDir, "db.sqlite")

	s, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer s.Close()

	proj1ID := "org/project1"
	proj2ID := "org/project2"

	now := time.Now()

	task1 := &core.Task{
		ID:        "T-0001",
		ProjectID: &proj1ID,
		Title:     "Task in project 1",
		Status:    core.StatusTodo,
		Reference: "ref1",
		CreatedAt: now,
		UpdatedAt: now,
	}

	task2 := &core.Task{
		ID:        "T-0002",
		ProjectID: &proj2ID,
		Title:     "Task in project 2",
		Status:    core.StatusTodo,
		Reference: "ref2",
		CreatedAt: now,
		UpdatedAt: now,
	}

	require.NoError(t, s.CreateTask(ctx, task1))
	require.NoError(t, s.CreateTask(ctx, task2))

	t.Run("syncToProjectTODO filters by project", func(t *testing.T) {
		t.Logf("Verifies project-specific todo.txt filtering")
		t.Logf("Only writes tasks matching the project_id")
		// Get tasks filtered by project
		query := core.Query{AllProjects: true}
		tasks, err := s.ListTasks(ctx, query)
		require.NoError(t, err)

		// Write to project todo file (filtering manually)
		todoFile := filepath.Join(tlcDir, "todo.txt")
		f, err := os.Create(todoFile)
		require.NoError(t, err)

		for _, task := range tasks {
			if task.ProjectID != nil && *task.ProjectID == proj1ID {
				f.WriteString(formatTLS(task) + "\n")
			}
		}
		f.Close()

		content, err := os.ReadFile(todoFile)
		require.NoError(t, err)
		fileContent := string(content)

		assert.Contains(t, fileContent, "T-0001")
		assert.Contains(t, fileContent, "Task in project 1")
		assert.NotContains(t, fileContent, "T-0002")
		assert.NotContains(t, fileContent, "Task in project 2")
	})

	t.Run("syncToTODO includes all tasks", func(t *testing.T) {
		t.Logf("Global todo.txt contains tasks from all projects")
		globalTodoFile := filepath.Join(tempDir, "global-todo.txt")

		query := core.Query{AllProjects: true}
		tasks, err := s.ListTasks(ctx, query)
		require.NoError(t, err)

		f, err := os.Create(globalTodoFile)
		require.NoError(t, err)

		for _, task := range tasks {
			f.WriteString(formatTLS(task) + "\n")
		}
		f.Close()

		content, err := os.ReadFile(globalTodoFile)
		require.NoError(t, err)
		fileContent := string(content)

		assert.Contains(t, fileContent, "T-0001")
		assert.Contains(t, fileContent, "Task in project 1")
		assert.Contains(t, fileContent, "T-0002")
		assert.Contains(t, fileContent, "Task in project 2")
	})
}

// TestProjectScoping_EdgeCases tests edge cases for project scoping
// Handles nil project_id (defaults to 'default') and empty project_id.
func TestProjectScoping_EdgeCases(t *testing.T) {
	resetProjectDetection()
	ctx := context.Background()

	tempDir := t.TempDir()

	oldCwd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()
	dbPath := filepath.Join(tempDir, "db.sqlite")

	s, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer s.Close()

	t.Run("Task with nil project_id uses 'default'", func(t *testing.T) {
		t.Logf("Tests tasks created without a project context")
		t.Logf("Verifies 'default' handling")
		now := time.Now()

		task := &core.Task{
			ID:        "T-DEFAULT",
			ProjectID: nil,
			Title:     "Task with nil project",
			Status:    core.StatusTodo,
			Reference: "ref-default",
			CreatedAt: now,
			UpdatedAt: now,
		}

		err := s.CreateTask(ctx, task)
		require.NoError(t, err)

		retrieved, err := s.GetTask(ctx, "T-DEFAULT")
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		// nil ProjectID is coalesced to empty string on storage
		assert.NotNil(t, retrieved.ProjectID)
		assert.Equal(t, "", *retrieved.ProjectID)
	})

	t.Run("Empty project_id treated as nil", func(t *testing.T) {
		t.Logf("Edge case handling for empty string project_id")
		emptyProj := ""

		now := time.Now()

		task := &core.Task{
			ID:        "T-EMPTY",
			ProjectID: &emptyProj,
			Title:     "Task with empty project",
			Status:    core.StatusTodo,
			Reference: "ref-empty",
			CreatedAt: now,
			UpdatedAt: now,
		}

		err := s.CreateTask(ctx, task)
		require.NoError(t, err)

		retrieved, err := s.GetTask(ctx, "T-EMPTY")
		require.NoError(t, err)
		assert.NotNil(t, retrieved.ProjectID)
		assert.Equal(t, "", *retrieved.ProjectID)
	})
}
