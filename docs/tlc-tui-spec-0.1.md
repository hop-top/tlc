# TLC TUI Specification v0.1

## Overview

This document defines the Terminal User Interface (TUI) for TLC (Task Line CLI), providing an interactive, keyboard-driven interface for task management, flow execution monitoring, and collaboration.

## Design Principles

1. **Keyboard-first**: All operations accessible via keyboard (Vim-style bindings)
2. **CLI-backed**: TUI calls CLI commands, not database directly
3. **Real-time updates**: Live sync with external systems
4. **Responsive**: Adapts to terminal size and content
5. **Accessible**: Screen reader support, high contrast modes
6. **Fast**: Instant feedback with optimistic updates

---

## Architecture

### Component Stack

```
┌─────────────────────────────────────┐
│         TUI Layer (Rendering)       │
│  ┌─────────────┐  ┌──────────────┐ │
│  │ View Engine │  │ Event System │ │
│  └─────────────┘  └──────────────┘ │
├─────────────────────────────────────┤
│      State Management Layer         │
│  ┌──────────┐  ┌─────────────────┐ │
│  │  Cache   │  │ Optimistic UI   │ │
│  └──────────┘  └─────────────────┘ │
├─────────────────────────────────────┤
│         CLI Integration Layer       │
│  ┌──────────────────────────────┐  │
│  │  tlc task list --format json │  │
│  │  tlc task update ...         │  │
│  │  tlc flow run ...            │  │
│  └──────────────────────────────┘  │
└─────────────────────────────────────┘
```

### CLI Integration

TUI does NOT access database directly. All operations via CLI:

```go
// Get tasks
tasks := exec("tlc task list --format json")

// Update task
exec("tlc task update T-0042 --status DONE")

// Run flow
exec("tlc flow run flows/feature-dev.yaml --watch")
```

**Benefits**:
- Consistent business logic
- Same validation rules
- Audit logs automatically created
- Works with any storage backend

---

## Views

### 1. Dashboard View (Main)

Entry point showing task overview grouped by status/milestone.

```
┌─ TLC Dashboard ─────────────────────────────────────────────┐
│ ● sprint-2025-w03  [████████░░] 8/10 tasks  Due: 3 days     │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ IN PROGRESS (3)                                             │
│ ● T-0042 Add API rate limiting          @codex  feat:api   │
│ ● T-0089 Refactor auth module           @codex  chore:auth │
│ ○ T-0013 GitHub OAuth integration       @agent  feat:oauth │
│                                                              │
│ READY (5)                                                   │
│ □ T-0091 Add session tests      priority:high  domain:auth │
│ □ T-0092 Update API docs        priority:low   domain:docs │
│ □ T-0093 Fix login timeout      priority:high  domain:auth │
│ □ T-0094 Add rate limit config  priority:med   domain:api  │
│ □ T-0095 Refactor DB queries    priority:low   domain:db   │
│                                                              │
│ DONE (2)                                                    │
│ ✓ T-0040 Setup CI pipeline              @codex  ci:github  │
│ ✓ T-0039 Add Docker config              @codex  chore:infra│
│                                                              │
│ [n]ew  [e]dit  [f]ilter  [v]iew  [/]search  [q]uit         │
└─────────────────────────────────────────────────────────────┘
```

**Features**:
- Milestone progress bar
- Tasks grouped by status
- Status indicators (●=active, ○=claimed, □=ready, ✓=done)
- Assignee and labels inline
- Quick action menu at bottom

**Keybindings**:
| Key | Action |
|-----|--------|
| `j/k` or `↓/↑` | Navigate tasks |
| `Enter` | Open task detail |
| `n` | New task |
| `e` | Edit selected task |
| `d` | Delete task |
| `c` | Claim task |
| `u` | Unclaim task |
| `s` | Change status |
| `a` | Assign task |
| `f` | Open filter dialog |
| `v` | Switch view (list/kanban/timeline) |
| `/` | Search tasks |
| `r` | Refresh |
| `q` | Quit |

---

