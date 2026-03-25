package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

const (
	testEngineer1 = "engineer-1"
	testEngineer2 = "engineer-2"
	testTagBug    = "bug"
	testUser      = "testuser"
)

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}

func setupTestDir(t *testing.T) (ctx context.Context, cleanup func()) {
	t.Helper()
	dbPath := resetTestDB(t)
	viper.Set("storage.db_path", dbPath)
	ctx = context.Background()
	// Cleanup is a no-op: resetTestDB's t.Cleanup already removes tmpDir and
	// restores CWD. Calling os.RemoveAll here races with that cleanup and can
	// fail if the process CWD is still inside the directory.
	return ctx, func() {}
}

func testListTaskFormat(t *testing.T, format, titleSuffix, expectID, expectTitle string) {
	t.Helper()
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	task := &core.Task{
		ID:     "T-0001",
		Title:  titleSuffix + " test task",
		Status: core.StatusTodo,
	}
	s.CreateTask(ctx, task)

	viper.Set("output.format", format)
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task list with %s format failed: %v", format, err)
	}

	output := buf.String()
	if !contains(output, expectID) {
		t.Errorf("expected output with '%s' field", expectID)
	}
	if !contains(output, expectTitle) {
		t.Errorf("expected output with '%s' field", expectTitle)
	}
}

func testShowTaskFormat(t *testing.T, format, expectID, expectTitle string) {
	t.Helper()
	ctx, cleanup := setupTestDir(t)
	defer cleanup()
	s, _ := getStorageRaw()
	defer s.Close()

	task := &core.Task{
		ID:     "T-0001",
		Title:  "Show " + format + " test",
		Status: core.StatusTodo,
	}
	s.CreateTask(ctx, task)

	viper.Set("output.format", format)
	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "show", "T-0001"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task show with %s format failed: %v", format, err)
	}

	output := buf.String()
	if !contains(output, expectID) {
		t.Errorf("expected output with '%s' field", expectID)
	}
	if !contains(output, expectTitle) {
		t.Errorf("expected output with '%s' field", expectTitle)
	}
}

func testClearAssignee(t *testing.T, clearValue string) {
	t.Helper()
	ctx, cleanup := setupTestDir(t)
	defer cleanup()

	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()
	assignee1 := testEngineer1
	task := &core.Task{
		ID:         "T-0001",
		Title:      "Task with assignee",
		Status:     core.StatusTodo,
		AssignedTo: &assignee1,
	}
	s.CreateTask(ctx, task)

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--assigned-to", clearValue})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update to clear assignee with %s failed: %v", clearValue, err)
	}

	updatedTask, _ := s.GetTask(ctx, "T-0001")
	if updatedTask == nil {
		t.Fatal("task not found after update")
	}

	if updatedTask.AssignedTo != nil {
		t.Errorf("assignee field is %s, expected nil (cleared)", *updatedTask.AssignedTo)
	}
}

func createAndCompleteTask(t *testing.T, title, description string) {
	t.Helper()

	// Create task
	args := []string{"task", "create", title, "--status", "IN_PROGRESS"}
	if description != "" {
		args = append(args, "--description", description)
	}
	cmd1 := newTestCmd()
	cmd1.AddCommand(TaskCmd)
	buf1 := new(bytes.Buffer)
	cmd1.SetOut(buf1)
	cmd1.SetErr(buf1)
	cmd1.SetArgs(args)
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("task create failed: %v", err)
	}

	// Complete
	cmd2 := newTestCmd()
	cmd2.AddCommand(TaskCmd)
	buf2 := new(bytes.Buffer)
	cmd2.SetOut(buf2)
	cmd2.SetErr(buf2)
	cmd2.SetArgs([]string{"task", "complete", "T-0001"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("task complete failed: %v", err)
	}
}

// resetTestDB creates a fresh isolated database for testing.
// Resets viper, sync guards, detection cache, and task flags.
// Changes CWD to tmpDir (restored via t.Cleanup) to prevent git repo detection.
// Writes a minimal config file and sets cfgFile so initConfig reads it
// instead of the real user config, keeping our db_path intact across Execute().
// Returns the db path.
func resetTestDB(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "test.sqlite")

	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	t.Cleanup(func() {
		cfgFile = ""
		_ = os.Chdir(origDir)
		core.ResetDetectionCache()
		_ = os.RemoveAll(tmpDir)
	})

	// Write a minimal config so initConfig reads this instead of ~/.config/tlc/config.yaml.
	// Include task.todo_file pointing to a non-existent path so ingestTODOWith
	// returns early and does not read the real user todo.txt into the test DB.
	cfgPath := filepath.Join(tmpDir, ".tlc.yaml")
	todoPath := filepath.Join(tmpDir, "todo.txt") // does not exist; ingestTODOWith returns early
	cfgContent := "storage:\n  backend: sqlite\n  db_path: " + dbPath + "\ntask:\n  todo_file: " + todoPath + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	cfgFile = cfgPath

	viper.Reset()
	viper.Set("storage.backend", "sqlite")
	viper.Set("task.todo_file", todoPath)
	viper.Set("storage.db_path", dbPath)
	dbSyncOnce = sync.Once{}
	touchOnce = sync.Once{}
	core.ResetDetectionCache()
	resetTaskFlags()
	return dbPath
}

