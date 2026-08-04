# tlc Cheatsheet — Task Management (Agent)

Quick reference for autonomous agents, scripts, and LLMs using the CLI.
Scannable in 30 seconds.

---

## Prerequisites

```bash
tlc version                              # verify install
tlc doctor                               # check env + config health
tlc init                                 # initialise project (first time only)

# Target another project per-call
tlc -c /path/to/.tlc/config.yaml task list
```

---

## Agent Loop Contract

```
1. Claim      →  tlc task claim <id>              (transitions TODO → IN_PROGRESS)
2. Work       →  implement, commit, test
3. Log        →  tlc task update <id> -d "<prev>\n\nSessions:\n- <session-id> (<agent>)"
4. Complete   →  tlc task complete <id>           (transitions IN_PROGRESS → DONE)
```

**DO:** claim before starting. **DON'T:** make plan/code/doc changes without a claimed task.
**DO:** read current description before `-d` (it replaces, not appends).
**DON'T:** use built-in TaskCreate/TaskUpdate/TaskList — use `tlc` exclusively.

---

## Create

```bash
tlc task create "Title"                          # tlc auto-assigns ID
tlc task create "Title" --tag feat --priority P1
tlc task create "Title" --assigned-to exo
tlc task create "Title" --blocked-by T-0041 --blocked-by other-project/T-0007
tlc task create "Title" --effort M -d "Details"
```

| Flag | Values | Purpose |
|------|--------|---------|
| `--tag` | string (repeatable) | categorise |
| `--priority` | P0 P1 P2 P3 | importance |
| `--effort` | XS S M L XL | size estimate |
| `--blocked-by` | task ID (repeatable) | dependency |
| `--assigned-to` | profile/username | ownership |
| `--timeout` | `2h`, `30m` | per-task stale timeout |
| `--due` | `tomorrow`, `in 3d`, ISO | deadline |
| `--remind-at` | datetime | one-shot reminder |
| `--remind-every` | `1h`, `30m` | recurring reminder |
| `--no-auto-remind` | flag | suppress 12h auto-remind |

---

## Reminders

```bash
tlc task remind                          # upcoming + overdue
tlc task remind --check                  # exit 1 if overdue
```

---

## List / Query — Deterministic First

```bash
tlc task list                            # IN_PROGRESS first, then TODO
tlc task list --status TODO
tlc task list --status IN_PROGRESS --status TODO
tlc task list --priority P0 --priority P1
tlc task list --assigned-to exo
tlc task list --mine                     # current user
tlc task list --tag feat
tlc task list --blocked                  # has a blocker set
tlc task list --blocked-by T-0041        # blocked by specific task
tlc task list --stale                    # past stale timeout
tlc task list --limit 30                 # default is 100; narrow when needed
tlc task list --counters                 # flat status counts (for monitoring)
tlc task list --all-projects             # across all registered projects
tlc task list --format json              # machine-readable
```

### ID flexibility

`46`, `0046`, `T-0046`, `@T-0046` all resolve to the same task.

---

## Inspect

```bash
tlc task show T-0046                     # full detail
tlc task show T-0046 --format json       # structured output
tlc log T-0046                           # audit trail
tlc log --all --action CLAIMED --by exo  # filtered log
```

---

## State Transitions

```
TODO → IN_PROGRESS → DONE
     ↘              ↗
       SKIPPED ←────
```

```bash
tlc task claim   T-0046                              # TODO → IN_PROGRESS
tlc task unclaim T-0046                              # IN_PROGRESS → TODO
tlc task complete T-0046                             # IN_PROGRESS → DONE
tlc task complete T-0046 --no-verify                 # bypass state machine (NOT --force)
tlc task update   T-0046 --status IN_PROGRESS --note "reason"  # records on STATUS_CHANGED
tlc task update   T-0046 --priority P0 --note "reason"         # records on UPDATED
tlc task update   T-0046 --note "<sha> linkage"                # note alone; records on UPDATED
tlc task update   T-0046 --status IN_PROGRESS --force          # force any transition
tlc task reopen   T-0046 --note "reason"             # DONE/SKIPPED → TODO (--note required)
tlc task delete   T-0046 --yes --note "reason"       # --note required by default policy
```

Bypass flags (NOT interchangeable):

| Flag | Scope | Effect |
|------|-------|--------|
| `complete --no-verify` | complete only | skip state machine |
| `update --force` | update only | force status transition |
| `delete --yes` / `-y` | delete only | skip confirmation prompt |
| `--no-prompt` | batch ops | skip batch confirmation prompts |

