---
status: shipped-no-e2e
---

# 077 - Conditional Branching

**ID**: 077
**Feature**: Flow Orchestration — Conditional Steps
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md),
  [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a Solo Developer, I want steps to run conditionally based on flow
inputs so that one flow definition handles multiple scenarios (e.g.
staging vs production deploy) without duplicating definitions.

## Acceptance Scenarios

1. **Given** a flow with `condition: "inputs.target == 'staging'"` on a
   step, **When** I run the flow with `target: staging`, **Then** the
   step executes and completes successfully.

2. **Given** a flow with `condition: "inputs.target == 'staging'"` on a
   step, **When** I run the flow with `target: production`, **Then** the
   step is skipped with status `SKIPPED` (not `FAILED`).

3. **Given** a flow with two mutually exclusive conditional paths
   (staging steps + production steps), **When** I run with
   `target: staging`, **Then** only staging steps execute and production
   steps show `SKIPPED`.

4. **Given** a flow with two mutually exclusive conditional paths,
   **When** I run with `target: production`, **Then** only production
   steps execute and staging steps show `SKIPPED`.

5. **Given** a conditional step that is skipped, **When** the flow
   completes, **Then** dependent steps that reference only skipped
   parents are also skipped.

## Tests

### E2E
- `tests/e2e/flow_conditional_test.go::TestFlow_ConditionalStaging`
- `tests/e2e/flow_conditional_test.go::TestFlow_ConditionalProduction`
- planned: `tests/e2e/flow_conditional_test.go::TestFlow_ConditionalSkipPropagatesToDependents`
- planned: `tests/e2e/flow_conditional_test.go::TestFlow_ConditionalMutuallyExclusivePaths`

Fixtures:
- `examples/flows/conditional.yaml` — flow definition
- `examples/flows/fixtures/conditional/staging/` — staging run cassettes
- `examples/flows/fixtures/conditional/production/` — production run
  cassettes
- Run via:
  - `tlc flow test examples/flows/conditional.yaml staging`
  - `tlc flow test examples/flows/conditional.yaml production`

### Unit
- Condition evaluation with string equality
- Condition evaluation with missing input (defaults to false)
- Step skip propagation to dependents

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Condition true -> step runs | staging fixture | COVERED |
| 2 | Condition false -> SKIPPED | production fixture | COVERED |
| 3 | Staging path only | staging fixture | COVERED |
| 4 | Production path only | production fixture | COVERED |
| 5 | Skip propagation | both fixtures | COVERED |
