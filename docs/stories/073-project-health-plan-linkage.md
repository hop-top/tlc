---
status: shipped
---

# 073 - Project Health and Plan Linkage

**ID**: 073
**Feature**: Track Management — Project Health and Plan Linkage
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**:
  [Solo Developer](../personas/solo-developer.md),
  [AI Agent](../personas/ai-agent.md)
**Priority**: P2

## Story

As a Team Lead, I want to see project health metrics and link plans
to tracks with automatic task extraction so that I can manage team
workload and connect planning to execution.

## Acceptance Scenarios

1. **Given** 5 active tracks (default max-active=3), **When**
   `tlc track summary`, **Then** health shows overcommit warning.

2. **Given** mixed tracks, **When** `tlc track summary`,
   **Then** status counts:
   `Active: N, Pending: N, Completed: N, Abandoned: N`.

3. **Given** active tracks, **When** `tlc track summary`,
   **Then** active tracks listed with Progress%, State, Updated
   columns.

4. **Given** plan file with frontmatter tasks, **When**
   `tlc track update <id> --add-plan docs/plans/my-plan.md`,
   **Then** plan linked to `track.Meta["plans"]` and tasks created
   from frontmatter.

5. **Given** plan with `blocked-by` index refs `[0, 1]`, **When**
   `--add-plan` runs, **Then** blocked-by resolved to real task IDs
   of previously created tasks.

6. **Given** plan without tasks field, **When** `--add-plan` runs,
   **Then** plan linked without task creation.

7. **Given** plan-extractor configured in config.yaml and
   frontmatter has no tasks, **When** `--add-plan` runs,
   **Then** extractor command called and its tasks created.

8. **Given** `config tracks.health.max-active=5`, **When**
   4 tracks active, **Then** no overcommit warning.

## Tests

### E2E
- ✅ `internal/cli/track_summary_test.go`
  — `TestTrackSummary_Empty` (scenario 2 baseline)
- ✅ `internal/cli/track_summary_test.go`
  — `TestTrackSummary_WithActiveTracks` (scenarios 2, 3)
- ✅ `internal/cli/track_summary_test.go`
  — `TestTrackSummary_Overcommitted` (scenario 1)
- ✅ `internal/cli/track_health_e2e_test.go`
  — `TestTrackHealth_E2E_SummaryStatusCounts` (scenario 2)
- ✅ `internal/cli/track_health_e2e_test.go`
  — `TestTrackHealth_E2E_OvercommitWarning` (scenario 1)
- ✅ `internal/cli/track_health_e2e_test.go`
  — `TestTrackHealth_E2E_NoWarningUnderThreshold` (scenario 8)
- ✅ `internal/cli/track_health_e2e_test.go`
  — `TestTrackPlan_E2E_AddPlanWithTasks` (scenario 4)
- ✅ `internal/cli/track_health_e2e_test.go`
  — `TestTrackPlan_E2E_AddPlanLinkOnly` (scenario 6)
- ✅ `internal/cli/track_health_e2e_test.go`
  — `TestTrackPlan_E2E_BlockedByResolution` (scenario 5)

### Unit
- ✅ `internal/core/track_health_test.go`
  — `TestComputeProjectHealth`
- ✅ `internal/core/plan_parser_test.go`
  — frontmatter parse tests

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | overcommit warning when active >= max | E2E Overcommit | ✅ |
| 2 | status counts in summary | E2E StatusCounts | ✅ |
| 3 | active table with Progress/State/Updated | E2E WithActive | ✅ |
| 4 | --add-plan links + creates tasks | E2E AddPlanTasks | ✅ |
| 5 | blocked-by indices resolved | E2E BlockedBy | ✅ |
| 6 | plan link only (no tasks) | E2E PlanLinkOnly | ✅ |
| 7 | plan-extractor fallback | — | ❌ NOT COVERED |
| 8 | custom max-active threshold | E2E NoWarning | ✅ |

## TODO

- [ ] Add E2E test for plan-extractor fallback (scenario 7)
