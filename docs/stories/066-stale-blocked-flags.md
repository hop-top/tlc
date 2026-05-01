---
status: shipped
---

# 066 - Stale Timeout and Blocked Reason Flags

**ID**: 066
**Feature**: Task Management — Stale Detection
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md), [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

Before this story, there was no way to mark a task as blocked with a reason or set a
per-task stale timeout from the CLI. After: `tlc task update --blocked`, `--unblock`, and
`--timeout` give agents and developers direct control over blocked state and stale
detection, feeding the stale/blocked detection pipeline.

## Acceptance Scenarios

1. **Given** a task `T-0001`, **When** I run
   `tlc task update T-0001 --blocked "waiting on T-0002"`, **Then** `BlockedReason`
   is set to `"waiting on T-0002"`.

2. **Given** a task `T-0001` with `BlockedReason = "waiting on T-0002"`, **When** I run
   `tlc task update T-0001 --unblock`, **Then** `BlockedReason` is nil.

3. **Given** a task `T-0001`, **When** I run
   `tlc task update T-0001 --timeout 2h`, **Then** `StaleTimeout` is `2h` and
   `StaleFiredAt` is reset to nil.

4. **Given** an invalid duration string, **When** I run
   `tlc task update T-0001 --timeout notaduration`, **Then** the command returns an
   error and the task is not modified.

5. **Given** a new task creation, **When** I run
   `tlc task create "My Task" --timeout 4h`, **Then** the created task has
   `StaleTimeout = 4h`.

## Tests

### E2E
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdate_BlockedReason`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdate_Unblock`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdate_Timeout`
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdate_TimeoutInvalid`
- ✅ `internal/cli/task_update_test.go` — `TestTaskCreate_Timeout`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | --blocked sets BlockedReason | `TestTaskUpdate_BlockedReason` | ✅ COVERED |
| 2 | --unblock clears BlockedReason | `TestTaskUpdate_Unblock` | ✅ COVERED |
| 3 | --timeout sets StaleTimeout + resets StaleFiredAt | `TestTaskUpdate_Timeout` | ✅ COVERED |
| 4 | invalid --timeout returns error | `TestTaskUpdate_TimeoutInvalid` | ✅ COVERED |
| 5 | task create --timeout sets StaleTimeout | `TestTaskCreate_Timeout` | ✅ COVERED |
