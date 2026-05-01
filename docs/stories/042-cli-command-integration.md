---
status: shipped-no-e2e
---

# 042 - CLI Command Integration

**ID**: 042
**Feature**: AI Agent Integration
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P3

## Story

As an AI Agent, I want to use TLC CLI commands directly, so that I can perform task management operations without requiring MCP or HTTP API infrastructure.

## Acceptance Scenarios

1. **Given** TLC CLI is installed and available in PATH, **When** AI agent executes `tlc task create`, **Then** a new task is created with the specified parameters
2. **Given** existing tasks in the system, **When** AI agent executes `tlc task list` with filters, **Then** only matching tasks are returned in a parseable format
3. **Given** a pending task, **When** AI agent executes `tlc task claim`, **Then** task is assigned to the agent and returned in JSON format
4. **Given** a claimed task in progress, **When** AI agent executes `tlc task update --status DONE`, **Then** task state transitions to DONE
5. **Given** a flow definition exists, **When** AI agent executes `tlc flow run`, **Then** flow execution begins and returns a run ID for tracking

## Notes

- CLI output should be machine-readable (JSON) by default when called by AI agents
- Exit codes should indicate success/failure clearly for programmatic handling
- Error messages should be structured to aid in programmatic error recovery

## Tests

### E2E
- `tests/integration/cli_integration_test.go` — NOT YET CREATED
  - Needed: `TestAIAgentCLITaskLifecycle`

### Unit
- `internal/cli/task_test.go` — Check for CLI command tests
- `internal/core/task_test.go` — Check for task lifecycle tests

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Task creation via CLI | Partially tested | ⚠️ PARTIAL |
| 2 | Task listing via CLI | Partially tested | ⚠️ PARTIAL |
| 3 | Task claiming via CLI | Not tested | ❌ NOT COVERED |
| 4 | Task status update via CLI | Not tested | ❌ NOT COVERED |
| 5 | Flow execution via CLI | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Create E2E test suite in `tests/integration/cli_integration_test.go` for all 5 scenarios
- [ ] Add CLI command tests to `internal/cli/task_test.go`
- [ ] Verify JSON output format for AI agent consumption
- [ ] Test exit codes and error handling
- [ ] Verify task creation with all parameters
- [ ] Verify task listing with all filter options
- [ ] Verify task claiming and state transitions
- [ ] Verify task status updates and result storage
- [ ] Verify flow execution and run ID tracking

## Status

📋 Planned - Story defined, detailed work pending
