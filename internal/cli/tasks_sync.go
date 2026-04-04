package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

var tasksSyncDryRun bool

// TasksCmd is the parent command for filesystem projection operations.
var TasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Filesystem projection operations",
}

// TasksSyncCmd rebuilds the .tlc/tasks/ filesystem projection from SQLite.
var TasksSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Rebuild filesystem projection from database",
	Long:  "Wipes .tlc/tasks/ and rebuilds all canonical files and symlinks from SQLite.",
	RunE:  runTasksSync,
}

func init() {
	TasksSyncCmd.Flags().BoolVar(&tasksSyncDryRun, "dry-run", false,
		"Print what would be done without touching disk")

	TasksCmd.AddCommand(TasksSyncCmd)
	RootCmd.AddCommand(TasksCmd)
}

func runTasksSync(cmd *cobra.Command, _ []string) error {
	cfg := filesystemConfigFromViper()
	if !cfg.Enabled {
		fmt.Fprintln(cmd.OutOrStdout(),
			"filesystem projection is disabled; enable with storage.filesystem: true in config")
		return nil
	}

	s, err := getStorage()
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer s.Close()

	tasks, err := s.ListTasks(context.Background(), core.Query{
		IncludeArchived: true,
		AllProjects:     true,
		Limit:           0,
	})
	if err != nil {
		return fmt.Errorf("list tasks: %w", err)
	}

	baseDir := projectionBaseDir()

	if tasksSyncDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "would rebuild %s with %d task(s)\n",
			baseDir, len(tasks))
		fmt.Fprintf(cmd.OutOrStdout(), "group_by: %v\n", cfg.GroupBy)
		fmt.Fprintf(cmd.OutOrStdout(), "sort_by:  %v\n", cfg.SortBy)
		for _, t := range tasks {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s  %s\n", t.ID, t.Status, t.Title)
		}
		return nil
	}

	p := storage.NewFilesystemProjector(storage.FilesystemProjectorConfig{
		BaseDir: baseDir,
		GroupBy: cfg.GroupBy,
		SortBy:  cfg.SortBy,
	})

	if err := p.RebuildAll(tasks); err != nil {
		return fmt.Errorf("rebuild projection: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "rebuilt %s with %d task(s)\n",
		baseDir, len(tasks))
	return nil
}

// filesystemConfigFromViper reads filesystem projection config from viper.
func filesystemConfigFromViper() filesystemCfg {
	enabled := viper.GetBool("storage.filesystem.enabled")
	if !enabled {
		// Check if set as bare bool (storage.filesystem: true)
		if viper.GetBool("storage.filesystem") {
			enabled = true
		}
	}

	groupBy := viper.GetStringSlice("storage.filesystem.group_by")
	if len(groupBy) == 0 {
		groupBy = []string{"status"}
	}
	sortBy := viper.GetStringSlice("storage.filesystem.sort_by")
	if len(sortBy) == 0 {
		sortBy = []string{"id"}
	}

	return filesystemCfg{
		Enabled: enabled,
		GroupBy: groupBy,
		SortBy:  sortBy,
	}
}

type filesystemCfg struct {
	Enabled bool
	GroupBy []string
	SortBy  []string
}

// projectionBaseDir returns the base directory for filesystem projection.
// Defaults to .tlc/tasks relative to cwd.
func projectionBaseDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ".tlc/tasks"
	}
	return filepath.Join(cwd, ".tlc", "tasks")
}

// SetupProjector wires a FilesystemProjector into the given storage
// if filesystem projection is enabled in config. Returns true if wired.
func SetupProjector(s *storage.SQLiteStorage) bool {
	cfg := filesystemConfigFromViper()
	if !cfg.Enabled {
		return false
	}

	p := storage.NewFilesystemProjector(storage.FilesystemProjectorConfig{
		BaseDir: projectionBaseDir(),
		GroupBy: cfg.GroupBy,
		SortBy:  cfg.SortBy,
	})
	s.SetProjector(p)

	// Auto-sync: if tasks dir doesn't exist, rebuild on first access.
	baseDir := projectionBaseDir()
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		tasks, listErr := s.ListTasks(context.Background(), core.Query{
			IncludeArchived: true,
			AllProjects:     true,
			Limit:           0,
		})
		if listErr == nil && len(tasks) > 0 {
			_ = p.RebuildAll(tasks)
		}
	}

	return true
}
