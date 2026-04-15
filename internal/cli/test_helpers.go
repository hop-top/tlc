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
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	task := &core.Task{
		ID:     "T-0001",
		Title:  titleSuffix + " test task",
		Status: core.StatusTodo,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

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
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer s.Close()

	task := &core.Task{
		ID:     "T-0001",
		Title:  "Show " + format + " test",
		Status: core.StatusTodo,
	}
	if err := s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

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
	if err = s.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	cmd := newTestCmd()
	cmd.AddCommand(TaskCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"task", "update", "T-0001", "--assigned-to", clearValue})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("task update to clear assignee with %s failed: %v", clearValue, err)
	}

	updatedTask, err := s.GetTask(ctx, "T-0001")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
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

	origDir, _ := os.Getwd() //nolint:errcheck // test setup
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	t.Cleanup(func() {
		cfgFile = ""
		_ = os.Chdir(origDir) //nolint:errcheck // test cleanup
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

// resetTaskFlags clears CLI flag state between tests.
// Uses cobra's ResetFlags to clear both package vars and
// pflag's internal Changed bits (T-0231).
func resetTaskFlags() {
	ResetCreateFlags()
	taskID = ""
	taskDescription = ""
	taskStatus = "TODO"
	taskAssignedTo = ""
	taskEffort = ""
	taskPriority = ""
	taskBlockedBy = []string{}
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
	taskListCounters = false
	taskListWorkspace = ""
	taskListSpace = ""
	taskListProfile = ""
	taskListSquad = ""
	taskListStale = false
	taskListBlocked = false
	taskListPriority = []string{}
	taskListBlockedBy = []string{}

	taskStaleRunHooks = false

	taskShowLogs = false
	taskShowLogSortDirection = ""

	taskUpdateTitle = ""
	taskUpdateDescription = ""
	taskUpdateStatus = ""
	taskUpdateAssignedTo = ""
	taskUpdateEffort = ""
	taskUpdatePriority = ""
	taskUpdateAddBlockedBy = []string{}
	taskUpdateRemoveBlockedBy = []string{}
	taskUpdateClearBlockedBy = false
	taskUpdateAddTags = []string{}
	taskUpdateRemoveTags = []string{}

	taskTrack = ""
	taskListTrack = ""
	taskUpdateTrack = ""

	taskDeleteYes = false
	taskGraphFormat = ""
	taskClaimNote = ""
	taskUnclaimNote = ""
	taskAssignNote = ""
	taskCompleteNote = ""
	taskUpdateForce = false
	taskNoPrompt = false

	tagFilterAllStatuses = false

	// Reset alias flags.
	aliasGlobal = false

	// Reset logCmd vars (package-level state not covered by task flag resets).
	logTaskID = ""
	logAction = ""
	logBy = ""
	logSince = ""
	logUntil = ""
	logLimit = 100
	logOffset = 0
	logSortDirection = ""
	logAll = false

	// Reset track flags.
	resetTrackFlags()
	resetTrackGraphFlags()

	// Reset flow flags.
	resetFlowFlags()

	tasksSyncDryRun = false

	// Reset prompt flags.
	promptJSON = false

	// Reset NL prompt flags (live on RootCmd).
	taskNLExecute = false
	taskNLDryRun = false
	taskNLJSON = false

	// Clear Cobra's "changed" state on all flags
	for _, cmd := range []*cobra.Command{
		TaskCreateCmd, TaskListCmd, TaskGraphCmd, TaskStaleCmd, TaskShowCmd,
		TaskUpdateCmd, TaskDeleteCmd, TaskClaimCmd,
		TaskUnclaimCmd, TaskAssignCmd, TaskCompleteCmd,
		SyncCmd, SyncPullCmd, SyncPushCmd, SyncConfigCmd, SyncStatusCmd,
		TagCmd, TagListCmd,
		logCmd,
		aliasAddCmd, aliasRemoveCmd,
		trackCreateCmd, trackUpdateCmd, trackArchiveCmd, trackAbandonCmd, trackDeleteCmd,
		trackListCmd, trackShowCmd, trackSummaryCmd, trackGraphCmd,
		TasksSyncCmd,
		PromptTaskCmd,
	} {
		if cmd != nil {
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
			})
		}
	}
	TaskCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})

	// Reset extension manager and bus globals.
	extMgr = nil
	eventBus = nil
	auditSub = nil
	busPublisher = nil
}

