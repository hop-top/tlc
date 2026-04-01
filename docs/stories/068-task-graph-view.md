# 068 - Task Dependency Graph View

**ID**: 068
**Feature**: Task Management — Dependency Visualization
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md), [Team Lead](../personas/team-lead.md)
**Priority**: P1

## Story

Before this story, blocked-by relationships existed in the data model but had no visual
representation. After: `tlc task graph` renders a dependency graph showing tasks as nodes
and blocked-by relationships as directed edges, helping developers and agents see what is
blocking what at a glance.

## Acceptance Scenarios

1. **Given** tasks with no blocked-by relationships, **When** I run `tlc task graph`,
   **Then** each task appears as a root node with no indentation.

2. **Given** task T-B blocked by T-A, **When** I run `tlc task graph`,
   **Then** T-A appears at the top level and T-B is indented beneath it.

3. **Given** `tlc task graph --format dot`, **When** I run the command,
   **Then** output starts with `digraph` and contains directed edges as `"T-A" -> "T-B"`.

4. **Given** tasks with mixed statuses, **When** I run `tlc task graph --status TODO`,
   **Then** only TODO tasks appear in the graph.

5. **Given** a cross-project blocked-by reference, **When** I run `tlc task graph`,
   **Then** only edges whose source task is present in the filtered result are shown.

## Tests

### E2E
- ✅ `internal/cli/task_graph_e2e_test.go` — `TestTaskGraph_E2E_NoBlockers`
- ✅ `internal/cli/task_graph_e2e_test.go` — `TestTaskGraph_E2E_BlockerChain`
- ✅ `internal/cli/task_graph_e2e_test.go` — `TestTaskGraph_E2E_DotFormat`
- ✅ `internal/cli/task_graph_e2e_test.go` — `TestTaskGraph_E2E_StatusFilter`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | flat root nodes when no blockers | `TestTaskGraph_E2E_NoBlockers` | ✅ COVERED |
| 2 | blocked task indented under blocker | `TestTaskGraph_E2E_BlockerChain` | ✅ COVERED |
| 3 | dot format contains digraph + edges | `TestTaskGraph_E2E_DotFormat` | ✅ COVERED |
| 4 | --status filter reduces graph | `TestTaskGraph_E2E_StatusFilter` | ✅ COVERED |
