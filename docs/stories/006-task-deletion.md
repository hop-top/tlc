# 006 - Task Deletion

**ID**: 006
**Feature**: Task Management
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [Team Lead](../personas/team-lead.md)
**Priority**: P2

## Story

Before this story, a Solo Developer who needs to discard a duplicate or mis-filed task must
either mark it `SKIPPED` (polluting the backlog) or hunt through data files manually.
After: `tlc task delete` removes the task cleanly; the interactive confirmation prevents
accidents in the common case, while `--yes` enables scriptable automation without prompts.
The backlog stays uncluttered; automation pipelines stay unblocked.

## Acceptance Scenarios

1. **Given** a task `T-0001` exists, **When** I run `tlc task delete T-0001 --yes`,
   **Then** the task is permanently removed from storage and a confirmation message is
   printed; subsequent `tlc task show T-0001` returns "not found".

2. **Given** a task `T-0001` exists, **When** I run `tlc task delete T-0001` without
   `--yes` in a non-interactive context, **Then** the command exits with an error
   (requires explicit confirmation).

3. **Given** no task `T-9999` exists, **When** I run `tlc task delete T-9999 --yes`,
   **Then** the command returns a "not found" error.

4. **Given** a task `T-0001` exists, **When** I run `tlc task delete T-0001 -y`
   (short flag), **Then** the task is deleted (same behaviour as `--yes`).

5. **Given** tasks `T-0001`, `T-0002` exist, **When** I run
   `tlc task delete T-0001 T-0002 --yes`, **Then** both tasks are permanently removed.

6. **Given** tasks `T-0001`, `T-0002` exist, **When** I run
   `tlc task delete T-0001 T-0002` without `--yes` or `--no-prompt` in a non-TTY,
   **Then** the command exits with an error requiring explicit confirmation.

7. **Given** tasks `T-0001`, `T-0002` exist, **When** I run
   `tlc task delete T-0001 T-0002 --no-prompt`, **Then** both tasks are removed without prompt.

## Tests

### E2E
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskDelete`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskDeleteYesShortFlag`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskDeleteNotFound`

### Unit
- (no additional unit tests required beyond e2e coverage)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Delete with `--yes` removes task | `TestTaskDelete` | ✅ COVERED |
| 2 | Delete without `--yes` fails | `TestTaskDeleteRequiresYes` | ✅ COVERED |
| 3 | Delete non-existent task errors | `TestTaskDeleteNotFound` | ✅ COVERED |
| 4 | Short `-y` flag works | `TestTaskDeleteYesShortFlag` | ✅ COVERED |
| 5 | Multi-ID delete with `--yes` | `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt` | ✅ COVERED |
| 6 | Multi-ID delete without `--yes` errors | `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt` | ✅ COVERED |
| 7 | Multi-ID delete with `--no-prompt` | `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt` | ✅ COVERED |
