# 005 - Task Update

**ID**: 005
**Feature**: Task Management
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [Team Lead](../personas/team-lead.md), [AI Agent](../personas/ai-agent.md)
**Priority**: P1

## Story

Before this story, a Solo Developer who discovers a misnamed task, needs to add a tag,
or wants to hand off ownership must delete and recreate the task — losing all history.
After: `tlc task update` lets them amend any field in-place, leaving the audit trail intact.
Their backlog stays accurate; no phantom duplicates.

## Acceptance Scenarios

1. **Given** a task `T-0001` with title "Fxi auth bug", **When** I run
   `tlc task update T-0001 --title "Fix auth bug"`, **Then** the task title is updated and
   all other fields are preserved unchanged.

2. **Given** a task `T-0001` with description "Draft.", **When** I run
   `tlc task update T-0001 --description "Detailed spec."`, **Then** the description is
   replaced with "Detailed spec." (not appended).

3. **Given** a task `T-0001` with tags `["bug"]`, **When** I run
   `tlc task update T-0001 --add-tag urgent`, **Then** the task has tags `["bug", "urgent"]`.

4. **Given** a task `T-0001` with tags `["chore", "bug"]`, **When** I run
   `tlc task update T-0001 --remove-tag chore`, **Then** the task has tags `["bug"]` only.

5. **Given** a task `T-0001` assigned to `engineer-1`, **When** I run
   `tlc task update T-0001 --assigned-to engineer-2`, **Then** the assignee is `engineer-2`.

6. **Given** a task `T-0001` assigned to `engineer-1`, **When** I run
   `tlc task update T-0001 --assigned-to -` (or `--assigned-to null`), **Then** the
   assignee field is cleared (nil).

7. **Given** a task `T-0001` with status `TODO`, **When** I run
   `tlc task update T-0001 --status IN_PROGRESS`, **Then** status transitions to
   `IN_PROGRESS` following the state machine rules.

8. **Given** a task `T-0001` with status `DONE`, **When** I run
   `tlc task update T-0001 --status TODO --force`, **Then** status is set to `TODO`
   (force bypasses terminal-state guard).

## Tests

### E2E
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateCommands/UpdateTask`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateCommands/UpdateAssignee`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateCommands/ClearAssigneeWithNull`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateCommands/ClearAssigneeWithDash`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateCommands/AddTagToTask`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateCommands/RemoveTagFromTask`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateAssignee`
- ✅ `internal/cli/task_update_test.go` — `TestTaskClearAssigneeNull`
- ✅ `internal/cli/task_update_test.go` — `TestTaskClearAssigneeDash`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdate`
- ✅ `internal/cli/task_update_test.go` — `TestTaskRemoveTag`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskUpdateTitle`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskUpdateDescription`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskUpdateForceStatus`

### Unit
- ✅ `internal/core/task_test.go` — `TestTask_Transition` (state machine rules)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Update title in-place | `TestTaskUpdateTitle` | ✅ COVERED |
| 2 | Replace description | `TestTaskUpdateDescription` | ✅ COVERED |
| 3 | Add tag, preserve existing | `TestTaskUpdateCommands/AddTagToTask` | ✅ COVERED |
| 4 | Remove specific tag | `TestTaskUpdateCommands/RemoveTagFromTask` | ✅ COVERED |
| 5 | Change assignee | `TestTaskUpdateCommands/UpdateAssignee` | ✅ COVERED |
| 6 | Clear assignee with `-`/`null` | `TestTaskClearAssigneeDash` / `TestTaskClearAssigneeNull` | ✅ COVERED |
| 7 | Status transition via update | `TestTaskUpdateCommands/UpdateTask` | ✅ COVERED |
| 8 | Force transition from terminal state | `TestTaskUpdateForceStatus` | ✅ COVERED |
