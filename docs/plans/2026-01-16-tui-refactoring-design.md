# TUI Model Refactoring Design

**Date:** 2026-01-16
**Goal:** Reduce model.go from 984 lines to ~600-700 lines by splitting into feature-based files

## Overview

Refactor `internal/tui/model.go` into 4 files organized by feature:
- **model.go** - Core model, initialization, and update dispatcher
- **handlers.go** - View-specific update handlers
- **commands.go** - All tea.Cmd returning functions
- **views.go** - All view rendering functions

## File Structure

### model.go (~350-400 lines)

**Contents:**
- Model struct definition with all fields
- `NewModel()` constructor
- `Init()` method
- Main `Update()` dispatcher (delegates by view)
- Common message handlers:
  - `tasksMsg`, `logsMsg`, `flowRunsMsg`
  - `error` handling
  - `tea.WindowSizeMsg` handling
- Helper methods:
  - `addFilter(field, value string) Model`
  - `syncViewport() Model`
  - `getLineOfSelected() int`
  - `groupTasksByStatus(tasks []*core.Task) map[core.TaskStatus][]*core.Task`

**Update dispatcher pattern:**
```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // 1. Handle common messages
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        // handle resize
    case error:
        m.err = msg
        return m, nil
    case tasksMsg:
        m.tasks = msg
        // ...
    case logsMsg:
        m.taskLogs = msg
        // ...
    case flowRunsMsg:
        m.flowRuns = msg
        return m, nil
    }

    // 2. Delegate to view-specific handlers
    switch m.view {
    case "dashboard":
        return handleDashboardUpdate(m, msg)
    case "detail":
        return handleDetailUpdate(m, msg)
    case "kanban":
        return handleKanbanUpdate(m, msg)
    case "flows":
        return handleFlowsUpdate(m, msg)
    case "search":
        return handleSearchUpdate(m, msg)
    case "form":
        return handleFormUpdate(m, msg)
    case "theme_picker":
        return handleThemePickerUpdate(m, msg)
    default:
        return handleDashboardUpdate(m, msg)
    }
}
```

### handlers.go (~200-250 lines)

**Contents:**
View-specific update handlers that process key/message events:

- `handleDashboardUpdate(m Model, msg tea.Msg) (Model, tea.Cmd)`
  - Navigation (j/k/up/down)
  - Actions (n for new, c/u for claim/unclaim, s for status, f/a for filters)
  - View switching (v, enter, esc)
  - Search (/)

- `handleDetailUpdate(m Model, msg tea.Msg) (Model, tea.Cmd)`
  - Actions (c/u for claim/unclaim, s for status)
  - Log sorting (o for order toggle)
  - Navigation (esc back to dashboard)

- `handleKanbanUpdate(m Model, msg tea.Msg) (Model, tea.Cmd)`
  - Navigation (j/k for task selection, h/l for moving tasks)
  - View switching (v, enter)

- `handleFlowsUpdate(m Model, msg tea.Msg) (Model, tea.Cmd)`
  - Navigation (j/k)
  - Refresh (r)
  - View switching (v)

- `handleSearchUpdate(m Model, msg tea.Msg) (Model, tea.Cmd)`
  - Text input handling
  - Live search triggering
  - Exit (enter/esc)

- `handleFormUpdate(m Model, msg tea.Msg) (Model, tea.Cmd)`
  - Form input delegation to huh.Form
  - Completion/abortion handling

- `handleThemePickerUpdate(m Model, msg tea.Msg) (Model, tea.Cmd)`
  - Theme picker delegation
  - Theme selection and application
  - Remote theme fetching (R key)

**Shared helpers:**
- Common key handlers (q/ctrl+c for quit, r for refresh, etc.)

### commands.go (~150-200 lines)

**Contents:**
All functions that return `tea.Cmd` or `tea.Msg`:

**Data fetching:**
- `(m Model) fetchTasks() tea.Msg`
- `(m Model) fetchLogs() tea.Msg`
- `(m Model) fetchFlowRuns() tea.Msg`
- `(m Model) syncPull() tea.Msg`

**Task actions:**
- `(m Model) claimTask(id string) tea.Cmd`
- `(m Model) unclaimTask(id string) tea.Cmd`
- `(m Model) rotateStatus(task *core.Task) tea.Cmd`
- `(m Model) moveTask(task *core.Task, dir int) tea.Cmd`

**Task creation:**
- `(m Model) createTask() tea.Cmd` - Initializes form
- `(m Model) saveTask(title, description string) tea.Cmd` - Persists new task

All commands follow the pattern of performing service calls and returning messages to update the model.

### views.go (~250-300 lines)

**Contents:**

**Constants:**
```go
const (
    headerSpacing      = 2
    viewportOffset     = 6
    searchInputPadding = 10
    kanbanMinColWidth  = 20
    kanbanColCount     = 3
)

var statusOrder = []core.TaskStatus{
    core.StatusTodo,
    core.StatusInProgress,
    core.StatusDone,
    core.StatusSkipped,
}
```

**Main view:**
- `(m Model) View() string` - Main dispatcher and viewport setup

**View renderers:**
- `(m Model) headerView() string` - Title and filter display
- `(m Model) helpView() string` - Context-sensitive help text
- `(m Model) dashboardContent() string` - Task list grouped by status
- `(m Model) detailView() string` - Single task details with logs
- `(m Model) kanbanView() string` - 3-column kanban board
- `(m Model) flowsContent() string` - Flow execution list

**Formatters:**
- `formatStatus(status core.TaskStatus) string` - Status icons
- `formatAssignee(assignee *string) string` - Assignee display
- `(m Model) getTagStyle(tag string) lipgloss.Style` - Tag coloring

## Dead Code Cleanup

**Remove during refactoring:**

1. **Duplicate `formatAssignee` function** (lines 979-984)
   - Keep only the standalone version in views.go
   - Remove the method version (lines 929-934)

2. **Excessive whitespace** (lines 779-807)
   - Normalize to standard Go formatting
   - Collapse unnecessary blank lines

3. **Inline magic numbers**
   - Extract to constants in views.go

## Implementation Order

1. **Create views.go**
   - Copy all view rendering functions
   - Extract constants
   - Remove duplicate formatAssignee
   - Clean up whitespace

2. **Create commands.go**
   - Move all tea.Cmd returning methods
   - Keep methods as receivers for service access

3. **Create handlers.go**
   - Extract view-specific update logic
   - Create handleXxxUpdate functions
   - Keep shared navigation helpers

4. **Update model.go**
   - Remove moved functions
   - Update Update() to dispatch to handlers
   - Keep common message handlers
   - Add groupTasksByStatus helper

5. **Test**
   - Run TUI to verify all views work
   - Test all key bindings
   - Verify theme picker functionality

## Expected Outcome

**File sizes:**
- model.go: ~350-400 lines (from 984)
- handlers.go: ~200-250 lines (new)
- commands.go: ~150-200 lines (new)
- views.go: ~250-300 lines (new)
- **Total: ~950-1150 lines**

**Benefits:**
- Much easier to find and maintain related code
- Clear separation of concerns (model/update/commands/views)
- Each file has a single responsibility
- Better follows Bubble Tea architectural patterns
- Easier to test individual components

**Trade-offs:**
- Slightly more total lines due to function signatures
- More files to navigate (but better organized)
- Some cross-file references (mitigated by same package)
