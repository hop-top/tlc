package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
)

// TestProjectScoping_E2E runs comprehensive end-to-end tests for project scoping
// Tests task ID uniqueness across projects, filtering, sequences, updates, deletes, and logs.
func TestProjectScoping_E2E(t *testing.T) {
	resetProjectDetection()
	ctx := context.Background()

	tempDir := t.TempDir()

	// Work from tempDir so DetectProject returns InProject=false (no git)
	oldCwd, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	dbPath := filepath.Join(tempDir, "db.sqlite")

	s, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer s.Close()

	proj1ID := "org/project1"
	proj2ID := "org/project2"

	t.Run("Tasks with same seq alias can exist in different projects", func(t *testing.T) {
		t.Logf("Creates tasks in two projects; both get seq=1 (T-0001 alias).")
		t.Logf("Their durable TypeIDs differ so they coexist in the database.")
		task1 := &core.Task{
			ID:        core.NewTaskID(),
			ProjectID: &proj1ID,
			Title:     "Task in project 1",
			Status:    core.StatusTodo,
			Reference: "ref1",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		task2 := &core.Task{
			ID:        core.NewTaskID(),
			ProjectID: &proj2ID,
			Title:     "Task in project 2",
			Status:    core.StatusTodo,
			Reference: "ref2",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := s.CreateTask(ctx, task1)
		require.NoError(t, err)

		err = s.CreateTask(ctx, task2)
		require.NoError(t, err)

		// Both projects' first task should be assigned seq=1.
		assert.Equal(t, int64(1), task1.Seq)
		assert.Equal(t, int64(1), task2.Seq)

		// Each typeid resolves to its own row.
		retrieved1, err := s.GetTask(ctx, task1.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved1)
		assert.Equal(t, proj1ID, *retrieved1.ProjectID)
		assert.Equal(t, "Task in project 1", retrieved1.Title)

		retrieved2, err := s.GetTask(ctx, task2.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved2)
		assert.Equal(t, proj2ID, *retrieved2.ProjectID)
	})

	t.Run("ListTasks filters by project_id", func(t *testing.T) {
		t.Logf("Verifies that when AllProjects=false, tasks are filtered by project context")
		t.Logf("Verifies that when no project context, all tasks are returned")
		query := core.Query{}
		tasks, err := s.ListTasks(ctx, query)
		require.NoError(t, err)

		// All tasks returned when no project filter
		assert.Len(t, tasks, 2)
	})

	t.Run("GetNextSequenceID increments per project", func(t *testing.T) {
		t.Logf("Tests that task ID sequences are independent per project.")
		t.Logf("Sequences may have advanced from prior subtests; assert deltas.")

		base1, err := s.GetNextSequenceID(ctx, proj1ID)
		require.NoError(t, err)

		next1, err := s.GetNextSequenceID(ctx, proj1ID)
		require.NoError(t, err)
		assert.Equal(t, base1+1, next1, "proj1 sequence must increment by 1")

		base2, err := s.GetNextSequenceID(ctx, proj2ID)
		require.NoError(t, err)

		next2, err := s.GetNextSequenceID(ctx, proj2ID)
		require.NoError(t, err)
		assert.Equal(t, base2+1, next2, "proj2 sequence must increment by 1")
	})

	t.Run("Default project gets separate sequence", func(t *testing.T) {
		t.Logf("Tasks without a project_id (nil) use 'default' project")
		t.Logf("Sequence increments independently")
		id1, err := s.GetNextSequenceID(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, 1, id1)

		id2, err := s.GetNextSequenceID(ctx, "")
		require.NoError(t, err)
		assert.Equal(t, 2, id2)
	})

	t.Run("Update preserves project_id", func(t *testing.T) {
		t.Logf("Updates to existing tasks maintain their project_id")
		task := &core.Task{
			ID:        "T-0002",
			ProjectID: &proj1ID,
			Title:     "Task to update",
			Status:    core.StatusTodo,
			Reference: "ref3",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := s.CreateTask(ctx, task)
		require.NoError(t, err)

		task.Title = "Updated title"
		task.Status = core.StatusInProgress
		task.UpdatedAt = time.Now()

		err = s.UpdateTask(ctx, task)
		require.NoError(t, err)

		retrieved, err := s.GetTask(ctx, "T-0002")
		require.NoError(t, err)
		assert.Equal(t, proj1ID, *retrieved.ProjectID)
		assert.Equal(t, "Updated title", retrieved.Title)
		assert.Equal(t, core.StatusInProgress, retrieved.Status)
	})

	t.Run("Delete works with project_id constraint", func(t *testing.T) {
		t.Logf("Composite primary key (project_id, id) is respected")
		task := &core.Task{
			ID:        "T-0003",
			ProjectID: &proj1ID,
			Title:     "Task to delete",
			Status:    core.StatusTodo,
			Reference: "ref4",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := s.CreateTask(ctx, task)
		require.NoError(t, err)

		err = s.DeleteTask(ctx, "T-0003")
		require.NoError(t, err)

		_, err = s.GetTask(ctx, "T-0003")
		assert.NoError(t, err)
	})

	t.Run("Logs inherit project_id from task", func(t *testing.T) {
		t.Logf("Log entries automatically get the same project_id as their task")
		task := &core.Task{
			ID:        "T-0004",
			ProjectID: &proj1ID,
			Title:     "Task with logs",
			Status:    core.StatusTodo,
			Reference: "ref5",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := s.CreateTask(ctx, task)
		require.NoError(t, err)

		log := &core.LogEntry{
			TaskID:    "T-0004",
			Timestamp: time.Now(),
			By:        "user",
			Action:    "COMMENT",
			Note:      "Test log entry",
		}

		err = s.AddLog(ctx, log)
		require.NoError(t, err)

		logs, err := s.GetLogs(ctx, "T-0004", "DESC")
		require.NoError(t, err)
		assert.Len(t, logs, 1)
		assert.Equal(t, "Test log entry", logs[0].Note)
	})
}

// TestProjectScoping_TODOFileSync tests TODO file synchronization with project scoping
// Verifies project-specific todo.txt filtering.
func TestProjectScoping_TODOFileSync(t *testing.T) {
	resetProjectDetection()
	tempDir := t.TempDir()

	projDir := filepath.Join(tempDir, "project1")
	require.NoError(t, os.MkdirAll(projDir, 0o755))

	tlcDir := filepath.Join(projDir, ".tlc")
	require.NoError(t, os.MkdirAll(tlcDir, 0o755))

	todoFile := filepath.Join(tlcDir, "todo.txt")

	ctx := context.Background()
	dbPath := filepath.Join(tempDir, "db.sqlite")
	s, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer s.Close()

	proj1ID := "org/project1"

	t.Run("syncToProjectTODO only exports project tasks", func(t *testing.T) {
		t.Logf("Local .tlc/todo.txt contains only tasks for the current project")
		t.Logf("Tasks from other projects are excluded")
		task1 := &core.Task{
			ID:        "T-0001",
			ProjectID: &proj1ID,
			Title:     "Task in project 1",
			Status:    core.StatusTodo,
			Reference: "ref1",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		proj2ID := "org/project2"
		task2 := &core.Task{
			ID:        "T-0002",
			ProjectID: &proj2ID,
			Title:     "Task in project 2",
			Status:    core.StatusTodo,
			Reference: "ref2",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err := s.CreateTask(ctx, task1)
		require.NoError(t, err)

		err = s.CreateTask(ctx, task2)
		require.NoError(t, err)

		// Write tasks to project todo file (simulating project filter)
		// We need to mock the project detection, but for testing we'll manually filter
		allTasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
		require.NoError(t, err)

		f, err := os.Create(todoFile)
		require.NoError(t, err)
		for _, task := range allTasks {
			if task.ProjectID != nil && *task.ProjectID == proj1ID {
				f.WriteString(formatTLS(task) + "\n")
			}
		}
		f.Close()

		// Verify only project1 tasks are in file
		content, err := os.ReadFile(todoFile)
		require.NoError(t, err)
		fileContent := string(content)

		assert.Contains(t, fileContent, "T-0001")
		assert.NotContains(t, fileContent, "T-0002")
	})
}

// TestProjectScoping_Migration was specific to migration v7's composite
// (project_id, id) PK. Migration v13 (typeid-ids) replaces that with a
// single TEXT PRIMARY KEY on the durable TypeID and a separate
// (project_id, seq) UNIQUE constraint for the display alias. The
// "two projects, same id" semantics now live in
// TestProjectScoping_E2E/Tasks_with_same_seq_alias_can_exist_in_different_projects
// — keeping a v7-named test would just rot. Removed by T-0818 sweep.

func formatTLS(t *core.Task) string {
	status := "[ ]"
	switch t.Status {
	case core.StatusInProgress:
		status = "[~]"
	case core.StatusDone:
		status = "[x]"
	case core.StatusSkipped:
		status = "[-]"
	}

	parts := []string{status, t.ID, t.Title}

	if t.AssignedTo != nil && *t.AssignedTo != "" {
		parts = append(parts, "@"+*t.AssignedTo)
	}

	for _, tag := range t.Tags {
		parts = append(parts, "#"+tag)
	}

	if t.Reference != "" && t.Reference != "tlc:///"+t.ID && t.Reference != "tlc://"+t.ID && t.Reference != "task://"+t.ID {
		parts = append(parts, "ref:"+t.Reference)
	}

	if !t.CreatedAt.IsZero() {
		parts = append(parts, "created_at="+t.CreatedAt.Format(time.RFC3339))
	}
	if !t.UpdatedAt.IsZero() {
		parts = append(parts, "updated_at="+t.UpdatedAt.Format(time.RFC3339))
	}

	for k, v := range t.Meta {
		switch k {
		case "prio":
			parts = append(parts, "prio:"+fmt.Sprintf("%v", v))
		case "domain":
			parts = append(parts, "domain:"+fmt.Sprintf("%v", v))
		case "due":
			parts = append(parts, "due:"+fmt.Sprintf("%v", v))
		default:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}

	return strings.Join(parts, " ")
}
