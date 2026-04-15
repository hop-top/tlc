# 079 - Multi-Agent Step Dispatch

**ID**: 079
**Feature**: Flow Orchestration — Per-Step Agent Selection
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**:
  [Team Lead](../personas/team-lead.md),
  [Solo Developer](../personas/solo-developer.md)
**Priority**: P2

## Story

As an AI agent orchestrating a flow, I want different steps to use
different agent adapters so I can leverage each agent's strengths
(e.g. claude for planning, codex for implementation, gemini for
review).

## Scope

A flow definition declares `agent` at the flow level (default) and
optionally overrides per step. The `FlowExecutor` + `AgentRunner`
dispatch the correct agent for each step. This story validates:

1. Step-level `agent:` overrides the flow-level default.
2. The correct agent name reaches the `AgentRunner.Run` call.
3. Results are collected from each agent independently.

## Acceptance Scenarios

1. **Given** a 3-step flow with agents `claude`, `codex`, `gemini`,
   **When** executed via `FlowExecutor` with a mock `AgentRunner`,
   **Then** the runner receives exactly 3 calls, each with the
   correct `step.Agent.Name`.

2. **Given** a flow where step B has no `agent:` field but the
   flow-level default is `claude`, **When** executed, **Then**
   step B inherits `claude` as its agent (verified via context
   builder, not runner routing).

3. **Given** dry-run on the multi-agent flow, **When**
   `tlc flow run --dry-run multi-agent.yaml`, **Then** output
   shows the correct agent per step.

## Tests

### E2E
- `internal/cli/flow_multi_agent_e2e_test.go`
  - `TestFlowMultiAgent_E2E_CorrectAgentPerStep` (scenario 1)
  - `TestFlowMultiAgent_E2E_DryRunShowsAgents` (scenario 3)

## Fixtures

- `examples/flows/multi-agent.yaml`
- `examples/flows/fixtures/multi-agent/happy-path/`
  - `record/.gitkeep`
  - `contracts/.gitkeep`

## Related

- Task: T-0621
- `core.AgentRef` in `internal/core/flow.go`
- `Step.Agent` field in `internal/core/flow.go`
- `MockAgentRunner` in `internal/core/mocks.go`