### 2. Task List View

Detailed table view with sorting and filtering.

```
┌─ Tasks ─ Filtered by: @me status:IN_PROGRESS ──────────────┐
│                                                              │
│ ┌──────┬─────────────────────┬──────────┬────────┬────────┐│
│ │ ID   │ Title               │ Status   │ Assign │ Tags   ││
│ ├──────┼─────────────────────┼──────────┼────────┼────────┤│
│ │►T-042│ Add API rate limit  │ PROGRESS │ codex  │ feat   ││
│ │ T-089│ Refactor auth       │ PROGRESS │ codex  │ chore  ││
│ │ T-091│ Add session tests   │ READY    │ -      │ test   ││
│ └──────┴─────────────────────┴──────────┴────────┴────────┘│
│                                                              │
│ Sort: created_at ▼  │  Showing 3 of 156 tasks               │
│                                                              │
│ [s]ort  [f]ilter  [c]lear filters  [Enter] view  [Esc] back│
└─────────────────────────────────────────────────────────────┘
```

**Features**:
- Sortable columns (click or `s` key)
- Active filter display
- Result count
- Current selection highlighted (`►`)

**Keybindings**:
| Key | Action |
|-----|--------|
| `j/k` or `↓/↑` | Navigate rows |
| `h/l` or `←/→` | Scroll horizontal |
| `Enter` | View task detail |
| `s` | Sort menu |
| `f` | Filter dialog |
| `C` | Clear filters |
| `Esc` | Back to dashboard |

---

### 3. Task Detail View

Full task information with logs and metadata.

```
┌─ T-0042: Add API rate limiting ─────────────────────────────┐
│                                                              │
│ Status: IN_PROGRESS  │  Priority: HIGH  │  Effort: MD       │
│ Created: 2025-01-16 10:30  │  Updated: 2025-01-16 14:22     │
│                                                              │
│ Labels: feat:api, domain:api, priority:high                 │
│ Milestone: sprint-2025-w03                                  │
│ Assigned: @codex                                            │
│                                                              │
│ ┌─ Description ─────────────────────────────────────────┐  │
│ │ Implement rate limiting for API endpoints using Redis│  │
│ │                                                       │  │
│ │ Requirements:                                         │  │
│ │ - Support per-user and per-IP limits                 │  │
│ │ - Configurable time windows                          │  │
│ │ - Return 429 with Retry-After header                 │  │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ ┌─ Git Integration ────────────────────────────────────┐   │
│ │ Branch: feat/0042-api-rate-limiting                  │   │
│ │ Worktree: .worktrees/feat-0042-api-rate-limiting     │   │
│ │ Status: 3 files changed, 152 insertions(+)           │   │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ ┌─ External Sync ──────────────────────────────────────┐   │
│ │ Origin: github.com/org/repo/issues/42                │   │
│ │ Last sync: 2 mins ago                                │   │
│ │ Sync status: ✓ No conflicts                          │   │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ ┌─ Recent Activity ────────────────────────────────────┐   │
│ │ 14:22  COMMENT     "Added Redis client" (@codex)     │   │
│ │ 11:15  ASSIGNED    @codex (codex)                    │   │
│ │ 10:35  STATUS_CHG  TODO → IN_PROGRESS (codex)        │   │
│ │ 10:30  CREATED     (system)                          │   │
│ │                                                       │   │
│ │ [more] Show all 12 log entries                       │   │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ [e]dit  [s]tatus  [a]ssign  [c]omment  [l]ogs  [←]back     │
└─────────────────────────────────────────────────────────────┘
```

**Features**:
- Full task metadata
- Description with markdown rendering
- Git integration status
- External sync status
- Recent activity timeline
- Quick actions

**Keybindings**:
| Key | Action |
|-----|--------|
| `e` | Edit task (opens editor) |
| `s` | Change status |
| `a` | Assign/reassign |
| `c` | Add comment |
| `l` | View full logs |
| `b` | Open branch in terminal |
| `g` | Open in GitHub (if synced) |
| `Tab` | Cycle focus (description/logs/metadata) |
| `Esc` or `←` | Back to list |

