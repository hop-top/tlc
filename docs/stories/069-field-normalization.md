# 069 - Case-Insensitive + Alias + Fuzzy Field Matching

**ID**: 069
**Feature**: Task Management — Filter UX
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

Before this story, `--status` and `--priority` flags required exact uppercase values
(e.g. `DONE`, `P1`). After: inputs are normalized via case-insensitive lookup, built-in
aliases, and fuzzy matching — reducing friction for both humans and agents.

## Acceptance Scenarios

1. **Given** `tlc task list --status done`, **When** I run the command,
   **Then** it returns the same results as `--status DONE`.

2. **Given** `tlc task list --status complete`, **When** I run the command,
   **Then** it resolves `complete → DONE` and returns done tasks.

3. **Given** `tlc task list --status comp` (fuzzy), **When** I run the command,
   **Then** it fuzzy-matches `comp → complete → DONE` and returns done tasks.

4. **Given** `tlc task list --priority high`, **When** I run the command,
   **Then** it resolves `high → P1` and returns P1 priority tasks.

5. **Given** `tlc task list --status xyz` (unrecognized), **When** I run the command,
   **Then** it returns an error `unknown status "xyz"`.

## Tests

### E2E
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_StatusLowercase`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_StatusAlias`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_PriorityAlias`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UnknownStatus`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | lowercase status accepted | `TestFieldNorm_E2E_StatusLowercase` | ✅ COVERED |
| 2 | alias resolves to canonical | `TestFieldNorm_E2E_StatusAlias` | ✅ COVERED |
| 3 | fuzzy match resolves to canonical | `TestFieldNorm_E2E_StatusAlias` | ✅ COVERED |
| 4 | priority alias resolves | `TestFieldNorm_E2E_PriorityAlias` | ✅ COVERED |
| 5 | unknown input returns error | `TestFieldNorm_E2E_UnknownStatus` | ✅ COVERED |
