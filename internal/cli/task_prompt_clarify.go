package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// ClarifyResult holds the outcome of a clarification interaction.
type ClarifyResult struct {
	// Resolved commands after clarification (nil if user aborted).
	Commands []ResolvedCommand
	// Aborted is true if the user cancelled the interaction.
	Aborted bool
}

// clarifyInline asks a single follow-up question and re-classifies
// the combined context. Used when the router returns a disambiguation
// question (low confidence, single ambiguity).
func clarifyInline(
	ctx context.Context,
	originalPrompt string,
	question string,
	w io.Writer,
	r io.Reader,
) (ClarifyResult, error) {
	_, _ = fmt.Fprintf(w, "Clarification needed: %s\n> ", question)

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return ClarifyResult{Aborted: true}, nil
	}
	answer := strings.TrimSpace(scanner.Text())
	if answer == "" {
		return ClarifyResult{Aborted: true}, nil
	}

	// Re-classify with combined context.
	combined := originalPrompt + " " + answer
	cmds := ClassifyPrompt(combined)
	if cmds != nil {
		return ClarifyResult{Commands: cmds}, nil
	}

	// If classifier still misses, try the answer alone (e.g. user
	// replied with just a task ID like "42").
	cmds = ClassifyPrompt(answer)
	if cmds != nil {
		return ClarifyResult{Commands: cmds}, nil
	}

	return ClarifyResult{}, fmt.Errorf(
		"could not resolve after clarification; use explicit commands",
	)
}

// clarifyREPL runs a multi-turn conversational loop for complex
// prompts. Each turn re-classifies accumulated context. User exits
// with ctrl+c, "quit", or "exit".
func clarifyREPL(
	ctx context.Context,
	originalPrompt string,
	w io.Writer,
	r io.Reader,
) (ClarifyResult, error) {
	_, _ = fmt.Fprintf(w, "Multi-turn clarification (type 'quit' to exit):\n")
	_, _ = fmt.Fprintf(w, "Original: %s\n", originalPrompt)

	scanner := bufio.NewScanner(r)
	accumulated := originalPrompt

	for {
		_, _ = fmt.Fprint(w, "> ")
		if !scanner.Scan() {
			// EOF / ctrl+c
			return ClarifyResult{Aborted: true}, nil
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "quit" || input == "exit" {
			return ClarifyResult{Aborted: true}, nil
		}

		accumulated = accumulated + " " + input

		// Try classifier with accumulated context.
		cmds := ClassifyPrompt(accumulated)
		if cmds != nil {
			return ClarifyResult{Commands: cmds}, nil
		}

		// Try just the latest input.
		cmds = ClassifyPrompt(input)
		if cmds != nil {
			return ClarifyResult{Commands: cmds}, nil
		}

		_, _ = fmt.Fprintf(w, "Still unclear. Add more detail or type 'quit'.\n")
	}
}
