# 001 - Task Creation

**ID**: 001
**Feature**: Task Management
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md), [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a Solo Developer, I want to create tasks with title, description, tags, and assignee so that I can track work and organize it by topic and ownership.

## Acceptance Scenarios

1. **Given** TLC is initialized, **When** I run `tlc task create "Fix auth bug"`, **Then** a task is created with:
    - Auto-generated ID (e.g., `T-0001`)
    - Status: `TODO`
    - Title: `Fix auth bug`
    - A task:// URL reference (e.g., `task://T-0001`)
    - Auto-detected project_id if in a project directory

2. **Given** TLC is initialized, **When** I run:
    ```
    tlc task create "Implement OAuth" \
      --tag auth \
      --tag security \
      --assigned-to engineer-1 \
      -d "Add OAuth2 flow with PKCE support"
    ```
    **Then** task is created with all fields populated and is queryable by tag and assignee.

3. **Given** TLC is initialized, **When** I run `tlc task create -i` (interactive mode), **Then** an interactive form prompts for:
    - Title (required)
    - Description (optional, 5-line text area)
    - Status (select: TODO, IN_PROGRESS, DONE)
    - Assigned-to (optional)
    - Tags (multi-select from: feat, fix, chore, docs, urgent)
    - Priority (select: P0-P3, defaults to P3)
    - Domain (optional)

4. **Given** a task already exists with ID `T-0001`, **When** I run `tlc task create "Another task"`, **Then** new task receives next ID (`T-0002`) and no conflict occurs.

5. **Given** TLC is initialized, **When** I run `tlc task create "Task with ID" --id T-9999 --reference "https://github.com/repo/issues/42"`, **Then** task is created with:
    - Custom ID: `T-9999`
    - Reference: `https://github.com/repo/issues/42`

6. **Given** TLC is initialized with domain context, **When** I run `tlc task create "Core task"`, **Then** task includes auto-detected project_id in metadata.

## Tests

### E2E
- ❌ `tests/integration/task_test.go` — NOT YET IMPLEMENTED
  - Needed: `TestTaskCreation`, `TestTaskCreationWithMetadata`, `TestInteractiveTaskCreation`, `TestTaskIDGeneration`

### Unit
- ✅ `internal/cli/task_test.go` — PARTIAL
   - ✅ Exists: `TestTaskCommands/CreateTask` (covers scenarios 1-2, tags creation)
   - ❌ Missing: Interactive mode validation (scenario 3)
   - ❌ Missing: ID sequencing/conflict validation (scenario 4)
   - ❌ Missing: Assignee field verification in created tasks
   - ❌ Missing: Tag queryability after creation (scenario 2)
- ✅ `internal/core/task_test.go` — PARTIAL
   - ✅ Exists: `TestTask_Transition` (basic state management)
   - ❌ Missing: Task creation and ID generation tests
   - ❌ Missing: Assignee field verification tests

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Basic task creation (title, auto-ID, status, reference, project_id) | `TestTaskCommands/CreateTask` | ⚠️ PARTIAL |
| 2a | Create with tags (`--tag auth --tag security`) | `TestTaskCommands/CreateTask` | ✅ COVERED |
| 2b | Create with assignee (`--assigned-to engineer-1`) | Not tested | ❌ NOT COVERED |
| 2c | Queryable by tag | Not tested | ❌ NOT COVERED |
| 2d | Queryable by assignee | Not tested | ❌ NOT COVERED |
| 3 | Interactive mode prompts (status limited to TODO/IN_PROGRESS/DONE, tags limited to predefined) | Not tested | ❌ NOT COVERED |
| 4 | ID sequencing (no conflicts) | Not tested | ❌ NOT COVERED |
| 5 | Custom ID and reference fields | Not tested | ❌ NOT COVERED |
| 6 | Auto-detect project_id | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Create E2E test suite in `tests/integration/task_test.go` covering all 6 scenarios
- [ ] Add interactive mode test to `internal/cli/task_test.go` (verify status options limited to TODO/IN_PROGRESS/DONE)
- [ ] Add ID generation and sequencing tests to `internal/core/task_test.go`
- [ ] Validate `task://` URL reference format in tests
- [ ] **HIGH PRIORITY**: Test assignee field creation (`--assigned-to engineer-1`) and verification in storage
- [ ] **HIGH PRIORITY**: Test tag queryability after creation (filter by `--tag auth`)
- [ ] **HIGH PRIORITY**: Test assignee queryability after creation (filter by `--assigned-to engineer-1`)
- [ ] Test custom task ID (--id flag)
- [ ] Test reference field (--reference flag)
- [ ] Test auto-detect project_id in task creation
- [ ] Verify TODO.md sync after task creation (syncTODOAll)
