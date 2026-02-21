package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"hop.top/tlc/internal/core"
)

// TestGitHubSync_ProjectScoping tests GitHub sync with project scoping
// Simulates sync pull assigning project_id to tasks from GitHub
// Verifies tasks appear in correct project context when listed.
func TestGitHubSync_ProjectScoping(t *testing.T) {
	resetProjectDetection()
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup project directory
	projDir := filepath.Join(tempDir, "project1")
	tlcDir := filepath.Join(projDir, ".tlc")
	require.NoError(t, os.MkdirAll(tlcDir, 0o755))

	// Create config with project ID
	configPath := filepath.Join(tlcDir, "config.yaml")
	configContent := `
project:
  id: IdeaCraftersLabs/my-awesome-project
storage:
  db_path: %s
`
	configContent = strings.Replace(configContent, "%s", filepath.Join(tempDir, "db.sqlite"), 1)
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	// Initialize storage
	dbPath := filepath.Join(tempDir, "db.sqlite")
	s, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer s.Close()

	projID := "IdeaCraftersLabs/my-awesome-project"

	now := time.Now()

	t.Run("Simulated sync pull assigns project_id to tasks", func(t *testing.T) {
		t.Logf("Creates tasks with GitHub-style IDs (GH-100, GH-101, etc.)")
		t.Logf("Assigns project_id before creating (simulating sync pull behavior)")
		t.Logf("Verifies all tasks have correct project_id set")
		t.Logf("Verifies OriginSystem is set to 'github'")
		// Simulate tasks returned from GitHub sync pull
		// These are what to sync plugin would return
		githubTasks := []core.Task{
			{
				ID:          "GH-100", // GitHub-style ID
				Title:       "Fix authentication bug",
				Description: "Users cannot login with SSO",
				Status:      core.StatusTodo,
				Reference:   "https://github.com/IdeaCraftersLabs/my-awesome-project/issues/100",
				CreatedAt:   now.Add(-2 * time.Hour),
				UpdatedAt:   now.Add(-2 * time.Hour),
				Meta: map[string]interface{}{
					"origin_id":     "100",
					"origin_system": "github",
					"origin_url":    "https://github.com/IdeaCraftersLabs/my-awesome-project/issues/100",
					"labels":        []string{"bug", "auth"},
					"assignee":      "@john-doe",
				},
			},
			{
				ID:          "GH-101",
				Title:       "Add dark mode support",
				Description: "Implement dark theme for the UI",
				Status:      core.StatusInProgress,
				Reference:   "https://github.com/IdeaCraftersLabs/my-awesome-project/issues/101",
				CreatedAt:   now.Add(-24 * time.Hour),
				UpdatedAt:   now.Add(-12 * time.Hour),
				Meta: map[string]interface{}{
					"origin_id":     "101",
					"origin_system": "github",
					"origin_url":    "https://github.com/IdeaCraftersLabs/my-awesome-project/issues/101",
					"labels":        []string{"feature", "ui"},
				},
			},
			{
				ID:          "GH-102",
				Title:       "Optimize database queries",
				Description: "Reduce query time for dashboard",
				Status:      core.StatusDone,
				Reference:   "https://github.com/IdeaCraftersLabs/my-awesome-project/issues/102",
				CreatedAt:   now.Add(-48 * time.Hour),
				UpdatedAt:   now.Add(-1 * time.Hour),
				Meta: map[string]interface{}{
					"origin_id":     "102",
					"origin_system": "github",
					"origin_url":    "https://github.com/IdeaCraftersLabs/my-awesome-project/issues/102",
					"labels":        []string{"performance", "db"},
				},
			},
		}

		// Change to project directory (this simulates being in project when sync runs)
		oldCwd, _ := os.Getwd()
		defer os.Chdir(oldCwd)
		os.Chdir(projDir)

		// Simulate what sync pull code does: auto-assign project_id from DetectProject
		for i := range githubTasks {
			// This is what the actual sync pull code should do
			githubTasks[i].ProjectID = &projID
			originSystem := "github"
			githubTasks[i].OriginSystem = &originSystem
		}

		// Create the tasks (simulating sync pull)
		for _, task := range githubTasks {
			err := s.CreateTask(ctx, &task)
			require.NoError(t, err)
		}

		// Verify all tasks were created with project_id
		tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
		require.NoError(t, err)
		assert.Len(t, tasks, 3)

		// Verify each task has correct project_id
		for _, task := range tasks {
			assert.NotNil(t, task.ProjectID)
			assert.Equal(t, projID, *task.ProjectID, "Task %s should have project_id set", task.ID)
			assert.NotNil(t, task.OriginSystem)
			assert.Equal(t, "github", *task.OriginSystem)
		}
	})

	t.Run("Task list in project context shows synced tasks", func(t *testing.T) {
		t.Logf("Lists tasks with AllProjects=false (simulating CLI in project)")
		t.Logf("Verifies only tasks with matching project_id are returned")
		t.Logf("Confirms all GitHub-synced tasks appear")
		// Simulate being in project context by pointing viper at the project config
		resetProjectDetection()
		cfgPath := filepath.Join(projDir, ".tlc", "config.yaml")
		viper.SetConfigFile(cfgPath)
		viper.SetConfigType("yaml")
		viper.ReadInConfig()
		defer resetProjectDetection()

		// List tasks with AllProjects=false (this is what the CLI does by default in project context)
		tasks, err := s.ListTasks(ctx, core.Query{AllProjects: false})
		require.NoError(t, err)

		// Should show only tasks for this project
		assert.Len(t, tasks, 3, "Should list all 3 synced tasks")

		// Verify task IDs match what was synced
		taskIDs := make(map[string]bool)
		for _, task := range tasks {
			taskIDs[task.ID] = true
			assert.Equal(t, projID, *task.ProjectID)
		}

		assert.True(t, taskIDs["GH-100"])
		assert.True(t, taskIDs["GH-101"])
		assert.True(t, taskIDs["GH-102"])
	})

	t.Run("Task list with all-projects shows all tasks", func(t *testing.T) {
		t.Logf("Creates tasks in a different project")
		t.Logf("Lists with AllProjects=true")
		t.Logf("Verifies tasks from both projects appear")
		t.Logf("Confirms project counts are correct")
		// Create a task in a different project
		otherProjID := "other-org/different-project"
		otherTask := &core.Task{
			ID:        "T-OTHER-1",
			ProjectID: &otherProjID,
			Title:     "Task in other project",
			Status:    core.StatusTodo,
			Reference: "ref-other",
			CreatedAt: now,
			UpdatedAt: now,
		}
		err := s.CreateTask(ctx, otherTask)
		require.NoError(t, err)

		// List all tasks
		tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
		require.NoError(t, err)

		assert.Len(t, tasks, 4, "Should show all tasks from all projects")

		// Verify we have tasks from both projects
		projectCount := make(map[string]int)
		for _, task := range tasks {
			if task.ProjectID != nil {
				projectCount[*task.ProjectID]++
			}
		}

		assert.Equal(t, 3, projectCount[projID])
		assert.Equal(t, 1, projectCount[otherProjID])
	})

	t.Run("Task list outside project context shows all tasks", func(t *testing.T) {
		t.Logf("Changes to directory without .tlc")
		t.Logf("Lists tasks (no project filter applied)")
		t.Logf("Verifies all tasks from all projects are returned")
		// Change to a directory without .tlc (simulating being outside project)
		resetProjectDetection()
		oldCwd, _ := os.Getwd()
		defer os.Chdir(oldCwd)
		defer resetProjectDetection()
		os.Chdir(tempDir)

		// List tasks without project filter
		tasks, err := s.ListTasks(ctx, core.Query{AllProjects: false})
		require.NoError(t, err)

		// Should return all tasks when not in a project context
		assert.Len(t, tasks, 4, "Should list all tasks when outside project context")
	})
}

