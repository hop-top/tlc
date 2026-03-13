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

### E2E
- ❌ `tests/integration/task_test.go` — NOT YET IMPLEMENTED
  - Needed: `TestTaskListing`, `TestTaskListingWithFilters`, `TestTaskListingFormats`

### Unit
- ✅ `internal/cli/task_test.go` — PARTIAL
   - ✅ Exists: `TestTaskCommands/ListTasks` (covers basic listing)
   - ❌ Missing: Filter tests (status, tag, assignee, format)
   - ❌ Missing: `--assigned-to me` resolution test
   - ❌ Missing: Tag filter verification test
- ✅ `internal/core/query_test.go` — PARTIAL
   - ✅ Exists: `TestQuery_Structure` (query structure validation)
   - ❌ Missing: Actual filter execution tests
   - ❌ Missing: Assignee filter logic tests
   - ❌ Missing: Tag filter logic tests

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Basic list with columns (ID, Title, Status, Assigned) | `TestTaskCommands/ListTasks` | ⚠️ PARTIAL (columns not verified) |
| 2 | Filter by status (`--status TODO`) | Not tested | ❌ NOT COVERED |
| 3 | Filter by tag (`--tag auth`) | Not tested | ❌ NOT COVERED |
| 4a | Filter by assignee (`--assigned-to username`) | Not tested | ❌ NOT COVERED |
| 4b | Filter by current user (`--assigned-to me`) | Not tested | ❌ NOT COVERED |
| 5 | JSON format output | Not tested | ❌ NOT COVERED |
| 6 | TLS format output (task line syntax) | Not tested | ❌ NOT COVERED |
| 7 | Show all projects | Not tested | ❌ NOT COVERED |
| 8 | Include archived tasks | Not tested | ❌ NOT COVERED |
| 9 | Pagination (limit/offset) | Not tested | ❌ NOT COVERED |
| 10 | Sort by field and direction | Not tested | ❌ NOT COVERED |
| 11 | Full-text search | Not tested | ❌ NOT COVERED |
| 12 | YAML format output | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Create E2E test suite in `tests/integration/task_test.go` covering all 12 scenarios
- [ ] **HIGH PRIORITY**: Add tag filter test to `internal/cli/task_test.go` (verify `--tag auth` filters correctly)
- [ ] **HIGH PRIORITY**: Add assignee filter test to `internal/cli/task_test.go` (verify `--assigned-to engineer-1` filters correctly)
- [ ] **HIGH PRIORITY**: Add `--assigned-to me` resolution test (verify it resolves to current user from `GetCurrentUser()`)
- [ ] **HIGH PRIORITY**: Add status filter test to `internal/cli/task_test.go` (verify `--status TODO` filters correctly)
- [ ] Add format validation tests (JSON, YAML, TLS, table, summary) to CLI tests
- [ ] Verify table column ordering (ID, Title, Status, Assigned)
- [ ] Validate JSON output structure for scripting compatibility
- [ ] Validate YAML output structure
- [ ] Validate TLS format syntax: `[status] id title @assignee #tags ref:...`
- [ ] Test pagination (--limit, --offset)
- [ ] Test sorting (--sort-by, --sort-direction)
- [ ] Test full-text search
- [ ] Test --all-projects and --archived flags
