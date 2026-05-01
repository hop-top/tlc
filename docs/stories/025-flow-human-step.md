# 025 - Flow Human Approval Step

**ID**: 025
**Feature**: Flow Orchestration
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md),
[AI Agent](../personas/ai-agent.md)
**Priority**: P1
**Task**: T-0119
**Status**: paper
**Author**: jadb

## Story

As a flow author, I want a `type: human` step that pauses the
flow until external approval so that a human can gate sensitive
transitions (send email, merge PR, deploy prod) without
embedding manual checks in CI scripts.

## Acceptance Scenarios

1. **Given** a flow with a step `type: human`,
   **When** the flow run reaches that step,
   **Then** the step transitions to state `awaiting_approval`
   and downstream steps (linked via `depends_on`) remain
   `pending`; the run does not progress.

2. **Given** a flow run paused at a human step,
   **When** I run `tlc flow status <run-id>`,
   **Then** the step is shown as `awaiting_approval` with
   the step's title and a stable approval token (URL-safe).

3. **Given** a flow run paused at a human step,
   **When** I run `tlc flow approve <run-id> --step <step-id>`,
   **Then** the step transitions to `succeeded`, downstream
   steps proceed per their `depends_on`, and the run continues.

4. **Given** a flow run paused at a human step,
   **When** I run
   `tlc flow reject <run-id> --step <step-id> --reason <text>`,
   **Then** the step transitions to `rejected`, the run
   transitions to `failed`, and the reason is captured in the
   run audit log.

5. **Given** a flow run paused at a human step,
   **When** I run `tlc flow cancel <run-id>`,
   **Then** the run transitions to `canceled` (distinct from
   `rejected`); no reason required; downstream steps marked
   `canceled`.

6. **Given** a human step with `timeout: 24h` and
   `on_timeout: approve`,
   **When** the timeout elapses with no signal,
   **Then** the step auto-transitions to `succeeded`, audit
   log records `approved_by: timeout`, downstream proceeds.

7. **Given** a human step with `timeout: 24h` and
   `on_timeout: reject`,
   **When** the timeout elapses with no signal,
   **Then** the step auto-transitions to `rejected` and the
   run transitions to `failed`.

8. **Given** a human step with `timeout: 24h` and
   `on_timeout: keep_waiting`,
   **When** the timeout elapses,
   **Then** the step remains `awaiting_approval`; audit log
   records the timeout event but no state change occurs.

9. **Given** a human step approved via CLI,
   **When** I inspect the run audit log,
   **Then** the entry records: who (aps profile id from
   caller env), when (utc timestamp), via (channel:
   `cli | webhook | api`).

10. **Given** a human step with
    `approver_capability: founder-approval`,
    **When** an aps profile lacking that capability calls
    `tlc flow approve`,
    **Then** approval is rejected with error
    `caller missing capability: founder-approval`; step
    remains `awaiting_approval`.

11. **Given** a human step with
    `webhook: https://hooks.example.com/notify`,
    **When** the step transitions to `awaiting_approval`,
    **Then** the URL is fired once with payload
    `{run_id, step_id, title, approval_url, expires_at}`;
    delivery failure is logged but does not block the pause.

12. **Given** a flow run paused at a human step,
    **When** the REST endpoint
    `POST /flows/runs/<run-id>/steps/<step-id>/approve` is
    called with a valid token,
    **Then** approval semantics match the CLI path
    (scenarios 3 and 9 apply).

## Implementation Notes

- New step type registered in flow executor:
  `type: human`.
- Approval state lives on the flow run row in tlc DB
  (`awaiting_approval_step_id`, `approval_token`,
  `approval_expires_at`).
- Resume signals accepted from: `tlc flow approve` CLI,
  REST `POST /flows/runs/<id>/steps/<step-id>/approve`,
  webhook callback URL with token.
- Audit columns on run: `approved_by`, `approved_at`,
  `approval_channel`, `rejection_reason`.
- New flow yaml fields on step:
  `type: human`, `timeout`, `on_timeout`,
  `approver_capability`, `webhook`.
- Token = URL-safe random; scoped to run+step; one-shot.

## Tests

### E2E (planned, paper status — fixtures + cassettes TBD)
- `tests/e2e/flow_human/human_pause_test.go`
  - `TestHuman_StepPausesFlow`
  - `TestHuman_DownstreamBlocked`
- `tests/e2e/flow_human/human_approve_test.go`
  - `TestHuman_ApproveResumesFlow`
- `tests/e2e/flow_human/human_reject_test.go`
  - `TestHuman_RejectCancelsRun`
- `tests/e2e/flow_human/human_timeout_test.go`
  - `TestHuman_TimeoutDefaultApprove`
  - `TestHuman_TimeoutDefaultReject`
- `tests/e2e/flow_human/human_audit_test.go`
  - `TestHuman_ApprovalAuditCaptured`
- `tests/e2e/flow_human/human_capability_test.go`
  - `TestHuman_CapabilityGated`
- `tests/e2e/flow_human/human_webhook_test.go`
  - `TestHuman_WebhookFiredOnPause`

### Unit (planned)
- `internal/core/flow/human_step_test.go`
  - state transitions, token generation, timeout policy,
    capability gate.
- `internal/core/flow_test.go`
  - `TestParseFlow_YAML_HumanStep`

## Dependencies

- Prereq for: scenario 4a (newsletter-weekly-publish).
- Builds on: 020-flow-execution, 022-sequential-chain-execution.
- Composes with: 077-gate-step-eva-contract (eva gates +
  human approval can both gate the same step chain).
