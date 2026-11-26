---
status: paper
---

# 093 - Default Status Workflow

**ID**: 093
**Feature**: Task Management — Lifecycle
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md), [AI Agent](../personas/ai-agent.md)
**Priority**: P1
**Related**: [004 - Configurable Task Statuses](004-configurable-statuses.md)

## Story

Before this story, the built-in workflow had four statuses. Review had no
home, blocked work was a flag invisible to boards and counts, and a move
was checked only against the list of allowed transitions.
After: seven default statuses (`backlog`, `todo`, `in_progress`,
`in_review`, `blocked`, `done`, `cancelled`) fit most teams as-is, and a
transition can require a note, a reason, an assignee, or no open
blockers. Every refusal names the rule that blocked it. The whole default
is expressible in config, so a team can replace or extend it without code.

## Acceptance Scenarios

1. **Given** no custom config, **When** I run `tlc workflow statuses`,
   **Then** I see the seven default statuses with their roles and TLS
   markers (`-`, ` `, `>`, `?`, `!`, `x`, `~`).

2. **Given** no custom config, **When** I run `tlc task create "Title"`,
   **Then** the task lands in `todo`.

3. **Given** a task in `in_progress` with no assignee, **When** I run
   `tlc task submit T-0001`, **Then** the move is refused with an error
   naming the transition, the `assignee` guard and the failed check.

4. **Given** a task in `in_review`, **When** I run `tlc task approve T-0001`
   and it has an open `blocked_by` task, **Then** the move is refused:
   `in_review → done refused: blockers_clear: T-0002 is still open`.

5. **Given** a task in `in_review`, **When** I run
   `tlc task reject T-0001 --reason "missing tests"`, **Then** it returns
   to `in_progress`.

6. **Given** a task in `in_progress`, **When** I run
   `tlc task block T-0001 --reason "waiting on vendor"`, **Then** it moves
   to `blocked` and shows in status counts and board columns; **When** I
   run `tlc task unblock T-0001`, **Then** it returns to `in_progress`.

7. **Given** a task in `todo`, **When** I run `tlc task cancel T-0001`
   without `--note`, **Then** the move is refused naming the `note` guard;
   with `--note "dropped"` it moves to `cancelled`.

8. **Given** any guard that would refuse a move, **When** I add `--force`,
   **Then** the move succeeds and the audit entry records that it was
   forced.

9. **Given** `task.state_machine.guards` in config, **When** a guard names
   an unknown check, an undeclared status, or sets neither `from` nor
   `to`, **Then** config validation fails with an error naming the guard.

10. **Given** a per-tag `workflows.<tag>.state_machine.guards`, **When** I
    move a task carrying that tag, **Then** that list replaces the default
    guards for it.

11. **Given** tasks in all seven statuses, **When** I export `todo.txt`
    and read it back, **Then** each task keeps its status through the
    marker round trip.

12. **Given** tasks in all seven statuses, **When** I export VTODO, **Then**
    `backlog` and `todo` write `STATUS:NEEDS-ACTION`; `in_progress`,
    `in_review` and `blocked` write `IN-PROCESS`; `done` writes
    `COMPLETED`; `cancelled` writes `CANCELLED`.

13. **Given** a store written under earlier uppercase names, **When** I
    read it, **Then** `TODO`, `IN_PROGRESS`, `DONE`, `SKIPPED` and
    `BACKLOG` resolve to the lowercase default names without a migration.

14. **Given** a terminal task, **When** I run `tlc task reopen T-0001
    --note "regression"`, **Then** it lands on `todo`; no other verb
    leaves a terminal status.

## Tests

### Unit
- `internal/config/config_test.go` — default vocabulary, guard validation
- `internal/core/workflow_test.go` — default rules, guard checks
- `internal/vtodo/status_vocab_test.go` — role-based `STATUS` mapping
- `internal/cli/sync_local_test.go` — TLS marker round trip
- `internal/labels/axes_test.go` — `status:*` labels from the vocabulary

### E2E
- `internal/cli/task_lifecycle_test.go` — submit, approve, reject, block,
  unblock, cancel, guard refusals, `--force`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Status |
|---|---|---|
| 1-2 | Default vocabulary and landing status | planned |
| 3-8 | Verbs, guards and `--force` | planned |
| 9-10 | Guard config validation and per-tag override | planned |
| 11-12 | TLS and VTODO projections | planned |
| 13 | Legacy names read case-insensitively | planned |
| 14 | Terminal exit only via `reopen` | planned |
