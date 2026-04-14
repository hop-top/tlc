package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// Confidence thresholds for command execution gating.
const (
	confidenceHigh   = 0.9
	confidenceMedium = 0.7
)

// ExecuteResult captures the outcome of a batch command execution.
type ExecuteResult struct {
	Executed           []ResolvedCommand // commands that ran successfully
	Failed             *ResolvedCommand  // command that failed (nil if all succeeded)
	Remaining          []ResolvedCommand // commands not yet run
	Error              error             // the failure error
	NeedsClarification bool              // true when confidence too low
	Clarification      string            // question to ask user
}

// ExecuteOptions controls execution behaviour.
type ExecuteOptions struct {
	DryRun   bool      // print resolved commands without executing
	Execute  bool      // force execute (skip confidence confirmation)
	NoPrompt bool      // skip all confirmation prompts
	Writer   io.Writer // output destination (defaults to os.Stdout)
	Reader   io.Reader // input source for confirmations (defaults to os.Stdin)
}

// runCommandFn is the function used to execute a single resolved command.
// Package-level variable so tests can replace it.
var runCommandFn = runCommand

// executeCommands runs resolved commands sequentially, gated by confidence.
func executeCommands(ctx context.Context, cmds []ResolvedCommand, opts ExecuteOptions) ExecuteResult {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	r := opts.Reader
	if r == nil {
		r = os.Stdin
	}

	if len(cmds) == 0 {
		return ExecuteResult{}
	}

	// Check for low-confidence commands first.
	for _, cmd := range cmds {
		if cmd.Confidence < confidenceMedium {
			return ExecuteResult{
				NeedsClarification: true,
				Clarification: fmt.Sprintf(
					"low confidence (%.0f%%) for %q; rephrase or be more specific",
					cmd.Confidence*100, formatCmd(cmd),
				),
				Remaining: cmds,
			}
		}
	}

	// Dry-run: print and return.
	if opts.DryRun {
		_, _ = fmt.Fprintf(w, "dry-run: %d command(s) resolved\n", len(cmds))
		for i, cmd := range cmds {
			_, _ = fmt.Fprintf(w, "  %d. %s %s\n", i+1, cmd.Cmd, strings.Join(cmd.Args, " "))
		}
		return ExecuteResult{Remaining: cmds}
	}

	// Medium confidence: ask for confirmation (unless --execute or --no-prompt).
	if !opts.Execute && !opts.NoPrompt {
		needsConfirm := false
		for _, cmd := range cmds {
			if cmd.Confidence < confidenceHigh {
				needsConfirm = true
				break
			}
		}
		if needsConfirm {
			_, _ = fmt.Fprintf(w, "resolved %d command(s):\n", len(cmds))
			for i, cmd := range cmds {
				_, _ = fmt.Fprintf(w, "  %d. %s %s (%.0f%%)\n",
					i+1, cmd.Cmd, strings.Join(cmd.Args, " "), cmd.Confidence*100)
			}
			_, _ = fmt.Fprintf(w, "proceed? [y/N] ")
			if !readConfirm(r) {
				return ExecuteResult{
					Error:     fmt.Errorf("aborted by user"),
					Remaining: cmds,
				}
			}
		}
	}

	// Execute sequentially with destructive guard.
	var executed []ResolvedCommand
	for i, cmd := range cmds {
		// Destructive guard: always confirm unless --no-prompt.
		if isDestructiveArgs(cmd.Args) && !opts.NoPrompt {
			_, _ = fmt.Fprintf(w, "destructive: %s %s — confirm? [y/N] ",
				cmd.Cmd, strings.Join(cmd.Args, " "))
			if !readConfirm(r) {
				return ExecuteResult{
					Executed:  executed,
					Failed:    &cmds[i],
					Error:     fmt.Errorf("destructive command rejected by user"),
					Remaining: cmds[i+1:],
				}
			}
		}

		if err := runCommandFn(ctx, cmd); err != nil {
			return ExecuteResult{
				Executed:  executed,
				Failed:    &cmds[i],
				Error:     err,
				Remaining: cmds[i+1:],
			}
		}
		executed = append(executed, cmd)
	}

	return ExecuteResult{Executed: executed}
}

// resetAllFlags resets all CLI flag state before dispatching a resolved command.
// Uses cobra's ResetFlags to clear pflag internal state (T-0231).
func resetAllFlags() {
	ResetCreateFlags()
	resetTaskFlags()
}

// runCommand executes a single resolved command via the RootCmd cobra tree,
// so any subcommand (task, track, flow, …) can be dispatched.
func runCommand(_ context.Context, cmd ResolvedCommand) error {
	resetAllFlags()
	RootCmd.SetArgs(append([]string{cmd.Cmd}, cmd.Args...))
	return RootCmd.Execute()
}

// formatCmd returns a human-readable representation of a resolved command.
func formatCmd(cmd ResolvedCommand) string {
	return cmd.Cmd + " " + strings.Join(cmd.Args, " ")
}

// readConfirmFn is the function used to read user confirmation.
// Package-level variable so tests can replace it.
var readConfirmFn = readConfirmFromReader

// readConfirm delegates to the replaceable readConfirmFn with the given reader.
func readConfirm(r io.Reader) bool {
	return readConfirmFn(r)
}

// readConfirmFromReader reads a single line from r and returns true for "y"/"yes".
func readConfirmFromReader(r io.Reader) bool {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false
	}
	ans := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return ans == "y" || ans == "yes"
}
