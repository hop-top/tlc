// Package cli: `tlc flow approve|reject|cancel` subcommands.
//
// CLI surface for story 025 (T-0199) — thin wrappers around
// core.FileApprovalStore that allow an out-of-process operator to
// resolve a flow run paused at a type:human step.
//
// Author: jadb
package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var (
	flowApproveStep   string
	flowApproveBy     string
	flowApproveNote   string
	flowRejectStep    string
	flowRejectBy      string
	flowRejectReason  string
	flowCancelBy      string
	flowApprovalsRoot string // override store root; default core.DefaultApprovalsDir()
)

// FlowApproveCmd resolves a paused human step as approved.
var FlowApproveCmd = &cobra.Command{
	Use:   "approve <run-id>",
	Short: "Approve a human step in a flow run",
	Long: `Resolve a paused type:human step as approved. The flow run
must be paused at the named step. Approval is one-shot — once
resolved, a second approve|reject returns "already resolved".

Examples:
  tlc flow approve run:abc123 --step send-email
  tlc flow approve run:abc123 --step send-email --by jad --note "lgtm"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		runID := args[0]
		if flowApproveStep == "" {
			return fmt.Errorf("--step is required")
		}
		store := approvalStore()
		by := approvalBy(flowApproveBy)
		ctx, cancel := context.WithTimeout(cmdContext(cmd), 5*time.Second)
		defer cancel()
		if err := store.Approve(ctx, runID, flowApproveStep, by, flowApproveNote); err != nil {
			return fmt.Errorf("approve: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Approved run %s step %s (by %s)\n", runID, flowApproveStep, by)
		return nil
	},
}

// FlowRejectCmd resolves a paused human step as rejected.
var FlowRejectCmd = &cobra.Command{
	Use:   "reject <run-id>",
	Short: "Reject a human step in a flow run",
	Long: `Resolve a paused type:human step as rejected. The flow run
transitions to FAILED with the provided reason recorded in the
audit log.

Examples:
  tlc flow reject run:abc123 --step send-email --reason "wrong recipient"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		runID := args[0]
		if flowRejectStep == "" {
			return fmt.Errorf("--step is required")
		}
		if flowRejectReason == "" {
			return fmt.Errorf("--reason is required")
		}
		store := approvalStore()
		by := approvalBy(flowRejectBy)
		ctx, cancel := context.WithTimeout(cmdContext(cmd), 5*time.Second)
		defer cancel()
		if err := store.Reject(ctx, runID, flowRejectStep, by, flowRejectReason); err != nil {
			return fmt.Errorf("reject: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Rejected run %s step %s (by %s): %s\n",
			runID, flowRejectStep, by, flowRejectReason)
		return nil
	},
}

// FlowCancelCmd cancels an entire flow run by marking every awaiting
// human step as canceled. Resolved steps are left untouched.
var FlowCancelCmd = &cobra.Command{
	Use:   "cancel <run-id>",
	Short: "Cancel a flow run",
	Long: `Cancel a flow run. Marks every awaiting human step under
the run as canceled. Already-resolved steps are left untouched.

Example:
  tlc flow cancel run:abc123`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		runID := args[0]
		store := approvalStore()
		by := approvalBy(flowCancelBy)
		ctx, cancel := context.WithTimeout(cmdContext(cmd), 5*time.Second)
		defer cancel()
		if err := store.CancelRun(ctx, runID, by); err != nil {
			return fmt.Errorf("cancel: %w", err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"Canceled run %s (by %s)\n", runID, by)
		return nil
	},
}

func approvalStore() *core.FileApprovalStore {
	root := flowApprovalsRoot
	if root == "" {
		root = core.DefaultApprovalsDir()
	}
	return core.NewFileApprovalStore(root)
}

func approvalBy(flag string) string {
	if flag != "" {
		return flag
	}
	return core.GetCurrentUser()
}

// cmdContext returns the cobra command's context, falling back to
// context.Background when nil (tests bypass cobra's full lifecycle).
func cmdContext(cmd *cobra.Command) context.Context {
	if c := cmd.Context(); c != nil {
		return c
	}
	return context.Background()
}

func init() {
	a := FlowApproveCmd.Flags()
	a.StringVar(&flowApproveStep, "step", "", "Step ID to approve (required)")
	a.StringVar(&flowApproveBy, "by", "", "Approver identity (default: current user)")
	a.StringVar(&flowApproveNote, "note", "", "Optional note recorded in audit")
	a.StringVar(&flowApprovalsRoot, "approvals-root", "",
		"Override approvals store root (default: $TLC_DATA_PATH/approvals)")

	r := FlowRejectCmd.Flags()
	r.StringVar(&flowRejectStep, "step", "", "Step ID to reject (required)")
	r.StringVar(&flowRejectBy, "by", "", "Rejecter identity (default: current user)")
	r.StringVar(&flowRejectReason, "reason", "", "Reason for rejection (required)")
	r.StringVar(&flowApprovalsRoot, "approvals-root", "",
		"Override approvals store root")

	c := FlowCancelCmd.Flags()
	c.StringVar(&flowCancelBy, "by", "", "Canceller identity (default: current user)")
	c.StringVar(&flowApprovalsRoot, "approvals-root", "",
		"Override approvals store root")

	FlowCmd.AddCommand(FlowApproveCmd)
	FlowCmd.AddCommand(FlowRejectCmd)
	FlowCmd.AddCommand(FlowCancelCmd)
}
