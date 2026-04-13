# Output Format Map: tlc -> kit/output

Author: $USER
Date: 2026-04-13

## Format Classification

| Format | Category | Handler | Notes |
|--------|----------|---------|-------|
| `json` | A: kit covers | `kit/output.Render(w, JSON, v)` | all commands |
| `yaml` | A: kit covers | `kit/output.Render(w, YAML, v)` | all commands |
| `table` | B: custom | `renderTTYTable` (lipgloss) | bespoke; colored rows, width-aware |
| `tls` | B: custom | `formatTLS()` | tlc-specific text serialization |
| `summary` | B: custom | `renderSummary()` | status-grouped task counts |
| `counters` | B: custom | `renderCounters()` | flat status counts |

## Output Paths by Command

### formatTasks (task list, task create result)
- json -> A: use kit/output.Render
- yaml -> A: use kit/output.Render
- tls -> B: keep formatTLS
- summary -> B: keep renderSummary
- counters -> B: keep renderCounters
- table -> B: keep renderTable (lipgloss bespoke)

### printTask (task show)
- json -> A: use kit/output.Render
- yaml -> A: use kit/output.Render
- default -> B: keep renderTaskDetail (lipgloss bespoke)

### formatLogs (log)
- json -> A: use kit/output.Render
- yaml -> A: use kit/output.Render
- table -> B: keep renderLogTable (lipgloss bespoke)

### flow (formatFlowRuns, printFlowRun)
- json -> A: use kit/output.Render
- yaml -> A: use kit/output.Render
- table -> B: keep renderFlowRunsTable (lipgloss bespoke)

### track list (renderTrackListJSON/YAML)
- json -> A: use kit/output.Render
- yaml -> A: use kit/output.Render
- table -> B: keep renderTrackListTable (lipgloss bespoke)

### track show (renderTrackShowJSON/YAML)
- json -> A: use kit/output.Render
- yaml -> A: use kit/output.Render
- default -> B: keep renderTrackShowDetail (lipgloss bespoke)

### workspace list
- json -> A: use kit/output.Render
- table -> B: keep printWorkspaceListTable (lipgloss bespoke)

### project list
- json -> A: use kit/output.Render
- yaml -> A: use kit/output.Render
- table -> B: keep renderProjectTable (lipgloss bespoke)

## HintSet Candidates (C)

| Command | Hint |
|---------|------|
| `task create` | "Run `tlc task claim <id>` to start working" |
| `task complete` | "Run `tlc task list` to see remaining tasks" |
| `track create` | "Run `tlc track update <id> --add-plan plan.md` to link a plan" |
| `upgrade` | "Run `tlc version` to verify" (kit provides RegisterUpgradeHints) |

## Migration Strategy

1. Replace all `json.MarshalIndent` + `yaml.Marshal` calls with
   `kit/output.Render(w, format, v)` for json/yaml paths
2. Pre-dispatch tlc-specific formats (tls, summary, counters) before
   calling Render
3. Table path continues to use bespoke lipgloss renderers
4. Wire HintSet for contextual post-command hints
