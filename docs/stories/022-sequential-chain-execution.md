---
status: shipped-no-e2e
---

# 022 - Sequential Chain Execution

**ID**: 022
**Feature**: Flow Orchestration
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md)
**Priority**: P1
**Task**: T-0617

## Story

As a Solo Developer, I want flow steps to execute in dependency
order (A then B then C) so that each step can consume the output
of its predecessor and the overall chain produces a deterministic,
auditable result.

## Acceptance Scenarios

1. **Given** a flow with steps A -> B -> C linked via `depends_on`,
   **When** I run `tlc flow run sequential-chain.yaml`,
   **Then** step A completes before B starts,
   and B completes before C starts.

2. **Given** a 3-step sequential flow,
   **When** all steps succeed,
   **Then** `tlc flow status <run-id>` shows every step as
   `SUCCEEDED` and the run as `SUCCEEDED`.

3. **Given** a 3-step sequential flow,
   **When** step B fails,
   **Then** step C is never dispatched and the run status
   is `FAILED`.

4. **Given** a 3-step sequential flow that succeeds,
   **When** I inspect step timestamps,
   **Then** step A end <= step B start <= step B end <= step C
   start (strict ordering).

5. **Given** a 3-step sequential flow,
   **When** the engine dispatches each step,
   **Then** each step receives the correct task_template from
   its definition, not from a sibling step.

## Tests

### E2E
- `tests/e2e/flow_sequential_test.go::TestFlow_Sequential_OrderRespected`
- planned: `tests/e2e/flow_sequential_test.go::TestFlow_Sequential_AllSucceededReportsRunSucceeded`
- planned: `tests/e2e/flow_sequential_test.go::TestFlow_Sequential_StepBFailureSkipsC`
- planned: `tests/e2e/flow_sequential_test.go::TestFlow_Sequential_StrictTimestampOrdering`
- planned: `tests/e2e/flow_sequential_test.go::TestFlow_Sequential_PerStepTaskTemplate`

Fixture: `examples/flows/fixtures/sequential-chain/happy-path/`
- `test.yaml` — expected_exit: 0
- `record/` — cassettes per step (analyze, transform, report)
- Run via: `tlc flow test examples/flows/sequential-chain.yaml happy-path`

### Unit
- `internal/core/flow_test.go`
  - `TestFlowExecutor_Sequential` — verifies ordered execution
  - `TestParseFlow_YAML` — parses multi-step flow with depends_on
