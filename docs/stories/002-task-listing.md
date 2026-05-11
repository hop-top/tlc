---
status: shipped
---

# 002 - Task Listing

**ID**: 002
**Feature**: Task Management
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a Solo Developer, I want to query and list tasks with flexible filtering so that I can review my work, track progress, and find tasks by status, tag, or assignee.

## Acceptance Scenarios

1. **Given** multiple tasks exist in TLC, **When** I run `tlc task list`, **Then** active tasks (`IN_PROGRESS` and `TODO`) are displayed in a table with columns: ID, Title, Status, Assignee, with `IN_PROGRESS` tasks shown first.

2. **Given** tasks with mixed statuses, **When** I run `tlc task list --status TODO`, **Then** only tasks with status `TODO` are displayed.

3. **Given** tasks with tags `auth` and `security`, **When** I run `tlc task list --tag auth`, **Then** only tasks tagged with `auth` are shown.

4. **Given** tasks assigned to different people, **When** I run `tlc task list --assigned-to me`, **Then** only tasks assigned to current user are shown.

5. **Given** a large list of tasks, **When** I run `tlc task list --format json`, **Then** output is valid JSON suitable for scripting or piping to other tools.

6. **Given** tasks for quick reference, **When** I run `tlc task list --format tls`, **Then** tasks are displayed in TLS (Task Line Syntax) format: `[status] id title @assignee #tags ref:...`

7. **Given** tasks across projects, **When** I run `tlc task list --all-projects`, **Then** tasks from all projects are shown (not filtered by current project).

8. **Given** including completed work, **When** I run `tlc task list --archived`, **Then** archived tasks are included in results.

9. **Given** many tasks, **When** I run `tlc task list --limit 10 --offset 20`, **Then** only 10 tasks are shown starting from position 20 (pagination).

10. **Given** tasks, **When** I run `tlc task list --sort-by created_at --sort-direction asc`, **Then** tasks are sorted by created_at in ascending order.

11. **Given** tasks, **When** I run `tlc task list "search term"`, **Then** only tasks matching the full-text search term are shown.

12. **Given** tasks, **When** I run `tlc task list --format yaml`, **Then** output is valid YAML format.

## Tests

### Unit / CLI E2E
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/ListTasks` (basic listing)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/FilterTasksByStatus` (status filter)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/FilterTasksByTag` (tag filter)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/FilterTasksByAssignee` (assignee filter)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/FilterTasksByAssigneeMe` (`--assigned-to me` resolution)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/ListTasksJSONFormat` (JSON output)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/ListTasksYAMLFormat` (YAML output)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/ListTasksTLSFormat` (TLS task line syntax)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/ListTasksSort` (sort by created_at)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/ListTasksFullTextSearch` (full-text search)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/ListTasksAllProjects` (`--all-projects`)
- ✅ `internal/cli/task_list_test.go` — `TestTaskList/ListTasksArchived` (`--archived`)
- ✅ `internal/cli/task_create_test.go` — `TestTaskCreatePagination` (`--limit` / `--offset`)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Basic list with columns (ID, Title, Status, Assigned) | `TestTaskList/ListTasks` | ⚠️ PARTIAL (rows present; explicit column-ordering assertion not made) |
| 2 | Filter by status (`--status TODO`) | `TestTaskList/FilterTasksByStatus` | ✅ COVERED |
| 3 | Filter by tag (`--tag auth`) | `TestTaskList/FilterTasksByTag` | ✅ COVERED |
| 4a | Filter by assignee (`--assigned-to username`) | `TestTaskList/FilterTasksByAssignee` | ✅ COVERED |
| 4b | Filter by current user (`--assigned-to me`) | `TestTaskList/FilterTasksByAssigneeMe` | ✅ COVERED |
| 5 | JSON format output | `TestTaskList/ListTasksJSONFormat` | ✅ COVERED |
| 6 | TLS format output (task line syntax) | `TestTaskList/ListTasksTLSFormat` | ✅ COVERED |
| 7 | Show all projects | `TestTaskList/ListTasksAllProjects` | ✅ COVERED |
| 8 | Include archived tasks | `TestTaskList/ListTasksArchived` | ✅ COVERED |
| 9 | Pagination (limit/offset) | `TestTaskCreatePagination` | ✅ COVERED |
| 10 | Sort by field and direction | `TestTaskList/ListTasksSort` | ✅ COVERED |
| 11 | Full-text search | `TestTaskList/ListTasksFullTextSearch` | ✅ COVERED |
| 12 | YAML format output | `TestTaskList/ListTasksYAMLFormat` | ✅ COVERED |

## TODO

- [ ] Tighten scenario 1: assert explicit table column ordering (ID, Title, Status, Assigned)