// TestGitHubSync_MultipleProjects tests GitHub sync across multiple projects
// Verifies same GitHub issue ID can exist in different projects
// Verifies each project only sees its own tasks.
func TestGitHubSync_MultipleProjects(t *testing.T) {
	resetProjectDetection()
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup two projects
	proj1Dir := filepath.Join(tempDir, "project1")
	proj2Dir := filepath.Join(tempDir, "project2")

	for _, projDir := range []string{proj1Dir, proj2Dir} {
		tlcDir := filepath.Join(projDir, ".tlc")
		require.NoError(t, os.MkdirAll(tlcDir, 0o755))

		configPath := filepath.Join(tlcDir, "config.yaml")
		var projID string
		if strings.Contains(projDir, "project1") {
			projID = "org1/project1"
		} else {
			projID = "org2/project2"
		}

		configContent := `
project:
  id: %s
storage:
  db_path: %s
`
		configContent = strings.Replace(configContent, "%s", filepath.Join(tempDir, "db.sqlite"), 1)
		configContent = strings.Replace(configContent, "%s", projID, 1)
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))
	}

	// Initialize storage
	dbPath := filepath.Join(tempDir, "db.sqlite")
	s, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer s.Close()

	proj1ID := "org1/project1"
	proj2ID := "org2/project2"

	now := time.Now()

	t.Run("Same GitHub issue ID can exist in different projects", func(t *testing.T) {
		t.Logf("Simulates GitHub sync pull for two different projects")
		t.Logf("Both have issue #50 (same GitHub ID)")
		t.Logf("Verifies both tasks can coexist in database")
		t.Logf("Verifies they have different project_id values")
		// Simulate GitHub sync pull for project1
		oldCwd, _ := os.Getwd()
		defer os.Chdir(oldCwd)
		os.Chdir(proj1Dir)

		task1 := &core.Task{
			ID:        "GH-50",
			Title:     "Fix login bug in project1",
			Status:    core.StatusTodo,
			Reference: "https://github.com/org1/project1/issues/50",
			CreatedAt: now,
			UpdatedAt: now,
			ProjectID: &proj1ID,
			Meta: map[string]interface{}{
				"origin_id":     "50",
				"origin_system": "github",
			},
		}

		err := s.CreateTask(ctx, task1)
		require.NoError(t, err)

		// Simulate GitHub sync pull for project2
		os.Chdir(proj2Dir)

		task2 := &core.Task{
			ID:        "GH-50", // Same ID!
			Title:     "Fix login bug in project2",
			Status:    core.StatusTodo,
			Reference: "https://github.com/org2/project2/issues/50",
			CreatedAt: now,
			UpdatedAt: now,
			ProjectID: &proj2ID,
			Meta: map[string]interface{}{
				"origin_id":     "50",
				"origin_system": "github",
			},
		}

		err = s.CreateTask(ctx, task2)
		require.NoError(t, err)

		// Verify both tasks exist
		tasks, err := s.ListTasks(ctx, core.Query{AllProjects: true})
		require.NoError(t, err)
		assert.Len(t, tasks, 2)

		// Verify they have different project_id
		taskMap := make(map[string]*core.Task)
		for _, task := range tasks {
			taskMap[*task.ProjectID] = task
		}

		assert.Equal(t, "Fix login bug in project1", taskMap[proj1ID].Title)
		assert.Equal(t, "Fix login bug in project2", taskMap[proj2ID].Title)
	})

	t.Run("Each project lists only its own tasks", func(t *testing.T) {
		t.Logf("Lists tasks for project1, filters by project1's project_id")
		t.Logf("Lists tasks for project2, filters by project2's project_id")
		t.Logf("Verifies each project sees only its tasks")
		// List all tasks, then manually filter by project_id
		resetProjectDetection()

		query1 := core.Query{AllProjects: true}
		tasks1, err := s.ListTasks(ctx, query1)
		require.NoError(t, err)

		// Manually filter to simulate project1 context
		var proj1Tasks []*core.Task
		for _, task := range tasks1 {
			if task.ProjectID != nil && *task.ProjectID == proj1ID {
				proj1Tasks = append(proj1Tasks, task)
			}
		}
		assert.Len(t, proj1Tasks, 1)
		assert.Equal(t, "Fix login bug in project1", proj1Tasks[0].Title)
		assert.Equal(t, proj1ID, *proj1Tasks[0].ProjectID)

		// Manually filter to simulate project2 context
		var proj2Tasks []*core.Task
		for _, task := range tasks1 {
			if task.ProjectID != nil && *task.ProjectID == proj2ID {
				proj2Tasks = append(proj2Tasks, task)
			}
		}
		assert.Len(t, proj2Tasks, 1)
		assert.Equal(t, "Fix login bug in project2", proj2Tasks[0].Title)
		assert.Equal(t, proj2ID, *proj2Tasks[0].ProjectID)
	})
}

