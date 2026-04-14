# E2E Coverage Matrix: Story Acceptance Criteria vs Tests

Author: $USER
Date: 2026-04-13

Audit of all story acceptance criteria against e2e test coverage in
`internal/cli/*_test.go`. Categories:

- **COVERED**: e2e test validates the criterion
- **PARTIAL**: test exists but does not fully validate the criterion
- **MISSING**: no e2e test covers this criterion
- **SKIP**: criterion is not e2e-testable (infra, design, UX-only)

---

## 001 — Task Creation

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Create task with auto-ID, status TODO | `TestTaskCreate` | COVERED |
| 2 | Create with tags, assignee, description | `TestTaskCreateWithTags`, `TestTaskCreateWithAssignee` | COVERED |
| 3 | Interactive mode (`-i`) | none | MISSING |
| 4 | Auto-increment ID, no conflict | `TestTaskCreatePagination` | COVERED |
| 5 | Custom ID + reference | none | MISSING |
| 6 | Auto-detected project_id | `TestFlowInvoke_ProjectScoped_*` (partial) | PARTIAL |

## 002 — Task Listing

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Default list: IN_PROGRESS then TODO | `TestTaskList` | COVERED |
| 2 | Filter by status | `TestTaskFilterByStatus` | COVERED |
| 3 | Filter by tag | `TestTaskFilterByTag` | COVERED |
| 4 | Filter by assignee | `TestTaskFilterByAssignee`, `TestTaskFilterByAssigneeMe` | COVERED |
| 5 | JSON format | `TestTaskListFormat` | COVERED |
| 6 | TLS format | `TestFormatTLS_*` | PARTIAL |
| 7 | `--all-projects` | `TestTrackList_AllProjects` (tracks only) | MISSING |
| 8 | `--archived` | none | MISSING |
| 9 | Pagination (limit/offset) | `TestTaskCreatePagination` | PARTIAL |
| 10 | Sort by field | none | MISSING |
| 11 | Full-text search | none | MISSING |
| 12 | YAML format | none | MISSING |

## 003 — Task Claiming

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | List unclaimed tasks | none (MCP-level) | SKIP |
| 2 | Claim sets IN_PROGRESS + assignee | `TestTaskLifecycle` | COVERED |
| 3 | Complete sets DONE + result | `TestTaskLifecycle` | COVERED |
| 4 | Failed state + re-claimable | none | MISSING |
| 5 | Concurrent claim conflict | none | MISSING |
| 6 | Claim with `--note` | none | MISSING |
| 7 | Unclaim sets TODO + clears assignee | `TestTaskLifecycle` | COVERED |
| 8 | Sync back to origin on claim | none | MISSING |
| 9 | State machine transitions | `TestTaskUpdateForceStatus` | PARTIAL |
| 11 | Multiple IDs in one claim | `TestTaskClaim_MultipleIDs` | COVERED |
| 12 | Regex claim `--no-prompt` | `TestBatchE2E_CompleteAllWithGlob` (glob) | PARTIAL |
| 13 | Multi-unclaim | `TestTaskReopen_MultipleIDs` (reopen, not unclaim) | MISSING |
| 14 | Regex confirm prompt | none | MISSING |

## 004 — Configurable Statuses

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Default 4-status workflow | `TestTaskLifecycle` | COVERED |
| 2 | `tlc workflow statuses` | none | MISSING |
| 3 | Invalid transition error | `TestTrackUpdate_InvalidTransition` | PARTIAL |
| 4 | `--force` bypasses rules | `TestTaskUpdateForceStatus` | COVERED |
| 5 | Per-tag overrides | none | MISSING |
| 6 | Custom TLS markers | none | MISSING |