---

### 4. Flow Execution View

Live monitoring of flow execution with progress indicators.

```
┌─ Flow: Feature Development ─────────────────────────────────┐
│                                                              │
│ Execution ID: exec-1234                                     │
│ Started: 2025-01-16 14:30:00  │  Duration: 4m 23s           │
│                                                              │
│ ┌─ Progress ───────────────────────────────────────────┐   │
│ │ [████████████░░░░░░░░░░░░░░] 50%  (4/8 steps)       │   │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ ┌─ Step Tree ──────────────────────────────────────────┐   │
│ │ ✓ setup-worktree          [COMPLETED] 2.1s           │   │
│ │ ✓ parallel-setup          [COMPLETED] 4.3s           │   │
│ │   ✓ install-deps          [COMPLETED] 4.2s           │   │
│ │   ✓ create-branch         [COMPLETED] 0.1s           │   │
│ │ ▶ implement-feature       [RUNNING]   258.5s         │   │
│ │ ⋯ run-tests               [PENDING]                  │   │
│ │ ⋯ quality-checks          [PENDING]                  │   │
│ │   ⋯ lint                  [PENDING]                  │   │
│ │   ⋯ typecheck             [PENDING]                  │   │
│ │   ⋯ unit-tests            [PENDING]                  │   │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ ┌─ Current Step: implement-feature ────────────────────┐   │
│ │ Task: T-0042                                          │   │
│ │ Status: RUNNING                                       │   │
│ │                                                       │   │
│ │ Output (last 10 lines):                              │   │
│ │ > npm run build                                       │   │
│ │ > Compiling TypeScript...                            │   │
│ │ > ✓ src/middleware/rateLimit.ts                      │   │
│ │ > ✓ src/config/redis.ts                              │   │
│ │ > Build complete                                      │   │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ [p]ause  [x]cancel  [d]etails  [l]ogs  [r]efresh           │
└─────────────────────────────────────────────────────────────┘
```

**Features**:
- Overall progress bar
- Step tree with hierarchical display
- Status indicators (✓=done, ▶=running, ⋯=pending, ✗=failed)
- Timing for each step
- Live output from current step
- Real-time updates

**Keybindings**:
| Key | Action |
|-----|--------|
| `j/k` or `↓/↑` | Navigate steps |
| `Enter` | View step details |
| `p` | Pause execution |
| `x` | Cancel execution |
| `d` | Show step details |
| `l` | Full logs view |
| `r` | Manual refresh |
| `Esc` | Back to dashboard |

**Auto-refresh**: Updates every 1 second while flow is running.

---

### 5. Kanban Board View

Visual board with drag-and-drop (keyboard navigation).

```
┌─ Kanban Board ──────────────────────────────────────────────┐
│                                                              │
│ ┌──────TODO──────┐┌───IN_PROGRESS───┐┌──────DONE──────┐   │
│ │ 5 tasks        ││ 3 tasks         ││ 12 tasks       │   │
│ ├────────────────┤├─────────────────┤├────────────────┤   │
│ │                ││                 ││                │   │
│ │┌──────────────┐││┌───────────────┐││┌──────────────┐│   │
│ ││►T-091        │││ T-042         │││ T-040        ││   │
│ ││Session tests ││││ API rate lim  │││ Setup CI     ││   │
│ ││              │││ @codex        │││ @codex       ││   │
│ ││priority:high │││ feat:api      │││ ci:github    ││   │
│ │└──────────────┘││└───────────────┘││└──────────────┘│   │
│ │                ││                 ││                │   │
│ │┌──────────────┐││┌───────────────┐││┌──────────────┐│   │
│ ││ T-092        │││ T-089         │││ T-039        ││   │
│ ││Update docs   │││ Refactor auth │││ Docker cfg   ││   │
│ ││              │││ @codex        │││ @codex       ││   │
│ ││priority:low  │││ chore:auth    │││ chore:infra  ││   │
│ │└──────────────┘││└───────────────┘││└──────────────┘│   │
│ │                ││                 ││                │   │
│ │     ...        ││       ...       ││      ...       │   │
│ └────────────────┘└─────────────────┘└────────────────┘   │
│                                                              │
│ [j/k] navigate  [h/l] move card  [Enter] view  [f]ilter    │
└─────────────────────────────────────────────────────────────┘
```

