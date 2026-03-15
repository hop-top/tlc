package cli

import (
	"github.com/spf13/cobra"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uri"
	"hop.top/uri/completions"
)

// setupURICompletion wires URI-aware completion into the CLI.
func setupURICompletion(s *storage.SQLiteStorage) error {
	reg, err := uri.GetRegistry(s)
	if err != nil {
		return err
	}

	completer := completions.NewCobraCompleter(reg)

	// Task commands
	taskComp := completer.Complete("task")
	TaskShowCmd.ValidArgsFunction = taskComp
	TaskClaimCmd.ValidArgsFunction = taskComp
	TaskUnclaimCmd.ValidArgsFunction = taskComp
	TaskCompleteCmd.ValidArgsFunction = taskComp
	TaskReopenCmd.ValidArgsFunction = taskComp
	TaskDeleteCmd.ValidArgsFunction = taskComp
	TaskUpdateCmd.ValidArgsFunction = taskComp

	// Assignee commands
	assigneeComp := completer.Complete("assignee")
	TaskAssignCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return taskComp(cmd, args, toComplete)
		}
		if len(args) == 1 {
			return assigneeComp(cmd, args, toComplete)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Flow commands
	flowComp := completer.Complete("flow")
	FlowRunCmd.ValidArgsFunction = flowComp
	FlowInvokeCmd.ValidArgsFunction = flowComp

	// Project commands
	projectComp := completer.Complete("project")
	ProjectExportCmd.ValidArgsFunction = projectComp
	ProjectImportCmd.ValidArgsFunction = projectComp

	return nil
}
