// Package cli — confirm bridge.
//
// Kit 12fcc-leak auto-enforces a --confirm policy on every command
// annotated kit/side-effect=destructive* (see kit/go/console/cli/
// policy_runE.go). In a non-TTY context the kit gate refuses the
// operation unless --confirm=yes is passed. Several tlc destructive
// commands shipped with their own local "skip the prompt" flags
// (--yes / --no-prompt / --force) before the kit gate existed.
//
// Strategy C (hybrid): keep the local flags as silent aliases for
// --confirm=yes. A PreRunE on each affected command translates the
// local boolean into a --confirm=yes setting before kit's RunE
// wrapper runs the policy gate. The local flag's existing
// downstream semantics (skipping the in-RunE huh/readConfirm prompt)
// continue to work because the global --confirm is now also "yes".
//
// This keeps scripts using `tlc task delete --yes`, `tlc track
// abandon --no-prompt`, `tlc project prune -y`, etc. working as
// before, with zero deprecation noise. Operators who prefer the
// kit-canonical surface can pass --confirm=yes directly.
//
// See docs/12fcc-conformance-split-plan.md (§ Phase 4) for the full
// bridge table.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// isFlagTrue reports whether the named bool flag is registered on
// cmd (or an inherited ancestor) AND parsed to true.
func isFlagTrue(cmd *cobra.Command, name string) bool {
	f := cmd.Flag(name)
	if f == nil {
		return false
	}
	return f.Value.String() == "true"
}

// setConfirmYes walks up to the root and sets the persistent
// --confirm flag to "yes". When the flag isn't registered (e.g.
// kit policy globals were disabled) the call is a no-op.
func setConfirmYes(cmd *cobra.Command) error {
	for c := cmd; c != nil; c = c.Parent() {
		if f := c.PersistentFlags().Lookup("confirm"); f != nil {
			if err := f.Value.Set("yes"); err != nil {
				return fmt.Errorf("set --confirm: %w", err)
			}
			return nil
		}
	}
	return nil
}

// installConfirmBridge attaches a PreRunE to cmd that flips the
// persistent --confirm to "yes" whenever any of the supplied local
// flag NAMES is set to true. Preserves an existing PreRunE if any
// — the bridge runs first, then the original.
//
// Idempotent: calling twice with the same flag list re-wires the
// PreRunE but produces equivalent behaviour. Safe to call from
// init() blocks.
func installConfirmBridge(cmd *cobra.Command, localFlags ...string) {
	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		for _, name := range localFlags {
			if isFlagTrue(c, name) {
				if err := setConfirmYes(c); err != nil {
					return err
				}
				break
			}
		}
		if prev != nil {
			return prev(c, args)
		}
		return nil
	}
}
