---
status: shipped
---

# 080 - Flow Retry Failed Steps

**ID**: 080
**Feature**: Flow Orchestration — Retry Failed Steps
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md)
**Priority**: P3

## Story

As a Solo Developer, I want to retry only failed steps in a
completed flow run so that I do not re-execute successful work
and can recover from transient failures efficiently.

## Acceptance Scenarios

1. **Given** a flow run where step 2 of 3 failed (step 1
   succeeded, step 3 never ran),
   **When** I retry the run,
   **Then** step 1 is skipped, step 2 is re-executed, and step 3
   runs after step 2 succeeds.

2. **Given** a flow run in FAILED status,
   **When** the run is retried,
   **Then** a new run is created linked to the original as parent.

3. **Given** a retry run where all steps succeed,
   **When** the retry completes,
   **Then** the new run status is SUCCEEDED.

4. **Given** a flow with StepType "retry" wrapping a child step,
   **When** the child step fails on attempt 1 and succeeds on
   attempt 2,
   **Then** the retry step succeeds and execution continues to the
   next step.

5. **Given** a retry step with max_attempts=3,
   **When** the child step fails on all 3 attempts,
   **Then** the retry step fails and the overall run fails.

## Tests

### E2E
- `internal/cli/flow_retry_failed_e2e_test.go`
  -- `TestFlowRetryStep_SucceedsAfterRetry` (scenario 4)
  -- `TestFlowRetryStep_ExhaustsAttempts` (scenario 5)
  -- `TestFlowInvoke_WithRetryStep_CreatesTask` (scenario 1-2)

### Unit
- `internal/core/flow_test.go`
  -- `TestFlowExecutor_Retry` (scenario 4)

### Fixtures
- `examples/flows/fixtures/retry-failed/happy-path/` -- cassettes
