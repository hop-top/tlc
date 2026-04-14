# Story Gap Analysis: README Features vs docs/stories/

Author: $USER
Date: 2026-04-13

Cross-reference of README "Key Features" section against existing
story coverage in `docs/stories/`.

## Legend

- COVERED: dedicated story exists with acceptance criteria
- PARTIAL: feature touched by a story but not primary focus
- GAP: no story covers this feature

## Feature-to-Story Matrix

| # | README Feature | Story Coverage | Status |
|---|---------------|----------------|--------|
| 1 | Hybrid Storage Model | 001, 002 | COVERED |
| 2 | Complex Orchestration (Task Flows) | 020, user-stories-taskflow-0.1 | COVERED |
| 3 | Flows & Assignees | 020 (partial) | PARTIAL |
| 4 | Multi-Agent Collaboration | 003, 008 | COVERED |
| 5 | Deterministic Task Execution | 020, user-stories-taskflow-0.1 | COVERED |
| 6 | Audit-Ready Logging | 009 | COVERED |
| 7 | Natural Language Interface | US-NL-001..007 | COVERED |
| 8 | Agent Context Dump | US-NL-004 (partial) | PARTIAL |
| 9 | Advanced Query Engine | 002 (partial) | PARTIAL |
| 10 | Extension System | 010 (GitHub only) | PARTIAL |
| 11 | Event Bus | none | GAP |
| 12 | Auto-Archiving | none | GAP |
| 13 | Configurable Workflows | 004 | COVERED |
| 14 | Workspace Model | none | GAP |
| 15 | Track Registry | 070-073, 075, 076 | COVERED |
| 16 | Modern TUI & CLI | 030, 074, 075 | COVERED |
| 17 | XDG Specification Compliance | 062 (partial) | PARTIAL |
| 18 | Stale Task Detection | 066 | COVERED |
| 19 | Multi-Clone Workflows | none | GAP |
| 20 | Theme Customization | 030 (partial) | PARTIAL |

## Identified Gaps (no story)

### GAP-1: Event Bus (`kit/bus`)

README describes decoupled state-change propagation driving audit
logging, stale-detection hooks, and track auto-transitions.
No story captures the event bus as a user-facing or system-facing
feature with acceptance criteria.

Suggested ID range: 010-019 (External Sync / Infrastructure)
Suggested title: "Event-Driven State Propagation"

### GAP-2: Auto-Archiving

README describes automatic cleanup of completed tasks after a
configurable duration (default 7 days). No story defines the
archiving behavior, configuration, or edge cases (e.g. what
happens to tracked tasks).

Suggested ID range: 080+
Suggested title: "Auto-Archive Completed Tasks"

### GAP-3: Workspace Model

README describes organizing projects into workspaces and spaces
with cross-project querying (`--workspace`, `--space`, `--summary`).
No story covers workspace creation, project linking, or
cross-workspace task queries.

Suggested ID range: 080+
Suggested title: "Workspace and Space Organization"

### GAP-4: Multi-Clone Workflows

README describes shared vs isolated task strategies, automatic
project detection from git remote, and duplicate-id-strategy
configuration. No story covers this.

Suggested ID range: 080+
Suggested title: "Multi-Clone Task Management"

## Partial Coverage (story exists but incomplete)

### PARTIAL-1: Flows & Assignees (feature 3)

Story 020 covers flow execution but not the assignee registry,
capability-based auto-assignment, or `tlc assignee list/show`
commands. The flow templates planned story (012) is still
"planned" with no content.

### PARTIAL-2: Agent Context Dump (feature 8)

US-NL-004 covers context dump as a natural language query but
`tlc task prompt <id>` as a dedicated command with structured
output (markdown/JSON, deps, track info, audit log) has no
dedicated story.

### PARTIAL-3: Advanced Query Engine (feature 9)

Story 002 covers basic listing/filtering. Power-user shorthands
(`@me`, `#tag`), logical operators (AND/OR/NOT), and
metadata-aware searching are not covered by any story.

### PARTIAL-4: Extension System (feature 10)

Story 010 covers GitHub sync specifically. The extension system
itself (`kit/ext` registration, capability declaration, lifecycle)
and other providers (Jira, Linear) have no story.

### PARTIAL-5: XDG Compliance (feature 17)

Story 062 covers storage location validation. XDG path compliance
for data, logs, config across platforms is not a dedicated story.

### PARTIAL-6: Theme Customization (feature 20)

Story 030 mentions the TUI but theme switching, 250+ community
themes, lazy loading, and the theme picker UX are not covered
by any acceptance criteria.

## Recommendations

Priority order for new stories:
1. **Auto-Archiving** (GAP-2) -- user-visible, config-driven
2. **Workspace Model** (GAP-3) -- user-visible, CLI surface
3. **Multi-Clone Workflows** (GAP-4) -- user-visible, onboarding
4. **Agent Context Dump** (PARTIAL-2) -- agent-facing, used daily
5. **Advanced Query Engine** (PARTIAL-3) -- power-user, CLI
6. **Theme Customization** (PARTIAL-6) -- TUI, user delight
7. **Event Bus** (GAP-1) -- internal, but drives multiple features
8. **Extension System** (PARTIAL-4) -- extensibility story
