---
status: shipped
---

# 092 - HTTP API Track Operations

**ID**: 092
**Feature**: HTTP API — Track Operations
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md), [Team Lead](../personas/team-lead.md)
**Priority**: P2

## Story

As an AI Agent, I want to list, create, and show tracks over HTTP with
the same slug validation `tlc track create` enforces, and with the same
tolerant ID-resolution the CLI uses for `tlc track show` (exact TypeID,
exact slug, unambiguous prefix, unambiguous fuzzy match), so I can
navigate and organize tracks without needing an exact canonical ID for
every lookup. The API must distinguish "your input was malformed or
ambiguous" (422) from "your input was well-formed but nothing matches"
(404), since an agent's retry/recovery logic differs for each case.

## Acceptance Scenarios

1. **Given** the server is running, **When** a client sends
   `POST /tracks` with a valid slug, title, and type, **Then** the
   response is 201 with the created track (default status `pending`),
   and a subsequent `GET /tracks/{id}` returns 200 with the same track.

2. **Given** a client sends `POST /tracks` with a slug that fails
   `core.ValidateTrackSlug` (e.g. too short or uppercase), **When** the
   request is processed, **Then** the response is 422.

3. **Given** two or more tracks exist, **When** a client sends
   `GET /tracks`, **Then** the response is 200 with a `tracks` array
   and `total` matching the count.

4. **Given** two tracks exist whose slugs share an ambiguous common
   prefix (e.g. `feat-alpha` and `feat-beta`, both matching input
   `feat`), **When** a client sends `GET /tracks/feat`, **Then**
   `resolveTrackID`'s prefix-match step finds more than one candidate,
   and the response is 422 (ambiguous input, not a lookup miss).

5. **Given** no track exists matching a given slug-shaped ID,
   **When** a client sends `GET /tracks/{id}`, **Then**
   `resolveTrackID` exhausts strict/slug/prefix/fuzzy resolution with
   zero matches, returns the typed `ErrTrackNotFound`, and the response
   is 404 — distinct from scenario 4's 422, because the input itself
   was well-formed, it simply doesn't exist.

6. **Given** a task references a track by its resolved ID (via
   `POST /tasks` or `PATCH /tasks/{id}` with `track_id`), **When** the
   track lookup happens as part of task creation/update,
   **Then** the same `resolveTrackID` function backs both the
   track-specific routes and the task routes' track-linking logic, so
   resolution behavior (exact/prefix/fuzzy, 404-vs-422) is consistent
   across both surfaces — covered indirectly by story 091's extended-
   fields test, which resolves a track by exact ID during a task
   update.

## Tests

### E2E
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TrackCreateListShow` (scenarios 1, 3)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TrackCreateInvalidSlug` (scenario 2)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TrackShowInvalidID` (scenario 4)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TrackShowNotFound` (scenario 5)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_TaskUpdateExtendedFields` (scenario 6, shared-resolver cross-check)

### Unit
- `internal/cli/track_resolve_test.go` — exact/prefix/fuzzy/ambiguous/
  not-found/empty-input coverage of `resolveTrackID` itself (the
  resolution logic the routes' 404-vs-422 mapping is built on)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | Create + show track | `TestServe_E2E_TrackCreateListShow` | COVERED |
| 2 | Invalid slug → 422 | `TestServe_E2E_TrackCreateInvalidSlug` | COVERED |
| 3 | List tracks with total count | `TestServe_E2E_TrackCreateListShow` | COVERED |
| 4 | Ambiguous prefix → 422 | `TestServe_E2E_TrackShowInvalidID` | COVERED |
| 5 | Well-formed, no match → 404 | `TestServe_E2E_TrackShowNotFound` | COVERED |
| 6 | Shared resolver used by task-track linking | `TestServe_E2E_TaskUpdateExtendedFields` | COVERED |
