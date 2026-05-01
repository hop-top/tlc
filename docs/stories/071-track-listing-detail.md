---
status: shipped
---

# 071 - Track Listing and Detail View

**ID**: 071
**Feature**: Track Management — Listing and Detail View
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md),
[Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a Solo Developer, I want to list tracks with filtering and view
detailed track information with phase breakdowns so that I can
monitor work stream progress.

## Acceptance Scenarios

1. **Given** tracks exist, **When** `tlc track list`, **Then** table
   shows columns: ID, Title, Type, Status, State, Progress, Assignee.

2. **Given** tracks of all statuses (pending, active, completed,
   abandoned, archived), **When** `tlc track list` runs with no
   `--status` flag, **Then** only pending + active tracks shown;
   terminal statuses (completed, abandoned, archived) excluded.

3. **Given** active and pending tracks, **When**
   `tlc track list --status active`, **Then** only active tracks
   shown.

4. **Given** tracks of different types, **When**
   `tlc track list --type feature`, **Then** only feature tracks
   shown.

5. **Given** stale and healthy tracks, **When**
   `tlc track list --state stale`, **Then** only stale tracks shown.

6. **Given** tracks, **When** `tlc track list --format json`,
   **Then** JSON array output with all fields.

7. **Given** track "browser-rendering" with phased tasks, **When**
   `tlc track show browser-rendering`, **Then** detail view shows:
   header (type, status, state, assignee), progress %, per-phase
   breakdown with task listings, unphased section.

8. **Given** track with tasks in phases 1-3, **When**
   `tlc track show <id>`, **Then** completed phases show checkmark,
   current phase shows progress, tasks colored by status.

9. **Given** track with no linked tasks, **When**
   `tlc track show <id>`, **Then** shows "no tasks linked" and state
   "unlinked".

10. **Given** nonexistent track, **When**
    `tlc track show nonexistent`, **Then** error with actionable
    message containing "not found".

11. **Given** user inside a `.tlc/`-configured project root with tracks
    belonging to multiple projects, **When** `tlc track list` runs with
    no `--all-projects` flag, **Then** only the current project's tracks
    are shown.

12. **Given** tracks, **When** `tlc track list --all-projects`,
    **Then** Project column appears in output.

13. **Given** user inside a `.tlc/`-configured project root with tracks
    belonging to multiple projects, **When**
    `tlc track list --all-projects` runs, **Then** tracks from all
    projects appear without erroring on cross-project state re-fetch.

## Tests

### E2E
- `internal/cli/track_query_e2e_test.go`:
  - `TestTrackList_E2E_BasicTable` (scenario 1)
  - `TestTrackList_E2E_FilterByStatus` (scenario 3)
  - `TestTrackList_E2E_FilterByType` (scenario 4)
  - `TestTrackList_E2E_FilterByState` (scenario 5)
  - `TestTrackList_E2E_JSONOutput` (scenario 6)
  - `TestTrackShow_E2E_PhaseBreakdown` (scenarios 7, 8)
  - `TestTrackShow_E2E_EmptyTrack` (scenario 9)
  - `TestTrackShow_E2E_NotFound` (scenario 10)
  - `TestTrackList_E2E_DefaultScopeCurrentProject` (scenario 11)
  - `TestTrackList_E2E_AllProjectsFromInsideProject` (scenario 13)
  - `TestTrackSummary_E2E_StatusCounts` (summary coverage)

### Unit
- `internal/cli/track_list_test.go` — existing table/filter tests;
  includes `TestTrackList_DefaultFilterExcludesTerminal` (scenario 2)
- `internal/cli/track_show_test.go` — existing detail/phase tests
- `internal/cli/track_summary_test.go` — existing summary tests

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Table columns | `TestTrackList_E2E_BasicTable` | Pending |
| 2 | Default filter excludes terminal | `TestTrackList_DefaultFilterExcludesTerminal` | Covered |
| 3 | --status filter | `TestTrackList_E2E_FilterByStatus` | Pending |
| 4 | --type filter | `TestTrackList_E2E_FilterByType` | Pending |
| 5 | --state filter | `TestTrackList_E2E_FilterByState` | Pending |
| 6 | JSON output | `TestTrackList_E2E_JSONOutput` | Pending |
| 7 | Phase detail view | `TestTrackShow_E2E_PhaseBreakdown` | Pending |
| 8 | Phase checkmarks | `TestTrackShow_E2E_PhaseBreakdown` | Pending |
| 9 | Empty track | `TestTrackShow_E2E_EmptyTrack` | Pending |
| 10 | Not found error | `TestTrackShow_E2E_NotFound` | Pending |
| 11 | Default project scope | `TestTrackList_E2E_DefaultScopeCurrentProject` | Pending |
| 12 | --all-projects | `TestTrackList_AllProjects` | Covered |
| 13 | --all-projects from inside project | `TestTrackList_E2E_AllProjectsFromInsideProject` | Covered |
