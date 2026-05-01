---
status: shipped
---

# 078 - Flow Dry-Run

**ID**: 078
**Feature**: Flow Orchestration — Dry-Run Preview
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**:
  [AI Agent](../personas/ai-agent.md),
  [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a solo developer, I want to dry-run a flow to preview the
execution plan so I can verify step ordering and agent assignments
before committing resources.

## Scope

`tlc flow run --dry-run <file>` parses + validates the flow, then
walks all steps in dependency order (topological sort via the flow's
depends_on graph). For each step it prints:

- Step ID and title
- Step type (task, parallel, branch, ...)
- Agent (if set on step or flow level)
- Dependencies

No agents are dispatched. No flow run record is persisted. No tasks
are created or modified. No storage is opened.

## Acceptance Scenarios

1. **Given** a valid 4-step flow YAML with linear dependencies,
   **When** `tlc flow run --dry-run flow.yaml`, **Then** exit 0
   and stdout lists all 4 steps in topological order.

2. **Given** a valid flow, **When** `tlc flow run --dry-run`,
   **Then** no agents are dispatched and no DB writes occur.

3. **Given** an invalid flow (cycle), **When**
   `tlc flow run --dry-run flow.yaml`, **Then** exit 1 with
   validation error (same as `flow validate`).

## Tests

### E2E
- `internal/cli/flow_dry_run_e2e_test.go`
  - `TestFlowDryRun_E2E_HappyPath` (scenario 1)
  - `TestFlowDryRun_E2E_InvalidFlow` (scenario 3)

## Fixtures

- `examples/flows/fixtures/dry-run/happy-path/flow.yaml`

## Related

- Task: T-0627
- Depends on: `core.ValidateFlow`, flow step topological ordering