**Features**:
- Columns for each status
- Card preview with key info
- Task count per column
- Visual selection
- Keyboard-based "drag and drop"

**Keybindings**:
| Key | Action |
|-----|--------|
| `j/k` or `↓/↑` | Navigate cards in column |
| `h/l` or `←/→` | Move card between columns (updates status) |
| `Enter` | View card details |
| `f` | Filter board |
| `v` | Switch view (list/kanban/timeline) |
| `Esc` | Back to dashboard |

---

### 6. Search/Filter Dialog

Fuzzy search and filter builder.

```
┌─ Search Tasks ──────────────────────────────────────────────┐
│                                                              │
│ Query: @me #auth priority:high_                             │
│                                                              │
│ ┌─ Results (8 tasks) ──────────────────────────────────┐   │
│ │ ►T-0091 Add session tests             @codex  test:auth│ │
│ │  T-0093 Fix login timeout             @codex  fix:auth │ │
│ │  T-0089 Refactor auth module          @codex  chore    │ │
│ │  T-0087 Add 2FA support               @codex  feat:auth│ │
│ │  T-0085 Update auth docs              @codex  docs     │ │
│ │  T-0083 Fix token refresh             @codex  fix:auth │ │
│ │  T-0081 Add OAuth provider            @codex  feat     │ │
│ │  T-0079 Audit auth logs               @codex  chore    │ │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ ┌─ Filters ────────────────────────────────────────────┐   │
│ │ Status: [x] TODO  [x] IN_PROGRESS  [ ] DONE  [ ] SKIP│  │
│ │ Assigned: [@me] [____________________]               │  │
│ │ Tags: [#auth] [#____________]                        │  │
│ │ Priority: [ ] critical [x] high [ ] medium [ ] low   │  │
│ │ Domain: [______________]                             │  │
│ └───────────────────────────────────────────────────────┘  │
│                                                              │
│ [Tab] toggle filter/results  [Enter] view  [Esc] cancel    │
└─────────────────────────────────────────────────────────────┘
```

**Features**:
- Real-time query preview
- Live results as you type
- Filter checkboxes for common fields
- Query history (up/down arrows)
- Fuzzy matching

**Keybindings**:
| Key | Action |
|-----|--------|
| `Type` | Update query |
| `j/k` or `↓/↑` | Navigate results |
| `Tab` | Toggle filter panel / results |
| `Enter` | View selected result |
| `Ctrl+F` | Focus filter panel |
| `Esc` | Cancel and return |

---

## Navigation Model

### View Hierarchy

```
Dashboard (main)
├── Task List
│   └── Task Detail
│       ├── Edit Task
│       ├── Logs View
│       └── Comment Dialog
├── Kanban Board
│   └── Task Detail
├── Flow Execution
│   ├── Step Details
│   └── Logs View
├── Search/Filter
│   └── Task Detail
└── Settings
```

### Global Keybindings

Available in all views:

| Key | Action |
|-----|--------|
| `?` | Show help (keybinding reference) |
| `/` | Quick search |
| `:` | Command palette |
| `Ctrl+R` | Refresh current view |
| `Ctrl+C` | Cancel/interrupt |
| `q` | Quit (from main views) |
| `Esc` | Back/cancel |

### Command Palette

Press `:` to open command palette with fuzzy search:

