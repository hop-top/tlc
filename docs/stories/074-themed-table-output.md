# 074 - Themed Table Output

**ID**: 074
**Feature**: CLI Output
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [Team Lead](../personas/team-lead.md)
**Priority**: P2

## Story

As a Solo Developer, I want `tlc task list` to use color to
distinguish task states at a glance so that I can scan a mixed
list and immediately see what needs my attention, what is
blocking progress, and what is waiting.

## Context

The default `tlc task list` shows `IN_PROGRESS` and `TODO`
tasks. With 10+ entries, a monochrome table forces me to read
every row. Color-coding by role (primary match, blocker,
blocked, other) lets me triage in seconds.

Colors are sourced from `hop.top/kit/cli.Theme` so they stay
consistent with help output, badges, and future TUI adoption.

## Acceptance Scenarios

### Scenario 1: Four-color default list

**Given** 10 tasks exist:

| ID      | Title                       | Status      | Blocked By |
|---------|-----------------------------|-------------|------------|
| T-0001  | Design auth API             | IN_PROGRESS | —          |
| T-0002  | Implement JWT middleware    | IN_PROGRESS | —          |
| T-0003  | Write auth unit tests       | IN_PROGRESS | —          |
| T-0004  | Integrate OAuth provider    | IN_PROGRESS | —          |
| T-0005  | Add rate limiting           | TODO        | —          |
| T-0006  | Update API docs             | TODO        | T-0010     |
| T-0007  | Migrate session store       | TODO        | —          |
| T-0008  | Refactor token refresh      | TODO        | T-0003     |
| T-0009  | Deploy auth service         | TODO        | T-0004     |
| T-0010  | Review security audit       | IN_PROGRESS | —          |

**When** I run `tlc task list`

**Then** the table renders with four distinct colors:

- **Green** (`theme.Accent`): T-0001, T-0002, T-0003, T-0004,
  T-0010 — entire rows; these are IN_PROGRESS (first filter
  value)
- **Pink** (`theme.Secondary`): T-0003, T-0004, T-0010 —
  NOTE: these are IN_PROGRESS so they render green (primary
  wins over blocker); blocker color only applies to
  non-primary rows
- **Muted** (`theme.Muted`): T-0006, T-0008, T-0009 — entire
  rows; these TODO tasks are themselves blocked
- **White** (default): T-0005, T-0007 — entire rows; these
  TODO tasks are neither blockers nor blocked

Revised color assignment:

| Row     | Primary? | Blocker? | Blocked? | Color |
|---------|----------|----------|----------|-------|
| T-0001  | yes      | no       | no       | green |
| T-0002  | yes      | no       | no       | green |
| T-0003  | yes      | yes      | no       | green |
| T-0004  | yes      | yes      | no       | green |
| T-0005  | no       | no       | no       | white |
| T-0006  | no       | no       | yes      | muted |
| T-0007  | no       | no       | no       | white |
| T-0008  | no       | no       | yes      | muted |
| T-0009  | no       | no       | yes      | muted |
| T-0010  | yes      | yes      | no       | green |

All four colors appear: green (5 rows), muted (3 rows),
white (2 rows). Pink appears when a non-primary task is a
blocker (not demonstrated here since all blockers happen to
be IN_PROGRESS).

### Scenario 2: Blocker in pink

**Given** the same 10 tasks but T-0003 is TODO (not
IN_PROGRESS):

| ID      | Title                       | Status      | Blocked By |
|---------|-----------------------------|-------------|------------|
| T-0003  | Write auth unit tests       | TODO        | —          |
| T-0008  | Refactor token refresh      | TODO        | T-0003     |

**When** I run `tlc task list`

**Then** T-0003 renders in **pink** — it is a non-primary
row that blocks T-0008. T-0008 renders **muted** (blocked).
Now all four colors are visible.

| Row     | Primary? | Blocker? | Blocked? | Color |
|---------|----------|----------|----------|-------|
| T-0001  | yes      | no       | no       | green |
| T-0002  | yes      | no       | no       | green |
| T-0003  | no       | yes      | no       | pink  |
| T-0004  | yes      | yes      | no       | green |
| T-0005  | no       | no       | no       | white |
| T-0006  | no       | no       | yes      | muted |
| T-0007  | no       | no       | no       | white |
| T-0008  | no       | no       | yes      | muted |
| T-0009  | no       | no       | yes      | muted |
| T-0010  | yes      | yes      | no       | green |

### Scenario 3: Single-status filter — no emphasis

**Given** the same tasks, **When** I run
`tlc task list --status TODO`

**Then** no primary-row emphasis is applied (single filter
value). Blocker and blocked coloring still applies: blockers
render pink, blocked tasks render muted, all others white.

### Scenario 4: Headers and borders

**Given** any task list, **When** rendered as a table,
**Then** headers and borders use `theme.Muted` (Squid gray
`#858392`), matching the fang help output style.

## Color Priority

When a row qualifies for multiple roles, the first match
wins:

1. Primary (green) — matches first filter value
2. Blocker (pink) — non-primary, blocks another task
3. Blocked (muted) — task is itself blocked
4. Default (white) — none of the above

## Implementation

- `internal/cli/ttytable.go` — `renderTTYTable` with
  `WithPrimaryRows`, `WithBlockerRows`, `WithBlockedRows`
  options
- `internal/cli/formatter.go` — `renderTable` computes row
  sets from filters and task graph, passes to
  `renderTTYTable`
- Colors from `kitRootInstance.Theme` (`hop.top/kit/cli`)

## Tests

### E2E
- planned: `tests/e2e/task_list_theme_test.go::TestTaskList_FourColorDefault`
- planned: `tests/e2e/task_list_theme_test.go::TestTaskList_NonPrimaryBlockerPink`
- planned: `tests/e2e/task_list_theme_test.go::TestTaskList_SingleStatusFilterNoEmphasis`
- planned: `tests/e2e/task_list_theme_test.go::TestTaskList_HeadersAndBordersMuted`

### Unit
- `internal/cli/ttytable_test.go`
  - `TestTTYTableRendersHeaders` — headers present
  - `TestTTYTableRespectsWidth` — width constraint honored
  - TODO: `TestPrimaryRowsFromFilters` — multi-value filter
    detection
  - TODO: `TestBlockerRowDetection` — blocker identification
  - TODO: `TestBlockedRowDetection` — blocked task muting
