package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Inspect workflow configuration",
}

var workflowStatusesCmd = &cobra.Command{
	Use:   "statuses",
	Short: "Show all configured statuses",
	RunE: func(cmd *cobra.Command, _ []string) error {
		wm := core.DefaultWorkflow()
		out := cmd.OutOrStdout()

		allStatuses := wm.GetAllStatuses()
		for _, name := range allStatuses {
			def, err := wm.GetStatusDef(core.TaskStatus(name))
			if err != nil {
				continue
			}

			label := def.Label
			if label == "" {
				label = def.Name
			}

			terminal := "non-terminal"
			if def.IsTerminal {
				terminal = "terminal"
			}

			role := ""
			if def.Role != "" {
				role = fmt.Sprintf("  [role: %s]", def.Role)
			}

			marker := ""
			if def.TLSMarker != "" {
				marker = fmt.Sprintf("  marker: %q", def.TLSMarker)
			}

			_, _ = fmt.Fprintf(out, "%-14s(%s)\t%s%s%s\n",
				name, label, terminal, role, marker,
			)
		}
		return nil
	},
}

var workflowRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Show state machine transition rules",
	RunE: func(cmd *cobra.Command, _ []string) error {
		wm := core.DefaultWorkflow()
		out := cmd.OutOrStdout()

		allStatuses := wm.GetAllStatuses()
		for _, name := range allStatuses {
			def, err := wm.GetStatusDef(core.TaskStatus(name))
			if err != nil {
				continue
			}

			if def.IsTerminal {
				_, _ = fmt.Fprintf(out, "%-14s -> (terminal)\n", name)
				continue
			}

			// Collect reachable statuses by testing transitions
			var targets []string
			for _, target := range allStatuses {
				if target == name {
					continue
				}
				err := wm.ValidateTransition(
					core.TaskStatus(name), core.TaskStatus(target), false,
				)
				if err == nil {
					targets = append(targets, target)
				}
			}

			if len(targets) == 0 {
				_, _ = fmt.Fprintf(out, "%-14s -> (none)\n", name)
			} else {
				_, _ = fmt.Fprintf(out, "%-14s -> %s\n",
					name, strings.Join(targets, ", "),
				)
			}
		}
		return nil
	},
}

var workflowValidateCmd = &cobra.Command{
	Use:   "validate <from> <to>",
	Short: "Dry-run a status transition",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		from := core.TaskStatus(strings.ToUpper(args[0]))
		to := core.TaskStatus(strings.ToUpper(args[1]))
		wm := core.DefaultWorkflow()
		out := cmd.OutOrStdout()

		err := wm.ValidateTransition(from, to, false)
		if err == nil {
			_, _ = fmt.Fprintf(out,
				"Transition allowed: %s -> %s\n", from, to,
			)
			return nil
		}

		_, _ = fmt.Fprintf(out,
			"Transition not allowed: %s -> %s (%s)\n", from, to, err,
		)
		return nil
	},
}

func init() {
	workflowCmd.AddCommand(workflowStatusesCmd)
	workflowCmd.AddCommand(workflowRulesCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
	RootCmd.AddCommand(workflowCmd)
}
