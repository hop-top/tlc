---
status: shipped-no-e2e
---

# 023 - Parallel Fan-Out and Fan-In

**ID**: 023
**Feature**: Flow Orchestration
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md)
**Priority**: P1
**Task**: T-0618

## Story

As a Solo Developer, I want independent flow steps to run in
parallel and a final step to wait for all of them so that flows
complete faster without sacrificing correctness at the join point.

## Acceptance Scenarios

1. **Given** a flow with 3 steps sharing the same dependency
   (fan-out) and a 4th step depending on all 3 (fan-in),
   **When** I run `tlc flow run parallel-fanout.yaml`,
   **Then** the 3 parallel steps start without blocking on
   each other.

2. **Given** a parallel fan-out flow,
   **When** all 3 parallel steps complete,
   **Then** the fan-in step starts only after the last
   parallel step finishes.

3. **Given** a parallel fan-out flow,
   **When** all steps succeed,
   **Then** `tlc flow status <run-id>` shows every step as
   `SUCCEEDED` and the run as `SUCCEEDED`.

4. **Given** a parallel fan-out flow,
   **When** one parallel step fails,
   **Then** the fan-in step is not dispatched and the run
   status is `FAILED`.

5. **Given** a parallel fan-out flow that succeeds,
   **When** I inspect step timestamps,
   **Then** the 3 parallel steps have overlapping time
   windows (start times within the same scheduling window)
   and the fan-in step starts after the latest parallel
   step end time.

## Tests

### E2E
- `tests/e2e/flow_parallel_test.go::TestFlow_FanOut_StartsConcurrently`
- planned: `tests/e2e/flow_parallel_test.go::TestFlow_FanIn_WaitsForAllParallel`
- planned: `tests/e2e/flow_parallel_test.go::TestFlow_Parallel_AllSucceededReportsSucceeded`
- planned: `tests/e2e/flow_parallel_test.go::TestFlow_Parallel_OneFailSkipsFanIn`
- planned: `tests/e2e/flow_parallel_test.go::TestFlow_Parallel_TimestampWindowsOverlap`

Fixture: `examples/flows/fixtures/parallel-fanout/happy-path/`
- `test.yaml` — expected_exit: 0
- `record/` — cassettes per step (prepare, lint, test,
  security-scan, merge-results)
- Run via: `tlc flow test examples/flows/parallel-fanout.yaml happy-path`

### Unit
- `internal/core/flow_test.go`
  - `TestFlowExecutor_Parallel` — verifies concurrent dispatch
  - `TestParseFlow_YAML` — parses fan-out/fan-in topology