```
┌─ Command Palette ───────────────────────────────────────────┐
│                                                              │
│ > task create_                                              │
│                                                              │
│ task create                    Create new task              │
│ task list                      Show task list               │
│ task update                    Update task                  │
│ flow run                       Run flow                     │
│ sync pull                      Pull from external systems   │
│ view kanban                    Switch to kanban view        │
│ help keybindings               Show all keybindings         │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

Type to filter, `Enter` to execute, `Esc` to cancel.

---

## State Management

### Local Cache

TUI maintains in-memory cache of tasks:

```go
type Cache struct {
    tasks       map[string]Task
    lastUpdate  time.Time
    filters     FilterSet
    sortBy      string
}

// Initial load
cache.Refresh() // calls: tlc task list --format json

// Periodic refresh (every 30s)
go func() {
    ticker := time.NewTicker(30 * time.Second)
    for range ticker.C {
        cache.Refresh()
    }
}()
```

### Optimistic Updates

Immediate UI feedback before CLI confirms:

```go
// User changes status
task.Status = "DONE"
render(task) // Show immediately

// Background CLI call
go func() {
    err := exec("tlc task update T-0042 --status DONE")
    if err != nil {
        // Rollback on error
        task.Status = "IN_PROGRESS"
        render(task)
        showError(err)
    } else {
        // Confirm success
        cache.Refresh()
    }
}()
```

### Real-Time Sync

Poll for external system changes:

```go
// Check for sync updates (every 5 minutes)
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        exec("tlc sync pull --all")
        cache.Refresh()
        notify("Synced with external systems")
    }
}()
```

---

## UI Components

### Task Card (Reusable)

```
┌──────────────────────────────────┐
│ T-0042                           │
│ Add API rate limiting            │
│                                  │
│ @codex  feat:api  priority:high │
└──────────────────────────────────┘
```

**Variants**:
- Compact (1 line): `T-0042 Add API rate limiting @codex`
- Normal (4 lines): As shown above
- Expanded (with description): Includes first 2 lines of description

### Status Badge

```
TODO         [   ]  Gray
IN_PROGRESS  [ ● ]  Blue
DONE         [ ✓ ]  Green
SKIPPED      [ - ]  Yellow
BLOCKED      [ ! ]  Red
```

### Progress Bar

```
[████████████░░░░░░░░░░░░░░] 50%
[████████████████████████░░] 90%
[██████████████████████████] 100%
```

### Notification Toast

Appears at bottom for 3 seconds:

```
┌────────────────────────────────┐
│ ✓ Task T-0042 updated          │
└────────────────────────────────┘

┌────────────────────────────────┐
│ ⚠ Sync conflict in T-0041      │
└────────────────────────────────┘

┌────────────────────────────────┐
│ ✗ Failed to update task        │
└────────────────────────────────┘
```

---

## Configuration

TUI settings in config file:

```yaml
# ~/.config/tlc/config.yaml
ui:
  tui:
    # Refresh interval
    refresh_interval: 30s
    sync_poll_interval: 5m

    # Theme
    theme: default  # default, dark, light, solarized
    color_scheme: 16  # 16, 256, truecolor

    # Behavior
    confirm_delete: true
    confirm_cancel: true
    auto_refresh: true

    # View defaults
    default_view: dashboard  # dashboard, list, kanban
    default_sort: created_at
    default_filter: "!status:DONE !status:SKIPPED"

    # Keybindings
    vim_mode: true
    custom_bindings:
      quit: ["q", "Q"]
      refresh: ["r", "Ctrl+R"]
```

### Theme System

Built-in themes:

```yaml
themes:
  default:
    primary: blue
    success: green
    warning: yellow
    error: red
    text: white
    background: black

  solarized:
    primary: "#268bd2"
    success: "#859900"
    warning: "#b58900"
    error: "#dc322f"
    text: "#839496"
    background: "#002b36"
```

---

## Accessibility

### Screen Reader Support

TUI provides accessible navigation:

```
Current view: Dashboard
Selected: Task T-0042, Add API rate limiting, Status: In Progress,
Assigned to codex, Labels: feat:api, priority:high
Press Enter to view details, press E to edit, press S to change status
3 tasks in progress, 5 tasks ready
```

### High Contrast Mode

```yaml
ui:
  tui:
    high_contrast: true
