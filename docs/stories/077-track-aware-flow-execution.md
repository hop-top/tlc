# 077 - Track-Aware Flow Execution

**ID**: 077
**Feature**: Flow Orchestration — Track-Aware Execution
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md),
[Solo Developer](../personas/solo-developer.md)
**Priority**: P2

## Story

As a Team Lead, I want to run a flow that operates on a track's
tasks in dependency order so that I can automate the entire lifecycle
of a track (execute, verify, review, ship) without manually
sequencing work.

## Acceptance Scenarios

1. **Given** a track `auth-rewrite` with 3 tasks (T-0001 blocked-by
   none, T-0002 blocked-by T-0001, T-0003 blocked-by T-0002),
   **When** I run `tlc flow invoke track-aware.yaml --var track=auth-rewrite`,
   **Then** tasks are created referencing the track, and the flow
   resolves task execution in dependency order.

2. **Given** a flow with `config.inputs.track.required: true`,
   **When** I omit `--var track=...`,
   **Then** the command fails with an error mentioning `track`.

3. **Given** a track with tasks that have dependencies,
   **When** the flow completes,
   **Then** all tasks are processed in topological order (no task
   runs before its blockers).

4. **Given** a track with mixed TODO and DONE tasks,
   **When** the flow runs,
   **Then** only TODO tasks are included in the execution plan.

## Tests

### E2E
- `internal/cli/flow_track_aware_e2e_test.go`
  -- `TestFlowInvoke_TrackAware_CreatesTasksForTrack` (scenario 1)
- `internal/cli/flow_invoke_var_inputs_e2e_test.go`
  -- `TestFlowInvoke_VarInputs_MissingRequiredFails` (scenario 2)

### Fixtures
- `examples/flows/track-aware.yaml` -- flow definition
- `examples/flows/fixtures/track-aware/happy-path/` -- cassettes