## 005 — Task Update

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Update title | `TestTaskUpdateTitle` | COVERED |
| 2 | Replace description | `TestTaskUpdateDescription` | COVERED |
| 3 | Add tag | `TestTagAdd` | COVERED |
| 4 | Remove tag | `TestTaskRemoveTag` | COVERED |
| 5 | Change assignee | `TestTaskUpdateAssignee` | COVERED |
| 6 | Clear assignee (`-` or `null`) | `TestTaskClearAssigneeDash`, `TestTaskClearAssigneeNull` | COVERED |
| 7 | Status transition | `TestTaskUpdateForceStatus` | COVERED |
| 8 | Force bypass terminal | `TestTaskUpdateForceStatus` | COVERED |
| 9 | Multi-ID tag update | `TestTaskUpdate_MultipleIDs` | COVERED |
| 10 | Multi-ID title rejected | `TestTaskUpdate_MultipleIDs` | COVERED |
| 11 | Regex `--no-prompt` | `TestBatchE2E_AssignRegexNoPrompt` | PARTIAL |

## 006 — Task Deletion

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Delete with `--yes` | `TestTaskDeleteWithYesFlag` | COVERED |
| 2 | Error without `--yes` | `TestTaskDeleteRequiresYes` | COVERED |
| 3 | Not found error | `TestTaskDeleteNotFound` | COVERED |
| 4 | Short flag `-y` | `TestTaskDeleteYesShortFlag` | COVERED |
| 5 | Multi-ID delete | `TestTaskDelete_MultipleIDs_WithNoPrompt` | COVERED |
| 6 | Multi-ID no confirm error | `TestBatchE2E_DeleteRequiresConfirmOrNoPrompt` | COVERED |
| 7 | `--no-prompt` batch delete | `TestTaskDelete_MultipleIDs_WithNoPrompt` | COVERED |

## 007 — Task Reopen

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Reopen DONE -> TODO, note appended | `TestTaskReopen`, `TestTaskReopenAppendsNoteToDescription` | COVERED |
| 2 | Reopen from SKIPPED | none (explicit) | MISSING |
| 3 | Error without `--note` | `TestTaskReopenRequiresNote` | COVERED |
| 4 | Audit trail continuity | `TestTaskReopenAuditContinuity` | COVERED |
| 5 | Not found error | `TestTaskReopenNotFound` | COVERED |
| 6 | Multi-ID reopen | `TestTaskReopen_MultipleIDs` | COVERED |

## 008 — Task Assignment

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Assign to unassigned, status unchanged | `TestTaskAssign` | COVERED |
| 2 | Reassign with note | none (note not tested) | PARTIAL |
| 3 | Unassign clears, note appended | `TestTaskUnassign`, `TestTaskUnassignAppendsNoteToDescription` | COVERED |
| 4 | Unassign without `--note` error | `TestTaskUnassignRequiresNote` | COVERED |
| 5 | Assign not found error | `TestTaskUnassignNotFound` | PARTIAL |
| 6 | Multi-ID assign | `TestTaskAssign_NewArgOrder` | COVERED |
| 7 | Regex assign `--no-prompt` | `TestTaskAssign_RegexPattern`, `TestBatchE2E_AssignRegexNoPrompt` | COVERED |

## 009 — Task Audit Log

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Log shows CLAIMED + DONE entries | `TestTaskAuditLog` | COVERED |
| 2 | `--all` returns all logs | `TestTaskAuditLogAll` | COVERED |
| 3 | Filter by action | `TestTaskAuditLogFilterByAction` | COVERED |
| 4 | Filter by actor | `TestTaskAuditLogFilterByActor` | COVERED |
| 5 | Pagination | `TestTaskAuditLogPagination` | COVERED |
| 6 | Error without task-id or `--all` | `TestTaskAuditLogRequiresTaskIDOrAll` | COVERED |
| 7 | JSON format | `TestTaskAuditLogJSONFormat` | COVERED |

## 010 — GitHub Sync

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Link task to GitHub issue | none | MISSING |
| 2a | Pull: status sync | none | MISSING |
| 2b | Pull: label sync | none | MISSING |
| 2c | Pull: assignee sync | none | MISSING |
| 3 | Push: close issue + comment | none | PARTIAL |
| 4 | Batch sync with progress | none | MISSING |
| 5 | Broken link detection | none | MISSING |