```

Uses bold text and clear status indicators:

```
TODO         [   ]  ⚪
IN_PROGRESS  [███]  🔵
DONE         [✓✓✓]  🟢
```

### Keyboard-Only Operation

All features accessible via keyboard:
- No mouse required
- Tab navigation between panels
- Arrow keys for lists
- Vim bindings supported

### Font Size Adaptation

Detects terminal font size and adjusts layout:

```go
// Small terminal (< 80 cols)
// Use compact view

// Medium terminal (80-120 cols)
// Normal view

// Large terminal (> 120 cols)
// Split panes, show more info
```

---

## Performance

### Lazy Loading

Load tasks on demand:

```go
// Initial load: first 100 tasks
tasks := exec("tlc task list --limit 100 --format json")

// Scroll to bottom: load next 100
if scrolledToBottom {
    tasks += exec("tlc task list --limit 100 --offset 100 --format json")
}
```

### Render Optimization

Only re-render changed components:

```go
// Diff-based rendering
oldView := renderView(state)
newView := renderView(newState)

diff := computeDiff(oldView, newView)
applyDiff(diff) // Only update changed cells
```

### Background Operations

Heavy operations run in background:

```go
// Non-blocking refresh
go func() {
    tasks := exec("tlc task list --format json")
    updateCache(tasks)
    triggerRender()
}()

// User can continue interacting while loading
```

---

## Error Handling

### Error Display

Errors shown as toasts with retry option:

```
┌─────────────────────────────────────────┐
│ ✗ Error updating task T-0042            │
│   Invalid status transition             │
│   Press R to retry, Esc to dismiss      │
└─────────────────────────────────────────┘
```

### Offline Mode

TUI detects offline state:

```
┌──────────────────────────────────────────┐
│ ⚠ Offline - External sync unavailable    │
│   Local operations still work            │
└──────────────────────────────────────────┘
```

### Conflict Resolution UI

Visual diff for sync conflicts:

```
┌─ Resolve Conflict: T-0042 ──────────────────────────────────┐
│                                                              │
│ Field: title                                                │
│                                                              │
│ Local (your changes):                                       │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Add API rate limiting with Redis backend              │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                              │
│ Remote (GitHub):                                            │
│ ┌────────────────────────────────────────────────────────┐ │
│ │ Implement API rate limiting                            │ │
│ └────────────────────────────────────────────────────────┘ │
│                                                              │
│ Resolution:                                                 │
│ ( ) Keep local  ( ) Keep remote  (●) Manual edit           │
│                                                              │
│ [Enter] confirm  [Esc] cancel                               │
└─────────────────────────────────────────────────────────────┘
```

---

## Launch & Initialization

### Starting TUI

```bash
# Launch TUI
tlc tui

# Launch with specific view
tlc tui --view kanban

# Launch with filter
tlc tui --filter "@me status:IN_PROGRESS"

# Launch in specific project
cd /path/to/project
tlc tui
```

### Initialization Sequence

```
1. Load config (~/.config/tlc/config.yaml)
2. Initialize cache (tlc task list --format json)
3. Start background workers (refresh, sync polling)
4. Render initial view
5. Enter event loop
```

### Graceful Shutdown

```
1. User presses 'q'
2. Confirm if unsaved changes
3. Stop background workers
4. Save UI state (view, filters, etc.)
5. Clear terminal
6. Exit
```

---

## Testing

### TUI Testing Framework

```bash
# Automated UI tests
tlc tui test --scenario task-creation
tlc tui test --scenario flow-execution
tlc tui test --all

# Output:
Running TUI tests...
✓ Task creation flow
✓ Status change
✓ Filter application
✓ Kanban drag-drop
✓ Flow monitoring
✓ Search functionality

