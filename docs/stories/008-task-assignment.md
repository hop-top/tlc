---
status: shipped
---

# 008 - Task Assignment and Unassign

**ID**: 008
**Feature**: Task Management
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md), [AI Agent](../personas/ai-agent.md)
**Priority**: P1

## Story

Before this story, a Team Lead distributing work must use `tlc task update --assigned-to`,
which changes ownership silently with no paper trail — future reviewers cannot tell who
reassigned what or why.
After: `tlc task assign` records a `REASSIGNED` log entry on every handoff;
`tlc task unassign --note` forces a written rationale, so the history always explains
why a task became ownerless. Team capacity planning and retrospectives become honest.

## Acceptance Scenarios

1. **Given** a task `T-0001` with status `TODO` and no assignee, **When** I run
   `tlc task assign engineer-1 T-0001`, **Then**:
   - `AssignedTo` is set to `engineer-1`
   - Status remains `TODO` (assign does not transition status unlike claim)
   - Output confirms "Assigned task T-0001 to engineer-1"

2. **Given** a task `T-0001` assigned to `engineer-1`, **When** I run
   `tlc task assign engineer-2 T-0001 --note "Reassigning for review"`, **Then**:
   - `AssignedTo` is updated to `engineer-2`
   - A `REASSIGNED` log entry exists for this task

3. **Given** a task `T-0001` assigned to `engineer-1` with status `IN_PROGRESS`,
   **When** I run `tlc task unassign T-0001 --note "Blocked on dependency"`, **Then**:
   - `AssignedTo` is cleared (nil)
   - Status remains `IN_PROGRESS` (unassign does not change status)
   - The note is appended to the task description
   - A `REASSIGNED` log entry with the note exists

4. **Given** a task `T-0001`, **When** I run `tlc task unassign T-0001` without `--note`,
   **Then** the command exits with an error mentioning `--note` is required.

5. **Given** no task `T-9999` exists, **When** I run `tlc task assign engineer-1 T-9999`,
   **Then** the command returns a "not found" error.

6. **Given** tasks `T-0001`, `T-0002` exist, **When** I run
   `tlc task assign engineer-1 T-0001 T-0002`, **Then** both tasks are assigned to
   `engineer-1`; status unchanged on each.

7. **Given** tasks matching `'T-001\d'` exist, **When** I run
   `tlc task assign engineer-2 'T-001\d' --no-prompt`, **Then** all matched tasks are
   assigned to `engineer-2` without a confirmation prompt.

8. **Given** tasks `T-0001`, `T-0002` are assigned, **When** I run
   `tlc task unassign T-0001 T-0002 --note "Releasing for triage"`, **Then** both are
   unassigned; note appended to each description.

9. **Given** tasks matching `'T-001\d'` exist with assignees, **When** I run
   `tlc task unassign 'T-001\d' --note "Batch release" --no-prompt`, **Then** all matched
   tasks are unassigned without a prompt.

## Tests

### E2E
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAssign/AssignTask`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAssign/AssignTaskWithNote`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAssign/AssignTaskNotFound`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskUnassign`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskUnassignRequiresNote`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskUnassignAppendsNoteToDescription`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskUnassignNotFound`

### Unit
- (fully covered by e2e tests above)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Assign sets assignee; status unchanged | `TestTaskAssign/AssignTask` | ✅ COVERED |
| 2 | Reassign logs REASSIGNED entry | `TestTaskAssign/AssignTaskWithNote` | ✅ COVERED |
| 3 | Unassign clears assignee; note appended | `TestTaskUnassign` / `TestTaskUnassignAppendsNoteToDescription` | ✅ COVERED |
| 4 | Unassign requires `--note` | `TestTaskUnassignRequiresNote` | ✅ COVERED |
| 5 | Assign to non-existent task errors | `TestTaskAssign/AssignTaskNotFound` | ✅ COVERED |
| 6 | Multi-ID assign (assignee first) | `TestBatchAssign` | ✅ COVERED |
| 7 | Regex assign + `--no-prompt` | `TestBatchE2E_AssignRegexNoPrompt` | ✅ COVERED |
| 8 | Multi-ID unassign with note | `TestBatchUnassign` | ✅ COVERED |
| 9 | Regex unassign + `--no-prompt` | `TestBatchUnassign` | ✅ COVERED |
