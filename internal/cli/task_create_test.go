package cli

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// TestTaskCreate tests basic task creation and ID sequencing.
func TestTaskCreate(t *testing.T) {
	viper.Set("storage.db_path", resetTestDB(t))

	ctx := context.Background()

	t.Run("CreateTask", func(t *testing.T) {
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Test Task", "--description", "Test Description", "--tag", "test", "--tag", "cli"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create failed: %v", err)
		}

		output := buf.String()
		t.Logf("Create output: %s", output)

		// Verify in storage
		s, _ := getStorageRaw()
		defer s.Close()
		tasks, _ := s.ListTasks(ctx, core.Query{})
		if len(tasks) != 1 {
			t.Errorf("expected 1 task, got %d", len(tasks))
		}
	})

	t.Run("CreateTaskWithAssignee", func(t *testing.T) {
		viper.Set("storage.db_path", resetTestDB(t))
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Task with Assignee", "--assigned-to", testEngineer1})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with assignee failed: %v", err)
		}

		s, _ := getStorageRaw()
		defer s.Close()
		task := getTaskByAlias(t, ctx, "T-0001")
		if task == nil {
			t.Fatal("task was not created")
		}
		if task.AssignedTo == nil {
			t.Error("assignee field is nil, expected 'engineer-1'")
		} else if *task.AssignedTo != testEngineer1 {
			t.Errorf("assignee field is %s, expected 'engineer-1'", *task.AssignedTo)
		}
	})

	t.Run("CreateTaskWithTags", func(t *testing.T) {
		viper.Set("storage.db_path", resetTestDB(t))
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Task with Tags", "--tag", "auth", "--tag", "security"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with tags failed: %v", err)
		}

		s, _ := getStorageRaw()
		defer s.Close()
		task := getTaskByAlias(t, ctx, "T-0001")
		if task == nil {
			t.Fatal("task was not created")
		}
		if len(task.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(task.Tags))
		}
		hasAuth := false
		hasSecurity := false
		for _, tag := range task.Tags {
			if tag == "auth" {
				hasAuth = true
			}
			if tag == "security" {
				hasSecurity = true
			}
		}
		if !hasAuth {
			t.Error("missing 'auth' tag")
		}
		if !hasSecurity {
			t.Error("missing 'security' tag")
		}
	})

	t.Run("CreateTaskWithCustomID", func(t *testing.T) {
		viper.Set("storage.db_path", resetTestDB(t))
		s, _ := getStorageRaw()
		defer s.Close()

		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Custom ID task", "--id", "T-9999"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with --id failed: %v", err)
		}

		// --id explicitly sets task.ID — look up by that literal,
		// not via the seq-based alias helper.
		task, _ := s.GetTask(ctx, "T-9999")
		if task == nil {
			t.Fatal("task not found after creation")
		}
		if task.ID != "T-9999" {
			t.Errorf("expected task ID T-9999, got %s", task.ID)
		}
	})

	t.Run("CreateTaskWithReference", func(t *testing.T) {
		viper.Set("storage.db_path", resetTestDB(t))
		s, _ := getStorageRaw()
		defer s.Close()

		refURL := "https://github.com/repo/issues/42"
		cmd := newTestCmd()
		cmd.AddCommand(TaskCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"task", "create", "Task with reference", "--reference", refURL})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("task create with --reference failed: %v", err)
		}

		task := getTaskByAlias(t, ctx, "T-0001")
		if task == nil {
			t.Fatal("task not found after creation")
		}
		if task.Reference != refURL {
			t.Errorf("expected reference %s, got %s", refURL, task.Reference)
		}
	})

	t.Run("VerifyIDSequencing", func(t *testing.T) {
		viper.Set("storage.db_path", resetTestDB(t))
		s, _ := getStorageRaw()
		defer s.Close()

		cmd1 := newTestCmd()
		cmd1.AddCommand(TaskCmd)
		buf1 := new(bytes.Buffer)
		cmd1.SetOut(buf1)
		cmd1.SetErr(buf1)
		cmd1.SetArgs([]string{"task", "create", "First task"})

		if err := cmd1.Execute(); err != nil {
			t.Fatalf("first task create failed: %v", err)
		}

		cmd2 := newTestCmd()
		cmd2.AddCommand(TaskCmd)
		buf2 := new(bytes.Buffer)
		cmd2.SetOut(buf2)
		cmd2.SetErr(buf2)
		cmd2.SetArgs([]string{"task", "create", "Second task"})

		if err := cmd2.Execute(); err != nil {
			t.Fatalf("second task create failed: %v", err)
		}

		tasks, _ := s.ListTasks(ctx, core.Query{})
		if len(tasks) < 2 {
			t.Fatal("expected at least 2 tasks")
		}

		taskIDs := make([]string, len(tasks))
		for i, task := range tasks {
			taskIDs[i] = task.ID
		}

		if taskIDs[0] == taskIDs[1] {
			t.Error("expected different task IDs (no sequencing conflict)")
		}
	})
}

// TestTaskCreateWithAssignee tests standalone task creation with assignee.
func TestTaskCreateWithAssignee(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Task with Assignee", "--assigned-to", testEngineer1})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create with assignee failed: %v", err)
	}

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	task := getTaskByAlias(t, ctx, "T-0001")
	if task == nil {
		t.Fatal("task was not created")
	}
	if task.AssignedTo == nil {
		t.Error("assignee field is nil, expected 'engineer-1'")
	} else if *task.AssignedTo != testEngineer1 {
		t.Errorf("assignee field is %s, expected 'engineer-1'", *task.AssignedTo)
	}
}

