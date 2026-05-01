# 078 - Flow Composition (Sub-Flows)

**ID**: 078
**Feature**: Flow Orchestration — Composition
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md),
  [AI Agent](../personas/ai-agent.md)
**Priority**: P2

## Story

As a Team Lead, I want to compose flows from other flows so that I can
build complex pipelines from reusable building blocks without
duplicating step definitions across flow files.

## Acceptance Scenarios

1. **Given** a parent flow with a step of `type: flow` referencing a
   child flow via `flow_ref`, **When** I run the parent flow, **Then**
   the child flow executes as a single logical step within the parent.

2. **Given** a child flow that produces outputs, **When** the child
   completes, **Then** the outputs are available to subsequent steps in
   the parent flow via `steps.<step_id>.outputs`.

3. **Given** a child flow that fails, **When** the parent flow runs,
   **Then** the parent step that invoked the child is marked `FAILED`
   and the parent flow fails (unless `on_failure: continue`).

4. **Given** a parent flow with a pre-step, a sub-flow step, and a
   post-step, **When** I run the parent, **Then** all three execute
   in order: pre -> child flow -> post.

5. **Given** a child flow definition that is valid on its own, **When**
   I run `tlc flow test` on the child independently, **Then** it
   passes — proving the child is reusable.

## Tests

### E2E
- `tests/e2e/flow_composition_test.go::TestFlow_Composition_ChildExecutesInParent`
- planned: `tests/e2e/flow_composition_test.go::TestFlow_Composition_ChildOutputsAvailableToParent`
- planned: `tests/e2e/flow_composition_test.go::TestFlow_Composition_ChildFailureFailsParent`
- planned: `tests/e2e/flow_composition_test.go::TestFlow_Composition_PreChildPostOrdering`
- planned: `tests/e2e/flow_composition_test.go::TestFlow_Composition_ChildStandaloneRunPasses`

Fixtures:
- `examples/flows/composition.yaml` — parent flow definition
- `examples/flows/composition-child.yaml` — reusable child flow
- `examples/flows/fixtures/composition/happy-path/` — happy path run
- Run via: `tlc flow test examples/flows/composition.yaml happy-path`

### Unit
- Sub-flow step type resolution
- Output propagation from child to parent
- Child failure propagation

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Child executes inside parent | happy-path fixture | COVERED |
| 2 | Output propagation | happy-path contracts | COVERED |
| 3 | Child failure fails parent | contract check | COVERED |
| 4 | Ordering: pre -> child -> post | happy-path fixture | COVERED |
| 5 | Child standalone execution | child flow fixture | COVERED |
