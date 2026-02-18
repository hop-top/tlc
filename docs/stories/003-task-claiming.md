# 003 - Task Claiming

**ID**: 003
**Feature**: Task Management
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI Agent, I want to claim available tasks and transition them through workflow states so that I can work autonomously on assigned work and report results.

## Acceptance Scenarios

1. **Given** tasks with status `TODO` or `PENDING`, **When** I query via MCP tool `list_available_tasks(assigned_to: null)`, **Then** I receive a list of unclaimed tasks with their full context (title, description, tags, priority).

2. **Given** an available task, **When** I call `claim_task(task_id: "T-0001", assignee: "agent-1")`, **Then**:
    - The task status transitions to `IN_PROGRESS`
    - The assignee field is set to `agent-1`
    - Subsequent queries by other agents do NOT return this task

3. **Given** a claimed task, **When** I call `transition_task_state(task_id: "T-0001", new_state: "DONE", result: "...")`, **Then**:
    - Task status transitions to `DONE`
    - A result artifact is attached to task
    - Logs record of state transition with timestamp and agent ID

4. **Given** a claimed task that fails, **When** I call `transition_task_state(task_id: "T-0001", new_state: "FAILED", error: "...")`, **Then**:
    - Task status transitions to `FAILED`
    - Error details are stored on task
    - The task becomes available for re-claiming or manual intervention

5. **Given** multiple agents running concurrently, **When** they simultaneously attempt to claim the same task, **Then** exactly one succeeds and others receive a conflict error (optimistic lock semantics).

6. **Given** a claimed task, **When** I run `tlc task claim T-0001 --note "Starting implementation"`, **Then** task is claimed with optional note.

7. **Given** a claimed task, **When** I run `tlc task unclaim T-0001 --note "Blocked, releasing"`, **Then**:
    - Task status transitions to `TODO`
    - Assignee is cleared (set to null)
    - Note is recorded

8. **Given** a synced task from GitHub/Jira, **When** I claim it, **Then** the task is synced back to the origin system.

9. **Given** valid state transitions, **When** I transition:
    - TODO → IN_PROGRESS ✅
    - TODO → SKIPPED ✅
    - IN_PROGRESS → DONE ✅
    - IN_PROGRESS → TODO ✅
    - IN_PROGRESS → SKIPPED ✅
    **Then** transitions succeed.

10. **Given** invalid state transitions, **When** I attempt:
    - DONE → anything ❌ (terminal state)
    - SKIPPED → anything ❌ (terminal state)
    - TODO → DONE ❌ (must go through IN_PROGRESS)
    **Then** transitions fail with validation errors.

## Tests

### E2E
- ❌ `tests/integration/agent_test.go` — NOT YET IMPLEMENTED
  - Needed: `TestAgentTaskClaiming`, `TestAgentStateTransition`, `TestAgentConcurrentClaiming`, `TestAgentErrorHandling`

### Unit
- ✅ `internal/core/task_test.go` — PARTIAL
   - ✅ Exists: `TestTask_Transition` (basic state transitions)
   - ✅ Exists: `TestValidateTransition` (state validation)
   - ❌ Missing: Assignee field verification in claiming tests
   - ❌ Missing: Assignee clearing in unclaiming tests
   - ❌ Missing: MCP tool interface tests
   - ❌ Missing: Concurrent claiming/locking tests
   - ❌ Missing: Error transition tests
- ❌ `internal/core/claim_test.go` — NOT YET CREATED
   - Needed: Claiming logic and lock semantics
   - Needed: Assignee field behavior verification

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | List available tasks via MCP (`assigned_to: null`) | Not tested | ❌ NOT COVERED |
| 2a | Claim sets status to IN_PROGRESS | Partial | ⚠️ PARTIAL |
| 2b | Claim sets assignee field | Not tested | ❌ NOT COVERED |
| 2c | Claim prevents re-claiming (exclusivity) | Not tested | ❌ NOT COVERED |
| 3 | Transition to DONE with result artifact | Partial | ⚠️ PARTIAL |
| 4 | Transition to FAILED with error details | Not tested | ❌ NOT COVERED |
| 5 | Concurrent claiming with optimistic locking | Not tested | ❌ NOT COVERED |
| 6 | Claim with optional note | Not tested | ❌ NOT COVERED |
| 7a | Unclaim sets status to TODO | Partial | ⚠️ PARTIAL |
| 7b | Unclaim clears assignee (set to null) | Not tested | ❌ NOT COVERED |
| 8 | Sync claimed task to origin system | Not tested | ❌ NOT COVERED |
| 9 | Valid state transitions (TODO→IN_PROGRESS→DONE/SKIPPED) | `TestValidateTransition` | ✅ COVERED |
| 10 | Invalid state transitions (terminal states, direct TODO→DONE) | `TestValidateTransition` | ✅ COVERED |

## TODO

- [ ] Create E2E test suite in `tests/integration/agent_test.go` with concurrent scenarios
- [ ] **HIGH PRIORITY**: Add assignee field verification test in claiming (verify assignee is set to claimant)
- [ ] **HIGH PRIORITY**: Add assignee clearing test in unclaiming (verify assignee is set to null)
- [ ] **HIGH PRIORITY**: Add exclusivity test (prevent re-claiming already claimed tasks)
- [ ] Create `internal/core/claim_test.go` for claiming and locking semantics
- [ ] Add MCP tool interface tests for `list_available_tasks()`, `claim_task()`, `transition_task_state()`
- [ ] Implement optimistic locking test (concurrent claim attempts)
- [ ] Test error transition and error artifact storage
- [ ] Test concurrent agent scenarios with race condition detection
- [ ] Test claim/unclaim with optional notes
- [ ] Test syncing to origin system on claim/unclaim
- [ ] Validate TODO.md sync after claim/unclaim (syncTODOAll)
