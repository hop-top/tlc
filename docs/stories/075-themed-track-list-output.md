---
status: paper
---

# 075 - Themed Track List Output

**ID**: 075
**Feature**: CLI Output
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [Team Lead](../personas/team-lead.md)
**Priority**: P2

## Story

As a Solo Developer, I want `tlc track list` to use color to
distinguish track health at a glance so that I can scan a
mixed list and immediately see which tracks are progressing,
which need intervention, and which have been abandoned.

## Context

Track list shows tracks across statuses (pending, active,
completed, abandoned) with computed state flags (healthy,
stale, blocked, unlinked). A monochrome table forces me to
read every row. Color-coding by health lets me triage in
seconds.

Colors are sourced from `hop.top/kit/cli.Theme` so they stay
consistent with task list, help output, and badges.

## Color Rules

Row color is determined by track status + state flags:

| Status    | State          | Color | Rationale                   |
|-----------|----------------|-------|-----------------------------|
| active    | healthy        | green | progressing, focus here     |
| active    | stale/blocked  | pink  | needs intervention          |
| abandoned | any            | muted | deprioritized, fade out     |
| pending   | any            | white | not started, neutral        |
| completed | any            | white | finished, neutral           |

Priority when multiple conditions apply:
1. Primary (green) — active + healthy
2. Flagged (pink) — active + stale or blocked
3. Faded (muted) — abandoned
4. Default (white) — pending, completed

## Acceptance Scenarios

### Scenario 1: Four-color track list

**Given** 8 tracks exist:

| ID        | Title                   | Status    | State   |
|-----------|-------------------------|-----------|---------|
| auth      | Auth system             | active    | healthy |
| payments  | Payment processing      | active    | healthy |
| cdn       | CDN migration           | active    | stale   |
| search    | Search rewrite          | active    | blocked |
| onboard   | Onboarding flow         | pending   | —       |
| docs      | Documentation refresh   | pending   | —       |
| legacy    | Legacy cleanup          | abandoned | —       |
| old-api   | Old API removal         | abandoned | —       |

**When** I run `tlc track list`

**Then** the table renders with four distinct colors:

| Track    | Color | Reason                       |
|----------|-------|------------------------------|
| auth     | green | active + healthy             |
| payments | green | active + healthy             |
| cdn      | pink  | active + stale               |
| search   | pink  | active + blocked             |
| onboard  | white | pending                      |
| docs     | white | pending                      |
| legacy   | muted | abandoned                    |
| old-api  | muted | abandoned                    |

### Scenario 2: Single-status filter — row coloring preserved

**Given** the same tracks, **When** I run
`tlc track list --status active`

**Then** all rows are active. Healthy tracks render green,
stale/blocked tracks render pink. Row-level coloring still
applies based on health state.

### Scenario 3: Headers and borders

**Given** any track list, **When** rendered as a table,
**Then** headers and borders use `theme.Muted` (Squid gray),
matching task list and fang help output style.

## Implementation

- `internal/cli/track_list.go` — `renderTrackListTable`
  computes `primaryRows` (active+healthy),
  `blockerRows` (active+stale/blocked),
  `blockedRows` (abandoned) from `trackRowData` state,
  passes to `renderTTYTable`
- Colors from `kitRootInstance.Theme` (`hop.top/kit/cli`)
- Reuses `WithPrimaryRows`, `WithBlockerRows`,
  `WithBlockedRows` options from `ttytable.go`

## Tests

### E2E
- planned: `tests/e2e/track_list_theme_test.go::TestTrackList_FourColorDefault`
- planned: `tests/e2e/track_list_theme_test.go::TestTrackList_StatusActiveFilterPreservesColors`
- planned: `tests/e2e/track_list_theme_test.go::TestTrackList_HeadersAndBordersMuted`

### Unit
- `internal/cli/track_list_test.go`
  - Existing tests cover listing and filtering
  - TODO: `TestTrackListTable_FourColors` — verify ANSI
    output for active+healthy (green), active+stale (pink),
    abandoned (muted), pending (white)
  - TODO: `TestTrackListTable_AllHealthy` — all active
    healthy tracks render green
