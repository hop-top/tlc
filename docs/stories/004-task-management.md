---
status: shipped-no-e2e
---

# 004 - Task Management (Update, Show, Delete)

**ID**: 004
**Feature**: Task Management
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [Team Lead](../personas/team-lead.md), [AI Agent](../personas/ai-agent.md)
**Priority**: P1

## Story

As a Solo Developer, I want to update, view details of, and delete tasks so that I can manage work progress, track changes, and clean up completed or obsolete tasks.

## Acceptance Scenarios

1. **Given** a task exists with ID `T-0001`, **When** I run `tlc task update T-0001 --title "Updated title" --description "New description"`, **Then**:
    - Task title and description are updated
    - UpdatedAt timestamp is refreshed
    - Log entry records the changes

2. **Given** a task exists, **When** I run `tlc task update T-0001 --status DONE`, **Then**:
    - Task status transitions to DONE
    - State transition is validated (must be allowed)
    - Log entry records the status change

3. **Given** a task exists, **When** I run `tlc task update T-0001 --assigned-to engineer-2`, **Then** assignee field is set to `engineer-2`.

4. **Given** a task exists, **When** I run `tlc task update T-0001 --assigned-to null`, **Then** assignee field is cleared (set to nil).

5. **Given** a task with tags, **When** I run `tlc task update T-0001 --add-tag urgent --remove-tag chore`, **Then**:
    - `urgent` tag is added to task
    - `chore` tag is removed from task
    - Other tags remain unchanged

6. **Given** a task exists, **When** I run `tlc task show T-0001`, **Then** task details are displayed in table format with:
    - ID, Title, Status, Assigned
    - Tags (comma-separated)
    - Reference URL
    - Created and Updated timestamps
    - Description (formatted with markdown)

7. **Given** a task exists, **When** I run `tlc task show T-0001 --logs`, **Then**:
    - Task details are displayed
    - All log entries are shown with:
      - Timestamp
      - Action
      - Who made the change
      - Note

8. **Given** a task exists, **When** I run `tlc task show T-0001 --format json`, **Then** output includes task and logs in JSON format.

9. **Given** a task exists, **When** I run `tlc task show T-0001 --format yaml`, **Then** output includes task and logs in YAML format.

10. **Given** a task exists, **When** I run `tlc task delete T-0001 --yes`, **Then**:
    - Task is deleted from storage
    - Task is deleted from origin system (if synced)
    - Confirmation prompt is skipped

11. **Given** a task exists, **When** I run `tlc task delete T-0001`, **Then**:
    - Interactive confirmation prompt appears
    - User must confirm before deletion
    - Task is deleted only after confirmation

12. **Given** a synced task from GitHub/Jira, **When** I update it, **Then** changes are synced back to the origin system.

13. **Given** a synced task, **When** I delete it, **Then** task is deleted from the origin system.

## Tests

### E2E
- ❌ `tests/integration/task_test.go` — NOT YET IMPLEMENTED
  - Needed: `TestTaskUpdate`, `TestTaskShow`, `TestTaskDelete`, `TestTaskUpdateSync`

### Unit
- ✅ `internal/cli/task_test.go` — PARTIAL
   - ✅ Exists: `TestTaskCommands/UpdateTask` (covers scenario 2, partial 1)
   - ✅ Exists: `TestTaskCommands/ShowTask` (covers scenario 7, partial 10)
   - ✅ Exists: `TestTaskCommands/DeleteTask` (covers scenario 10)
   - ❌ Missing: Assignee update test (scenario 3)
   - ❌ Missing: Assignee null/nil test (scenario 4)
   - ❌ Missing: Tag add tests (scenario 5)
   - ❌ Missing: Tag remove tests (scenario 5)
   - ❌ Missing: Show with --logs flag (scenario 7)
   - ❌ Missing: Show format tests (JSON, YAML) (scenarios 8-9)
   - ❌ Missing: Interactive confirmation test (scenario 11)
   - ❌ Missing: Sync tests (scenarios 12-13)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Update title and description | `TestTaskCommands/UpdateTask` | ⚠️ PARTIAL |
| 2 | Update status (with validation) | `TestTaskCommands/UpdateTask` | ⚠️ PARTIAL |
| 3 | Update assignee (`--assigned-to engineer-2`) | Not tested | ❌ NOT COVERED |
| 4a | Clear assignee with `--assigned-to null` | Not tested | ❌ NOT COVERED |
| 4b | Clear assignee with `--assigned-to -` | Not tested | ❌ NOT COVERED |
| 5a | Add tag (`--add-tag urgent`) | Not tested | ❌ NOT COVERED |
| 5b | Remove tag (`--remove-tag chore`) | Not tested | ❌ NOT COVERED |
| 5c | Other tags unchanged after add/remove | Not tested | ❌ NOT COVERED |
| 6 | Show task details (table format) | `TestTaskCommands/ShowTask` | ⚠️ PARTIAL |
| 7 | Show task with logs (`--logs`) | Not tested | ❌ NOT COVERED |
| 8 | Show JSON format (`--format json`) | Not tested | ❌ NOT COVERED |
| 9 | Show YAML format (`--format yaml`) | Not tested | ❌ NOT COVERED |
| 10 | Delete task (--yes flag) | `TestTaskCommands/DeleteTask` | ⚠️ PARTIAL |
| 11 | Delete with interactive confirmation | Not tested | ❌ NOT COVERED |
| 12 | Update syncs to origin system | Not tested | ❌ NOT COVERED |
| 13 | Delete syncs to origin system | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Create E2E test suite in `tests/integration/task_test.go` covering all 13 scenarios
- [ ] **HIGH PRIORITY**: Add assignee update test (`--assigned-to engineer-2`) to CLI tests
- [ ] **HIGH PRIORITY**: Add assignee clear test (`--assigned-to null` / `--assigned-to -`) to CLI tests
- [ ] **HIGH PRIORITY**: Add tag add test (`--add-tag urgent`) to CLI tests
- [ ] **HIGH PRIORITY**: Add tag remove test (`--remove-tag chore`) to CLI tests
- [ ] **HIGH PRIORITY**: Verify other tags remain unchanged after add/remove operations
- [ ] Add show with --logs flag test
- [ ] Add show format tests (JSON, YAML)
- [ ] Add interactive confirmation test for delete
- [ ] Test syncing to origin system on update/delete
- [ ] Validate TODO.md sync after update/delete (syncTODOAll)
- [ ] Test state transition validation on update
