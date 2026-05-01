# 021 - Single-Step Flow Execution

**ID**: 021
**Feature**: Flow Orchestration
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md)
**Priority**: P1
**Task**: T-0616

## Story

As a Solo Developer, I want to run a flow containing a single step
so that I can automate a one-off task with a repeatable, auditable
definition and verify the engine handles the simplest case correctly.

## Acceptance Scenarios

1. **Given** a flow YAML with one step and no dependencies,
   **When** I run `tlc flow run single-step.yaml`,
   **Then** the engine executes the step, records a run,
   and the run status is `SUCCEEDED`.

2. **Given** a single-step flow,
   **When** the step completes successfully,
   **Then** `tlc flow status <run-id>` shows the step as
   `SUCCEEDED` with a non-zero duration.

3. **Given** a single-step flow,
   **When** the step agent is dispatched,
   **Then** it receives the task_template title and description
   from the flow definition.

4. **Given** a single-step flow,
   **When** the run finishes,
   **Then** the engine records step start/end timestamps
   and exit code 0.

## Tests

### E2E
- `tests/e2e/flow_single_step_test.go::TestFlow_SingleStep_HappyPath`
- planned: `tests/e2e/flow_single_step_test.go::TestFlow_SingleStep_StatusReportsSucceeded`
- planned: `tests/e2e/flow_single_step_test.go::TestFlow_SingleStep_AgentReceivesTaskTemplate`
- planned: `tests/e2e/flow_single_step_test.go::TestFlow_SingleStep_RecordsTimestampsAndExitCode`

Fixture: `examples/flows/fixtures/single-step/happy-path/`
- `test.yaml` — expected_exit: 0
- `record/` — cassettes for the single step execution
- Run via: `tlc flow test examples/flows/single-step.yaml happy-path`

### Unit
- `internal/core/flow_test.go`
  - `TestParseFlow_YAML` — parses single-step flow
  - `TestFlowExecutor_Sequential` — covers single-step as degenerate case
