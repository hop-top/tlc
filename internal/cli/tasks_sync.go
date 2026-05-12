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
//
// Deprecated: the plural "tasks" parent violates kit CLI conventions
// (§3.2: one noun, multiple verbs; never asymmetric singular/plural pairs).
// Use `tlc task sync-projection` instead. Hidden + deprecation warning kept
// for one release.
var TasksCmd = &cobra.Command{
	Use:        "tasks",
	Short:      "Filesystem projection operations (deprecated)",
	Hidden:     true,
	Deprecated: "use 'tlc task sync-projection' instead",
}

// TasksSyncCmd is the deprecated alias for `tlc task sync-projection`.
//
// Deprecated: use `tlc task sync-projection`. Hidden + deprecation warning
// kept for one release; forwards to the new handler.
var TasksSyncCmd = &cobra.Command{
	Use:        "sync",
	Short:      "Rebuild filesystem projection from database (deprecated)",
	Hidden:     true,
	Deprecated: "use 'tlc task sync-projection' instead",
	Long: `Wipes .tlc/tasks/ and rebuilds all canonical files and symlinks from SQLite.

DEPRECATED: use 'tlc task sync-projection'. This alias will be removed in a
future release.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
		"kit/idempotent":  "yes",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
			"warning: 'tlc tasks sync' is deprecated; "+
				"use 'tlc task sync-projection' instead")
		return runTasksSyncProjection(cmd, args)
	},
}

func init() {
	TasksSyncCmd.Flags().BoolVar(&tasksSyncDryRun, "dry-run", false,
		"Print what would be done without touching disk")

	TasksCmd.AddCommand(TasksSyncCmd)
	RootCmd.AddCommand(TasksCmd)
}

// runTasksSyncProjection rebuilds the .tlc/tasks/ filesystem projection from
// SQLite. Shared by `tlc task sync-projection` and the deprecated
// `tlc tasks sync` alias.
func runTasksSyncProjection(cmd *cobra.Command, _ []string) error {
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
	enabled := false
	if viper.IsSet("storage.filesystem.enabled") {
		enabled = viper.GetBool("storage.filesystem.enabled")
	} else if viper.IsSet("storage.filesystem") {
		switch viper.Get("storage.filesystem").(type) {
		case map[string]interface{}, map[interface{}]interface{}:
			// Object form implies enabled
			enabled = true
		default:
			// Bare bool form
			enabled = viper.GetBool("storage.filesystem")
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
	dir := projectionDirFromConfig()
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(".tlc", dir)
	}
	return filepath.Join(cwd, ".tlc", dir)
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
		if listErr != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to auto-sync task projection: %v\n", listErr)
		} else if len(tasks) > 0 {
			if err := p.RebuildAll(tasks); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to rebuild task projection: %v\n", err)
			}
		}
	}

	return true
}
