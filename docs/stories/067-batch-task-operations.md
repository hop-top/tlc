# 067 - Batch Task Operations

**ID**: 067
**Feature**: Task Management
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Team Lead](../personas/team-lead.md), [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

Before this story, acting on many tasks requires one command per task — tedious and error-prone
when bulk-transitioning a sprint, releasing a feature, or reassigning a whole workstream.
After: `tlc task <cmd>` accepts multiple IDs and/or regex patterns on every lifecycle command;
a single invocation fans out across all matching tasks. Confirmation guards prevent accidents;
`--no-prompt` enables scriptable, headless automation.

## Acceptance Scenarios

### Multiple exact IDs

1. **Given** tasks `T-0001`, `T-0002`, `T-0003` exist with status `TODO`, **When** I run
   `tlc task claim T-0001 T-0002 T-0003`, **Then** all three tasks transition to `IN_PROGRESS`
   and output lists each task claimed.

2. **Given** tasks `T-0001`, `T-0002` are `IN_PROGRESS`, **When** I run
   `tlc task complete T-0001 T-0002 --no-verify`, **Then** both transition to `DONE`.

3. **Given** tasks `T-0001`, `T-0002` are `DONE`, **When** I run
   `tlc task reopen T-0001 T-0002 --note "Tests regressed"`, **Then** both return to `TODO`
   with note appended to each description.

### Regex patterns

4. **Given** tasks `T-0010`, `T-0011`, `T-0019` exist, **When** I run
   `tlc task claim 'T-001\d'`, **Then** all three tasks are claimed; output lists each.

5. **Given** `*` glob passed, **When** I run `tlc task unclaim '*' --no-prompt`, **Then**
   all `IN_PROGRESS` tasks owned by `$USER` are unclaimed.

6. **Given** a regex matches more than one task and `--no-prompt` is absent, **When** the
   command runs in a TTY, **Then** a confirmation prompt lists the matched tasks and asks
   for `y/N`; declining aborts with no mutation.

7. **Given** a regex matches more than one task, **When** `--no-prompt` is passed,
   **Then** no confirmation prompt is shown; all matched tasks are processed.

### `assign` — assignee-first signature

8. **Given** tasks `T-0001`, `T-0002` are unassigned, **When** I run
   `tlc task assign alice T-0001 T-0002`, **Then** both tasks are assigned to `alice`; status
   unchanged.

9. **Given** tasks matching pattern `'auth-.*'` exist, **When** I run
   `tlc task assign bob 'auth-.*' --no-prompt`, **Then** all matched tasks are assigned to
   `bob` without a prompt.

### `update` — multi-target; `--title` blocked

10. **Given** tasks `T-0001`, `T-0002`, **When** I run
    `tlc task update T-0001 T-0002 --add-tag hotfix`, **Then** `hotfix` tag is added to both.

11. **Given** multiple IDs passed to `update`, **When** `--title` is also supplied,
    **Then** the command exits with an error: title cannot be set on multiple targets.

### `delete` — requires confirmation for >1

12. **Given** tasks `T-0001`, `T-0002` exist, **When** I run
    `tlc task delete T-0001 T-0002` in a non-TTY without `--yes` or `--no-prompt`, **Then**
    the command exits with an error requiring explicit confirmation.

13. **Given** tasks `T-0001`, `T-0002` exist, **When** I run
    `tlc task delete T-0001 T-0002 --yes`, **Then** both tasks are permanently removed.

14. **Given** tasks `T-0001`, `T-0002` exist, **When** I run
    `tlc task delete T-0001 T-0002 --no-prompt`, **Then** both tasks are removed without prompt.

### `--no-prompt` persistent flag

15. **Given** `--no-prompt` is passed on the `task` parent command, **Then** it applies to any
    subcommand in the chain (persistent flag, not per-subcommand).

### Mixed valid/invalid IDs

16. **Given** `T-0001` exists and `T-9999` does not, **When** I run
    `tlc task claim T-0001 T-9999`, **Then** `T-0001` is claimed, `T-9999` returns a
    "not found" error; the command exits non-zero.

## Tests

### E2E
- ✅ `internal/cli/task_lifecycle_test.go` — `TestBatchClaim`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestBatchComplete`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestBatchReopen`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestBatchUnclaim`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestBatchAssign`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestBatchUnassign`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestBatchUpdateMultiTag`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestBatchUpdateTitleBlocked`
- ✅ `internal/cli/task_batch_e2e_test.go`  — `TestBatchE2E_CompleteGlob`
- ✅ `internal/cli/task_batch_e2e_test.go`  — `TestBatchE2E_AssignRegexNoPrompt`
- ✅ `internal/cli/task_batch_e2e_test.go`  — `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt`

### Unit
- ✅ `internal/cli/task_resolve_test.go` — pattern detection, resolution, confirmation logic

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Multiple IDs claimed in one command | `TestBatchClaim` | ✅ COVERED |
| 2 | Multiple IDs completed | `TestBatchComplete` | ✅ COVERED |
| 3 | Multiple IDs reopened with note | `TestBatchReopen` | ✅ COVERED |
| 4 | Regex pattern matches and claims | `TestBatchClaim` | ✅ COVERED |
| 5 | `*` glob unclaims all | `TestBatchE2E_CompleteGlob` | ✅ COVERED |
| 6 | Pattern >1 match prompts in TTY | `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt` | ✅ COVERED |
| 7 | `--no-prompt` skips confirmation | `TestBatchE2E_AssignRegexNoPrompt` | ✅ COVERED |
| 8 | `assign` assignee-first; two IDs | `TestBatchAssign` | ✅ COVERED |
| 9 | `assign` regex + `--no-prompt` | `TestBatchE2E_AssignRegexNoPrompt` | ✅ COVERED |
| 10 | `update` adds tag to multiple IDs | `TestBatchUpdateMultiTag` | ✅ COVERED |
| 11 | `update --title` blocked for multi | `TestBatchUpdateTitleBlocked` | ✅ COVERED |
| 12 | `delete` multi without `--yes` errors | `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt` | ✅ COVERED |
| 13 | `delete` multi with `--yes` removes | `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt` | ✅ COVERED |
| 14 | `delete` with `--no-prompt` removes | `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt` | ✅ COVERED |
| 15 | `--no-prompt` is persistent on `task` | `task_list_test.go/NoPromptFlagExists` | ✅ COVERED |
| 16 | Mixed valid/invalid IDs; partial success | `TestBatchClaim` (mixed variant) | ✅ COVERED |
