# 020 - Flow Execution

**ID**: 020
**Feature**: Flow Orchestration
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md), [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a Solo Developer, I want to define and execute flows that orchestrate multiple tasks in sequence or parallel with branching and retry logic so that I can automate complex workflows and leverage flow execution engine semantics (determinism, auditability, fault tolerance).

## Acceptance Scenarios

1. **Given** a flow definition (YAML or JSON) with sequential steps, **When** I run `tlc flow execute --flow-id flow-123`, **Then**:
   - Steps execute in order
   - Each step waits for the previous to succeed before starting
   - The flow completes with status `succeeded` or `failed`
   - A run ID is generated for traceability

2. **Given** a flow with parallel steps, **When** I run `tlc flow execute --flow-id flow-456`, **Then**:
   - Independent steps execute concurrently
   - The flow waits for all parallel steps to complete before proceeding to dependent steps
   - Concurrency limits (if specified) are respected

3. **Given** a flow with branching logic (condition → path selection), **When** I run `tlc flow execute --flow-id flow-789 --var env=production`, **Then**:
   - The branch condition is evaluated with the provided variables
   - The matching path executes
   - Unselected paths are marked as `skipped` in logs

4. **Given** a step that fails, **When** flow has retry policy configured, **Then**:
   - The step is retried up to the configured `max_attempts`
   - Backoff delay is applied between attempts
   - After exhausting retries, the flow either fails or continues to an error path

5. **Given** a running flow, **When** I run `tlc flow cancel --run-id run-abc123`, **Then**:
   - In-progress steps are signaled to stop
   - Pending steps transition to `canceled`
   - The flow transitions to `canceled` state

6. **Given** a completed flow, **When** I run `tlc flow logs --run-id run-abc123`, **Then**:
   - A complete execution log is displayed showing step ordering, timings, and outcomes
   - Logs are sorted consistently (by step_id) for reproducibility

## Tests

### E2E
- ❌ `tests/integration/flow_test.go` — NOT YET IMPLEMENTED
  - Needed: `TestFlowSequentialExecution`, `TestFlowParallelExecution`, `TestFlowBranching`, `TestFlowRetry`, `TestFlowCancellation`
- ❌ `tests/integration/flow_engine_test.go` — NOT YET IMPLEMENTED
  - Needed: `TestFlowDeterminism`, `TestFlowConcurrency`, `TestFlowLogGeneration`

### Unit
- ✅ `internal/core/flow_test.go` — GOOD COVERAGE
  - ✅ Exists: `TestParseFlow_YAML`, `TestParseFlow_JSON`, `TestParseFlow_Validation`
  - ✅ Exists: `TestFlowExecutor_Sequential`, `TestFlowExecutor_Parallel`, `TestFlowExecutor_Retry`, `TestFlowExecutor_Cancel`, `TestFlowExecutor_PauseResume`
  - ❌ Missing: Branching with variables, run ID generation, determinism verification
- ❌ `internal/cli/flow_test.go` — PARTIAL
  - Check for CLI command tests

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Sequential execution, run ID, status | Partial | ⚠️ PARTIAL |
| 2 | Parallel execution, concurrency limits | ✅ | ✅ COVERED |
| 3 | Branching with variables and skipping | Not tested | ❌ NOT COVERED |
| 4 | Retry with backoff | ✅ | ✅ COVERED |
| 5 | Flow cancellation | ✅ | ✅ COVERED |
| 6 | Log consistency and ordering | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Create E2E test suite in `tests/integration/flow_test.go` covering all 6 scenarios
- [ ] Add branching with variable substitution tests
- [ ] Add run ID generation and traceability tests
- [ ] Add flow logs consistency and ordering tests
- [ ] Test determinism: same flow + same inputs = same execution order
- [ ] Test step status tracking and timing information
- [ ] Add flow command tests to `internal/cli/flow_test.go`
- [ ] Verify step skip logic for unselected branches
