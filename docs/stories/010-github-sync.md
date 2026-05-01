---
status: shipped-no-e2e
---

# 010 - GitHub Sync

**ID**: 010
**Feature**: External Sync
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As a Team Lead, I want to sync task state with GitHub issues so that team
work is visible in both TLC and GitHub, and I can use GitHub's existing
workflows and integrations.

## Context

GitHub sync is implemented as a `kit/ext.Extension` via the extension
manager in `internal/extensions/`. The extension declares capabilities
(sync provider) and is wired into the `kit/bus` event system so that
task lifecycle events (claim, complete, reopen) can trigger push
operations automatically. Configuration lives in the standard
`sync.github` config section loaded through `kit/config`.

## Acceptance Scenarios

1. **Given** a TLC task and a GitHub issue, **When** I run `tlc sync github --repo owner/repo --issue-number 123`, **Then**:
   - TLC links to local task to GitHub issue
   - A bidirectional reference is established (link metadata stored on both sides)

2. **Given** a linked TLC task and an updated GitHub issue (status changed, labels modified), **When** I run `tlc sync github --pull`, **Then**:
   - TLC task state is updated to reflect GitHub changes
   - Tags are synchronized (GitHub labels → TLC tags)
   - Assignee is synchronized

3. **Given** a TLC task with status `DONE` and a linked GitHub issue, **When** I run `tlc sync github --push`, **Then**:
   - GitHub issue status transitions to `closed`
   - A comment is posted linking back to TLC task
   - Timestamp is recorded

4. **Given** multiple tasks to sync, **When** I run `tlc sync github --repo owner/repo --batch`, **Then**:
   - All tasks matching the repo filter are synced
   - Progress is reported
   - Errors are collected and reported at the end

5. **Given** a broken sync link (task deleted externally), **When** I run `tlc sync github --validate`, **Then**:
   - Broken links are detected
   - User is notified and offered options to repair or delete the link

## Tests

### E2E
- ❌ `tests/integration/sync_test.go` — NOT YET IMPLEMENTED
  - Needed: `TestGitHubSync`, `TestGitHubSyncBidirectional`, `TestGitHubSyncBatch`, `TestSyncValidation`
- ❌ `tests/integration/github_integration_test.go` — NOT YET CREATED
  - Needed: `TestGitHubIssueMapping`, `TestGitHubLabelSync`

### Unit
- ✅ `internal/core/sync_test.go` — PARTIAL
  - ✅ Exists: `TestTask_NeedsPush`, `TestTask_NeedsPush_EdgeCases` (partial)
  - ❌ Missing: Bidirectional sync, label mapping, conflict resolution
- ✅ `internal/cli/sync_test.go` — GOOD COVERAGE
  - ✅ Exists: `TestSyncCommands/SyncConfig`, `TestSyncCommands/GitHubAutoConfiguration`, `TestSyncCommands/AutoConfigure direction upgrades`
- ✅ `internal/storage/github_sync_project_scoping_test.go` — EXISTS
  - ✅ Exists: GitHub sync scoping tests
- ❌ `internal/core/storage/github_sync_test.go` — NOT FULLY TESTED
  - Needed: Bidirectional sync, label mapping, conflict resolution

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Link task to GitHub issue | Not tested | ❌ NOT COVERED |
| 2a | Pull: GitHub status → TLC status | Not tested | ❌ NOT COVERED |
| 2b | Pull: GitHub labels → TLC tags | Not tested | ❌ NOT COVERED |
| 2c | Pull: GitHub assignee → TLC assignee | Not tested | ❌ NOT COVERED |
| 3 | Push: TLC→GitHub (close issue, comment) | Partial | ⚠️ PARTIAL |
| 4 | Batch sync with progress reporting | Not tested | ❌ NOT COVERED |
| 5 | Validation and broken link detection | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Create E2E test suite in `tests/integration/sync_test.go` covering all 5 scenarios
- [ ] Create `tests/integration/github_integration_test.go` with real GitHub API mocking
- [ ] **HIGH PRIORITY**: Add GitHub label→TLC tag mapping test (scenario 2b)
- [ ] **HIGH PRIORITY**: Add GitHub assignee→TLC assignee mapping test (scenario 2c)
- [ ] **HIGH PRIORITY**: Add bidirectional sync tests for both labels and assignees
- [ ] Add GitHub link creation tests (scenario 1)
- [ ] Add batch sync with error collection and reporting tests
- [ ] Add validation tests for broken links and repair workflows
- [ ] Test comment creation on GitHub when pushing changes
- [ ] Test conflict resolution when sync direction conflicts
