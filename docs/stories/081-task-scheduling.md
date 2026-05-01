---
status: shipped
---

# 081 - Task Scheduling: Due Dates, Reminders & Recurrence

**ID**: 081
**Feature**: Task Management — Scheduling
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md),
[Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

Before this story, tasks had no concept of deadlines or
reminders. After: `--due`, `--remind-at`, `--remind-every`,
`--no-auto-remind` on create/update; `tlc task remind` for
upcoming/overdue view; auto-scheduling via config by priority
and task age.

## Acceptance Scenarios

1. **Given** a task create with `--due tomorrow`, **Then** `DueAt`
   is set to ~24h from now.

2. **Given** a task create with `--due "2025-05-01"`, **Then**
   `DueAt` is `2025-05-01T00:00:00Z`.

3. **Given** a task create with `--remind-every 1h`, **Then**
   `RemindEvery` is 1h.

4. **Given** a task create with `--due friday --no-auto-remind`,
   **Then** `DueAt` is set and `NoAutoRemind` is true.

5. **Given** an overdue task, **When** `tlc task remind --check`,
   **Then** exit code is 1.

6. **Given** no overdue tasks, **When** `tlc task remind --check`,
   **Then** exit code is 0.

7. **Given** `task.scheduling.by_priority.P0.due: 24h` in config,
   **When** creating a P0 task without `--due`, **Then** `DueAt`
   is auto-set to created_at + 24h.

8. **Given** `task.scheduling.by_priority.P0.due: 24h` in config,
   **When** creating a P0 task with explicit `--due "in 3d"`,
   **Then** explicit value takes precedence.

9. **Given** an IN_PROGRESS task idle >48h and age nudge config,
   **When** `tlc task remind`, **Then** "AGE NUDGES" section
   shows the task.

10. **Given** a task update with `--due -`, **Then** `DueAt` is
    cleared (nil).

11. **Given** a task list, **Then** Due column shows dates and
    overdue tasks prefixed with `!`.

## Tests

### E2E
- `internal/cli/task_scheduling_e2e_test.go`

## Acceptance Criteria Validation Status

| # | Criteria | Test | Status |
|---|---------|------|--------|
| 1 | --due tomorrow | `TestTaskCreate_Due` | ✅ |
| 2 | --due ISO date | `TestTaskCreate_DueISO` | ✅ |
| 3 | --remind-every | `TestTaskCreate_RemindEvery` | ✅ |
| 4 | --no-auto-remind | `TestTaskCreate_NoAutoRemind` | ✅ |
| 5 | remind --check overdue | `TestTaskRemind_CheckOverdue` | ✅ |
| 6 | remind --check clean | `TestTaskRemind_CheckClean` | ✅ |
| 7 | auto-due from config | `TestTaskCreate_AutoDueFromConfig` | ✅ |
| 8 | explicit --due overrides config | `TestTaskCreate_ExplicitDueOverridesConfig` | ✅ |
| 9 | age nudge | `TestTaskRemind_AgeNudge` | ✅ |
| 10 | --due - clears | `TestTaskUpdate_ClearDue` | ✅ |
| 11 | list shows due column | `TestTaskList_DueColumn` | ✅ |
