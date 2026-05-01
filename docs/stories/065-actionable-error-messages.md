---
status: shipped
---

# Actionable Error Messages

**ID**: 065
**Feature**: Error UX / Agent Ergonomics
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI agent, I want every TLC error to include a concrete next step,
so that I can self-recover without consulting documentation.

## Background

Agents operating on TLC are tool-driven — they cannot ask for help.
A bare `task not found` forces a retry loop or hallucination.
An actionable message (`…; run 'tlc task list'…`) closes the loop immediately.

## Acceptance Scenarios

1. **Given** a non-existent task ID, **When** `tlc task show T-XXXX` is run,
   **Then** the error contains the task ID AND the hint `tlc task list`.

2. **Given** a task in terminal status (DONE), **When** a transition is attempted,
   **Then** the error names the current status and offers `tlc task reopen`.

3. **Given** a command that requires `--note` (reopen, unassign), **When** it is
   run without `--note`, **Then** the error shows the exact re-run command with
   `--note "<reason>"` appended.

4. **Given** `tlc task delete <id>` without `--yes`, **When** run non-interactively,
   **Then** the error shows the exact re-run with `--yes`.

5. **Given** a project URI referencing an unregistered project, **When** resolved,
   **Then** the error names the project ID and suggests `tlc init`.

6. **Given** the actionable hint references a sibling Hop tool (wsm, aps),
   **When** that tool is absent, **Then** TLC still starts and the hint is omitted
   or degraded gracefully — no hard dependency.

## Implementation

### Sentinel error types

| Type | Package | Hint embedded |
|------|---------|---------------|
| `ErrTaskNotFound` | `internal/uri` | `tlc task list` |
| `ErrProjectNotFound` | `internal/uri` | `tlc init` |
| `ErrInvalidTransition` | `internal/core` | lists `Allowed []string` |

### CLI helpers (`internal/cli/errors.go`)

- `errNoteRequired(cmdHint)` — exact re-run with `--note "<reason>"`
- `errDeleteRequiresYes(taskID)` — exact re-run with `--yes`
- `errTerminalState(taskID, status)` — reopens via `tlc task reopen`
- `errTransitionNotAllowed(from, to, allowed)` — lists valid transitions

## Tests

### E2E

- `internal/cli/task_show_test.go` — `TestTaskShowNotFound`
  Runs `tlc task show T-9999` against an empty DB; asserts error contains
  `T-9999` AND `tlc task list`.

### Unit

- `internal/uri/errors_test.go` —
  `TestErrTaskNotFound_SimpleID`,
  `TestErrTaskNotFound_WithProject`,
  `TestErrProjectNotFound`
- `internal/core/task_error_test.go` — transition error with `Allowed` field