## 020 — Flow Execution

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Sequential step execution | none (e2e) | MISSING |
| 2 | Parallel step execution | none | MISSING |
| 3 | Branch condition evaluation | none | MISSING |
| 4 | Retry policy | none | MISSING |
| 5 | Flow cancel | none | MISSING |
| 6 | Flow logs (execution log) | none | MISSING |

## 030 — TUI Navigation

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | TUI startup, full-screen, navigation | none | MISSING |
| 2 | Filter prompt | none | MISSING |
| 3 | Task detail view | none | MISSING |
| 4 | Edit dialog | none | MISSING |
| 5 | Delete with confirmation | none | MISSING |
| 6 | Contextual help | none | MISSING |
| 7 | Command mode | none | MISSING |

## 042 — CLI Command Integration

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Agent creates task via CLI | `TestTaskCreate` (implicit) | COVERED |
| 2 | Agent lists with filters | `TestTaskFilterByStatus` (implicit) | COVERED |
| 3 | Agent claims via CLI | `TestTaskLifecycle` (implicit) | COVERED |
| 4 | Agent completes via CLI | `TestTaskLifecycle` (implicit) | COVERED |
| 5 | Agent runs flow | none | MISSING |

## 060 — Configuration Validation

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Config cascade discovery | `TestInitConfig` | COVERED |
| 2 | Invalid values -> error | none (e2e) | MISSING |
| 3 | Env var override | none (e2e) | MISSING |
| 4 | Multi-file merge | `TestFindAllConfigs` | PARTIAL |
| 5 | `config validate` error output | none | MISSING |
| 6 | User config merged first | `TestInitConfig_UsesOSUserConfigBeforeSystem` | COVERED |
| 7 | Global write to user config | `TestConfigSet_WritesUserConfigWhenSystemConfigLoaded` | COVERED |

## 061 — Environment Setup Verification

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | `TLC_` env overrides config | none (e2e) | MISSING |
| 2 | Missing tool uses default | none | MISSING |
| 3 | Default editor fallback | none | SKIP |
| 4 | XDG path usage | none (e2e) | MISSING |
| 6 | Missing git -> clear error | none | MISSING |
| 7 | Local config precedence | `TestInitConfig` | PARTIAL |

## 062 — Storage Location Validation

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Unsupported backend error | none (e2e) | MISSING |
| 2 | Default XDG path | none (e2e) | MISSING |
| 3 | Auto-create directory 0755 | none (e2e) | MISSING |
| 4 | (duplicate of 3) | — | — |
| 5 | Directory creation failure error | none (e2e) | MISSING |
| 6 | Config merge for storage | `TestFindAllConfigs` | PARTIAL |

## 063 — Hierarchical Config Discovery

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Cascade merge from cwd to root | `TestFindAllConfigs`, `TestInitConfig_BoundedWalkUpIgnoresAboveBoundary` | PARTIAL |
| 2 | Nested project merge | none (e2e) | MISSING |
| 3 | Child overrides parent | none (e2e) | MISSING |
| 4 | Task merge from hierarchy | none | MISSING |
| 5 | Task dedup by ID | none | MISSING |
| 6 | Root boundary case | `TestCommonAncestorDir_RootBoundary` | COVERED |
| 7 | Windows paths | none | SKIP |
| 8 | Unix paths | `TestInitConfig_UsesOSUserConfigBeforeSystem` | PARTIAL |
| 9 | No .tlc dirs -> use global | `TestInitConfig` | COVERED |

## 064 — Task Effort Estimation

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Update effort | `TestTaskUpdateEffort` | COVERED |
| 2 | Create with effort | `TestTaskCreateWithEffort` | COVERED |
| 3 | TLS export includes effort | `TestFormatTLS_*` | PARTIAL |
| 4 | TLS import with effort | none | MISSING |
| 5 | Invalid effort error | `TestTaskUpdateEffortInvalid` | COVERED |
| 6 | Omitted when empty | `TestTaskShow` (implicit) | PARTIAL |

