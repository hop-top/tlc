---
status: paper
---

# Configurable Task Statuses

**ID**: 004
**Feature**: Task Management
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md),
[AI Agent](../personas/ai-agent.md)
**Priority**: P2

## Story

As a team lead, I want to define custom task statuses and workflow
rules, so that my team's process is enforced by the tool.

## Acceptance Scenarios

1. **Given** no custom config, **When** I use TLC, **Then** default
   4-status workflow works unchanged
2. **Given** custom statuses in config, **When** I run
   `tlc workflow statuses`, **Then** I see all configured statuses
3. **Given** state machine rules, **When** I attempt an invalid
   transition, **Then** I get a clear error message
4. **Given** `--force` flag, **When** I update status, **Then**
   workflow rules are bypassed
5. **Given** per-tag overrides, **When** I claim a tagged task,
   **Then** the tag-specific workflow applies
6. **Given** custom TLS markers, **When** I export to TLS format,
   **Then** custom markers appear in brackets

## Tests

### E2E
- planned: `tests/e2e/workflow_test.go::TestWorkflow_DefaultStatuses`
- planned: `tests/e2e/workflow_test.go::TestWorkflow_CustomStatusesListed`
- planned: `tests/e2e/workflow_test.go::TestWorkflow_InvalidTransitionRejected`
- planned: `tests/e2e/workflow_test.go::TestWorkflow_ForceFlagBypassesRules`
- planned: `tests/e2e/workflow_test.go::TestWorkflow_PerTagOverrideApplies`
- planned: `tests/e2e/workflow_test.go::TestWorkflow_TLSMarkerExport`

### Unit
- `internal/core/workflow_test.go` — TestValidateTransition*,
  TestStatusForRole, TestStatusForTLSMarker, TestGetAllStatuses,
  TestIsTerminal, TestGetWorkflowForTags, TestCustomConfig
- `internal/config/config_test.go` — TestTaskConfig_*

### Integration
- `internal/cli/task_test.go` — TestTaskCommands (claim, unclaim,
  complete, update with status)
