# 009 - Task Audit Log

**ID**: 009
**Feature**: Task Management
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md), [Test Engineer](../personas/test-engineer.md)
**Priority**: P1

## Story

Before this story, a Team Lead who wants to understand why a task changed status must
read the raw description field, which may or may not contain formatted audit entries.
There is no way to query "what did engineer-1 do last week?" across all tasks.
After: `tlc log` exposes the structured audit table with filters for `--action`,
`--by`, and pagination via `--limit`/`--offset`. The team gets a queryable history
without leaving the terminal, enabling async accountability and fast incident response.

## Acceptance Scenarios

1. **Given** a task `T-0001` has been claimed and completed, **When** I run
   `tlc log T-0001`, **Then** I see at least two log entries (CLAIMED, DONE) with
   timestamps, actor, and action fields.

2. **Given** multiple tasks have logs, **When** I run `tlc log --all`, **Then** logs
   from all tasks are returned.

3. **Given** logs exist for actions `CLAIMED` and `DONE`, **When** I run
   `tlc log --all --action CLAIMED`, **Then** only `CLAIMED` entries are returned.

4. **Given** logs from actors `engineer-1` and `engineer-2`, **When** I run
   `tlc log --all --by engineer-1`, **Then** only entries by `engineer-1` are returned.

5. **Given** 15 log entries exist, **When** I run `tlc log --all --limit 5 --offset 5`,
   **Then** exactly 5 entries are returned starting from the 6th entry.

6. **Given** no task-id and no `--all` flag, **When** I run `tlc log`, **Then** the
   command returns an error asking for either a task-id or `--all`.

7. **Given** logs exist, **When** I run `tlc log --all --format json`, **Then** output
   is valid JSON containing log entry objects.

## Tests

### E2E
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAuditLog`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAuditLogAll`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAuditLogFilterByAction`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAuditLogFilterByActor`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAuditLogPagination`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAuditLogRequiresTaskIDOrAll`
- ✅ `internal/cli/task_lifecycle_test.go` — `TestTaskAuditLogJSONFormat`

### Unit
- ✅ `internal/cli/log_test.go` — `TestFormatLogAction`
- ✅ `internal/cli/task_audit_test.go` — `TestAppendAuditLog_*`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Log for task shows entries | `TestTaskAuditLog` | ✅ COVERED |
| 2 | `--all` returns all task logs | `TestTaskAuditLogAll` | ✅ COVERED |
| 3 | Filter by action | `TestTaskAuditLogFilterByAction` | ✅ COVERED |
| 4 | Filter by actor | `TestTaskAuditLogFilterByActor` | ✅ COVERED |
| 5 | Pagination with limit/offset | `TestTaskAuditLogPagination` | ✅ COVERED |
| 6 | Error without task-id or --all | `TestTaskAuditLogRequiresTaskIDOrAll` | ✅ COVERED |
| 7 | JSON format output | `TestTaskAuditLogJSONFormat` | ✅ COVERED |
