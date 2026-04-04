---
name: tlc
description: |
  Task management CLI for developers and AI agents. Use for creating,
  tracking, and completing work items with state machine enforcement,
  multi-agent coordination, and declarative flow orchestration.

  Automatically applies when: a `.tlc/` directory or `tlc` binary is
  available in the project, or the user references task IDs like T-0042.

  Use when: creating tasks, claiming work, tracking progress, running
  flows, querying status, or coordinating multi-agent work.
---

# tlc — Task Line CLI

CLI-first task management with state machine enforcement, multi-agent
coordination, and declarative flow execution.

## When to use tlc

- Track any unit of planned work → `tlc task create`
- Start working on a task → `tlc task claim`
- Hand off or delegate → `tlc task update --assigned-to`
- Query what's active → `tlc task list`
- Finish work → `tlc task complete`
- Orchestrate multi-step workflows → `tlc flow run`
- Group tasks into work streams → `tlc track create`
- Monitor work stream progress → `tlc track list`
- View phase breakdown → `tlc track show`
- Check project health → `tlc track summary`

**Do not use:** built-in TaskCreate/TaskUpdate/TaskGet/TaskList tools.
Use `tlc` exclusively for all task operations.

---

## Quick Reference

### Task lifecycle

| Command | Effect |
|---------|--------|
| `tlc task create "title"` | New task, status=TODO |
| `tlc task claim <id> [<id>...]` | Assign to self + status→IN_PROGRESS |
| `tlc task show <id>` | Read details |
| `tlc task update <id> [<id>...] [flags]` | Mutate fields (batch) |
| `tlc task complete <id> [<id>...]` | status→DONE |
| `tlc task unclaim <id> [<id>...]` | Release + status→TODO |
| `tlc task reopen <id> [<id>...] --note "reason"` | Reopen terminal task |
| `tlc task assign <assignee> <id> [<id>...]` | Assign to someone (assignee first) |
| `tlc task unassign <id> [<id>...] --note "reason"` | Remove assignee |
| `tlc task delete <id> [<id>...] --yes` | Delete (skip prompt) |

### Track lifecycle

| Command | Effect |
|---------|--------|
| `tlc track create "title" --type feature` | New track, status=pending |
| `tlc track list [--status active] [--type feature]` | List with progress/state |
| `tlc track show <id>` | Detail view with phase breakdown |
| `tlc track update <id> [flags]` | Mutate fields, validate transitions |
| `tlc track archive <id>` | completed/abandoned → archived |
| `tlc track abandon <id>` | active → abandoned |
| `tlc track delete <id>` | Delete (fails if linked tasks) |
| `tlc track summary` | Project pulse: health + status counts |

### Track status machine

```
pending → active → completed → archived
                 → abandoned → archived
```

Auto-transition: pending → active on first linked task claim.

### State machine

```
TODO → IN_PROGRESS → DONE
                   → SKIPPED
TODO → SKIPPED
```

Terminal states (`DONE`, `SKIPPED`) are immutable unless `--force`.

### ID formats (all equivalent)

```
46   0046   T-0046   @T-0046
```

---

## Core Workflows

### 1 · Create and claim a task

```bash
tlc task create "Add retry logic to sync" --tag feat --assigned-to eng-1
tlc task claim T-0046
```

### 2 · Update description without losing existing content

```bash
tlc task show T-0046                         # read first
tlc task update T-0046 -d "<existing>\n\nNew notes here"
```

`-d` REPLACES — always read before writing.

### 3 · Complete with session log

```bash
tlc task show T-0046                         # 1. read current description
tlc task update T-0046 -d "<existing desc>\n\nSessions:\n- <session-id> (<agent>)"
tlc task complete T-0046                     # 3. mark done
```

### 4 · Force a status transition

```bash
tlc task update T-0046 --status IN_PROGRESS --force   # bypass state machine
tlc task complete T-0046 --no-verify                  # skip state machine on complete
```

Bypass flags are NOT interchangeable:
- `complete --no-verify` — skip state check on complete
- `update --force` — force any status transition
- `delete --yes` / `-y` — skip confirmation prompt

### 5 · Query tasks

```bash
tlc task list                                # IN_PROGRESS first, then TODO
tlc task list --status TODO --limit 20
tlc task list --assigned-to eng-1
tlc task list --mine
tlc task list --tag feat --format json
tlc task list --summary                      # status counts only
```

### 6 · Assign and unassign

```bash
tlc task assign eng-2 T-0046                 # assign (assignee first)
tlc task assign eng-2 T-0046 T-0047          # assign multiple
tlc task update T-0046 --assigned-to eng-2   # reassign via update
tlc task update T-0046 --assigned-to -       # clear assignee
tlc task unassign T-0046 --note "reason"     # --note required
```

### 8 · Batch operations

```bash
# Multiple exact IDs
tlc task complete T-0046 T-0047 T-0048
tlc task claim T-0046 T-0047

# Regex pattern (prompts confirmation when >1 matched)
tlc task complete "T-004[678]" --no-prompt
tlc task assign eng-1 "T-00[12]\d" --no-prompt

# Match all
tlc task update "*" --assigned-to eng-1 --no-prompt

# --no-prompt flag: skip confirmation (also -y for delete)
tlc task delete T-0046 T-0047 --no-prompt
```

