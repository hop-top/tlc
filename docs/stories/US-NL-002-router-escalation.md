# US-NL-002 - Router Escalation

**ID**: US-NL-002
**Feature**: NL Prompt
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md)
**Priority**: P1

## Story

As a Solo Developer, I want to type a complex natural language
prompt like `tlc "create 3 tasks for auth: login, logout,
refresh"` and have the LLM router resolve it to multiple
commands, so that I can batch-create tasks without manual
repetition.

## Acceptance Scenarios

1. **Given** TLC is initialized and LLM provider configured,
   **When** I run
   `tlc "create 3 tasks for auth: login, logout, refresh"`,
   **Then** classifier misses, kit/llm router resolves 3 create
   commands with confidence >= 0.9, executor auto-runs all,
   and output lists:
   ```
   Created T-XXXX: Login flow
   Created T-XXXX: Logout flow
   Created T-XXXX: Refresh token flow
   ```

2. **Given** LLM provider configured,
   **When** router returns confidence 0.75 for a multi-create,
   **Then** executor shows resolved commands and waits for
   user confirmation before executing.

3. **Given** no LLM provider configured,
   **When** classifier misses on a complex prompt,
   **Then** output prints:
   `no LLM provider configured; use explicit commands or set
   TLC_PROMPT_LLM (e.g. ollama://llama3.2)`

4. **Given** LLM provider configured,
   **When** I run multi-create with `--dry-run`,
   **Then** all resolved commands are printed, none executed.

5. **Given** LLM provider configured and 3 commands resolved,
   **When** second command fails during execution,
   **Then** output shows first completed, second failed with
   error, and third listed as not executed.

## Tests

### E2E
- `tests/integration/task_prompt_test.go`
  - `TestNLPromptRouterMultiCreate`
  - `TestNLPromptNoLLMProvider`

### Unit
- `internal/cli/prompt_router_test.go`
  - `TestRouter_MultiCommandParsing`
  - `TestRouter_MalformedJSON`
  - `TestRouter_NoProvider`
- `internal/cli/prompt_exec_test.go`
  - `TestExecutor_MidConfidence_Confirm`
  - `TestExecutor_PartialFailure`
