---
status: shipped-no-e2e
---

# 030 - TUI Navigation

**ID**: 030
**Feature**: TUI Interface
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P2

## Story

As a Solo Developer, I want to navigate, view, and interact with tasks
via a terminal UI so that I can work efficiently without context
switching to web interfaces or multiple terminals.

## Context

The TUI is built on `kit/tui` components (`kit/tui.List`,
`kit/tui.Progress`) and styled via `kit/cli.Theme`. Layout uses
Bubbletea under the hood, but all list rendering, item formatting,
and progress bars delegate to kit's reusable widgets. Styles are
derived from the shared theme in `internal/tui/styles/` via
`styles.NewFromTheme`.

## Acceptance Scenarios

1. **Given** TLC TUI is running, **When** I start `tlc tui`, **Then**:
   - A full-screen TUI interface appears
   - The main view shows a list of tasks
   - Navigation keys (arrow keys, hjkl) are responsive

2. **Given** TUI task list, **When** I press `f` to filter, **Then**:
   - A filter prompt appears
   - I can enter filter criteria (status, tags, assignee)
   - The task list updates to show filtered results

3. **Given** a task in the list, **When** I select a task and press `Enter`, **Then**:
   - The task detail view opens
   - Full task metadata is displayed (title, description, tags, assignee, status, created/updated times)
   - Related tasks or links are shown

4. **Given** task detail view, **When** I press `e` to edit, **Then**:
   - An edit dialog opens
   - I can modify task title, description, tags, assignee, status
   - Changes are committed when I confirm

5. **Given** TUI with a task selected, **When** I press `d` to delete, **Then**:
   - A confirmation dialog appears
   - Upon confirmation, the task is deleted
   - The view returns to the task list

6. **Given** TUI, **When** I press `?` for help, **Then**:
   - A help dialog displays available key bindings
   - Help is contextual (different for different views)

7. **Given** TUI task list, **When** I press `:` to enter command mode, **Then**:
   - A command prompt appears
   - I can type commands like `create`, `list`, `flow`
   - Commands execute and results are shown in TUI

## Tests

### E2E
- ❌ `tests/integration/tui_test.go` — NOT YET IMPLEMENTED
  - Needed: `TestTUIStartup`, `TestTUINavigation`, `TestTUIFiltering`, `TestTUITaskDetail`, `TestTUIEdit`, `TestTUIDelete`, `TestTUICommandMode`
  - Note: TUI testing requires terminal automation framework (tcell/termbox mock or Bubbletea test utilities)

### Unit
- ❌ `internal/tui/screen_test.go` — NOT YET CREATED
  - Needed: `TestScreenRendering`, `TestDimensionsCalculation`
- ❌ `internal/tui/input_test.go` — NOT YET CREATED
  - Needed: `TestKeyBinding`, `TestFilterParsing`, `TestCommandParsing`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | TUI startup, full-screen, responsive navigation | Not tested | ❌ NOT COVERED |
| 2 | Filter prompt and criteria parsing | Not tested | ❌ NOT COVERED |
| 3 | Task detail view with metadata | Not tested | ❌ NOT COVERED |
| 4 | Edit dialog and changes commitment | Not tested | ❌ NOT COVERED |
| 5 | Delete with confirmation | Not tested | ❌ NOT COVERED |
| 6 | Contextual help display | Not tested | ❌ NOT COVERED |
| 7 | Command mode execution | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Create TUI test infrastructure using Bubbletea test utilities
- [ ] Create E2E test suite in `tests/integration/tui_test.go` covering all 7 scenarios
- [ ] Create `internal/tui/input_test.go` for key binding and command parsing
- [ ] Create `internal/tui/screen_test.go` for rendering and layout calculations
- [ ] Add key binding tests (arrow keys, hjkl, f, e, d, ?, :)
- [ ] Test filter prompt with various input combinations
- [ ] Test task detail view rendering with all metadata fields
- [ ] Test edit dialog with field validation
- [ ] Test delete confirmation with cancellation
- [ ] Test contextual help based on current view
- [ ] Add command mode parsing and execution tests
