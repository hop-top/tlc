package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
)

var taskStaleRunHooks bool

// TaskStaleCmd lists stale tasks and optionally fires configured stale hooks.
var TaskStaleCmd = &cobra.Command{
	Use:   "stale",
	Short: "List stale tasks",
	Long: `List tasks whose last update exceeds the configured stale timeout.

Tasks with no per-task timeout inherit the project default (task.stale.default_timeout).
Use --run-hooks to fire hook commands for each stale task and record StaleFiredAt.`,
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := context.Background()
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		format := viper.GetString("output.format")

		// Scan unfinished work only — staleness is a statement about work
		// that has not finished, so a DONE task cannot be stale.
		//
		// Derived from the vocabulary's ROLES, not named. The literal this
		// replaces — IN_PROGRESS + TODO — was config-blind, and here it
		// failed SILENTLY: a project whose `task.statuses` declares no
		// IN_PROGRESS got a filter matching nothing, so the command exited
		// 0 and printed "No stale tasks." while stale work sat in the
		// store. An empty result the user believes is worse than an error.
		//
		// Staleness is decided in Go, so the store cannot pre-filter it;
		// the cap bounds the scan for list output. Aggregate formats
		// count the match set rather than a page of it, so the cap is
		// lifted for them — a truncated count reads as a real one.
		limit := 1000
		if aggregateFormatValue(format) != "" {
			limit = 0
		}
		filters := []core.FieldFilter{}
		for _, st := range core.UnfinishedTaskStatuses() {
			filters = append(filters, core.FieldFilter{Field: "status", Value: st})
		}
		tasks, err := s.ListTasks(ctx, core.Query{
			Filters: filters,
			Limit:   limit,
		})
		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		// Load stale config; apply project default_timeout to tasks with nil StaleTimeout.
		var taskCfg config.TaskConfig
		_ = unmarshalConfigKey("task", &taskCfg) //nolint:errcheck // best-effort config load
		_ = taskCfg.Validate()                   //nolint:errcheck // best-effort validation

		stale := make([]*core.Task, 0, len(tasks))
		for _, t := range tasks {
			if t.StaleTimeout == nil && taskCfg.Stale.DefaultTimeout > 0 {
				d := taskCfg.Stale.DefaultTimeout
				t.StaleTimeout = &d
			}
			if t.IsStale() {
				stale = append(stale, t)
			}
		}

		if len(stale) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No stale tasks.")
			return nil
		}

		if err := formatTasks(cmd, stale, format, false); err != nil {
			return err
		}

		if taskStaleRunHooks && len(taskCfg.Stale.Hooks) > 0 {
			now := time.Now().UTC()
			for _, t := range stale {
				_ = core.RunStaleHooks(t, taskCfg.Stale.Hooks) //nolint:errcheck // best-effort stale hook
				t.StaleFiredAt = &now
				_ = s.UpdateTask(ctx, t) //nolint:errcheck // best-effort stale timestamp persist
			}
		}

		return nil
	},
}

func init() {
	TaskStaleCmd.Flags().BoolVar(&taskStaleRunHooks, "run-hooks", false,
		"Fire stale hooks for all stale tasks (re-fires even if previously fired)")
}
