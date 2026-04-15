# 078 - Flow Test-Run with Contracts

**ID**: 078
**Feature**: Flow Orchestration — Test-Run with Contract Validation
**Persona**: [Test Engineer](../personas/test-engineer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md),
[Solo Developer](../personas/solo-developer.md)
**Priority**: P2

## Story

As a Test Engineer, I want to test-run a flow with contract
validation against step outputs so that I can verify flow
correctness in staging before committing to a full execution.

## Acceptance Scenarios

1. **Given** a flow with a contracts directory containing evaluator
   YAML files,
   **When** I run `tlc flow test <flow.yaml> happy-path`,
   **Then** contracts are evaluated against step outputs and the
   run exits 0 when all pass.

2. **Given** a flow with a contracts directory containing a failing
   evaluator,
   **When** I run `tlc flow test <flow.yaml> happy-path`,
   **Then** the run exits non-zero with a message identifying the
   failed contract.

3. **Given** a flow with no contracts directory,
   **When** I run `tlc flow test <flow.yaml> happy-path`,
   **Then** the test-run executes normally (no contract evaluation,
   exit 0 if steps pass).

4. **Given** a flow with `--test-run` flag,
   **When** the flow produces output matching contract evaluators,
   **Then** all evaluator results are reported (pass/fail per
   evaluator).

## Tests

### E2E
- `internal/cli/flow_test_run_e2e_test.go`
  -- `TestFlowTestRun_ContractPass` (scenario 1)
  -- `TestFlowTestRun_InvokesSuccessfully` (scenario 3)

### Fixtures
- `examples/flows/fixtures/test-run/happy-path/` -- cassettes
  and contracts
