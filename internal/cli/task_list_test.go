package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// TestTaskList tests task listing, filtering, sorting, and format output.
func TestTaskList(t *testing.T) {
	t.Run("ListTasks", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()
		s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "Test Task", Status: core.StatusTodo})

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list failed: %v", err)
		}

		output := buf.String()
		t.Logf("Captured output: %q", output)
		if !contains(output, "Test Task") {
			t.Errorf("list output missing task title. got: %s", output)
		}
	})

	t.Run("FilterTasksByTag", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task1 := &core.Task{
			ID:     "T-0001",
			Title:  "Auth task",
			Status: core.StatusTodo,
			Tags:   []string{"auth", "security"},
		}
		task2 := &core.Task{
			ID:     "T-0002",
			Title:  "Docs task",
			Status: core.StatusTodo,
			Tags:   []string{"docs"},
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--tag", "auth"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with tag filter failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Auth task") {
			t.Error("expected 'Auth task' in output")
		}
		if contains(output, "Docs task") {
			t.Error("did not expect 'Docs task' in output (filtered by tag 'auth')")
		}
	})

	t.Run("FilterTasksByAssignee", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		assignee1 := testEngineer1
		assignee2 := testEngineer2
		task1 := &core.Task{
			ID:         "T-0001",
			Title:      "Task for engineer-1",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		task2 := &core.Task{
			ID:         "T-0002",
			Title:      "Task for engineer-2",
			Status:     core.StatusTodo,
			AssignedTo: &assignee2,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--assigned-to", testEngineer1})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with assignee filter failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Task for engineer-1") {
			t.Error("expected 'Task for engineer-1' in output")
		}
		if contains(output, "Task for engineer-2") {
			t.Error("did not expect 'Task for engineer-2' in output (filtered by assignee)")
		}
	})

	t.Run("FilterTasksByStatus", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task1 := &core.Task{
			ID:     "T-0001",
			Title:  "TODO task",
			Status: core.StatusTodo,
		}
		task2 := &core.Task{
			ID:     "T-0002",
			Title:  "IN_PROGRESS task",
			Status: core.StatusInProgress,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--status", "TODO"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with status filter failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "TODO task") {
			t.Error("expected 'TODO task' in output")
		}
		if contains(output, "IN_PROGRESS task") {
			t.Error("did not expect 'IN_PROGRESS task' in output (filtered by status TODO)")
		}
	})

	t.Run("FilterTasksByAssigneeMe", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		currentUser := core.GetCurrentUser()
		if currentUser == "" {
			currentUser = testUser
		}

		assignee1 := currentUser
		assignee2 := "other-user"
		task1 := &core.Task{
			ID:         "T-0001",
			Title:      "My task",
			Status:     core.StatusTodo,
			AssignedTo: &assignee1,
		}
		task2 := &core.Task{
			ID:         "T-0002",
			Title:      "Other's task",
			Status:     core.StatusTodo,
			AssignedTo: &assignee2,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--assigned-to", "me"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --assigned-to me failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "My task") {
			t.Errorf("expected 'My task' in output (assigned to current user: %s)", currentUser)
		}
		if contains(output, "Other's task") {
			t.Error("did not expect 'Other's task' in output (filtered by current user)")
		}
	})

	t.Run("ListTasksJSONFormat", func(t *testing.T) {
		testListTaskFormat(t, "json", "JSON", "\"id\"", "\"title\"")
	})

	t.Run("ListTasksYAMLFormat", func(t *testing.T) {
		testListTaskFormat(t, "yaml", "YAML", "id:", "title:")
	})

	t.Run("ListTasksTLSFormat", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		assignee := testUser
		task := &core.Task{
			ID:         "T-0001",
			Title:      "TLS test task",
			Status:     core.StatusTodo,
			AssignedTo: &assignee,
			Tags:       []string{testTagBug, "urgent"},
		}
		s.CreateTask(ctx, task)

		viper.Set("output.format", "tls")
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with tls format failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "[ ]") {
			t.Error("expected TLS format with [ ] status bracket")
		}
		if !contains(output, "@testuser") {
			t.Error("expected TLS format with @assignee")
		}
		if !contains(output, "#bug") {
			t.Error("expected TLS format with #tag")
		}
	})

	t.Run("ListTasksSort", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task1 := &core.Task{
			ID:        "T-0001",
			Title:     "Task A",
			Status:    core.StatusTodo,
			CreatedAt: time.Now().Add(-2 * time.Hour),
		}
		task2 := &core.Task{
			ID:        "T-0002",
			Title:     "Task B",
			Status:    core.StatusTodo,
			CreatedAt: time.Now().Add(-1 * time.Hour),
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--sort-by", "created_at", "--sort-direction", "asc"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with sort failed: %v", err)
		}

		output := buf.String()
		taskAPos := bytes.Index([]byte(output), []byte("Task A"))
		taskBpos := bytes.Index([]byte(output), []byte("Task B"))
		if taskAPos == -1 || taskBpos == -1 {
			t.Fatal("tasks not found in output")
		}
		if taskAPos > taskBpos {
			t.Error("expected Task A before Task B (ascending by created_at)")
		}
	})

	t.Run("ListTasksFullTextSearch", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task1 := &core.Task{
			ID:     "T-0001",
			Title:  "Search match task",
			Status: core.StatusTodo,
		}
		task2 := &core.Task{
			ID:     "T-0002",
			Title:  "No match task",
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "Search match"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with search term failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Search match task") {
			t.Error("expected matching task in output")
		}
		if contains(output, "No match task") {
			t.Error("did not expect non-matching task in output")
		}
	})

	t.Run("ListTasksAllProjects", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		project1 := "project-alpha"
		project2 := "project-beta"
		task1 := &core.Task{
			ID:        "T-0001",
			Title:     "Alpha task",
			Status:    core.StatusTodo,
			ProjectID: &project1,
		}
		task2 := &core.Task{
			ID:        "T-0002",
			Title:     "Beta task",
			Status:    core.StatusTodo,
			ProjectID: &project2,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--all-projects"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --all-projects failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Alpha task") {
			t.Error("expected Alpha task in output")
		}
		if !contains(output, "Beta task") {
			t.Error("expected Beta task in output")
		}
	})

	t.Run("NoPromptFlagExists", func(t *testing.T) {
		_, cleanup := setupTestDir(t)
		defer cleanup()
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		f := TaskCmd.PersistentFlags().Lookup("no-prompt")
		if f == nil {
			t.Error("expected --no-prompt persistent flag on TaskCmd")
		}
	})

	t.Run("ListTasksArchived", func(t *testing.T) {
		ctx, cleanup := setupTestDir(t)
		defer cleanup()
		s, _ := getStorageRaw()
		defer s.Close()

		task1 := &core.Task{
			ID:       "T-0001",
			Title:    "Archived task",
			Status:   core.StatusDone,
			Archived: true,
		}
		task2 := &core.Task{
			ID:       "T-0002",
			Title:    "Active task",
			Status:   core.StatusTodo,
			Archived: false,
		}
		s.CreateTask(ctx, task1)
		s.CreateTask(ctx, task2)

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "list", "--archived"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task list with --archived failed: %v", err)
		}

		output := buf.String()
		if !contains(output, "Archived task") {
			t.Error("expected archived task in output")
		}
		if !contains(output, "Active task") {
			t.Error("expected active task in output")
		}
	})
}

