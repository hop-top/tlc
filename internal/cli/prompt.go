package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	taskNLExecute bool
	taskNLDryRun  bool
	taskNLJSON    bool
)

func init() {
	// NL flags live on RootCmd so `tlc "complete T-42"` works.
	RootCmd.Flags().BoolVarP(&taskNLExecute, "execute", "x", false, "Execute resolved commands without confirmation")
	RootCmd.Flags().BoolVar(&taskNLDryRun, "dry-run", false, "Show resolved commands without executing")
	RootCmd.Flags().BoolVar(&taskNLJSON, "json", false, "Output resolved commands as JSON")
}

// routePromptFn is the function used to call the LLM router.
// Package-level variable so tests can replace it.
var routePromptFn = routePrompt

// runNLPrompt is the RunE handler wired to RootCmd when args are present.
// It treats the args as a natural-language prompt and runs the
// classify -> route -> execute pipeline.
func runNLPrompt(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}

	prompt := strings.Join(args, " ")

	// Stage 1: regex-based classifier (fast, deterministic, task-domain only).
	cmds := ClassifyPrompt(prompt)
	if cmds != nil {
		return handleResolvedCommands(cmd, cmds, prompt)
	}

	// Stage 2: cross-domain NLP classifier (no LLM).
	cmds = ClassifyPromptCrossDomain(prompt)
	if cmds != nil {
		return handleResolvedCommands(cmd, cmds, prompt)
	}

	// Stage 3: LLM-backed router (slower, requires schema).
	schemaJSON, err := GenerateSchemaJSON(RootCmd)
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	cmds, clarification, err := routePromptFn(cmd.Context(), prompt, schemaJSON)
	if err != nil {
		return fmt.Errorf("could not resolve prompt %q; %s", prompt, err)
	}

	if clarification != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Clarification: %s\n", clarification)
	}

	return handleResolvedCommands(cmd, cmds, prompt)
}

// handleResolvedCommands dispatches resolved commands based on the active
// flags (--json, --dry-run, --execute).
func handleResolvedCommands(cmd *cobra.Command, cmds []ResolvedCommand, prompt string) error {
	if len(cmds) == 0 {
		return fmt.Errorf("no commands resolved from prompt %q", prompt)
	}

	// --json: emit JSON array and exit.
	if taskNLJSON {
		return renderResolvedJSON(cmd, cmds)
	}

	ctx := cmd.Context()
	opts := ExecuteOptions{
		DryRun:   taskNLDryRun,
		Execute:  taskNLExecute,
		NoPrompt: taskNoPrompt,
		Writer:   cmd.OutOrStdout(),
	}

	result := executeCommands(ctx, cmds, opts)

	if result.NeedsClarification {
		// Attempt inline clarification when stdin is interactive.
		cr, err := clarifyInline(ctx, prompt, result.Clarification, cmd.OutOrStdout(), cmd.InOrStdin())
		if err != nil {
			return err
		}
		if cr.Aborted {
			return fmt.Errorf("aborted; rephrase or use explicit commands")
		}
		result = executeCommands(ctx, cr.Commands, opts)
	}

	if result.Error != nil {
		return result.Error
	}
	return nil
}

// renderResolvedJSON writes the resolved commands as a JSON array.
func renderResolvedJSON(cmd *cobra.Command, cmds []ResolvedCommand) error {
	data, err := json.MarshalIndent(cmds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal resolved commands: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
