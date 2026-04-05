# US-NL-001 - NL Prompt Happy Path

**ID**: US-NL-001
**Feature**: NL Prompt
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI Agent, I want to type a natural language prompt like
`tlc task "complete T-42"` and have the classifier resolve it
to the correct command, so that I avoid memorizing exact syntax.

## Acceptance Scenarios

1. **Given** task T-0042 exists with status IN_PROGRESS,
   **When** I run `tlc task "complete T-42"`,
   **Then** classifier matches `task complete T-0042`
   with confidence 1.0, executor auto-runs, and output
   confirms `Completed task T-0042`.

2. **Given** task T-0042 exists,
   **When** I run `tlc task "mark T-42 done"`,
   **Then** classifier matches `task complete T-0042`
   with confidence 1.0 and task transitions to DONE.

3. **Given** task T-0042 exists,
   **When** I run `tlc task "show T-42"`,
   **Then** classifier matches `task show T-0042`
   and task detail is printed.

4. **Given** task T-0042 exists,
   **When** I run `tlc task "complete T-42" --dry-run`,
   **Then** resolved command is printed but not executed.

5. **Given** task T-0042 exists,
   **When** I run `tlc task "complete T-42" --json`,
   **Then** resolved command is returned as JSON with
   `cmd`, `args`, and `confidence` fields.

## Tests

### E2E
- `tests/integration/task_prompt_test.go`
  - `TestNLPromptClassifierHit`
  - `TestNLPromptDryRun`

### Unit
- `internal/cli/task_prompt_classify_test.go`
  - `TestClassify_CompleteSyntax`
  - `TestClassify_MarkDone`
  - `TestClassify_Show`
- `internal/cli/task_prompt_exec_test.go`
  - `TestExecutor_HighConfidence_AutoRun`
