---
status: paper
---

# 064 - Task Effort Estimation

**ID**: 064
**Feature**: Task Metadata
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md), [Team Lead](../personas/team-lead.md)
**Priority**: P2

## Story

As a developer, I want to assign a standardized effort estimate (XS/S/M/L/XL)
to tasks so I can track sizing for planning and velocity measurement.

## Acceptance Scenarios

1. **Given** a task exists, **When** I run
   `tlc task update T-0001 --effort M`, **Then** the task's effort is
   set to `M` and shown in `tlc task show T-0001`.

2. **Given** I create a task, **When** I run
   `tlc task create "Fix bug" --effort S`, **Then** the created task
   has effort `S`.

3. **Given** a task has effort set, **When** I export to TLS format,
   **Then** the TLS line includes `effort:M` token.

4. **Given** a TLS file with `effort:L` token, **When** synced,
   **Then** the task is stored with effort `L`.

5. **Given** I run `tlc task update T-0001 --effort HUGE`,
   **Then** an error is returned: invalid effort must be XS/S/M/L/XL.

6. **Given** a task has no effort set, **When** shown,
   **Then** the `Effort:` line is omitted from output.

## Implementation Notes

- Effort stored as `TEXT NOT NULL DEFAULT ''` column (migration v3).
- Validated against enum: XS, S, M, L, XL, or empty.
- Persisted via `core.Effort` type; accessible in JSON/YAML export.

## Tests

### E2E
- planned: `tests/e2e/task_effort_test.go::TestTaskEffort_UpdateSetsEffort`
- planned: `tests/e2e/task_effort_test.go::TestTaskEffort_CreateWithEffortFlag`
- planned: `tests/e2e/task_effort_test.go::TestTaskEffort_TLSExportEmitsEffortToken`
- planned: `tests/e2e/task_effort_test.go::TestTaskEffort_TLSImportRoundtrip`
- planned: `tests/e2e/task_effort_test.go::TestTaskEffort_InvalidValueRejected`
- planned: `tests/e2e/task_effort_test.go::TestTaskEffort_UnsetOmitsLineFromShow`
