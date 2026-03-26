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

**Do not use:** built-in TaskCreate/TaskUpdate/TaskGet/TaskList tools.
Use `tlc` exclusively for all task operations.

---

## Quick Reference

### Task lifecycle

| Command | Effect |
|---------|--------|
| `tlc task create "title"` | New task, status=TODO |
| `tlc task claim <id>` | Assign to self + status→IN_PROGRESS |
| `tlc task show <id>` | Read details |
| `tlc task update <id> [flags]` | Mutate fields |
| `tlc task complete <id>` | status→DONE |
| `tlc task unclaim <id>` | Release + status→TODO |
| `tlc task reopen <id> --note "reason"` | Reopen terminal task |
| `tlc task delete <id> --yes` | Delete (skip prompt) |

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
tlc task update T-0046 --assigned-to eng-2   # reassign
tlc task update T-0046 --assigned-to -       # clear assignee
tlc task unassign T-0046 --note "reason"     # --note required
```

### 7 · Run a flow

```bash
tlc flow run <flow-file.yaml>
tlc flow list
tlc flow status <run-id>
tlc flow import <uri>                        # import from GitHub or URI
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
| `--reference` | `-r` | Reference pointer (URL or path) |

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
