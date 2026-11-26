---
status: shipped
---

# 094 - Recipe Test Gates

**ID**: 094
**Feature**: Recipe Orchestration - Test-Run with Human Gates
**Persona**: [Test Engineer](../personas/test-engineer.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md),
[AI Agent](../personas/ai-agent.md)
**Priority**: P2

## Story

As a Test Engineer, I want `tlc recipe test` to apply scripted decisions
to a recipe's human steps, and to follow the same create-time `when:`
rule as a real create, so that a recipe with an approval gate is tested
end to end, offline, without anyone answering a prompt.

## Acceptance Scenarios

1. **Given** a run whose `test.yaml` declares `gates: {review: approve}`,
   **When** I run `tlc recipe test <recipe> <run>`, **Then** the gate is
   approved, the run resumes past it, steps reading its decision see
   `approved`, and the run passes.

2. **Given** a gate declared `reject` with a reason, **When** the run
   executes, **Then** the gate stays blocked, nothing behind it runs, and
   the run passes with `expected_exit: 0`.

3. **Given** a human step with no declared decision, **When** the run
   reaches it, **Then** the run fails (exit 1) naming the gate.

4. **Given** a declared gate that is not a human step, is not a step of
   the recipe, or a reject without a reason, **When** the run is set up,
   **Then** it exits 3 before anything executes.

5. **Given** a declared gate left out by `--task`, or on a step whose
   create-time `when:` is false for the run's vars, **When** the run
   executes, **Then** the gate needs no decision and is not an error.

6. **Given** a step whose `when:` the run's vars make false, **When** the
   run executes, **Then** the step is not created and the run passes with
   it absent; a `when:` reading `results.*` is still skipped at run time
   and the run passes.

7. **Given** `--record`, **When** a run has gates, **Then** the same
   decisions are applied without prompting.

## Tests

### E2E
- `internal/cli/recipe_test_gates_e2e_test.go::TestRecipeTest_Gates_ApproveRunsPastTheGate` (1)
- `internal/cli/recipe_test_gates_e2e_test.go::TestRecipeTest_Gates_RejectBlocksTheRest` (2)
- `internal/cli/recipe_test_gates_e2e_test.go::TestRecipeTest_Gates_MissingDecisionFails` (3)
- `internal/cli/recipe_test_gates_e2e_test.go::TestRecipeTest_Gates_FixtureErrorsAreSetupFailures` (4)
- `internal/cli/recipe_test_gates_e2e_test.go::TestRecipeTest_Gates_TaskSubsetBeforeGateNeedsNoDecision` (5)
- `internal/cli/recipe_test_gates_e2e_test.go::TestRecipeTest_Gates_DeclaredGateOnVarsDroppedStepNeedsNoDecision` (5)
- `internal/cli/recipe_test_cmd_e2e_test.go::TestRecipeTest_E2E_VarsFalseStepAbsent` (6)
- `internal/cli/recipe_test_cmd_e2e_test.go::TestRecipeTest_E2E_WhenSkippedStepPasses` (6)
- `internal/cli/recipe_test_cmd_e2e_test.go::TestRecipeTest_E2E_FailedStepStillFails` (6)
- `internal/cli/recipe_test_gates_e2e_test.go::TestRecipeTest_Gates_RecordAppliesDecisionsNonInteractively` (7)

### Unit
- `internal/cli/recipe_test_gates_grade_test.go::TestGradeRecipeRun_Gates`
- `internal/flowtest/gates_test.go` - manifest gate parsing
- `internal/core/human_decision_test.go` - decision recorded in the result
- `internal/core/recipe_when_test.go` - create-time `when:` drop

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---------------|--------|
| 1 | Approve and resume | `TestRecipeTest_Gates_ApproveRunsPastTheGate` | COVERED |
| 2 | Reject blocks the rest | `TestRecipeTest_Gates_RejectBlocksTheRest` | COVERED |
| 3 | Undecided gate fails | `TestRecipeTest_Gates_MissingDecisionFails` | COVERED |
| 4 | Fixture errors exit 3 | `TestRecipeTest_Gates_FixtureErrorsAreSetupFailures` | COVERED |
| 5 | Left-out gate needs no decision | `..._TaskSubsetBeforeGateNeedsNoDecision`, `..._DeclaredGateOnVarsDroppedStepNeedsNoDecision` | COVERED |
| 6 | Vars-false absent; results-false skipped | `TestRecipeTest_E2E_VarsFalseStepAbsent`, `TestRecipeTest_E2E_WhenSkippedStepPasses` | COVERED |
| 7 | Record applies decisions | `TestRecipeTest_Gates_RecordAppliesDecisionsNonInteractively` | COVERED |
