package cli

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/IdeaCraftersLabs/oss-tlc-cli/internal/core"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// resetTestDB creates a fresh temporary DB for test isolation.
// It resets viper, the detection cache, and the dbSyncOnce guard so
// each test gets a clean slate. Changes CWD to the temp dir so
// DetectProject() doesn't pick up the real git repo.
// Returns a cleanup function that restores the original CWD.
func resetTestDB(t *testing.T) func() {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "tlc-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)

	viper.Reset()
	viper.Set("storage.backend", "sqlite")
	viper.Set("storage.db_path", filepath.Join(tmpDir, "test.sqlite"))
	viper.Set("task.todo_file", filepath.Join(tmpDir, "TODO"))
	core.ResetDetectionCache()
	dbSyncOnce = sync.Once{}
	resetTaskFlags()

	return func() {
		os.Chdir(origDir)
		core.ResetDetectionCache()
		os.RemoveAll(tmpDir)
	}
}

// resetTaskFlags resets all global flag variables and Cobra's internal
// flag state so that values don't carry between test executions.
func resetTaskFlags() {
	// Reset all globals to their init() defaults
	taskID = ""
	taskTitle = ""
	taskDescription = ""
	taskStatus = "TODO"
	taskAssignedTo = ""
	taskTags = nil
	taskReference = ""
	taskInteractive = false

	taskListStatus = nil
	taskListAssignedTo = ""
	taskListTag = nil
	taskListMine = false
	taskListArchived = false
	taskListAllProjects = false
	taskListSortBy = "created_at"
	taskListSortDirection = "desc"
	taskListLimit = 100
	taskListOffset = 0

	taskShowLogs = false
	taskShowLogSortDirection = ""

	taskUpdateTitle = ""
	taskUpdateDescription = ""
	taskUpdateStatus = ""
	taskUpdateAssignedTo = ""
	taskUpdateAddTags = nil
	taskUpdateRemoveTags = nil

	taskDeleteYes = false
	taskClaimNote = ""
	taskUnclaimNote = ""

	// Clear Cobra's "changed" state on all flags
	for _, cmd := range []*cobra.Command{
		taskCreateCmd, taskListCmd, taskShowCmd,
		taskUpdateCmd, taskDeleteCmd, taskClaimCmd, taskUnclaimCmd,
	} {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			f.Changed = false
		})
	}
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tlc",
		Short: "Task Line CLI - Multi-agent task orchestration",
	}

	cmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file")
	cmd.PersistentFlags().StringP("format", "f", "", "output format (table, json, yaml, tls)")
	cmd.PersistentFlags().Bool("no-color", false, "disable colored output")
	cmd.PersistentFlags().BoolP("verbose", "v", false, "verbose logging")
	cmd.PersistentFlags().BoolP("quiet", "q", false, "suppress non-essential output")

	viper.BindPFlag("output.format", cmd.PersistentFlags().Lookup("format"))
	viper.BindPFlag("output.color", cmd.PersistentFlags().Lookup("no-color"))
	viper.BindPFlag("output.verbose", cmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("output.quiet", cmd.PersistentFlags().Lookup("quiet"))

	return cmd
}

func newTestInitCmd() *cobra.Command {
	storageBackend := "sqlite"
	dbPath := ""
	force := false
	fallbackMode := ""
	duplicateIDStrategy := ""

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize TLC in current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
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