---

## Update

```bash
# Read description first — -d REPLACES, does not append
tlc task show T-0046
tlc task update T-0046 -d "<existing content>\n\nSessions:\n- sess-xyz (exo)"

tlc task update T-0046 --title "New title"
tlc task update T-0046 --priority P0
tlc task update T-0046 --effort L
tlc task update T-0046 --assigned-to alice
tlc task update T-0046 --assigned-to -           # clear assignee
tlc task update T-0046 --add-tag refactor
tlc task update T-0046 --remove-tag feat
tlc task update T-0046 --add-blocked-by T-0041
tlc task update T-0046 --remove-blocked-by T-0041
tlc task update T-0046 --clear-blocked-by
tlc task update T-0046 --blocked "waiting on deploy"
tlc task update T-0046 --unblock
tlc task update T-0046 --timeout 4h
```

---

## Assign / Unassign

```bash
tlc task assign alice T-0046             # assignee FIRST, then task IDs
tlc task assign alice T-0041 T-0042      # batch assign
tlc task unassign T-0046 --note "reason" # --note required
```

### Assignee lookup order

1. Check `wsm workspace list --json` + `wsm workspace show <ws> --json`
2. Fallback: `aps profile list` + `aps profile show <profile>`
3. If still unclear: ask user — never guess

---

## Batch Operations

```bash
# Multiple explicit IDs
tlc task complete T-0041 T-0042 T-0043 --no-prompt

# Regex pattern
tlc task complete "T-00[12]\d" --no-prompt

# Glob all
tlc task claim "*" --no-prompt

# Batch update
tlc task update T-0041 T-0042 --priority P1 --no-prompt
tlc task delete T-0041 T-0042 --yes --no-prompt
```

---

## Stale Detection

```bash
tlc task stale                           # list stale tasks
tlc task stale --run-hooks               # fire hooks + record StaleFiredAt
tlc task list --stale                    # stale in standard list view
tlc config get task.stale.default_timeout
```

---

## Tracks

Tracks group tasks into work streams with lifecycle, state, and progress.

### Create & manage

```bash
tlc track create "Browser rendering" --type feature
tlc track create "Auth fix" --type bug --id auth-fix --assigned-to @me
tlc track list                                   # table: ID, Title, Type, Status, State, Progress
tlc track list --status active --type feature
tlc track list --state stale
tlc track list --all-projects                    # adds Project column
tlc track show browser-rendering                 # detail + phase breakdown
tlc track update browser-rendering --title "Browser Engine"
tlc track update browser-rendering --status completed   # validated
tlc track archive browser-rendering              # completed/abandoned → archived
tlc track abandon browser-rendering              # active → abandoned
tlc track delete browser-rendering               # fails if linked tasks
tlc track summary                                # project pulse: health + counts
```

### Link tasks to tracks

```bash
tlc task create "Parse HTML" --track browser-rendering
tlc task update T-0046 --track browser-rendering # link existing
tlc task update T-0046 --track -                 # unlink
tlc task list --track browser-rendering          # filter by track
```

Auto-transition: claiming a linked task transitions pending track → active.

### Plan linkage

```bash
tlc track update browser-rendering --add-plan docs/plans/rendering.md
```

Extracts tasks from plan frontmatter `tasks:` field. Falls back to configured
`tracks.plan_extractor` command. Links plan without tasks if neither yields
results.

### Track state (computed, never stored)

| Flag | Condition |
|------|-----------|
| `stale` | pending/active, 1+ tasks, max(task.updated_at) < now − threshold |
| `unlinked` | 0 linked tasks |
| `blocked` | all non-terminal tasks have unresolved blocked_by |
| `healthy` | none of the above |

### Config

```yaml
tracks:
  stale_threshold: 48h           # default
  health:
    max_active: 3                # soft cap before overcommit warning
    min_progress_to_start: 50    # avg progress % threshold
  plan_extractor: ""             # optional external command
```

---

## Flows (Orchestration)

```bash
tlc flow run flow.yaml                   # execute declarative flow
tlc flow run tlc://owner/repo/flow:id:1.0
tlc flow invoke flow.yaml                # generate + auto-assign tasks from template
tlc flow list                            # list runs
tlc flow status <run-id>                 # check run status
tlc flow import <uri>                    # import flow definition
```

---

## Sync

```bash
tlc sync status                          # push queue + sync state
tlc sync push                            # push to external system (GitHub etc.)
tlc sync pull                            # pull updates
```

---

## Output Formats

