package cli

import (
	"github.com/spf13/cobra"
	kitcli "hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// registerHints wires contextual next-step hints for key commands.
// Hints render automatically via the PersistentPostRunE on the root.
func registerHints(hints *output.HintSet) {
	// Task hints — scoped to "task <verb>" to avoid collisions.
	hints.Register("task create", output.Hint{
		Message: "Run `tlc task claim <id>` to start working on it.",
	})
	hints.Register("task complete", output.Hint{
		Message: "Run `tlc task list` to see remaining tasks.",
	})
	hints.Register("task claim", output.Hint{
		Message: "Run `tlc task complete <id>` when finished.",
	})

	// Track hints.
	hints.Register("track create", output.Hint{
		Message: "Run `tlc track update <id> --add-plan plan.md` to link a plan.",
	})
	hints.Register("track show", output.Hint{
		Message: "Run `tlc task list --track <id>` to see linked tasks.",
	})

	// Upgrade hints (standard kit pattern).
	output.RegisterUpgradeHints(hints, "tlc", &upgradePerformed)
}

// upgradePerformed is set to true when the upgrade command succeeds.
// Used by RegisterUpgradeHints to conditionally show the verify hint.
var upgradePerformed bool

// renderPostRunHintsFor renders contextual hints after command output.
// Called from the root PersistentPostRunE. Takes root explicitly to
// avoid an init cycle with kitRootInstance.
func renderPostRunHintsFor(cmd *cobra.Command, root *kitcli.Root) {
	format := root.Viper.GetString("format")

	// Build the command path key for hint lookup.
	// For "tlc task create" the cmd.Name() is "create" — match on that.
	// Also try the parent+child combo for namespaced hints.
	keys := []string{cmd.Name()}
	if cmd.Parent() != nil && cmd.Parent() != root.Cmd {
		keys = append(keys, cmd.Parent().Name()+" "+cmd.Name())
	}

	for _, key := range keys {
		registered := root.Hints.Lookup(key)
		if len(registered) == 0 {
			continue
		}
		output.RenderHints(
			cmd.OutOrStdout(),
			registered,
			format,
			root.Viper,
			root.Theme.Muted,
		)
	}
}