func TestTaskList_StaleFilter(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	timeout := 1 * time.Minute // short timeout so old UpdatedAt triggers stale
	staleTask := &core.Task{
		ID:           "T-0001",
		Title:        "Stale task",
		Status:       core.StatusInProgress,
		UpdatedAt:    time.Now().Add(-2 * time.Hour), // well past 1m timeout
		StaleTimeout: &timeout,
	}
	freshTask := &core.Task{
		ID:        "T-0002",
		Title:     "Fresh task",
		Status:    core.StatusTodo,
		UpdatedAt: time.Now(),
		// no StaleTimeout → never stale
	}
	s.CreateTask(ctx, staleTask)
	s.CreateTask(ctx, freshTask)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--stale", "--status", "IN_PROGRESS,TODO"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --stale failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "Stale task") {
		t.Errorf("expected 'Stale task' in --stale output; got: %s", output)
	}
	if contains(output, "Fresh task") {
		t.Errorf("did not expect 'Fresh task' in --stale output; got: %s", output)
	}
}


// TestTaskList_IDIntactBeyondSecondRow creates four tasks via the CLI (letting
// storage auto-generate IDs) and verifies that every task ID appears intact
// (including the leading "T") for all rows in the default table output.
// The bug manifests as rows 3+ showing "-0003" instead of "T-0003".
//
// TestMain in test_helpers.go runs the suite under both Ascii and TrueColor
// profiles, so this test exercises the ANSI rendering path automatically.
func TestTaskList_IDIntactBeyondSecondRow(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	titles := []string{"Task One", "Task Two", "Task Three", "Task Four"}
	for i, title := range titles {
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		cmd.SetArgs([]string{"task", "create", fmt.Sprintf("Task %d: %s", i+1, title)})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("create task %d: %v", i+1, err)
		}
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--status", "TODO"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list failed: %v", err)
	}

	output := buf.String()
	t.Logf("list output:\n%s", output)

	// Every row must show a full "T-XXXX" ID, not a truncated "-XXXX".
	// Count how many T- prefixed IDs appear; expect at least 4.
	count := strings.Count(output, "T-")
	if count < len(titles) {
		t.Errorf("expected at least %d full T-XXXX IDs in output, found %d; output:\n%s",
			len(titles), count, output)
	}
}

