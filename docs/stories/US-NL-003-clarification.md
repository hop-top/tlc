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
   **Then** router returns confidence < 0.7, clarifier
   prompts:
   `Which task? T-0042 (JWT refresh) or T-0068 (login page)`

2. **Given** clarification prompt is shown,
   **When** I answer `42`,
   **Then** combined context re-sent to router, confidence
   resolves to 1.0, and `task complete T-0042` executes.

3. **Given** a complex generative prompt triggers
   `multi_turn: true` in router response,
   **When** clarifier enters REPL mode,
   **Then** user can refine across multiple turns and exit
   with `ctrl+c` or `quit`.

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