## 065 — Actionable Error Messages

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Not-found includes ID + hint | `TestTaskShowNotFound` | COVERED |
| 2 | Terminal status offers reopen | none (explicit) | MISSING |
| 3 | Missing `--note` shows re-run cmd | `TestTaskReopenRequiresNote`, `TestTaskUnassignRequiresNote` | COVERED |
| 4 | Missing `--yes` shows re-run cmd | `TestTaskDeleteRequiresYes` | COVERED |
| 5 | Unregistered project suggests init | none | MISSING |
| 6 | Missing sibling tool graceful | `TestWsmDetected_NotFound` | COVERED |

## 066 — Stale/Blocked Flags

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | `--blocked` sets reason | `TestTaskUpdate_BlockedReason` | COVERED |
| 2 | `--unblock` clears reason | `TestTaskUpdate_Unblock` | COVERED |
| 3 | `--timeout` sets + resets | `TestTaskUpdate_Timeout` | COVERED |
| 4 | Invalid timeout error | `TestTaskUpdate_TimeoutInvalid` | COVERED |
| 5 | Create with timeout | `TestTaskCreate_Timeout` | COVERED |

## 067 — Batch Task Operations

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Multi-ID claim | `TestTaskClaim_MultipleIDs` | COVERED |
| 2 | Multi-ID complete | `TestBatchE2E_CompleteAllWithGlob` | COVERED |
| 3 | Multi-ID reopen | `TestTaskReopen_MultipleIDs` | COVERED |
| 4 | Regex claim | `TestTaskComplete_RegexPattern` (complete variant) | PARTIAL |
| 5 | Glob unclaim `*` | `TestBatchE2E_CompleteAllWithGlob` (complete variant) | PARTIAL |
| 6 | TTY confirm prompt | none (requires TTY) | SKIP |
| 7 | `--no-prompt` skips confirm | `TestBatchE2E_CompleteAllWithGlob` | COVERED |
| 8 | Assignee-first multi-assign | `TestTaskAssign_NewArgOrder` | COVERED |
| 9 | Regex assign | `TestBatchE2E_AssignRegexNoPrompt` | COVERED |
| 10 | Multi-ID delete `--no-prompt` | `TestTaskDelete_MultipleIDs_WithNoPrompt` | COVERED |

## 068 — Task Graph View

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | No blockers -> flat roots | `TestTaskGraph_E2E_NoBlockers` | COVERED |
| 2 | Blocked indented under blocker | `TestTaskGraph_E2E_BlockerChain` | COVERED |
| 3 | `--format dot` output | `TestTaskGraph_E2E_DotFormat` | COVERED |
| 4 | `--status` filter | `TestTaskGraph_E2E_StatusFilter` | COVERED |
| 5 | Cross-project edge stripped | `TestBuildGraph_CrossProjectEdgeStripped` | COVERED |

## 069 — Field Normalization

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Lowercase status accepted | `TestFieldNorm_E2E_StatusLowercase` | COVERED |
| 2 | Alias resolves to canonical | `TestFieldNorm_E2E_StatusAlias` | COVERED |
| 3 | Fuzzy match resolves | `TestFieldNorm_E2E_StatusAlias` | COVERED |
| 4 | Priority alias resolves | `TestFieldNorm_E2E_PriorityAlias` | COVERED |
| 5 | Unknown input error | `TestFieldNorm_E2E_UnknownStatus` | COVERED |