6/6 tests passed
```

### Manual Testing Checklist

```markdown
- [ ] Launch TUI
- [ ] Navigate between views
- [ ] Create new task
- [ ] Update task status
- [ ] Apply filters
- [ ] Search tasks
- [ ] View task details
- [ ] Monitor flow execution
- [ ] Handle errors gracefully
- [ ] Sync with external systems
- [ ] Resolve conflicts
- [ ] Quit cleanly
```

---

## Implementation Notes

### Recommended Libraries

**Go (Recommended)**:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — Elm-inspired TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — Styling and layout
- [Bubbles](https://github.com/charmbracelet/bubbles) — Common UI components (list, table, spinner, etc.)
- [Glamour](https://github.com/charmbracelet/glamour) — Markdown rendering

**Alternative Go**:
- [tview](https://github.com/rivo/tview) — Traditional terminal UI framework
- [tcell](https://github.com/gdamore/tcell) — Low-level terminal handling

**Rust**:
- [ratatui](https://github.com/ratatui-org/ratatui) — Terminal UI library
- [crossterm](https://github.com/crossterm-rs/crossterm) — Cross-platform terminal

**Python**:
- [textual](https://github.com/Textualize/textual) — Modern TUI framework

### Architecture Pattern

#### Bubble Tea (Recommended)

Bubble Tea uses The Elm Architecture (TEA) with Model-Update-View pattern:

```go
package main

import (
    "os/exec"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/lipgloss"
)

// Model holds application state
type model struct {
    tasks      []Task
    view       string // "dashboard", "list", "detail"
    selected   int
    loading    bool
    lastSync   time.Time
}

// Messages (events)
type tickMsg time.Time
type tasksLoadedMsg []Task
type taskUpdatedMsg Task

// Init returns initial commands
func (m model) Init() tea.Cmd {
    return tea.Batch(
        loadTasks(),      // Load initial data
        tickEvery(30*time.Second), // Refresh timer
    )
}

// Update handles messages and returns new state
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {

    case tea.KeyMsg:
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "j", "down":
            m.selected++
        case "k", "up":
            m.selected--
        case "enter":
            return m, viewTaskDetail(m.tasks[m.selected])
        case "n":
            return m, createTask()
        }

    case tasksLoadedMsg:
        m.tasks = []Task(msg)
        m.loading = false

    case tickMsg:
        // Auto-refresh
        return m, tea.Batch(
            loadTasks(),
            tickEvery(30*time.Second),
        )
    }

    return m, nil
}

// View renders the UI
func (m model) View() string {
    if m.loading {
        return "Loading tasks..."
    }

    // Use lipgloss for styling
    titleStyle := lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("#00FF00"))

    s := titleStyle.Render("TLC Dashboard") + "\n\n"

    for i, task := range m.tasks {
        cursor := " "
        if i == m.selected {
            cursor = "►"
        }
        s += fmt.Sprintf("%s %s %s\n", cursor, task.ID, task.Title)
    }

    s += "\n[n]ew  [q]uit"
    return s
}

// Commands (async operations)
func loadTasks() tea.Cmd {
    return func() tea.Msg {
        // Call CLI
        out, _ := exec.Command("tlc", "task", "list", "--format", "json").Output()
        var tasks []Task
        json.Unmarshal(out, &tasks)
        return tasksLoadedMsg(tasks)
    }
}

