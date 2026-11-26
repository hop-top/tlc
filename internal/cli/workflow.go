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
	Long:  "List every workflow status with its label, terminal flag, role, and TLS marker.",
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
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

			_, _ = fmt.Fprintf(
				out, "%-14s(%s)\t%s%s%s\n",
				name, label, terminal, role, marker,
			)
		}
		return nil
	},
}

var workflowRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Show state machine transition rules",
	Long:  "Show, for each status, the set of statuses it can transition to.",
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
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
				_, _ = fmt.Fprintf(
					out, "%-14s -> %s\n",
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
	Long:  "Report whether the workflow state machine allows the given <from> → <to> transition.",
	Args:  cobra.ExactArgs(2),
	Annotations: map[string]string{
		"kit/side-effect": "read",
		"kit/idempotent":  "yes",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		wm := core.DefaultWorkflow()
		from := resolveStatusName(wm, args[0])
		to := resolveStatusName(wm, args[1])
		out := cmd.OutOrStdout()

		err := wm.ValidateTransitionContext(core.TransitionContext{
			From:     from,
			To:       to,
			Note:     workflowValidateNote,
			Reason:   workflowValidateReason,
			Assignee: workflowValidateAssignee,
		})
		if err == nil {
			_, _ = fmt.Fprintf(
				out,
				"Transition allowed: %s -> %s\n", from, to,
			)
			return nil
		}

		_, _ = fmt.Fprintf(
			out,
			"Transition not allowed: %s -> %s (%s)\n", from, to, err,
		)
		return nil
	},
}

// resolveStatusName maps user input to a declared status name,
// ignoring case; input matching nothing is returned as typed so the
// workflow reports it as unknown.
func resolveStatusName(wm *core.WorkflowManager, in string) core.TaskStatus {
	for _, name := range wm.GetAllStatuses() {
		if strings.EqualFold(name, in) {
			return core.TaskStatus(name)
		}
	}
	return core.TaskStatus(in)
}

var (
	workflowValidateNote     string
	workflowValidateReason   string
	workflowValidateAssignee string
)

func init() {
	workflowValidateCmd.Flags().StringVar(&workflowValidateNote, "note", "", "Transition note (satisfies a note guard)")
	workflowValidateCmd.Flags().StringVar(&workflowValidateReason, "reason", "", "Transition reason (satisfies a reason guard)")
	workflowValidateCmd.Flags().StringVar(&workflowValidateAssignee, "assignee", "", "Assignee after the move (satisfies an assignee guard)")
	workflowCmd.AddCommand(workflowStatusesCmd)
	workflowCmd.AddCommand(workflowRulesCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
	RootCmd.AddCommand(workflowCmd)
}
