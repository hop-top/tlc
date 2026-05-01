---
status: shipped
---

# 070 - Track Creation & Lifecycle

**ID**: 070
**Feature**: Track Management — Creation and Lifecycle
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md),
[Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

As a Solo Developer, I want to create, update, archive, abandon,
and delete tracks so that I can organize related tasks into cohesive
work streams.

## Acceptance Scenarios

1. **Given** TLC is initialized,
   **When** I run
   `tlc track create "Browser rendering" --type feature`,
   **Then** a track is created with:
    - Slug-derived ID: `browser-rendering`
    - Status: `pending`
    - Type: `feature`

2. **Given** TLC is initialized,
   **When** I run
   `tlc track create "Auth" --type refactor --id auth-rewrite
   --assigned-to @me`,
   **Then** a track is created with custom ID `auth-rewrite` and
   assignee set to `me`.

3. **Given** an invalid type is provided,
   **When** I run `tlc track create "X" --type invalid`,
   **Then** error:
   `track type "invalid" invalid; valid types: feature, bug, refactor`

4. **Given** track `browser-rendering` exists (active),
   **When** I run
   `tlc track update browser-rendering --title "Browser Engine"`,
   **Then** the title is updated to `Browser Engine`.

5. **Given** track `browser-rendering` exists (active) and all linked
   tasks are DONE or SKIPPED,
   **When** I run
   `tlc track update browser-rendering --status completed`,
   **Then** status transitions to `completed`.

6. **Given** an active track with open (non-terminal) tasks,
   **When** I run `tlc track update <id> --status completed`,
   **Then** error: cannot complete track with non-terminal tasks.

7. **Given** an active track,
   **When** I run `tlc track abandon <id>`,
   **Then** status transitions to `abandoned`.

8. **Given** a completed track,
   **When** I run `tlc track archive <id>`,
   **Then** status transitions to `archived`.

9. **Given** a track with linked tasks,
   **When** I run `tlc track delete <id>`,
   **Then** error: cannot delete track with linked tasks.

10. **Given** a track with no linked tasks,
    **When** I run `tlc track delete <id>`,
    **Then** the track is deleted.

11. **Given** an ID that is too short (e.g. `"a"`),
    **When** I run `tlc track create "X" --type feature --id a`,
    **Then** error about ID format (min 3 chars).

## Tests

### E2E
- `internal/cli/track_lifecycle_e2e_test.go`
  - `TestTrackLifecycle_E2E_CreateDefaultSlug` (scenario 1)
  - `TestTrackLifecycle_E2E_CreateCustomID` (scenario 2)
  - `TestTrackLifecycle_E2E_CreateInvalidType` (scenario 3)
  - `TestTrackLifecycle_E2E_UpdateTitle` (scenario 4)
  - `TestTrackLifecycle_E2E_CompleteAllDone` (scenario 5)
  - `TestTrackLifecycle_E2E_CompleteOpenTasks` (scenario 6)
  - `TestTrackLifecycle_E2E_Abandon` (scenario 7)
  - `TestTrackLifecycle_E2E_Archive` (scenario 8)
  - `TestTrackLifecycle_E2E_DeleteLinked` (scenario 9)
  - `TestTrackLifecycle_E2E_DeleteUnlinked` (scenario 10)
  - `TestTrackLifecycle_E2E_InvalidID` (scenario 11)

### Unit
- `internal/core/track_test.go` — ID validation, slug derivation
- `internal/core/track_workflow_test.go` — transition validation
- `internal/core/track_service_test.go` — service CRUD operations

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|----------|----------|------|--------|
| 1 | Create w/ slug-derived ID, pending, feature | `CreateDefaultSlug` | ✅ |
| 2 | Create w/ custom ID + assignee | `CreateCustomID` | ✅ |
| 3 | Invalid type error | `CreateInvalidType` | ✅ |
| 4 | Update title | `UpdateTitle` | ✅ |
| 5 | Complete when all tasks terminal | `CompleteAllDone` | ✅ |
| 6 | Block complete w/ open tasks | `CompleteOpenTasks` | ✅ |
| 7 | Abandon active track | `Abandon` | ✅ |
| 8 | Archive completed track | `Archive` | ✅ |
| 9 | Delete blocked by linked tasks | `DeleteLinked` | ✅ |
| 10 | Delete unlinked track | `DeleteUnlinked` | ✅ |
| 11 | Invalid ID format error | `InvalidID` | ✅ |