func tickEvery(d time.Duration) tea.Cmd {
    return tea.Tick(d, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}

func main() {
    p := tea.NewProgram(model{loading: true})
    if _, err := p.Run(); err != nil {
        panic(err)
    }
}
```

#### Component Examples

**Using Bubbles components**:

```go
import (
    "github.com/charmbracelet/bubbles/list"
    "github.com/charmbracelet/bubbles/table"
    "github.com/charmbracelet/bubbles/spinner"
)

// Task list with bubbles/list
type model struct {
    list list.Model
}

func (m model) Init() tea.Cmd {
    items := []list.Item{
        taskItem{id: "T-0042", title: "Add API rate limiting"},
        taskItem{id: "T-0089", title: "Refactor auth module"},
    }

    m.list = list.New(items, list.NewDefaultDelegate(), 0, 0)
    m.list.Title = "Tasks"

    return nil
}

// Task table with bubbles/table
func createTaskTable(tasks []Task) table.Model {
    columns := []table.Column{
        {Title: "ID", Width: 10},
        {Title: "Title", Width: 40},
        {Title: "Status", Width: 15},
        {Title: "Assigned", Width: 15},
    }

    rows := []table.Row{}
    for _, t := range tasks {
        rows = append(rows, table.Row{
            t.ID, t.Title, t.Status, t.AssignedTo,
        })
    }

    t := table.New(
        table.WithColumns(columns),
        table.WithRows(rows),
        table.WithFocused(true),
    )

    return t
}

// Loading spinner
type model struct {
    spinner spinner.Model
}

func (m model) Init() tea.Cmd {
    m.spinner = spinner.New()
    m.spinner.Spinner = spinner.Dot
    return m.spinner.Tick
}
```

**Using Lip Gloss for styling**:

```go
import "github.com/charmbracelet/lipgloss"

var (
    // Status badge styles
    todoStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#888888")).
        Render

    inProgressStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#00AAFF")).
        Bold(true).
        Render

    doneStyle = lipgloss.NewStyle().
        Foreground(lipgloss.Color("#00FF00")).
        Render

    // Card style
    cardStyle = lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(lipgloss.Color("#874BFD")).
        Padding(1, 2).
        Width(40)

    // Layout
    docStyle = lipgloss.NewStyle().
        Padding(1, 2, 1, 2)
)

func renderTask(t Task) string {
    var statusBadge string
    switch t.Status {
    case "TODO":
        statusBadge = todoStyle("[ ]")
    case "IN_PROGRESS":
        statusBadge = inProgressStyle("[●]")
    case "DONE":
        statusBadge = doneStyle("[✓]")
    }

    content := fmt.Sprintf(
        "%s %s\n%s\n@%s  %s",
        statusBadge,
        t.ID,
        t.Title,
        t.AssignedTo,
        strings.Join(t.Tags, ", "),
    )

    return cardStyle.Render(content)
}
```

**Using Glamour for markdown**:

```go
import "github.com/charmbracelet/glamour"

func renderDescription(markdown string) string {
    r, _ := glamour.NewTermRenderer(
        glamour.WithAutoStyle(),
        glamour.WithWordWrap(80),
    )

    out, _ := r.Render(markdown)
    return out
}

// In task detail view
func (m model) View() string {
    task := m.currentTask

    s := fmt.Sprintf("# %s: %s\n\n", task.ID, task.Title)
    s += fmt.Sprintf("**Status**: %s  |  **Priority**: %s\n\n",
        task.Status, task.Meta["priority"])

    // Render markdown description
    s += "## Description\n\n"
    s += renderDescription(task.Description)

    return s
}
```

---

## Future Enhancements

### Mouse Support

Optional mouse support:

```yaml
ui:
  tui:
    mouse_enabled: true
```

- Click to select tasks
- Drag cards in kanban
- Scroll with mouse wheel
- Right-click context menus

### Split Panes

Multiple views simultaneously:

```
┌─ Task List ─────────────┬─ Task Detail ──────────────┐
│ ►T-042 Add API...       │ T-0042: Add API rate lim.. │
│  T-089 Refactor auth    │                            │
│  T-091 Add tests        │ Status: IN_PROGRESS        │
│                         │ Assigned: @codex           │
│                         │                            │
│                         │ Description:               │
│                         │ Implement rate limiting... │
└─────────────────────────┴────────────────────────────┘
```

### Collaboration Indicators

Show who's viewing/editing:

```
T-0042 Add API rate limiting  @codex  👁 agent (viewing)
T-0089 Refactor auth module   @codex  ✏️  codex (editing)
```

---

## References

- [tlc-cli-spec-0.1.md](tlc-cli-spec-0.1.md) — CLI commands
- [tlc-config-spec-0.1.md](tlc-config-spec-0.1.md) — Configuration
- [task-crud-spec-0.1.md](task-crud-spec-0.1.md) — Task schema
- [task-flow-spec-0.1.md](task-flow-spec-0.1.md) — Flow execution

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
