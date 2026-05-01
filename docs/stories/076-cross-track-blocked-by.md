---
status: shipped
---

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

Plan ingestion runs in **two phases** so that a set of plans with
circular cross-track references can be ingested in any order. A
true cycle at the *plan* level (plan A refers to plan B and vice
versa) is still a valid DAG at the *task* level; the two-phase
model lets both plans be ingested without forcing one to exist
first.

### Phase 1 — structural pass (per `--add-plan` invocation)

For the plan being ingested:

1. Parse the frontmatter and pre-validate every `blocked-by`
   entry. Intra-track int indices and concrete `"T-NNNN"` refs
   are resolved immediately (same rules as before).
2. Cross-track refs `"<track-id>#N"` are NOT resolved yet.
   Hard-error cases — index out of range against the target's
   plan frontmatter, or a target track/title ambiguity (multiple
   DB matches within the same project) — still fail the entire
   ingest. Soft cases — target track doesn't exist yet, target
   track has no linked plan, target task doesn't exist in the DB
   yet — are **deferred** instead of failing.
3. Tasks are created with the task-level `Meta["blocked_by"]`
   containing only the already-resolved IDs (intra-track +
   explicit `T-NNNN` + cross-track refs that *could* be resolved
   now). Any unresolved cross-track refs for a given task are
   stored in `Meta["blocked_by_unresolved"]` as a `[]string` of
   raw `"<track>#<N>"` strings.
4. The source plan.md is NOT rewritten in phase 1 — refs may
   still be pending resolution.

### Phase 2 — resolution pass (after every phase 1, project-wide)

5. Walk every task in the current project that still has a
   non-empty `Meta["blocked_by_unresolved"]` list. For each
   entry, try to resolve it again using the same rules as phase
   1's cross-track path. Newly resolved entries are promoted
   into `Meta["blocked_by"]` and dropped from the unresolved
   list.
6. For every plan file touched by the phase 1 or phase 2 passes
   that has **all** of its referenced tasks now resolved, the
   plan.md is rewritten on disk (frontmatter-only, atomic write,
   file mode preserved) so subsequent ingestions see stable
   `T-NNNN` IDs.
7. If any tasks still have unresolved refs after phase 2, a
   warning is emitted listing them. The ingest still succeeds —
   the tasks exist, the track is linked, and the remaining refs
   will be retried automatically the next time any `track
   update --add-plan` runs in the project.

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
   **succeeds**: beta's tasks are created, the unresolved ref is
   recorded in `Meta["blocked_by_unresolved"]`, a warning is
   emitted naming the still-pending ref, and plan B is NOT
   rewritten. (Changed in T-0435; previously hard-failed.)

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

### Circular and deferred refs (T-0435)

10. **Given** track A's plan references `"B#2"` and track B's
    plan references `"A#3"` (a true cross-plan cycle), **When**
    plans are ingested A-then-B, **Then**:
    - Phase 1 of A: A's tasks are created. The task referring to
      B#2 has `blocked_by_unresolved = ["B#2"]` and no cross-
      track entry in `blocked_by`. A's plan.md is NOT rewritten.
    - Phase 1 of B: B's tasks are created. The task referring to
      A#3 resolves immediately to the T-ID of A's task #3 (A
      exists now). Phase 2 runs project-wide and resolves A's
      pending `B#2` ref to B's newly created task #2. Both
      plans are rewritten. Both tasks have their blocked_by
      fully populated.

11. **Given** the same cycle as scenario 10, **When** plans are
    ingested B-then-A (the opposite order), **Then** the end
    state is **identical** to scenario 10 (symmetric two-phase
    ingestion). No matter which side is ingested first, both
    finish with fully resolved `blocked_by` and rewritten plan
    files.

12. **Given** plan A references `"C#1"` but track C is never
    ingested, **When** plan A is ingested on its own, **Then**
    ingestion succeeds, the task is created with
    `blocked_by_unresolved = ["C#1"]`, a warning lists the
    pending ref, and plan A's file is left untouched. A later
    `tlc track update` that touches any plan re-runs phase 2
    and is a no-op while C is still missing.

## Tests

### E2E
- `internal/cli/track_cross_track_blocked_by_e2e_test.go`
  — `TestTrackPlan_E2E_CrossTrackBlockedBy_HappyPath` (scenario 2)
  — `TestTrackPlan_E2E_CrossTrackBlockedBy_MissingTrack_Defers`
    (scenario 3, updated for T-0435)
  — `TestTrackPlan_E2E_CrossTrackBlockedBy_IndexOutOfRange`
    (scenario 4)
  — `TestTrackPlan_E2E_CrossTrackBlockedBy_BodyProseUntouched`
    (scenario 7)
- `internal/cli/track_cross_track_circular_e2e_test.go`
  — `TestTrackPlan_E2E_Circular_AThenB` (scenario 10)
  — `TestTrackPlan_E2E_Circular_BThenA` (scenario 11)
  — `TestTrackPlan_E2E_DeferredMissingTrackNoRewrite` (scenario 12)

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
| 3 | missing track now defers (not fails) | `_MissingTrack_Defers` | ⏳ |
| 4 | bad index fails cleanly (hard) | `_IndexOutOfRange` | ✅ |
| 5 | concrete `T-NNNN` accepted | `BlockedByMixedEntries` | ✅ |
| 6 | project_id scoped lookup | `ResolveCrossTrackRef_ScopesByProjectID` | ✅ |
| 7 | body prose untouched | `DoesNotTouchBody`, `_BodyProseUntouched` | ✅ |
| 8 | file mode preserved | `PreservesFileMode` | ✅ |
| 9 | null/empty JSON rejected | `UnmarshalJSON_RejectsNullAndEmpty` | ✅ |
| 10 | circular A→B→A ingested A then B | `Circular_AThenB` | ⏳ |
| 11 | circular ingested B then A (symmetric) | `Circular_BThenA` | ⏳ |
| 12 | missing target defers with warning | `DeferredMissingTrackNoRewrite` | ⏳ |

## Related

- Task: T-0434
- Context: `$HOME/.w/ideacrafterslabs/xray/.tlc/handoff/` plans use
  this notation to encode cross-track deps between six tracks.
