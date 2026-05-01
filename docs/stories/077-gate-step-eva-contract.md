---
status: shipped
---

# 077 - Gate Step with EVA Contract

**ID**: 077
**Feature**: Flow Orchestration — Gate Step
**Persona**: [Test Engineer](../personas/test-engineer.md)
**Related Personas**:
  [AI Agent](../personas/ai-agent.md),
  [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a Test Engineer, I want a gate step that validates a contract
before proceeding so I can enforce quality checkpoints in workflows.

## Context

Gate steps attach an EVA contract to any task step. After the step
executes, the flow executor sends the step output to the EVA
gateway for contract evaluation. If the contract passes, the flow
continues. If the contract is violated, the step is marked failed
and downstream steps are blocked.

This enables quality-gate patterns: code review must pass before
merge, plan must meet completeness criteria before implementation,
test results must satisfy coverage thresholds before deploy.

## Acceptance Scenarios

1. **Given** a flow with steps `produce-output → gated-step →
   final-step` where `gated-step` has a gate with contract
   `quality-check`, **When** the EVA gateway returns `pass` for
   `quality-check`, **Then** `gated-step` succeeds and
   `final-step` executes normally; the flow run status is
   `succeeded`.

2. **Given** the same flow, **When** the EVA gateway returns
   `contract_violation` for `quality-check`, **Then** `gated-step`
   is marked `failed`, `final-step` never executes, and the flow
   run status is `failed`. The error message includes the contract
   name and violation details.

3. **Given** a gate step with `eva_url` and `contract` fields in
   the flow YAML, **When** the flow is parsed, **Then** the
   `StepGate` struct is populated with the correct values; steps
   without a gate have `Gate == nil`.

4. **Given** a gate step where the EVA gateway is unreachable,
   **When** the step executes, **Then** the step fails with a
   descriptive network error; the flow does not hang.

## Tests

### E2E
- `internal/core/flow_gate_e2e_test.go`
  - `TestFlowGate_E2E_Pass` (scenario 1)
  - `TestFlowGate_E2E_Fail` (scenario 2)

### Unit
- `internal/core/flow_test.go`
  - `TestParseFlow_WithGate` (scenario 3)
  - `TestFlowExecutor_EvaGate_Pass` (scenario 1)
  - `TestFlowExecutor_EvaGate_Violation` (scenario 2)
- `internal/core/eva_gate_test.go`
  - `TestEvaGate_Pass`, `TestEvaGate_Violation`,
    `TestEvaGate_NilGate`, `TestEvaGate_ServerError`,
    `TestEvaGate_AuthHeader`, `TestEvaGate_Integration`

### Flow Test Fixtures
- `examples/flows/gate-checkpoint.yaml` — flow definition
- `examples/flows/fixtures/gate-checkpoint/pass/` — gate passes
- `examples/flows/fixtures/gate-checkpoint/fail/` — gate rejects
- `examples/flows/fixtures/gate-checkpoint/contracts/quality-check.yaml`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | gate pass → flow succeeds | `TestFlowGate_E2E_Pass` | ✅ |
| 2 | gate violation → flow fails | `TestFlowGate_E2E_Fail` | ✅ |
| 3 | YAML parsing populates StepGate | `TestParseFlow_WithGate` | ✅ |
| 4 | unreachable EVA → descriptive error | `TestEvaGate_ServerError` | ✅ |

## Related

- Task: T-0619
- Track: flow-use-cases
