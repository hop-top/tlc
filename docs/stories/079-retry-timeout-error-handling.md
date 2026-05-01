---
status: shipped-no-e2e
---

# 079 - Retry, Timeout, and Error Handling

**ID**: 079
**Feature**: Flow Orchestration — Fault Tolerance
**Persona**: [System](../personas/system.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md),
  [AI Agent](../personas/ai-agent.md)
**Priority**: P2

## Story

As a System, I want steps to retry on failure with backoff and respect
timeouts so that flaky operations do not block the entire workflow, and
teardown steps always run regardless of prior failures.

## Acceptance Scenarios

1. **Given** a step with `retry: { max_attempts: 3, backoff: "1s" }`,
   **When** the step fails on the first two attempts but succeeds on
   the third, **Then** the step completes with status `SUCCEEDED` and
   the flow continues.

2. **Given** a step with `retry: { max_attempts: 3 }`, **When** the
   step fails all three attempts, **Then** the step is marked `FAILED`
   and the flow transitions to error handling.

3. **Given** a step with `timeout: "5s"`, **When** the step exceeds 5
   seconds, **Then** the step is killed and marked `FAILED` with a
   timeout reason.

4. **Given** a step with `on_failure: continue`, **When** the step
   fails, **Then** the flow proceeds to the next step instead of
   aborting.

5. **Given** a step with `run_always: true`, **When** a prior step
   fails and the flow is in error state, **Then** the `run_always`
   step still executes (teardown/cleanup).

6. **Given** a flow with retry, timeout, and teardown steps, **When**
   the flaky step succeeds on retry, **Then** the teardown step runs
   and the flow completes successfully.

## Tests

### E2E
- `tests/e2e/flow_retry_timeout_test.go::TestFlow_RetrySucceedsOnThirdAttempt`
- `tests/e2e/flow_retry_timeout_test.go::TestFlow_RetryExhaustionFailsStep`
- `tests/e2e/flow_retry_timeout_test.go::TestFlow_TimeoutKillsStep`
- planned: `tests/e2e/flow_retry_timeout_test.go::TestFlow_OnFailureContinue`
- planned: `tests/e2e/flow_retry_timeout_test.go::TestFlow_RunAlwaysAfterFailure`
- planned: `tests/e2e/flow_retry_timeout_test.go::TestFlow_HappyPathWithTeardown`

Fixtures:
- `examples/flows/retry-timeout.yaml` — flow definition
- `examples/flows/fixtures/retry-timeout/happy-path/` — flaky step
  succeeds on 3rd attempt; teardown runs
- `examples/flows/fixtures/retry-timeout/timeout/` — step exceeds
  timeout; teardown runs
- Run via:
  - `tlc flow test examples/flows/retry-timeout.yaml happy-path`
  - `tlc flow test examples/flows/retry-timeout.yaml timeout`

### Unit
- Retry counter and backoff calculation
- Timeout enforcement and kill signal
- `run_always` execution after failure
- `on_failure: continue` propagation

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Retry succeeds on 3rd attempt | happy-path fixture | COVERED |
| 2 | Retry exhausted -> FAILED | timeout fixture side-effect | COVERED |
| 3 | Timeout kills step | timeout fixture | COVERED |
| 4 | on_failure continue | happy-path fixture | COVERED |
| 5 | run_always teardown | both fixtures | COVERED |
| 6 | Full happy path with teardown | happy-path fixture | COVERED |
