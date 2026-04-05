# US-NL-003 - Clarification

**ID**: US-NL-003
**Feature**: NL Prompt
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md)
**Priority**: P2

## Story

As a Solo Developer, I want ambiguous prompts like
`tlc task "mark the auth task done"` to trigger inline
clarification with candidate tasks, so that I resolve
the right task without re-typing the command.

## Acceptance Scenarios

1. **Given** tasks T-0042 (JWT refresh) and T-0068 (login
   page) both match "auth",
   **When** I run `tlc task "mark the auth task done"`,
   **Then** classifier returns low confidence, inline
   clarifier prompts a follow-up question and waits for
   user input.

2. **Given** inline clarification prompt is shown,
   **When** I answer `42`,
   **Then** combined context (original + answer) is
   re-classified locally via `ClassifyPrompt`, resolves
   to `task complete T-0042`, and executes.

3. **Given** user explicitly enters REPL mode (multi-turn
   clarification),
   **When** each turn is entered,
   **Then** accumulated context is re-classified locally
   via `ClassifyPrompt`; user exits with `ctrl+c`, `quit`,
   or `exit`.

4. **Given** destructive command resolved (e.g. delete),
   **When** confidence is 1.0,
   **Then** executor still prompts for confirmation.
   `--no-prompt` overrides this guard.

## Tests

### E2E
- `tests/integration/task_prompt_test.go`
  - `TestNLPromptClarification`
  - `TestNLPromptDestructiveGuard`

### Unit
- `internal/cli/task_prompt_clarify_test.go`
  - `TestClarify_InlineDisambiguation`
  - `TestClarify_REPLMode`
- `internal/cli/task_prompt_exec_test.go`
  - `TestExecutor_LowConfidence_Clarify`
  - `TestExecutor_DestructiveAlwaysConfirm`
