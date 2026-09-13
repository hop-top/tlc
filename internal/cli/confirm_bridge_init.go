// Package cli — wire the confirm bridge into every destructive leaf
// that has a pre-existing local "skip the prompt" flag.
//
// One init() centralises the binding so the 12-command map lives in
// a single place; each leaf's own init() stays focused on its
// domain flags.

package cli

func init() {
	// Task destructives. taskNoPrompt is a persistent flag on TaskCmd
	// inherited by every task subcommand; the bridge resolves it via
	// cobra flag inheritance.
	installConfirmBridge(TaskDeleteCmd, "yes", "no-prompt")
	installConfirmBridge(TaskUnassignCmd, "no-prompt")
	installConfirmBridge(TaskUnclaimCmd, "no-prompt")

	// Track destructives. trackAbandonCmd has --no-prompt; archive/
	// delete inherit nothing today, so the kit gate is their only
	// confirmation surface.
	installConfirmBridge(trackAbandonCmd, "no-prompt")

	// Orchestration destructives. agent cancel ships no local skip
	// flag — the kit --confirm gate is already its single source of
	// truth. Project prune has -y/--yes.
	installConfirmBridge(ProjectPruneCmd, "yes")

	// Admin destructives. alias remove and auth logout ship no local
	// skip flag; the kit --confirm gate suffices.
}