var (
	testMu sync.Mutex
)

// withTestLock prevents parallel CLI tests from clobbering global state
func withTestLock(fn func()) {
	testMu.Lock()
	defer testMu.Unlock()
	fn()
}

// resetTaskFlags clears CLI flag state between tests
func resetTaskFlags() {
	taskID = ""
	taskDescription = ""
	taskStatus = "TODO"
	taskAssignedTo = ""
	taskTags = []string{}
	taskReference = ""
	taskInteractive = false

	taskListStatus = []string{}
	taskListAssignedTo = ""
	taskListTag = []string{}
	taskListMine = false
	taskListArchived = false
	taskListAllProjects = false
	taskListSortBy = "created_at"
	taskListSortDirection = "desc"
	taskListLimit = 100
	taskListOffset = 0
	taskListSummary = false
	taskListWorkspace = ""
	taskListSpace = ""
	taskListProfile = ""
	taskListSquad = ""

	taskShowLogs = false
	taskShowLogSortDirection = ""

	taskUpdateTitle = ""
	taskUpdateDescription = ""
	taskUpdateStatus = ""
	taskUpdateAssignedTo = ""
	taskUpdateAddTags = []string{}
	taskUpdateRemoveTags = []string{}

	taskDeleteYes = false
	taskClaimNote = ""
	taskUnclaimNote = ""
	taskAssignNote = ""
	taskCompleteNote = ""
	taskUpdateForce = false

	tagFilterAllStatuses = false

	// Clear Cobra's "changed" state on all flags
	for _, cmd := range []*cobra.Command{
		TaskCreateCmd, TaskListCmd, TaskShowCmd,
		TaskUpdateCmd, TaskDeleteCmd, TaskClaimCmd,
		TaskUnclaimCmd, TaskAssignCmd, TaskCompleteCmd,
		SyncCmd, SyncPullCmd, SyncPushCmd, SyncConfigCmd, SyncStatusCmd,
		TagCmd, TagListCmd,
	} {
		if cmd != nil {
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
			})
		}
	}
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tlc",
		Short: "Task Line CLI - Multi-agent task orchestration",
	}

	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file")
	cmd.PersistentFlags().StringP("format", "f", "", "output format (table, json, yaml, tls, summary)")
	cmd.PersistentFlags().Bool("no-color", false, "disable colored output")
	cmd.PersistentFlags().BoolP("verbose", "v", false, "verbose logging")
	cmd.PersistentFlags().BoolP("quiet", "q", false, "suppress non-essential output")

	_ = viper.BindPFlag("output.format", cmd.PersistentFlags().Lookup("format"))
	_ = viper.BindPFlag("output.color", cmd.PersistentFlags().Lookup("no-color"))
	_ = viper.BindPFlag("output.verbose", cmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("output.quiet", cmd.PersistentFlags().Lookup("quiet"))

	return cmd
}

func newTestInitCmd() *cobra.Command {
	var (
		storageBackend      string
		dbPath              string
		force               bool
		fallbackMode        string
		duplicateIDStrategy string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize TLC in current directory",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, &storageBackend, &dbPath, &force, &fallbackMode, &duplicateIDStrategy)
		},
	}

	cmd.Flags().StringVar(&storageBackend, "storage", "sqlite", "Storage backend: local, sqlite")
	cmd.Flags().StringVar(&dbPath, "db-path", "", "Database file path (default: global)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config")
	cmd.Flags().Bool("track", false, "Add .tlc/ to .gitignore")
	cmd.Flags().Bool("no-track", false, "Do not add .tlc/ to .gitignore")
	cmd.Flags().StringVar(&fallbackMode, "fallback-mode", "", "Project fallback mode: auto, detected, prompt (default: auto)")
	cmd.Flags().StringVar(&duplicateIDStrategy, "duplicate-id-strategy", "", "Duplicate ID strategy: share, unique, prompt (default: share)")

	return cmd
}
