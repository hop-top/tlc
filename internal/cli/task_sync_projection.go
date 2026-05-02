// Package cli: `tlc task sync-projection` subcommand.
//
// Rebuilds the .tlc/tasks/ filesystem projection from SQLite. Replaces the
// deprecated `tlc tasks sync` (asymmetric singular/plural pair, kit
// CLI-conventions §3.2). The deprecated alias lives in tasks_sync.go and
// forwards to the same handler.
package cli

import (
	"github.com/spf13/cobra"
)

var taskSyncProjectionDryRun bool

// TaskSyncProjectionCmd rebuilds the .tlc/tasks/ filesystem projection from
// SQLite. Shares its handler (runTasksSyncProjection) with the deprecated
// `tlc tasks sync` alias.
var TaskSyncProjectionCmd = &cobra.Command{
	Use:   "sync-projection",
	Short: "Rebuild filesystem projection from database",
	Long:  "Wipes .tlc/tasks/ and rebuilds all canonical files and symlinks from SQLite.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// The shared handler reads the package-level tasksSyncDryRun flag.
		// Mirror our own flag onto it so both commands behave identically.
		tasksSyncDryRun = taskSyncProjectionDryRun
		return runTasksSyncProjection(cmd, args)
	},
}

func init() {
	TaskSyncProjectionCmd.Flags().BoolVar(&taskSyncProjectionDryRun, "dry-run", false,
		"Print what would be done without touching disk")

	TaskCmd.AddCommand(TaskSyncProjectionCmd)
}