func TestTaskList_BlockedFilter(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	reason := "waiting on external API"
	blockedTask := &core.Task{
		ID:            "T-0001",
		Title:         "Blocked task",
		Status:        core.StatusTodo,
		BlockedReason: &reason,
	}
	freeTask := &core.Task{
		ID:     "T-0002",
		Title:  "Free task",
		Status: core.StatusTodo,
	}
	s.CreateTask(ctx, blockedTask)
	s.CreateTask(ctx, freeTask)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--blocked", "--status", "TODO"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --blocked failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "Blocked task") {
		t.Errorf("expected 'Blocked task' in --blocked output; got: %s", output)
	}
	if contains(output, "Free task") {
		t.Errorf("did not expect 'Free task' in --blocked output; got: %s", output)
	}
}

func TestTaskListFilterByPriority(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "P0 task", Status: core.StatusTodo, Priority: core.PriorityP0})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "P2 task", Status: core.StatusTodo, Priority: core.PriorityP2})
	s.CreateTask(ctx, &core.Task{ID: "T-0003", Title: "No priority task", Status: core.StatusTodo})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--priority", "P0"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --priority failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "P0 task") {
		t.Errorf("expected 'P0 task' in --priority P0 output; got: %s", output)
	}
	if contains(output, "P2 task") {
		t.Errorf("did not expect 'P2 task' in --priority P0 output; got: %s", output)
	}
	if contains(output, "No priority task") {
		t.Errorf("did not expect 'No priority task' in --priority P0 output; got: %s", output)
	}
}

func TestTaskListFilterByPriorityMultiple(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	s.CreateTask(ctx, &core.Task{ID: "T-0001", Title: "P0 task", Status: core.StatusTodo, Priority: core.PriorityP0})
	s.CreateTask(ctx, &core.Task{ID: "T-0002", Title: "P1 task", Status: core.StatusTodo, Priority: core.PriorityP1})
	s.CreateTask(ctx, &core.Task{ID: "T-0003", Title: "P3 task", Status: core.StatusTodo, Priority: core.PriorityP3})

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--priority", "P0,P1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --priority P0,P1 failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "P0 task") {
		t.Errorf("expected 'P0 task'; got: %s", output)
	}
	if !contains(output, "P1 task") {
		t.Errorf("expected 'P1 task'; got: %s", output)
	}
	if contains(output, "P3 task") {
		t.Errorf("did not expect 'P3 task'; got: %s", output)
	}
}

func TestTaskListFilterByBlockedBy(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	blocker := &core.Task{ID: "T-0001", Title: "Blocker", Status: core.StatusTodo}
	blocked := &core.Task{ID: "T-0002", Title: "Blocked by T-0001", Status: core.StatusTodo,
		Meta: map[string]interface{}{"blocked_by": []string{"T-0001"}}}
	free := &core.Task{ID: "T-0003", Title: "Free task", Status: core.StatusTodo}

	s.CreateTask(ctx, blocker)
	s.CreateTask(ctx, blocked)
	s.CreateTask(ctx, free)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--blocked-by", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list --blocked-by failed: %v", err)
	}

	output := buf.String()
	if !contains(output, "Blocked by T-0001") {
		t.Errorf("expected blocked task in output; got: %s", output)
	}
	if contains(output, "Free task") {
		t.Errorf("did not expect 'Free task' in --blocked-by output; got: %s", output)
	}
	if contains(output, "Blocker") {
		t.Errorf("did not expect blocker itself in --blocked-by output; got: %s", output)
	}
}