`--no-prompt` is a persistent flag on `task` — applies to all subcommands.
`--title` is blocked when updating multiple tasks.

### 7 · Run a flow

```bash
tlc flow run <flow-file.yaml>
tlc flow list
tlc flow status <run-id>
tlc flow import <uri>                        # import from GitHub or URI
```

### 9 · Manage tracks

```bash
# Create a feature track
tlc track create "Browser rendering" --type feature

# Link tasks to the track
tlc task create "Parse HTML" --track browser-rendering
tlc task create "Render DOM" --track browser-rendering --tag phase:1

# View progress
tlc track show browser-rendering
tlc track list --status active

# Link a plan with automatic task extraction
tlc track update browser-rendering --add-plan docs/plans/rendering.md

# Project health
tlc track summary
```

---

## task create — flags reference

| Flag | Short | Description |
|------|-------|-------------|
| `--assigned-to` | `-a` | Assignee username |
| `--tag` | | Tag (repeatable) |
| `--description` | `-d` | Task description |
| `--priority` | `-p` | P0–P3 |
| `--effort` | `-e` | XS, S, M, L, XL |
| `--status` | `-s` | Initial status (default: TODO) |
| `--blocked-by` | | Blocking task ID (repeatable; local ID or cross-project `project/task`) |
| `--reference` | `-r` | Reference pointer (URL or path) |
| `--track` | | Link task to a track |

### task update — blocker flags

| Flag | Description |
|------|-------------|
| `--add-blocked-by` | Add blocking task ID (repeatable; validates existence) |
| `--remove-blocked-by` | Remove blocking task ID (repeatable) |
| `--clear-blocked-by` | Clear all blockers |

### task list — flags reference

| Flag | Short | Description |
|------|-------|-------------|
| `--status` | `-s` | Filter: TODO, IN_PROGRESS, DONE, SKIPPED |
| `--assigned-to` | `-a` | Filter by assignee |
| `--mine` | | Filter to current user |
| `--tag` | | Filter by tag |
| `--limit` | `-n` | Max results (default 100) |
| `--summary` | | Status counts only |
| `--all-projects` | | Cross-project query |
| `--workspace` | | Query across workspace projects |
| `--track` | | Filter by track ID |

### track create — flags reference

| Flag | Description |
|------|-------------|
| `--type` | Track type: feature, bug, refactor (required) |
| `--id` | Custom ID slug (derived from title if omitted) |
| `--assigned-to` | Assignee |

### track update — flags reference

| Flag | Description |
|------|-------------|
| `--title` | Update title |
| `--status` | Transition status (validated) |
| `--type` | Update type |
| `--assigned-to` | Update assignee |
| `--add-plan` | Link plan file + extract tasks |

### track list — flags reference

| Flag | Description |
|------|-------------|
| `--status` | Filter: pending, active, completed, abandoned, archived |
| `--state` | Filter: stale, unlinked, blocked, healthy |
| `--type` | Filter by type |
| `--all-projects` | Cross-project query |

---

## Output formats

```bash
tlc -f json task list        # JSON
tlc -f yaml task show T-0046 # YAML
tlc -f tls task list         # Task Line Syntax (plain text)
tlc -q task create "title"   # quiet: suppress non-essential output
```

---

## Target another project

```bash
tlc -c /path/to/.tlc/config.yaml task list
```

---

## Common mistakes to avoid

| Mistake | Correct |
|---------|---------|
| `tlc task complete T-0046 --force` | `tlc task complete T-0046 --no-verify` |
| `tlc task delete T-0046 --force` | `tlc task delete T-0046 --yes` |
| `tlc task update T-0046 -d "..."` without reading first | `tlc task show T-0046` → then update |
| Prefix titles with "Task N:" | Let tlc assign IDs; omit prefix |
| Long `&&` chains with tlc | Run one command at a time |
| Use built-in task tools | Use `tlc task` exclusively |
| `tlc task reopen T-0046` | `tlc task reopen T-0046 --note "reason"` |
| `tlc task assign T-0046 alice` | `tlc task assign alice T-0046` (assignee first) |
| `tlc task delete T-0046 T-0047` (no flag) | `tlc task delete T-0046 T-0047 --no-prompt` |
| Delete track with linked tasks | Unlink tasks first: `tlc task update <id> --track -` |

---

## Assignee lookup order

1. `wsm workspace show <workspace> --json` — check current workspace
2. `aps profile list` + `aps profile show <profile>` — fallback
3. Still unclear → ask user; do not guess

---

## Advanced

### Statuses (uppercase only)

`TODO` · `IN_PROGRESS` · `DONE` · `SKIPPED`

`claim` → IN_PROGRESS; `unclaim` → TODO

### Tag operations

```bash
tlc task update T-0046 --add-tag bug
tlc task update T-0046 --remove-tag wip
tlc tag list
tlc label list
```

### Audit log

```bash
tlc log                      # all task state transitions
```

### TUI

```bash
tlc tui                      # keyboard-driven Kanban + dashboard
```

### URI scheme

```bash
tlc uri register             # register tlc:// with OS
tlc uri snippet              # print OS-specific config snippet
```

### Health check

```bash
tlc doctor                   # environment + config health
```

### Self-upgrade

```bash
tlc upgrade
```
