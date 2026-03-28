# T-0122 — TUI Slow / Freezing

Author: $USER
Date: 2026-03-28
Status: Investigated; fixes partially implemented

---

## Task list

| # | Severity | Issue | Status |
|---|----------|-------|--------|
| 1 | HIGH | File I/O inside `View()` render path | Fixed |
| 2 | HIGH | No debounce on live search — floods storage | Documented |
| 3 | MED | New `glamour.TermRenderer` per render frame | Fixed |
| 4 | LOW | `tea.WithMouseCellMotion()` floods update loop | Documented |
| 5 | LOW | `saveTask` queries all tasks to guess next ID | Pre-existing |

---

## Root causes

### 1 — File I/O inside `View()` (HIGH, deterministic freeze)

`views.go:getTagStyle` writes to disk on **every render frame** when a new
tag color is first assigned.

```
views.go:92-99
  viper.Set("ui.tag_colors", colors)
  config.PrepareViperForWrite(viper.GetViper())
  viper.WriteConfig()     // <-- synchronous disk write in hot path
```

`View()` is called by bubbletea after every `Update()`. Any key press, mouse
move, or async message triggers a full re-render including this call.
First time each unique tag is seen → freeze of 5–50 ms per tag (fsync on
SSD). On a list with many distinct tags this is additive and **deterministic**.

**Fix:** remove disk write from `getTagStyle`; persist tag colors lazily via
a dedicated `tea.Cmd` after tags are first computed (or on quit/save).

### 2 — Unbounded live search fires storage query per keystroke (HIGH, intermittent)

`handlers.go:274`
```
// Live search: trigger fetch on every update
return m, tea.Batch(cmd, m.fetchTasks)
```

`fetchTasks` calls `m.service.ListTasks(...)` — a SQLite query. Bubbletea
runs each `tea.Cmd` in its own goroutine, so rapid typing spawns N concurrent
DB reads. Results arrive out-of-order; each one triggers another render.
On slow storage or large task lists this causes visible lag / partial-freeze
after each keystroke.

**Fix:** debounce with a timer cmd; replace immediate `m.fetchTasks` with
`tea.Tick(150ms, ...)` that resets on each keystroke, then fires fetch.
Pseudocode:
```
type searchDebounceMsg struct{}

case keyMsg in handleSearchUpdate:
    m.searchInput, cmd = m.searchInput.Update(msg)
    debounce := tea.After(150*time.Millisecond, func() tea.Msg {
        return searchDebounceMsg{}
    })
    return m, tea.Batch(cmd, debounce)

case searchDebounceMsg in Update():
    return m, m.fetchTasks
```

### 3 — New `glamour.TermRenderer` per render frame (MED, additive latency)

`views.go:32-45`
```
func renderMarkdown(content string, wrapWidth int) string {
    renderer, err := glamour.NewTermRenderer(...)   // allocates new renderer
    ...
    out, _ := renderer.Render(content)
    return out
}
```

Called inside `detailView()` which is called inside `View()` on every frame.
`glamour.NewTermRenderer` parses style config, builds style tables — not cheap.

**Fix:** cache renderer at `Model` level; recreate only when `wrapWidth`
changes (i.e., on `tea.WindowSizeMsg`). Implemented.

### 4 — `tea.WithMouseCellMotion()` generates high-frequency mouse events (LOW)

`cli/tui.go:26`
```
tea.NewProgram(..., tea.WithMouseCellMotion())
```

`CellMotion` reports mouse position on every cell the cursor crosses —
typically hundreds of events/sec when the user moves the mouse. Each event
goes through `Update()` and then `viewport.Update()`. No mouse interaction
is implemented (`model.go:87-89` comment: "not fully implemented").

**Fix:** downgrade to `tea.WithMouseAllMotion()` or `tea.WithMouseClick()`
(click-only). Since mouse selection is unimplemented, clicks suffice.

### 5 — `saveTask` queries all tasks to compute next ID (LOW, one-shot)

`commands.go:181`
```
tasks, _ := m.service.ListTasks(ctx, core.Query{})
id := fmt.Sprintf("T-%04d", len(tasks)+1)
```

Unbounded full scan to pick an ID; also ignores deletions (IDs may collide).
Pre-existing "hack" per comment. Not a freeze cause but wasteful.

---

## Severity summary

| Issue | Freeze? | Deterministic? |
|-------|---------|----------------|
| File I/O in View() | Yes | Yes — first render with new tag |
| Live search flood | Yes | Intermittent — typing speed + DB latency |
| glamour per frame | Partial | Yes — detail view only |
| MouseCellMotion flood | Partial | Yes — whenever mouse moves |
| saveTask ListTasks | No | One-shot on save |

---

## Implemented fixes

- `views.go`: cache glamour renderer on Model; recreate on window resize only
- `views.go`: remove `viper.WriteConfig()` from `getTagStyle`; accumulate
  color assignments in model, persist via `persistTagColors` cmd on first use

See implementation in `internal/tui/views.go` and `internal/tui/model.go`.

## Recommended follow-on

- Debounce live search (issue #2) — ~15 lines, moderate risk (state machine)
- Downgrade mouse mode to `WithMouseClick()` (issue #4) — 1 line, low risk
