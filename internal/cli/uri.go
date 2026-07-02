package cli

import (
	"github.com/spf13/cobra"
	"hop.top/uri/scheme"

	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uri"
)

// cobraCompleter adapts a hop.top/uri/scheme Registry to cobra ValidArgsFunction.
type cobraCompleter struct {
	reg *scheme.Registry
}

func (c *cobraCompleter) Complete(typeName string) func(
	cmd *cobra.Command, args []string, toComplete string,
) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		vals, err := c.reg.Complete(cmd.Context(), typeName, toComplete)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		return vals, cobra.ShellCompDirectiveNoFileComp
	}
}

// setupURICompletion wires URI-aware completion into the CLI.
func setupURICompletion(s *storage.SQLiteStorage) error {
	reg, err := uri.GetRegistry(s, uriDirsConfig())
	if err != nil {
		return err
	}

	completer := &cobraCompleter{reg: reg}

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
