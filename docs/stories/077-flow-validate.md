# 077 - Flow Validate

**ID**: 077
**Feature**: Flow Orchestration — Pre-Run Validation
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**:
  [AI Agent](../personas/ai-agent.md),
  [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a solo developer, I want to validate a flow YAML file before
running it so I can catch structural errors (bad syntax, dangling
depends_on, cycles) without side effects.

## Scope

`tlc flow validate <file>` parses the flow file, runs the existing
`core.ValidateFlow` checks (YAML syntax, entry_step existence,
depends_on references, cycle detection), and reports success or
failure.

- Exit 0 + "flow is valid" on a structurally sound file.
- Exit 1 + error list on any validation failure.
- No storage required; no DB opened; no tasks created.

## Acceptance Scenarios

1. **Given** a valid 4-step flow YAML, **When**
   `tlc flow validate flow.yaml`, **Then** exit 0 and stdout
   contains "valid".

2. **Given** a flow YAML where step B depends_on non-existent
   step "phantom", **When** `tlc flow validate flow.yaml`,
   **Then** exit 1 and stderr contains "non-existent step phantom".

3. **Given** a flow YAML with a cycle (A -> B -> A via depends_on),
   **When** `tlc flow validate flow.yaml`, **Then** exit 1 and
   stderr contains "cycle detected".

## Tests

### E2E
- `internal/cli/flow_validate_e2e_test.go`
  - `TestFlowValidate_E2E_Valid` (scenario 1)
  - `TestFlowValidate_E2E_InvalidMissingDep` (scenario 2)
  - `TestFlowValidate_E2E_InvalidCycle` (scenario 3)

## Fixtures

- `examples/flows/fixtures/validate/valid/flow.yaml`
- `examples/flows/fixtures/validate/invalid-cycle/flow.yaml`
- `examples/flows/fixtures/validate/invalid-missing-dep/flow.yaml`

## Related

- Task: T-0626
- Depends on: `core.ValidateFlow` in `internal/core/flow_parser.go`
