# 076 - Cross-Track `blocked-by` References in Plan Ingestion

**ID**: 076
**Feature**: Track Management — Cross-Track Plan Dependencies
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**:
  [Team Lead](../personas/team-lead.md),
  [Solo Developer](../personas/solo-developer.md)
**Priority**: P2

## Story

As an AI Agent ingesting multi-track plans (e.g. the xray graphify
handoff set), I want `blocked-by` in `plan.md` frontmatter to accept
cross-track references so that real DAGs between tracks survive
ingestion instead of getting silently dropped.

## Scope

Extend the `PlanTaskSpec.blocked-by` field to accept **mixed entries**:

- Integer `N` — intra-track, 0-based index into the same plan's
  `tasks:` list (existing behaviour, unchanged).
- String `"T-NNNN"` — reference to an already-existing task ID
  (resolved and persisted as-is; no extra work).
- String `"<track-id>#N"` — cross-track reference where `<track-id>`
  is another track and `N` is the 1-based task number inside that
  track's plan (matches the `## Tasks` numbering convention in
  handoff docs).

Other string forms (bare track ids, future notations) are rejected.

## Resolution rules

At ingestion time (`tlc track update <id> --add-plan <path>`), each
cross-track reference `"<track-id>#N"` is resolved as follows:

1. The target track **must already exist**. If not, ingestion fails
   with a clear "prerequisite track not created" error listing the
   missing refs. Nothing is written.
2. The target track must have at least one linked plan in
   `track.Meta["plans"]`. The ingester re-reads the first linked
   plan, parses its frontmatter, and takes `tasks[N-1].title`.
3. The ingester then looks up the task in the database by
   `track_id = <track-id>` **and** `title = <title>`. If no such
   task exists (or more than one matches), ingestion fails.
4. On success, the string reference is replaced with the resolved
   `T-NNNN` ID both in the created task's `blocked_by` metadata
   **and** in the ingested plan file on disk (so subsequent
   ingestions read stable IDs).

The existing intra-track resolution (int indices) is unchanged.

## Acceptance Scenarios

1. **Given** a plan with `blocked-by: [0]` (int), **When**
   `tlc track update --add-plan`, **Then** behaviour is unchanged;
   task created with resolved intra-track ID.

2. **Given** track `alpha` already has tasks `T-0001 "Foo"` and
   `T-0002 "Bar"` linked via plan A, **When** plan B for track
   `beta` is ingested with
   `blocked-by: ["alpha#2"]` on one of its tasks, **Then** that
   task is created with `blocked_by = ["T-0002"]` and plan B on
   disk is rewritten so the entry reads `"T-0002"` instead of
   `"alpha#2"`.

3. **Given** plan B for track `beta` has
   `blocked-by: ["alpha#1"]` but track `alpha` does not exist yet,
   **When** `track update beta --add-plan`, **Then** ingestion
   fails with error naming `alpha` as missing; no tasks for beta
   are created; plan B is not modified.

4. **Given** plan B references `"alpha#99"` but track `alpha` only
   has 2 tasks in its plan, **When** ingestion runs, **Then** it
   fails with an out-of-range error pointing at the bad reference.

5. **Given** a `blocked-by` entry is already a concrete
   `"T-0001"`, **When** ingestion runs and `T-0001` exists,
   **Then** it is accepted as-is and persisted verbatim.

6. **Given** two projects both contain a track with the same
   id/task-title (e.g. both have track `alpha` with task
   `"Foo"`), **When** ingesting a cross-track ref into a track
   belonging to project B, **Then** the resolver scopes lookups
   by the target track's owning `project_id`. A bare
   `track_id + title` filter would merge rows from project A and
   fail with a spurious ambiguity error. (Regression guard for
   PR #46 review.)

7. **Given** a plan whose markdown body prose mentions
   `"alpha#2"` outside the YAML frontmatter, **When** ingestion
   rewrites the frontmatter to resolve that ref, **Then** the
   body prose is untouched; only the frontmatter block (between
   the leading `---` delimiters) is rewritten. (Regression guard
   for PR #46 review.)

8. **Given** a plan file with mode `0600`, **When** ingestion
   rewrites it, **Then** the file mode stays `0600` (not
   widened to `0644`). The write is atomic via sibling temp
   file + rename so a mid-write crash leaves the original plan
   intact. (Regression guard for PR #46 review.)

9. **Given** a plan extractor emits `blocked-by: [null]` (or an
   empty JSON value), **When** ingestion decodes the entry,
   **Then** it fails with an explicit error; it does NOT silently
   default to the zero value (`Index=0`), which would create an
   unintended dependency on the first task. (Regression guard
   for PR #46 review.)

## Tests

### E2E
- `internal/cli/track_cross_track_blocked_by_e2e_test.go`
  — `TestTrackPlan_E2E_CrossTrackBlockedBy_HappyPath` (scenario 2)
  — `TestTrackPlan_E2E_CrossTrackBlockedBy_MissingTrack` (scenario 3)
  — `TestTrackPlan_E2E_CrossTrackBlockedBy_IndexOutOfRange`
    (scenario 4)
  — `TestTrackPlan_E2E_CrossTrackBlockedBy_BodyProseUntouched`
    (scenario 7)

### Unit
- `internal/core/plan_parser_test.go`
  — mixed `blocked-by` entries parse correctly
- `internal/core/plan_parser_blockedby_test.go`
  — `TestBlockedByRef_UnmarshalJSON_RejectsNullAndEmpty`
    (scenario 9)
- `internal/core/plan_tasks_test.go`
  — cross-track resolver happy path, missing track, bad index
- `internal/core/plan_tasks_project_scope_test.go`
  — `TestResolveCrossTrackRef_ScopesByProjectID` (scenario 6)
- `internal/core/plan_rewrite_test.go`
  — `TestRewritePlanBlockedByRefs_DoesNotTouchBody` (scenario 7)
  — `TestRewritePlanBlockedByRefs_PreservesFileMode` (scenario 8)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | int refs unchanged | existing `BlockedByResolution` | ✅ |
| 2 | cross-track resolves + plan rewritten | `CrossTrackBlockedBy_HappyPath` | ✅ |
| 3 | missing track fails cleanly | `_MissingTrack` | ✅ |
| 4 | bad index fails cleanly | `_IndexOutOfRange` | ✅ |
| 5 | concrete `T-NNNN` accepted | `BlockedByMixedEntries` | ✅ |
| 6 | project_id scoped lookup | `ResolveCrossTrackRef_ScopesByProjectID` | ✅ |
| 7 | body prose untouched | `DoesNotTouchBody`, `_BodyProseUntouched` | ✅ |
| 8 | file mode preserved | `PreservesFileMode` | ✅ |
| 9 | null/empty JSON rejected | `UnmarshalJSON_RejectsNullAndEmpty` | ✅ |

## Related

- Task: T-0434
- Context: `$HOME/.w/ideacrafterslabs/xray/.tlc/handoff/` plans use
  this notation to encode cross-track deps between six tracks.
