# 072 - Task Track Integration

**ID**: 072
**Feature**: Track Management — Task Integration
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI Agent, I want to link tasks to tracks and have track status
auto-transition when I start working so that track progress reflects
real work.

## Acceptance Scenarios

1. **Given** track exists, **When**
   `tlc task create "Parse HTML" --track browser-rendering`,
   **Then** task created with TrackID set to `"browser-rendering"`.

2. **Given** task exists, **When**
   `tlc task update T-0001 --track browser-rendering`,
   **Then** task linked to track.

3. **Given** task linked to track, **When**
   `tlc task update T-0001 --track -`,
   **Then** task unlinked (TrackID set to nil).

4. **Given** tasks linked to `"browser-rendering"`, **When**
   `tlc task list --track browser-rendering`,
   **Then** only tasks with that track shown.

5. **Given** invalid track, **When**
   `tlc task create "X" --track nonexistent`,
   **Then** error: track not found.

6. **Given** pending track with linked task, **When**
   `tlc task claim T-0001` (task IN_PROGRESS),
   **Then** track auto-transitions from pending to active.

7. **Given** already-active track, **When**
   `tlc task claim T-0002` (another linked task),
   **Then** track stays active (no re-trigger).

8. **Given** task with TrackID, **When** `tlc task show T-0001`,
   **Then** output includes `Track: browser-rendering`.

## Tests

### E2E
- ✅ `internal/cli/track_integration_test.go`
  — `TestTaskCreateWithTrack` (scenario 1)
- ✅ `internal/cli/track_integration_test.go`
  — `TestTaskCreateWithInvalidTrack` (scenario 5)
- ✅ `internal/cli/track_integration_test.go`
  — `TestTaskListFilterByTrack` (scenario 4)
- ✅ `internal/cli/track_integration_test.go`
  — `TestTaskUpdateTrack` (scenarios 2, 3)
- ✅ `internal/cli/track_integration_test.go`
  — `TestAutoTransitionPendingToActive` (scenario 6)
- ✅ `internal/cli/track_integration_test.go`
  — `TestAutoTransitionNoOpWhenTrackAlreadyActive` (scenario 7)
- ✅ `internal/cli/track_task_integration_e2e_test.go`
  — `TestTaskTrackIntegration_E2E_CreateWithTrack` (scenario 1)
- ✅ `internal/cli/track_task_integration_e2e_test.go`
  — `TestTaskTrackIntegration_E2E_UpdateTrackLink` (scenario 2)
- ✅ `internal/cli/track_task_integration_e2e_test.go`
  — `TestTaskTrackIntegration_E2E_UnlinkTrack` (scenario 3)
- ✅ `internal/cli/track_task_integration_e2e_test.go`
  — `TestTaskTrackIntegration_E2E_ListByTrack` (scenario 4)
- ✅ `internal/cli/track_task_integration_e2e_test.go`
  — `TestTaskTrackIntegration_E2E_InvalidTrack` (scenario 5)
- ✅ `internal/cli/track_task_integration_e2e_test.go`
  — `TestTaskTrackIntegration_E2E_AutoTransition` (scenario 6)
- ✅ `internal/cli/track_task_integration_e2e_test.go`
  — `TestTaskTrackIntegration_E2E_AlreadyActive` (scenario 7)

### Unit
- ✅ `internal/core/track_service_test.go`
  — `TestAutoTransitionOnTaskClaim` (scenario 6)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | task create --track sets TrackID | E2E CreateWithTrack | ✅ |
| 2 | task update --track links task | E2E UpdateTrackLink | ✅ |
| 3 | task update --track - unlinks | E2E UnlinkTrack | ✅ |
| 4 | task list --track filters | E2E ListByTrack | ✅ |
| 5 | invalid track returns error | E2E InvalidTrack | ✅ |
| 6 | claim triggers pending→active | E2E AutoTransition | ✅ |
| 7 | active track stays active | E2E AlreadyActive | ✅ |
| 8 | task show includes Track: | E2E ShowTrack | ✅ |
