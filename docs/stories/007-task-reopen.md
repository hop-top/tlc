# 007 - Task Reopen

**ID**: 007
**Feature**: Task Management
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md), [AI Agent](../personas/ai-agent.md)
**Priority**: P1

## Story

Before this story, a Team Lead who discovers a "DONE" task needs rework (failed CI, edge case
missed) must create a new task that has no connection to the original history — breaking
traceability and confusing the audit trail.
After: `tlc task reopen` pulls a terminal task back to `TODO`, appends the mandatory `--note`
as an audit entry in the description, and leaves every prior log intact. The audit trail reads
as a continuous narrative from first creation through every reopen, making regression root-cause
trivial.

## Acceptance Scenarios

1. **Given** a task `T-0001` with status `DONE`, **When** I run
   `tlc task reopen T-0001 --note "Tests failing after merge"`, **Then**:
   - Status transitions to `TODO`
   - The note is appended to the task description
   - All previous description content is preserved

2. **Given** a task `T-0001` with status `SKIPPED`, **When** I run
   `tlc task reopen T-0001 --note "Revived for next sprint"`, **Then** status transitions
   to `TODO` (works from any terminal state).

3. **Given** a task `T-0001` with status `DONE`, **When** I run `tlc task reopen T-0001`
   without `--note`, **Then** the command exits with an error mentioning `--note` is
   required.

4. **Given** a task `T-0001` that has been claimed, completed, then reopened, **When** I
   query its description, **Then** the audit trail shows the full lifecycle: creation,
   claim, completion, and reopen entries in chronological order.

5. **Given** no task `T-9999` exists, **When** I run
   `tlc task reopen T-9999 --note "Reopen missing"`, **Then** the command returns a
   "not found" error.

## Tests

### E2E
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskReopenRequiresNote`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskReopenAppendsNoteToDescription`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskReopen`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskReopenNotFound`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskReopenAuditContinuity`

### Unit
- ✅ `internal/core/task_test.go` — `TestTask_Transition` (terminal → TODO via force)
- ✅ `internal/cli/task_audit_test.go` — `TestAppendAuditLog_MultipleEntries`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Reopen DONE task; note appended | `TestTaskReopenAppendsNoteToDescription` | ✅ COVERED |
| 2 | Reopen from any terminal state | `TestTaskReopen` | ✅ COVERED |
| 3 | Note required; error if missing | `TestTaskReopenRequiresNote` | ✅ COVERED |
| 4 | Audit trail continuity | `TestTaskReopenAuditContinuity` | ✅ COVERED |
| 5 | Non-existent task errors | `TestTaskReopenNotFound` | ✅ COVERED |
