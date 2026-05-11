---
status: shipped-no-e2e
---

# 064 - Task Effort Estimation

**ID**: 064
**Feature**: Task Metadata
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [AI Agent](../personas/ai-agent.md), [Team Lead](../personas/team-lead.md)
**Priority**: P2

## Story

As a developer, I want to assign a standardized effort estimate (XS/S/M/L/XL)
to tasks so I can track sizing for planning and velocity measurement.

## Acceptance Scenarios

1. **Given** a task exists, **When** I run
   `tlc task update T-0001 --effort M`, **Then** the task's effort is
   set to `M` and shown in `tlc task show T-0001`.

2. **Given** I create a task, **When** I run
   `tlc task create "Fix bug" --effort S`, **Then** the created task
   has effort `S`.

3. **Given** a task has effort set, **When** I export to TLS format,
   **Then** the TLS line includes `effort:M` token.

4. **Given** a TLS file with `effort:L` token, **When** synced,
   **Then** the task is stored with effort `L`.

5. **Given** I run `tlc task update T-0001 --effort garbage`,
   **Then** an error is returned: invalid effort must be XS/S/M/L/XL.
   *(Note: as of PR #114, effort input is case-insensitive and accepts
   aliases — e.g. `huge` → `XL`, `medium`/`med` → `M`, `small` → `S`,
   `tiny` → `XS`, `large` → `L`. Only truly unrecognised values are
   rejected.)*

6. **Given** a task has no effort set, **When** shown,
   **Then** the `Effort:` line is omitted from output.

## Implementation Notes

- Effort stored as `TEXT NOT NULL DEFAULT ''` column (migration v3).
- Validated against enum: XS, S, M, L, XL, or empty.
- Persisted via `core.Effort` type; accessible in JSON/YAML export.

## Tests

### Unit / CLI E2E
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateEffort` (scenario 1: `--effort M` sets and persists)
- ✅ `internal/cli/task_update_test.go` — `TestTaskCreateWithEffort` (scenario 2: `--effort S` on create)
- ✅ `internal/cli/task_update_test.go` — `TestTaskUpdateEffortInvalid` (scenario 5: `--effort garbage` rejected)
- ✅ `internal/cli/fieldnorm_test.go` — `TestNormalizeEffort` (unit: canonical/lowercase/alias mapping table)
- ✅ `internal/cli/fieldnorm_test.go` — `TestNormalizeEffort_FuzzyTieDeterministic` (unit: alias fuzz-match determinism)
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreateEffortLowercase` (5 sub-cases: xs/s/m/l/xl on create)
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreateEffortAlias` (6 sub-cases: tiny/small/medium/med/large/huge on create)
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UpdateEffortLowercase` (5 sub-cases on update)
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UpdateEffortAlias` (5 sub-cases on update)
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UpdateEffortClearEmpty` (clear via empty string)
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UpdateEffortClearDash` (clear via `-`)
- ❌ MISSING: scenario 3 — TLS export emits `effort:M` token (no test in `sync_local_test.go` or `vtodo_format_test.go`)
- ❌ MISSING: scenario 4 — TLS import roundtrip with `effort:L` token (no test)
- ❌ MISSING: scenario 6 — `tlc task show` omits `Effort:` line when unset (no assertion in `task_show_test.go`)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | `task update --effort M` persists | `TestTaskUpdateEffort`, `TestFieldNorm_E2E_UpdateEffortLowercase` | ✅ COVERED |
| 2 | `task create --effort S` persists | `TestTaskCreateWithEffort`, `TestFieldNorm_E2E_CreateEffortLowercase` | ✅ COVERED |
| 3 | TLS export emits `effort:M` token | Not tested | ❌ NOT COVERED |
| 4 | TLS import roundtrips `effort:L` | Not tested | ❌ NOT COVERED |
| 5 | Invalid effort rejected (post PR #114: only true non-matches like `garbage`; `HUGE`/`medium`/etc are aliased) | `TestTaskUpdateEffortInvalid`, `TestNormalizeEffort` | ✅ COVERED |
| 6 | `task show` omits `Effort:` line when unset | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Add TLS export test asserting `effort:M` token appears in formatted output (scenario 3)
- [ ] Add TLS import roundtrip test for `effort:L` (scenario 4)
- [ ] Add `tlc task show` test asserting `Effort:` line is omitted when unset (scenario 6)