## 070 — Track Creation & Lifecycle

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Create with slug ID | `TestTrackLifecycle_E2E_CreateDefaultSlug` | COVERED |
| 2 | Create with custom ID + assignee | `TestTrackLifecycle_E2E_CreateCustomID`, `TestTrackCreate_WithAssignee` | COVERED |
| 3 | Invalid type error | `TestTrackLifecycle_E2E_CreateInvalidType` | COVERED |
| 4 | Update title | `TestTrackLifecycle_E2E_UpdateTitle` | COVERED |
| 5 | Complete (all tasks done) | `TestTrackLifecycle_E2E_CompleteAllDone` | COVERED |
| 6 | Complete blocked by open tasks | `TestTrackLifecycle_E2E_CompleteOpenTasks` | COVERED |
| 7 | Abandon | `TestTrackLifecycle_E2E_Abandon` | COVERED |
| 8 | Archive | `TestTrackLifecycle_E2E_Archive` | COVERED |
| 9 | Delete unlinked | `TestTrackLifecycle_E2E_DeleteUnlinked` | COVERED |
| 10 | Delete linked fails | `TestTrackLifecycle_E2E_DeleteLinked` | COVERED |
| 11 | Invalid ID error | `TestTrackLifecycle_E2E_InvalidID` | COVERED |

## 071 — Track Listing & Detail

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Table columns | `TestTrackList_E2E_BasicTable` | COVERED |
| 2 | Filter by status | `TestTrackList_E2E_FilterByStatus` | COVERED |
| 3 | Filter by type | `TestTrackList_E2E_FilterByType` | COVERED |
| 4 | Filter by state | `TestTrackList_E2E_FilterByState` | COVERED |
| 5 | JSON output | `TestTrackList_E2E_JSONOutput` | COVERED |
| 6 | Show detail + phases | `TestTrackShow_E2E_PhaseBreakdown` | COVERED |
| 7 | Phase progress display | `TestTrackShow_E2E_PhaseBreakdown` | COVERED |
| 8 | Empty track "no tasks linked" | `TestTrackShow_E2E_EmptyTrack` | COVERED |
| 9 | Not found error | `TestTrackShow_E2E_NotFound` | COVERED |

## 071 — Track Blocked State

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Blocked state in list | `TestTrackList_E2E_BlockedWhenAnyTaskBlocked` | COVERED |
| 2 | Healthy state in list | `TestTrackList_E2E_BasicTable` | PARTIAL |
| 3 | `--state blocked` filter | `TestTrackList_E2E_FilterByState` | COVERED |
| 4 | Show blocked detail | `TestTrackShow_E2E_BlockedState` | COVERED |

## 072 — Task-Track Integration

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Create with `--track` | `TestTaskTrackIntegration_E2E_CreateWithTrack` | COVERED |
| 2 | Update track link | `TestTaskTrackIntegration_E2E_UpdateTrackLink` | COVERED |
| 3 | Unlink track (`-`) | `TestTaskTrackIntegration_E2E_UnlinkTrack` | COVERED |
| 4 | `--track` filter on list | `TestTaskTrackIntegration_E2E_ListByTrack` | COVERED |
| 5 | Invalid track error | `TestTaskTrackIntegration_E2E_InvalidTrack` | COVERED |
| 6 | Auto-transition pending->active | `TestTaskTrackIntegration_E2E_AutoTransition` | COVERED |
| 7 | Already active no re-trigger | `TestTaskTrackIntegration_E2E_AlreadyActive` | COVERED |
| 8 | Show includes track | `TestTaskTrackIntegration_E2E_ShowTrack` | COVERED |

## 072 — Track Directory Scaffold

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Scaffold files on create | `TestTrackCreate_ScaffoldsDirectory` | COVERED |
| 2 | Idempotent plan.md | `TestTrackCreate_PlanMDIdempotent` | COVERED |
| 3 | Permission error | none | MISSING |

## 073 — Project Health & Plan Linkage

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Overcommit warning | `TestTrackHealth_E2E_OvercommitWarning` | COVERED |
| 2 | Status counts | `TestTrackHealth_E2E_SummaryStatusCounts` | COVERED |
| 3 | Active tracks with progress | `TestTrackSummary_E2E_StatusCounts` | COVERED |
| 4 | Plan linked + tasks created | `TestTrackPlan_E2E_AddPlanWithTasks` | COVERED |
| 5 | blocked-by resolution | `TestTrackPlan_E2E_BlockedByResolution` | COVERED |
| 6 | Plan without tasks | `TestTrackPlan_E2E_AddPlanLinkOnly` | COVERED |
| 7 | Extractor command | none | MISSING |
| 8 | Custom max-active threshold | `TestTrackHealth_E2E_NoWarningUnderThreshold` | COVERED |