// resetFlowFlags clears the package-level state bound to flow.go's Cobra
// flags. Without this, --var values (and --by) leak between tests because
// FlowRunCmd / FlowInvokeCmd are package globals shared across test commands.
func resetFlowFlags() {
	flowDryRun = false
	flowRunBy = ""
	flowRunVars = nil
	flowStatusAll = false

	for _, cmd := range []*cobra.Command{
		FlowRunCmd, FlowInvokeCmd, FlowStatusCmd, FlowListCmd,
	} {
		if cmd != nil {
			cmd.Flags().VisitAll(func(f *pflag.Flag) {
				f.Changed = false
			})
		}
	}
}

// setupProjectScopedTestDir creates a fresh tmpDir, chdirs into it, writes
// a minimal .tlc/config.yaml with the given project ID, and wires viper so
// DetectProject() returns InProject=true with that ID. Used by flow tests
// that need to exercise the project-scoped path of CreateTask + AddLog.
//
// Returns (tmpDir, dbPath). All cleanup is registered via t.Cleanup.
func setupProjectScopedTestDir(t *testing.T, prefix, projectID string) (string, string) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", prefix+"*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	origDir, _ := os.Getwd() //nolint:errcheck // test setup
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) }) //nolint:errcheck // test cleanup

	tlcDir := filepath.Join(tmpDir, ".tlc")
	if err := os.MkdirAll(tlcDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .tlc: %v", err)
	}
	projectCfgPath := filepath.Join(tlcDir, "config.yaml")
	dbPath := filepath.Join(tmpDir, "test.sqlite")
	projectCfg := "project:\n  id: " + projectID +
		"\nstorage:\n  backend: sqlite\n  db_path: " + dbPath + "\n"
	if err := os.WriteFile(projectCfgPath, []byte(projectCfg), 0o600); err != nil {
		t.Fatalf("WriteFile config.yaml: %v", err)
	}

	viper.Reset()
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	touchOnce = sync.Once{}
	cfgFile = projectCfgPath
	viper.Set("config", projectCfgPath)
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)
	resetTaskFlags() // also resets flow flags via resetFlowFlags()
	t.Cleanup(func() {
		cfgFile = ""
		viper.Reset()
		core.ResetDetectionCache()
		dbSyncOnce = sync.Once{}
		touchOnce = sync.Once{}
		resetFlowFlags()
	})

	return tmpDir, dbPath
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

	_ = viper.BindPFlag("output.format", cmd.PersistentFlags().Lookup("format"))   //nolint:errcheck // test setup
	_ = viper.BindPFlag("output.color", cmd.PersistentFlags().Lookup("no-color")) //nolint:errcheck // test setup
	_ = viper.BindPFlag("output.verbose", cmd.PersistentFlags().Lookup("verbose")) //nolint:errcheck // test setup
	_ = viper.BindPFlag("output.quiet", cmd.PersistentFlags().Lookup("quiet"))    //nolint:errcheck // test setup

	return cmd
}

// newTestNLRootCmd creates a root command with the NL prompt handler wired,
// mirroring the production RootCmd setup. Use this for tests that exercise
// the full NL pipeline via root-level dispatch.
func newTestNLRootCmd() *cobra.Command {
	root := newTestCmd()
	root.Args = cobra.ArbitraryArgs
	root.RunE = runNLPrompt
	root.Flags().BoolVarP(&taskNLExecute, "execute", "x", false, "Execute resolved commands without confirmation")
	root.Flags().BoolVar(&taskNLDryRun, "dry-run", false, "Show resolved commands without executing")
	root.Flags().BoolVar(&taskNLJSON, "json", false, "Output resolved commands as JSON")
	root.AddCommand(TaskCmd)
	return root
}

// isolateInitTest sets up an isolated SQLite database for tests that call
// runInit directly (without going through resetTestDB). It sets storage.db_path
// to a temp file so that project registration never touches the real global db.
// Returns a cleanup func (called via t.Cleanup automatically).
func isolateInitTest(t *testing.T) {
	t.Helper()
	tmpDB, err := os.CreateTemp("", "tlc-init-testdb-*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp db file: %v", err)
	}
	tmpDB.Close()
	dbPath := tmpDB.Name()
	t.Cleanup(func() { _ = os.Remove(dbPath) })

	dbSyncOnce = sync.Once{}
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", dbPath)
	core.ResetDetectionCache()
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