// TestTaskCreateWithTags tests standalone task creation with tags.
func TestTaskCreateWithTags(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Task with Tags", "--tag", "auth", "--tag", "security"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create with tags failed: %v", err)
	}

	task := getTaskByAlias(t, ctx, "T-0001")
	if task == nil {
		t.Fatal("task was not created")
	}
	if len(task.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(task.Tags))
	}

	hasAuth := false
	hasSecurity := false
	for _, tag := range task.Tags {
		if tag == "auth" {
			hasAuth = true
		}
		if tag == "security" {
			hasSecurity = true
		}
	}
	if !hasAuth {
		t.Error("missing 'auth' tag")
	}
	if !hasSecurity {
		t.Error("missing 'security' tag")
	}
}

// TestTaskCreatePagination tests list pagination after creating multiple tasks.
func TestTaskCreatePagination(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	for i := 1; i <= 15; i++ {
		task := &core.Task{
			ID:     fmt.Sprintf("T-%04d", i),
			Title:  fmt.Sprintf("Task %d", i),
			Status: core.StatusTodo,
		}
		s.CreateTask(ctx, task)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list", "--limit", "10", "--offset", "5"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list with pagination failed: %v", err)
	}

	output := buf.String()
	taskCount := 0
	for i := 6; i <= 15; i++ {
		if contains(output, fmt.Sprintf("Task %d", i)) {
			taskCount++
		}
	}
	if taskCount != 10 {
		t.Errorf("expected 10 tasks (6-15), got %d", taskCount)
	}
}

func TestTaskCreateWithBlockedBy(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	if err := s.CreateTask(ctx, &core.Task{ID: "T-0009", Title: "Blocker 1", Status: core.StatusTodo}); err != nil {
		t.Fatalf("CreateTask(blocker1): %v", err)
	}
	if err := s.CreateTask(ctx, &core.Task{ID: "T-0010", Title: "Blocker 2", Status: core.StatusTodo}); err != nil {
		t.Fatalf("CreateTask(blocker2): %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{
		"task", "create", "Blocked task",
		"--blocked-by", "T-0009",
		"--blocked-by", "T-0010",
		"--blocked-by", "T-0009",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create --blocked-by failed: %v", err)
	}

	tasks, err := s.ListTasks(ctx, core.Query{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	var task *core.Task
	for _, candidate := range tasks {
		if candidate.Title == "Blocked task" {
			task = candidate
			break
		}
	}
	if task == nil {
		t.Fatal("task not found after creation")
	}

	blockedBy := task.BlockedBy()
	if len(blockedBy) != 2 {
		t.Fatalf("expected 2 unique blockers, got %d (%v)", len(blockedBy), blockedBy)
	}
	if blockedBy[0] != "T-0009" || blockedBy[1] != "T-0010" {
		t.Fatalf("blocked_by = %v, want [T-0009 T-0010]", blockedBy)
	}
}

func TestTaskCreateWithBlockedByRejectsMissingTask(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Blocked task", "--blocked-by", "T-9999"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected create to fail for missing blocker")
	}
	if !contains(err.Error(), "invalid blocked_by reference") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTaskCreateWithCrossProjectBlockedBy(t *testing.T) {
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	projectID := "other-project"
	otherDBPath := filepath.Join(t.TempDir(), "other.sqlite")
	otherStorage, err := storage.NewSQLiteStorage(otherDBPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage(other): %v", err)
	}
	defer otherStorage.Close()

	requireProject := projectID
	if err := otherStorage.CreateTask(ctx, &core.Task{
		ID:        "T-0007",
		ProjectID: &requireProject,
		Title:     "Cross project blocker",
		Status:    core.StatusTodo,
	}); err != nil {
		t.Fatalf("CreateTask(other): %v", err)
	}
	if err := s.RegisterProject(ctx, projectID, otherDBPath, "", "Other Project"); err != nil {
		t.Fatalf("RegisterProject: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "create", "Blocked task", "--blocked-by", "other-project/T-0007"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task create with cross-project blocker failed: %v", err)
	}

	task := getTaskByAlias(t, ctx, "T-0001")
	if task == nil {
		t.Fatal("task not found after creation")
	}
	blockedBy := task.BlockedBy()
	if len(blockedBy) != 1 || blockedBy[0] != "other-project/T-0007" {
		t.Fatalf("blocked_by = %v, want [other-project/T-0007]", blockedBy)
	}
}

func TestBuildTaskReference(t *testing.T) {
	t.Run("WithProject", func(t *testing.T) {
		proj := &core.ProjectDetection{ProjectID: "myorg/myrepo"}
		got := buildTaskReference("T-0042", proj)
		want := "tlc://myorg/myrepo/T-0042"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("WithoutProject", func(t *testing.T) {
		got := buildTaskReference("T-0042", nil)
		want := "tlc:///T-0042"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("EmptyProjectID", func(t *testing.T) {
		proj := &core.ProjectDetection{ProjectID: ""}
		got := buildTaskReference("T-0042", proj)
		want := "tlc:///T-0042"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("WithProjectMustNotUseTaskScheme", func(t *testing.T) {
		proj := &core.ProjectDetection{ProjectID: "hop-top/tlc"}
		got := buildTaskReference("T-0042", proj)
		if strings.HasPrefix(got, "task://") {
			t.Errorf("reference %q uses task:// scheme; must use tlc://", got)
		}
	})
	t.Run("ReferenceContainsProjectID", func(t *testing.T) {
		proj := &core.ProjectDetection{ProjectID: "hop-top/tlc"}
		got := buildTaskReference("T-0001", proj)
		if !strings.Contains(got, "hop-top/tlc") {
			t.Errorf("reference %q missing project identifier hop-top/tlc", got)
		}
		if !strings.HasPrefix(got, "tlc://") {
			t.Errorf("reference %q must start with tlc:// scheme", got)
		}
	})
}
