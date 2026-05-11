---
status: shipped
---

# 070 - Case-Insensitive + Alias Field Matching on Create + Update

**ID**: 070
**Feature**: Task Management — Write UX
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P0
**Extends**: [069 - Case-Insensitive + Alias + Fuzzy Field Matching](069-field-normalization.md)

## Story

Story 069 made `tlc task list --status` and `--priority` case-insensitive,
alias-aware, and fuzzy. The same normalization was never wired into the
write paths, so:

- `tlc task create x -p p1` was rejected with `invalid priority "p1"`
- `tlc task update T-x -e xl` was rejected with `invalid effort "xl"`
- `tlc task create x -s todo` SUCCEEDED but stored `status: "todo"`
  literal — the row then became invisible to every canonical filter
  (the list filter normalizes to `TODO` and never found it). This was
  data corruption, not just bad UX.

After this story: write paths use the same normalizer as the filter
path. Aliases that were valid on `task list` are now valid on `task
create` / `task update` too. A new `NormalizeEffort` helper rounds out
the trio so effort accepts inputs like `small`, `medium`, `huge` the
same way priority accepts `high`, `critical`.

Tags are explicitly out of scope — see "Out of Scope" below.

## Acceptance Scenarios

1. **Given** `tlc task create x --priority p1`, **When** I run the
   command, **Then** the task is created and stored with priority
   `P1`.

2. **Given** `tlc task create x --effort xl`, **When** I run the
   command, **Then** the task is created and stored with effort
   `XL`.

3. **Given** `tlc task create x --status todo`, **When** I run the
   command, **Then** the task is created and stored with status
   `TODO` — **NOT** the literal lowercase `"todo"` (regression
   guard).

4. **Given** `tlc task create x --priority high`, **When** I run the
   command, **Then** the task is stored with priority `P1` (alias
   resolution).

5. **Given** `tlc task create x --effort medium`, **When** I run the
   command, **Then** the task is stored with effort `M` (alias
   resolution).

6. **Given** `tlc task update T-x --priority p2`, **When** I run the
   command, **Then** the task's priority becomes `P2`.

7. **Given** `tlc task update T-x --effort s`, **When** I run the
   command, **Then** the task's effort becomes `S`.

8. **Given** `tlc task update T-x --priority critical`, **When** I run
   the command, **Then** the task's priority becomes `P0`.

9. **Given** `tlc task create x --priority garbage` (unresolvable),
   **When** I run the command, **Then** it returns an error
   `invalid priority "garbage"` and the task is not created.

10. **Given** a row created via `tlc task create x --status todo`,
    **When** I subsequently run `tlc task list --status TODO`,
    **Then** the new row appears in the results (the corruption
    regression — pre-070, this would have failed).

## Out of Scope

- **Tag normalization.** Tag filter and tag write paths remain
  case-sensitive (`--tag Foo` ≠ `--tag foo`). Separate track if/when
  needed.
- **Defensive normalize-on-read for existing corrupt rows.** Forward
  writes can no longer corrupt rows; rare existing corrupt rows can
  be healed manually with `tlc task update T-x -s todo --force`. The
  refactor needed for a clean read-side normalization (lifting
  `Normalize*` from `internal/cli` to `internal/core` so storage can
  call it) doesn't belong stapled to this story.
- **Retiring `core.ValidPriority`/`core.ValidEffort`.** Still used by
  `internal/inbox/parser.go` and `internal/vtodo/decode.go` —
  protocol parsers where strict validation is appropriate.

## Tests

### Unit
- ✅ `internal/cli/fieldnorm_test.go` — `TestNormalizeEffort` (31
  sub-cases: canonical, lowercase, descriptive aliases, mixed case,
  whitespace trimming, unknown, empty)

### E2E
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreateStatusLowercase`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreateStatusAlias`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreatePriorityLowercase`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreatePriorityAlias`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreateEffortLowercase`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreateEffortAlias`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreateStatusCorruptionRegression`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_CreateUnknownPriority`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UpdatePriorityLowercase`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UpdatePriorityAlias`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UpdateEffortLowercase`
- ✅ `internal/cli/task_fieldnorm_e2e_test.go` — `TestFieldNorm_E2E_UpdateEffortAlias`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | create accepts lowercase priority | `TestFieldNorm_E2E_CreatePriorityLowercase` | ✅ COVERED |
| 2 | create accepts lowercase effort | `TestFieldNorm_E2E_CreateEffortLowercase` | ✅ COVERED |
| 3 | create normalizes status to canonical | `TestFieldNorm_E2E_CreateStatusLowercase` | ✅ COVERED |
| 4 | create resolves priority alias | `TestFieldNorm_E2E_CreatePriorityAlias` | ✅ COVERED |
| 5 | create resolves effort alias | `TestFieldNorm_E2E_CreateEffortAlias` | ✅ COVERED |
| 6 | update accepts lowercase priority | `TestFieldNorm_E2E_UpdatePriorityLowercase` | ✅ COVERED |
| 7 | update accepts lowercase effort | `TestFieldNorm_E2E_UpdateEffortLowercase` | ✅ COVERED |
| 8 | update resolves priority alias | `TestFieldNorm_E2E_UpdatePriorityAlias` | ✅ COVERED |
| 9 | unresolvable input rejected | `TestFieldNorm_E2E_CreateUnknownPriority` | ✅ COVERED |
| 10 | corrupt-status regression | `TestFieldNorm_E2E_CreateStatusCorruptionRegression` | ✅ COVERED |
