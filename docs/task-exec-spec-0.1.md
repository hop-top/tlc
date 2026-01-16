# Task Execution Specification (TES) v0.1

## Version

- Version: 0.1
- Generated at: 2026-01-15T15:50:16Z

## Summary

Task Execution Spec defines the contract between:
- a caller (CLI / flow engine / agent)
and
- a runner (executor)

This spec OWNS:
- execution request/response payloads
- execution lifecycle states (runtime-level)
- stdout/stderr capture
- timeout semantics
- error normalization

This spec DOES NOT own:
- task schema (see task-crud-spec-0.1.md)
- ownership/claiming/reassignment (see task-collab-spec-1.0.md)
- flow orchestration graphs (see task-flow-spec-0.1.md)

This spec references:
- task-log-spec-0.1.md for audit logs

---

## Traceability Invariants (Normative)

To ensure a complete audit trail, every action performed by an agent MUST be associated with a valid Task ID.

### Unplanned Work Rule
In cases where an agent begins work on a task, modification, or investigation that is not already captured in the task store, the agent **MUST** create a self-assigned task before proceeding. This ensures that even "out-of-band" or spontaneous work leaves a durable trace in the system.

---

## Execution Request (Normative)

A runner MUST accept an execution request object:

- `task_id`: string (required)
- `runner_id`: string (required)
- `attempt`: integer >= 1 (required)
- `timeout_ms`: integer (optional)
- `args`: object/map (optional)
- `env`: object/map (optional)

### Semantics

- `task_id` identifies the task to run.
- `attempt` increments when the same task is re-attempted within the same run context.
- `timeout_ms` is a hard limit for the runner to enforce (best effort).

## Execution Response (Normative)

A runner MUST return an execution response object:

- `task_id`: string
- `runner_id`: string
- `attempt`: integer
- `status`: enum
  - SUCCEEDED
  - FAILED
  - CANCELED
- `started_at`: ISO8601 UTC
- `ended_at`: ISO8601 UTC
- `exit_code`: integer (optional)
- `stdout`: string (optional)
- `stderr`: string (optional)
- `outputs`: object/map (optional)
- `error`: object (optional)

## Runtime Status Model

These are execution-level states, distinct from CRUD task status:

- QUEUED (optional)
- RUNNING
- SUCCEEDED (terminal)
- FAILED (terminal)
- CANCELED (terminal)

## Error Model (Normalized)

If `status=FAILED`, the runner SHOULD populate:

- `error.code`: string
- `error.message`: string
- `error.retryable`: boolean
- `error.details`: object/map (optional)

### Canonical error codes (recommended)

- TIMEOUT
- INVALID_INPUT
- MISSING_CONTEXT
- EXECUTION_ERROR

## Timeout Rules

If `timeout_ms` is exceeded, runner MUST:
- stop work (best effort)
- return `status=FAILED` or `status=CANCELED`
- if FAILED, set `error.code=TIMEOUT`

## Idempotency Guidance (Non-Normative)

Executors SHOULD assume tasks may be retried and should:
- avoid non-idempotent destructive actions without checks
- write outputs to attempt-scoped paths where possible

## Logging Requirements

Each execution attempt MUST emit a log entry (see task-log-spec-0.1.md):

- `entity_type`: task
- `entity_id`: task_id
- `by`: runner_id
- `action`: EXEC_ATTEMPT
- `note`: includes attempt number and terminal status