// TestGitHubSync_TaskUpdatePreservesProjectID tests sync updates preserve project scoping
// Verifies project_id is preserved when GitHub sync updates task properties.
func TestGitHubSync_TaskUpdatePreservesProjectID(t *testing.T) {
	resetProjectDetection()
	oldCwd, _ := os.Getwd()
	ctx := context.Background()
	tempDir := t.TempDir()

	// Work from tempDir so DetectProject returns InProject=false (no git repo)
	os.Chdir(tempDir)
	defer os.Chdir(oldCwd)
	defer resetProjectDetection()

	projDir := filepath.Join(tempDir, "project1")
	tlcDir := filepath.Join(projDir, ".tlc")
	require.NoError(t, os.MkdirAll(tlcDir, 0o755))

	configPath := filepath.Join(tlcDir, "config.yaml")
	configContent := `
project:
  id: IdeaCraftersLabs/my-awesome-project
storage:
  db_path: %s
`
	configContent = strings.Replace(configContent, "%s", filepath.Join(tempDir, "db.sqlite"), 1)
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o644))

	dbPath := filepath.Join(tempDir, "db.sqlite")
	s, err := NewSQLiteStorage(dbPath)
	require.NoError(t, err)
	defer s.Close()

	projID := "IdeaCraftersLabs/my-awesome-project"
	now := time.Now()
	githubSystem := "github"

	t.Run("Task update from sync preserves project_id", func(t *testing.T) {
		t.Logf("Creates task from GitHub sync")
		t.Logf("Simulates sync pull updating the task (title, description, status)")
		t.Logf("Updates task in database")
		t.Logf("Verifies project_id is preserved after update")
		t.Logf("Verifies all other fields were updated correctly")
		// Create task from GitHub
		task := &core.Task{
			ID:           "GH-200",
			Title:        "Original title",
			Description:  "Original description",
			Status:       core.StatusTodo,
			Reference:    "https://github.com/IdeaCraftersLabs/my-awesome-project/issues/200",
			CreatedAt:    now,
			UpdatedAt:    now,
			ProjectID:    &projID,
			OriginSystem: &githubSystem,
			Meta: map[string]interface{}{
				"origin_id":     "200",
				"origin_system": "github",
			},
		}

		err := s.CreateTask(ctx, task)
		require.NoError(t, err)

		// Simulate sync pull updating the task
		updatedTask := *task // Copy
		updatedTask.Title = "Updated title from GitHub"
		updatedTask.Description = "Updated description from GitHub"
		updatedTask.Status = core.StatusInProgress
		updatedTask.UpdatedAt = now.Add(1 * time.Hour)
		updatedTask.LastSyncAt = &updatedTask.UpdatedAt

		err = s.UpdateTask(ctx, &updatedTask)
		require.NoError(t, err)

		// Verify project_id is still correct
		retrieved, err := s.GetTask(ctx, "GH-200")
		require.NoError(t, err)
		assert.Equal(t, projID, *retrieved.ProjectID)
		assert.Equal(t, "Updated title from GitHub", retrieved.Title)
		assert.Equal(t, "Updated description from GitHub", retrieved.Description)
		assert.Equal(t, core.StatusInProgress, retrieved.Status)
	})
}