## 074 — Themed Table Output

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Four-color default | `TestThemedTable_FourColors_E2E` | COVERED |
| 2 | Blocker in pink | `TestThemedTable_FourColors_E2E` | COVERED |
| 3 | Single-status no emphasis | `TestThemedTable_SingleFilter_E2E` | COVERED |
| 4 | Headers/borders muted | `TestThemedTable_HeadersInMuted_E2E` | COVERED |

## 075 — Themed Track List Output

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Four-color track list | `TestThemedTrackList_FourColors_E2E` | COVERED |
| 2 | Single-status filter coloring | none (explicit) | MISSING |
| 3 | Headers/borders muted | `TestThemedTable_HeadersInMuted_E2E` (task) | PARTIAL |

## 076 — Cross-Track Blocked-By

| # | Criterion | Test | Status |
|---|-----------|------|--------|
| 1 | Int index intra-track | `TestTrackPlan_E2E_BlockedByResolution` | COVERED |
| 2 | `"T-NNNN"` ref | `TestTrackPlan_E2E_BlockedByResolution` | COVERED |
| 3 | Cross-track `"<track>#N"` happy | `TestTrackPlan_E2E_CrossTrackBlockedBy_HappyPath` | COVERED |
| 4 | Cross-track index OOB | `TestTrackPlan_E2E_CrossTrackBlockedBy_IndexOutOfRange` | COVERED |
| 5 | Missing track defers | `TestTrackPlan_E2E_CrossTrackBlockedBy_MissingTrack_Defers` | COVERED |
| 6 | Body prose untouched | `TestTrackPlan_E2E_CrossTrackBlockedBy_BodyProseUntouched` | COVERED |
| 7 | Circular A->B | `TestTrackPlan_E2E_Circular_AThenB` | COVERED |
| 8 | Circular B->A | `TestTrackPlan_E2E_Circular_BThenA` | COVERED |
| 9 | Deferred no-rewrite | `TestTrackPlan_E2E_DeferredMissingTrackNoRewrite` | COVERED |

---

## Summary

| Status | Count |
|--------|-------|
| COVERED | 126 |
| PARTIAL | 22 |
| MISSING | 42 |
| SKIP | 5 |

### Highest-priority MISSING criteria

Stories with majority MISSING (entire feature untested e2e):

1. **030 — TUI Navigation** (7/7 MISSING) — requires Bubbletea
   test harness; not feasible with current CLI e2e pattern
2. **020 — Flow Execution** (6/6 MISSING) — flow engine e2e
   tests not implemented; unit tests exist in `internal/flow/`
3. **010 — GitHub Sync** (5/7 MISSING) — requires mock GitHub
   API; sync unit tests exist but no e2e
4. **062 — Storage Location Validation** (4/6 MISSING) — mostly
   validated by unit tests; e2e would test init edge cases
5. **061 — Environment Setup Verification** (4/6 MISSING) —
   env-based; harder to e2e without custom env fixtures

### Actionable gaps in otherwise well-covered stories

- **001.3** Interactive mode — need form test harness
- **001.5** Custom ID + reference — straightforward to add
- **002.7** `--all-projects` for tasks — test exists for tracks
- **002.8** `--archived` filter — add to task_list_test.go
- **002.10** Sort by field — add parametric test
- **002.12** YAML format — add format test case
- **003.5** Concurrent claim — requires goroutine/parallel test
- **007.2** Reopen from SKIPPED — add explicit case
- **065.2** Terminal status offers reopen hint — add assertion
- **065.5** Unregistered project hint — add init_test case
- **073.7** Extractor command — requires mock extractor
- **075.2** Single-status filter for tracks — add assertion