```bash
tlc task list --format json              # structured, machine-readable
tlc task list --format yaml
tlc task list --format table             # human table (default)
tlc task list --format tls               # compact TLS style
tlc task list --format summary           # status counts
```

---

## Task Completion Protocol

```bash
# 1. Read current description
tlc task show T-0046

# 2. Append session log (replaces; preserve existing content)
tlc task update T-0046 -d "<existing>\n\nSessions:\n- <session-id> (<agent>)"

# 3. Mark complete
tlc task complete T-0046
# or, if stuck in wrong state:
tlc task complete T-0046 --no-verify
```

---

## Error Handling

| Error | Cause | Fix |
|-------|-------|-----|
| `cannot transition from TODO to DONE` | skipped claim | `tlc task claim <id>` first |
| `--note is required` | reopen/unassign without note | add `--note "<reason>"` |
| `policy "delete-requires-note" denied: ...` (exit 4) | `tlc task delete` without `--note` | add `--note "<reason>"`; see [policies.md](policies.md) |
| `task delete requires confirmation` | missing --yes | add `--yes` |
| `task T-XXXX not found` | wrong ID or project | `tlc task list` to verify |
| `project not found in registry` | not initialised | `tlc init` in project root |
| DONE/SKIPPED immutable | terminal state | use `--force` or `reopen` first |
| `cannot delete track with linked tasks` | tasks reference this track | unlink first: `tlc task update <id> --track -` |
| `track type "X" invalid` | wrong --type value | use: feature, bug, refactor |

---

## Task Data Model (Key Fields)

| Field | Values | Notes |
|-------|--------|-------|
| `id` | `T-XXXX` | stable; auto-assigned |
| `title` | string | required |
| `status` | `TODO` `IN_PROGRESS` `DONE` `SKIPPED` | canonical uppercase; case-insensitive on input, accepts aliases (see below) |
| `priority` | `P0` `P1` `P2` `P3` | P0 = highest; case-insensitive on input, accepts aliases (see below) |
| `effort` | `XS` `S` `M` `L` `XL` | size estimate; case-insensitive on input, accepts aliases (see below) |
| `assigned_to` | profile/username | optional |
| `blocked_by` | `[]task-id` | dependency list |
| `blocked` | string | current blocker reason |
| `tags` | `[]string` | categories |
| `description` | string | freeform; replaced by `-d` |
| `created_at` | ISO 8601 | set on create |
| `updated_at` | ISO 8601 | updated on any change |
| `stale_timeout` | duration | per-task override; inherits project default |
| `track_id` | track slug | optional; links task to track |

### Accepted forms (status / priority / effort)

All three enum fields are case-insensitive and accept aliases on
**filter** (`task list`, since story 069), **create** (`task create`,
since story 082 / PR #114), and **update** (`task update`, since
story 082 / PR #114). Source of truth: `internal/cli/fieldnorm.go`.

| Field | Canonical | Accepts (case-insensitive) |
|-------|-----------|----------------------------|
| `status` | `TODO` | `todo`, `open`, `to-do` |
| `status` | `IN_PROGRESS` | `in_progress`, `wip`, `inprogress`, `in-progress` |
| `status` | `DONE` | `done`, `complete`, `completed`, `finish`, `finished` |
| `status` | `SKIPPED` | `skipped`, `skip` |
| `priority` | `P0` | `p0`, `0`, `critical` |
| `priority` | `P1` | `p1`, `1`, `high` |
| `priority` | `P2` | `p2`, `2`, `medium`, `med` |
| `priority` | `P3` | `p3`, `3`, `low` |
| `effort` | `XS` | `xs`, `extra-small`, `extrasmall`, `xsmall`, `tiny` |
| `effort` | `S` | `s`, `small` |
| `effort` | `M` | `m`, `medium`, `med` |
| `effort` | `L` | `l`, `large` |
| `effort` | `XL` | `xl`, `extra-large`, `extralarge`, `xlarge`, `huge` |

### Clear sentinels (update path)

To clear an optional field on `task update`, pass `""` or `-` (mirrors
the existing `--assigned-to -` pattern). Shipped in PR #114:

```bash
tlc task update T-0046 --assigned-to -   # clear assignee
tlc task update T-0046 --priority ""     # clear priority
tlc task update T-0046 --priority -      # clear priority
tlc task update T-0046 --effort ""       # clear effort
tlc task update T-0046 --effort -        # clear effort
tlc task update T-0046 --track -         # unlink from track
tlc task update T-0046 --rrule -         # clear recurring reminder
```
