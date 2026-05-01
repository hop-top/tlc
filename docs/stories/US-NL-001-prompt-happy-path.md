---
status: shipped
---

# US-NL-001 - NL Prompt Happy Path

**ID**: US-NL-001
**Feature**: NL Prompt
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI Agent, I want to type a natural language prompt like
`tlc "complete T-42"` and have the classifier resolve it
to the correct command, so that I avoid memorizing exact syntax.

## Acceptance Scenarios

1. **Given** task T-0042 exists with status IN_PROGRESS,
   **When** I run `tlc "complete T-42"`,
   **Then** classifier matches `task complete T-0042`
   with confidence 1.0, executor auto-runs, and output
   confirms `Completed task T-0042`.

2. **Given** task T-0042 exists,
   **When** I run `tlc "mark T-42 done"`,
   **Then** classifier matches `task complete T-0042`
   with confidence 1.0 and task transitions to DONE.

3. **Given** task T-0042 exists,
   **When** I run `tlc "show T-42"`,
   **Then** classifier matches `task show T-0042`
   and task detail is printed.

4. **Given** task T-0042 exists,
   **When** I run `tlc "complete T-42" --dry-run`,
   **Then** resolved command is printed but not executed.

5. **Given** task T-0042 exists,
   **When** I run `tlc "complete T-42" --json`,
   **Then** resolved command is returned as JSON with
   `cmd`, `args`, and `confidence` fields.

## Tests

### E2E
- `internal/cli/prompt_e2e_test.go`
  - `TestTaskPromptE2E_ClassifyAndComplete`
  - `TestNLRootCmd_CrossDomainClassifier`

### Unit
- `internal/cli/prompt_classify_test.go`
  - `TestClassify_CompleteSyntax`
  - `TestClassify_MarkDone`
  - `TestClassify_Show`
- `internal/cli/prompt_exec_test.go`
  - `TestExecutor_HighConfidence_AutoRun`
