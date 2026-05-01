---
status: shipped
---

# 079 - Flow Pause/Resume

**ID**: 079
**Feature**: Flow Orchestration — Pause and Resume
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md),
[Team Lead](../personas/team-lead.md)
**Priority**: P3

## Story

As a Solo Developer, I want to pause a running flow and resume it
later so that I can handle manual interventions, take breaks, or
adjust parameters mid-flow without losing progress.

## Acceptance Scenarios

1. **Given** a flow run in RUNNING status with step 1 completed,
   **When** PauseFlowRun is called,
   **Then** the run status transitions to PAUSED and step 2 does
   not start.

2. **Given** a flow run in PAUSED status,
   **When** ResumeFlowRun is called,
   **Then** the run status transitions back to RUNNING and execution
   continues from the next pending step.

3. **Given** a flow run in PAUSED status,
   **When** the executor checks status,
   **Then** it waits (polls) until the status changes to RUNNING
   before continuing step execution.

4. **Given** a flow run in QUEUED status,
   **When** PauseFlowRun is called,
   **Then** the operation fails (only RUNNING runs can be paused).

5. **Given** a flow run paused after step 1 of 3,
   **When** resumed,
   **Then** steps 2 and 3 execute and the final status is SUCCEEDED.

## Tests

### E2E
- `internal/cli/flow_pause_resume_e2e_test.go`
  -- `TestFlowPauseResume_CoreRoundTrip` (scenarios 1-2)
  -- `TestFlowPauseResume_ExecutorWaits` (scenario 3)

### Unit
- `internal/core/flow_test.go`
  -- `TestFlowExecutor_PauseResume` (scenario 3)
- `internal/core/domain_service_test.go`
  -- `TestTaskService_WithFlowDomainRepo_PauseResume` (scenarios 1-2)

### Fixtures
- `examples/flows/fixtures/pause-resume/happy-path/` -- cassettes
