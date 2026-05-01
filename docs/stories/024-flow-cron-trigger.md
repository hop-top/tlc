# 024 - Flow Cron Trigger

**ID**: 024
**Feature**: Flow Orchestration
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md),
[AI Agent](../personas/ai-agent.md)
**Priority**: P1
**Task**: T-0118
**Status**: paper
**Author**: jadb

## Story

As a flow author, I want a cron-style schedule trigger on a flow
yaml so the flow runs autonomously without an external scheduler,
so that recurring workflows (newsletters, audits, syncs) execute
on a deterministic schedule with no operator intervention.

## Acceptance Scenarios

1. **Given** a flow yaml with `triggers.cron: "0 9 * * 4"`,
   **When** the flow is loaded,
   **Then** the parser accepts the standard 5-field POSIX cron
   expression (`m h dom mon dow`) and registers the schedule.

2. **Given** a flow yaml with a 6-field cron expression
   (with seconds),
   **When** the flow is loaded,
   **Then** load fails fast with a descriptive error
   ("6-field cron not supported in v1; use 5-field POSIX").

3. **Given** a flow yaml with an invalid cron expression
   (e.g. `"@invalid"` or `"99 99 * * *"`),
   **When** the flow is loaded,
   **Then** load fails fast with a descriptive error citing
   the field that failed validation.

4. **Given** a registered cron schedule,
   **When** the cron expression matches the current time tick,
   **Then** the scheduler creates a new flow run with the
   flow's task templates instantiated and dispatches it
   through the existing executor path.

5. **Given** a flow with multiple cron triggers,
   **When** any trigger ticks,
   **Then** each tick creates exactly one run; concurrent
   ticks across distinct triggers create distinct runs.

6. **Given** a flow with `triggers.cron.missed: skip` (default),
   **When** the scheduler is offline during a scheduled tick,
   **Then** missed ticks are dropped on restart (no catch-up).

7. **Given** a flow with `triggers.cron.missed: catchup`,
   **When** the scheduler restarts after missing N ticks,
   **Then** N runs are created, one per missed tick, in
   chronological order.

8. **Given** a flow with `triggers.cron.concurrency: skip`
   (default),
   **When** a tick fires while a previous run is still active,
   **Then** the new tick is skipped and logged as
   `source: cron, status: skipped_concurrent`.

9. **Given** a flow with `triggers.cron.concurrency: queue`
   (with queue limit N),
   **When** ticks fire while previous runs are active,
   **Then** up to N ticks are queued; queue overflow drops
   oldest with audit log entry.

10. **Given** a flow with `triggers.cron.concurrency: parallel`,
    **When** a tick fires regardless of previous runs,
    **Then** runs execute concurrently with distinct run-ids.

11. **Given** a flow with both `triggers.cron` and
    `triggers.keywords`,
    **When** either trigger source fires,
    **Then** both produce runs; `tlc flow list` shows
    `source: cron` vs `source: keyword` per run.

12. **Given** a flow with `timezone: "America/New_York"`,
    **When** the cron expression is evaluated,
    **Then** the schedule is computed in that timezone
    (default: system tz).

13. **Given** an active cron-triggered flow,
    **When** I run `tlc flow disable <flow-id>`,
    **Then** subsequent cron ticks do not create runs;
    flow definition remains in the DB; `enable` resumes ticks.

14. **Given** a cron schedule registered before a restart,
    **When** the scheduler process restarts,
    **Then** schedule state is rehydrated from the tlc DB and
    next tick fires at the correct wall-clock time.

## Implementation Notes

- Likely uses `robfig/cron/v3` or equivalent in
  `internal/core/scheduler/`.
- Scheduler runs as part of `tlc serve` (if present) or a new
  `tlc flow scheduler` daemon command.
- Cron schedule state lives in tlc DB; survives restart;
  rehydrated on boot.
- State machine: cron tick -> run creation -> existing executor
  dispatch path takes over (no new execution semantics).
- New flow yaml fields: `triggers.cron`, `triggers.cron.missed`,
  `triggers.cron.concurrency`, `triggers.cron.queue_limit`,
  `timezone`.
- New CLI: `tlc flow disable <flow-id>`,
  `tlc flow enable <flow-id>`.
- `source` column added to `flow_runs` table:
  `cron | keyword | manual | api`.

## Tests

### E2E (planned, paper status — fixtures + cassettes TBD)
- `tests/e2e/flow_cron/cron_parse_test.go`
  - `TestCron_ValidExpression`
  - `TestCron_InvalidExpressionFailsLoad`
- `tests/e2e/flow_cron/cron_tick_test.go`
  - `TestCron_TickCreatesRun`
- `tests/e2e/flow_cron/cron_missed_test.go`
  - `TestCron_MissedTickSkipPolicy`
  - `TestCron_MissedTickCatchupPolicy`
- `tests/e2e/flow_cron/cron_concurrent_test.go`
  - `TestCron_ConcurrencySkip`
  - `TestCron_ConcurrencyQueue`
- `tests/e2e/flow_cron/cron_timezone_test.go`
  - `TestCron_TimezoneRespected`
- `tests/e2e/flow_cron/cron_disable_test.go`
  - `TestCron_DisableHaltsRuns`
- `tests/e2e/flow_cron/cron_compose_test.go`
  - `TestCron_ComposesWithKeywordTrigger`

### Unit (planned)
- `internal/core/scheduler/cron_test.go`
  - parsing, tick computation, tz handling, missed-policy,
    concurrency-policy state transitions.
- `internal/core/flow_test.go`
  - `TestParseFlow_YAML_CronTrigger`

## Dependencies

- Prereq for: scenario 4a (newsletter-weekly-publish).
- Builds on: 020-flow-execution, 022-sequential-chain-execution.
