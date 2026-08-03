---
status: shipped
---

# 091 - HTTP API Task Operations

**ID**: 091
**Feature**: HTTP API — Task Operations
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md), [Team Lead](../personas/team-lead.md)
**Priority**: P2

## Story

As an AI Agent, I want to list, create, show, update, claim, and complete
tasks over HTTP, enforcing the exact same validation and workflow rules
`tlc task ...` enforces from the CLI, so a fleet of agents can coordinate
work through a shared API instead of each shelling out to a local CLI
binary. Reads (GET) must work without a token so lightweight polling
doesn't require credential plumbing; writes (POST/PATCH) must require a
bearer token so a shared server isn't open to arbitrary mutation.

## Acceptance Scenarios

1. **Given** the server is running, **When** a client sends
   `POST /tasks` with a valid title, **Then** the response is 201 with
   the created task (default status `TODO`), and a subsequent
   `GET /tasks/{id}` returns 200 with the same task.

2. **Given** a client sends `POST /tasks` with an empty title,
   **When** the request is validated against the same config-driven
   rules `saveTask` applies, **Then** the response is 422.

3. **Given** no task exists with a given ID, **When** a client sends
   `GET /tasks/{id}`, **Then** the response is 404.

4. **Given** two or more tasks exist, **When** a client sends
   `GET /tasks?status=TODO`, **Then** the response is 200 with a
   `tasks` array and `total` matching the filtered count.

5. **Given** a TODO task, **When** a client sends
   `PATCH /tasks/{id}` with `status: "IN_PROGRESS"`, **Then** the
   response is 200 with the updated status; **when** the same client
   then sends an unrecognized status string, **then** the response is
   422 (routed through the same `WorkflowManager` transition
   validation the CLI uses).

6. **Given** a task and a track exist, **When** a client sends
   `PATCH /tasks/{id}` with `due`, `remind_at`, `rrule`, `track_id`,
   and `add_blocked_by` fields, **Then** all fields apply via the
   shared `applyTaskFieldChanges` function (also used by `task update`),
   and a follow-up PATCH with the CLI's `"-"` clear-sentinel on each of
   those fields unsets them identically to the CLI's behavior.

7. **Given** a task, **When** a client sends `PATCH /tasks/{id}` with
   a `track_id` that does not resolve to an existing track, **Then**
   the response is 422 — unlike the CLI's `task update`, the HTTP route
   never auto-creates a track for a typo'd ID (`TaskFieldChanges.AutoCreateTrack`
   is deliberately left unset over HTTP).

8. **Given** a TODO task, **When** a client sends
   `POST /tasks/{id}/claim`, **Then** the response is 200, the task's
   status transitions to `IN_PROGRESS`, and `assigned_to` is set to the
   current user; **when** the same client then sends
   `POST /tasks/{id}/complete`, **then** the response is 200 and status
   is `DONE`.

9. **Given** a DONE task, **When** a client sends
   `POST /tasks/{id}/complete` again, **Then** the response is 200
   (same-status transitions are always valid — idempotent re-complete,
   matching the CLI's documented `task complete` behavior); **when**
   the same client instead sends `POST /tasks/{id}/claim` on that DONE
   task, **then** the response is 409 (DONE is a terminal state; claim
   is a genuinely invalid transition).

10. **Given** a task is created with a `blocked_by` reference to a
    task ID that does not exist, **When** the create request is
    processed, **Then** the response is 422.

11. **Given** auth is enabled and no bearer token is supplied,
    **When** a client sends `POST /tasks`, **Then** the response is
    401.

12. **Given** auth is enabled and no bearer token is supplied,
    **When** a client sends `GET /tasks` or `GET /health`, **Then**
    the response is 200 — GET/HEAD requests bypass `requireAuth` by
    design (documented on `requireAuth` in `serve.go`), independent of
    whether a token is configured.

13. **Given** auth is enabled and the correct bearer token is
    supplied, **When** a client sends `POST /tasks`, **Then** the
    response is 201 — the write succeeds once authenticated.

## Tests

### E2E
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskCreateAndShow` (scenario 1)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskCreateValidationError` (scenario 2)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskShowNotFound` (scenario 3)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskList` (scenario 4)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskUpdateStatusTransition` (scenario 5)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskUpdateExtendedFields` (scenarios 6, 7)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskClaimAndComplete` (scenarios 8, 9)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskCreateWithInvalidBlockedBy` (scenario 10)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_AuthRequiredForWrites` (scenario 11)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_AuthBypassedForReads` (scenario 12)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_AuthCorrectTokenSucceeds` (scenario 13)

### Unit
- (no additional unit tests required beyond e2e coverage; task field
  validation and workflow-transition rules are unit-tested in
  `internal/core` under the existing task/workflow test files, reused
  as-is by the HTTP routes via `applyTaskFieldChanges`.)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Create + show task | `TestServe_E2E_TaskCreateAndShow` | COVERED |
| 2 | Empty title → 422 | `TestServe_E2E_TaskCreateValidationError` | COVERED |
| 3 | Show missing task → 404 | `TestServe_E2E_TaskShowNotFound` | COVERED |
| 4 | List with status filter | `TestServe_E2E_TaskList` | COVERED |
| 5 | Status transition + invalid status → 422 | `TestServe_E2E_TaskUpdateStatusTransition` | COVERED |
| 6 | Extended fields set + cleared via shared logic | `TestServe_E2E_TaskUpdateExtendedFields` | COVERED |
| 7 | Unresolvable track_id → 422 (no auto-create) | `TestServe_E2E_TaskUpdateExtendedFields` | COVERED |
| 8 | Claim then complete | `TestServe_E2E_TaskClaimAndComplete` | COVERED |
| 9 | Idempotent re-complete; claim on DONE → 409 | `TestServe_E2E_TaskClaimAndComplete` | COVERED |
| 10 | Invalid blocked_by → 422 | `TestServe_E2E_TaskCreateWithInvalidBlockedBy` | COVERED |
| 11 | Write without token → 401 | `TestServe_E2E_AuthRequiredForWrites` | COVERED |
| 12 | Read without token → 200 (GET/HEAD bypass) | `TestServe_E2E_AuthBypassedForReads` | COVERED |
| 13 | Write with correct token → 201 | `TestServe_E2E_AuthCorrectTokenSucceeds` | COVERED |
