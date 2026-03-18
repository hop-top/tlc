package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/uri"
)

var TaskShowCmd = &cobra.Command{
	Use:   "show <task-id>",
	Short: "Show task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		res, err := uri.NewResolver(s).ResolveTask(ctx, id)
		if err != nil {
			return err
		}
		task := res.Task
		if res.Storage != s {
			defer func() { _ = res.Storage.Close() }()
		}

		var logs []*core.LogEntry
		if taskShowLogs || viper.GetString("output.format") != formatTable {
			direction := taskShowLogSortDirection
			if direction == "" {
				direction = viper.GetString("ui.log_sort_direction")
			}
			if direction == "" {
				direction = "desc"
			}
			logs, err = res.Storage.GetLogs(ctx, task.ID, direction)
			if err != nil {
				return fmt.Errorf("failed to get logs: %w", err)
			}
		}

		format := viper.GetString("output.format")
		printTask(cmd, task, logs, format)
		return nil
	},
}

func init() {
	TaskShowCmd.Flags().BoolVar(&taskShowLogs, "logs", false, "Include audit logs")
	TaskShowCmd.Flags().StringVar(&taskShowLogSortDirection, "log-sort-direction", "", "Log sort direction (asc, desc)")
}
