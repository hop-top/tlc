---
status: shipped-no-e2e
---

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
    - A tlc:// URL reference (e.g., `tlc://T-0001`)
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

### Unit / CLI E2E
- ✅ `internal/cli/task_create_test.go` — `TestTaskCreate/CreateTask` (basic create, title, storage row)
- ✅ `internal/cli/task_create_test.go` — `TestTaskCreate/CreateTaskWithAssignee` + standalone `TestTaskCreateWithAssignee` (assignee persisted)
- ✅ `internal/cli/task_create_test.go` — `TestTaskCreate/CreateTaskWithTags` + standalone `TestTaskCreateWithTags` (tags persisted)
- ✅ `internal/cli/task_create_test.go` — `TestTaskCreate/CreateTaskWithCustomID` (`--id` flag)
- ✅ `internal/cli/task_create_test.go` — `TestTaskCreate/CreateTaskWithReference` (`--reference` flag)
- ✅ `internal/cli/task_create_test.go` — `TestTaskCreate/VerifyIDSequencing` (no-conflict sequencing)
- ✅ `internal/cli/task_create_test.go` — `TestBuildTaskReference` (`tlc://` URL format)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/FilterTasksByTag` (tag queryability)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/FilterTasksByAssignee` (assignee queryability)
- ❌ Interactive mode (`-i`) — no test
- ❌ Auto-detect `project_id` populated on created task — no direct test (only URL-builder unit test covers project formatting)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Basic task creation (title, auto-ID, status, reference, project_id) | `TestTaskCreate/CreateTask` + `TestBuildTaskReference` | ⚠️ PARTIAL (project_id auto-detect on task row not asserted) |
| 2a | Create with tags (`--tag auth --tag security`) | `TestTaskCreate/CreateTaskWithTags` | ✅ COVERED |
| 2b | Create with assignee (`--assigned-to engineer-1`) | `TestTaskCreate/CreateTaskWithAssignee` | ✅ COVERED |
| 2c | Queryable by tag | `TestTaskList/FilterTasksByTag` | ✅ COVERED |
| 2d | Queryable by assignee | `TestTaskList/FilterTasksByAssignee` | ✅ COVERED |
| 3 | Interactive mode prompts (status limited to TODO/IN_PROGRESS/DONE, tags limited to predefined) | Not tested | ❌ NOT COVERED |
| 4 | ID sequencing (no conflicts) | `TestTaskCreate/VerifyIDSequencing` | ✅ COVERED |
| 5 | Custom ID and reference fields | `TestTaskCreate/CreateTaskWithCustomID` + `TestTaskCreate/CreateTaskWithReference` | ✅ COVERED |
| 6 | Auto-detect project_id populated on task row | Not tested directly | ❌ NOT COVERED |

## TODO

- [ ] Add interactive mode test (`tlc task create -i`) — verify status options limited to TODO/IN_PROGRESS/DONE
- [ ] Add direct test that auto-detected `project_id` is persisted on the created task row (separate from `TestBuildTaskReference`)
- [ ] Verify TODO.md sync after task creation (syncTODOAll)
