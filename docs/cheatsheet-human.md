# tlc Cheatsheet — Task Management (Human)

Quick reference for daily task work. Scannable in 30 seconds.

---

## Setup

```bash
tlc init                             # initialise tlc in current dir
tlc doctor                           # check env + config health
tlc version                          # verify install
```

Config: `~/.config/tlc/config.yaml` (or project `.tlc/config.yaml`)

### Multi-project targeting

```bash
tlc -c /path/to/.tlc/config.yaml task list   # target another project
tlc project list                             # list registered projects
```

---

## Create

```bash
tlc task create "Fix auth bug"
tlc task create "Title" --tag feat --priority P1 --assigned-to alice
tlc task create "Title" --blocked-by T-0041 --blocked-by other-project/T-0007
tlc task create "Title" --effort M --description "Details here"
tlc task create "Title" --status IN_PROGRESS   # non-default initial status
tlc task create -i                             # interactive prompt mode
```

| Flag | Values | Purpose |
|------|--------|---------|
| `--tag` | any string (repeatable) | categorise |
| `--priority` | P0, P1, P2, P3 | importance |
| `--effort` | XS, S, M, L, XL | size estimate |
| `--blocked-by` | task ID (repeatable) | dependency |
| `--assigned-to` | profile/username | ownership |
| `--timeout` | `2h`, `30m` | stale timeout |
| `--due` | `tomorrow`, `in 3d`, ISO | deadline |
| `--remind-at` | datetime | one-shot reminder |
| `--remind-every` | `1h`, `30m` | recurring reminder |
| `--no-auto-remind` | flag | suppress 12h auto-remind |

---

## Reminders

```bash
tlc task remind                          # upcoming + overdue
tlc task remind --check                  # exit 1 if overdue (scripting)
```

---

## List & Filter

```bash
tlc task list                            # IN_PROGRESS first, then TODO
tlc task list --status TODO              # single status filter
tlc task list --status IN_PROGRESS --status TODO   # multiple statuses
tlc task list --priority P0 --priority P1          # multiple priorities
tlc task list --blocked                  # only tasks with a blocker set
tlc task list --blocked-by T-0041        # tasks blocked by specific task
tlc task list --stale                    # tasks past stale timeout
tlc task list --mine                     # assigned to current user
tlc task list --assigned-to alice        # assigned to someone specific
tlc task list --tag feat                 # by tag
tlc task list --limit 30                 # override default (100)
tlc task list --counters                 # flat status counts only
tlc task list --summary                  # status summary view
tlc task list --all-projects             # across all registered projects
```

---

## Inspect

```bash
tlc task show T-0046                     # full task detail
tlc task show 46                         # ID flexible: 46, 0046, T-0046, @T-0046
tlc task show T-0046 --format json
tlc log T-0046                           # audit log for task
tlc log --all                            # logs across all tasks
tlc log --all --action CLAIMED           # filter by action
tlc log --all --by alice                 # filter by actor
```

---

## Claim / Unclaim

```bash
tlc task claim T-0046                    # claim → IN_PROGRESS
tlc task claim T-0046 --note "picking this up"
tlc task unclaim T-0046                  # release → TODO
```

---

## Update

```bash
tlc task update T-0046 --title "New title"
tlc task update T-0046 -d "Full replacement description"   # replaces, read first
tlc task update T-0046 --priority P0
tlc task update T-0046 --effort L
tlc task update T-0046 --assigned-to alice
tlc task update T-0046 --assigned-to -           # clear assignee
tlc task update T-0046 --add-tag refactor
tlc task update T-0046 --remove-tag feat
tlc task update T-0046 --add-blocked-by T-0041
tlc task update T-0046 --remove-blocked-by T-0041
tlc task update T-0046 --clear-blocked-by
tlc task update T-0046 --blocked "waiting on deploy"   # set blocker reason
tlc task update T-0046 --unblock                       # clear blocker reason
tlc task update T-0046 --status IN_PROGRESS --force    # bypass state machine
tlc task update T-0046 --timeout 4h                    # per-task stale timeout
```

---

## Complete / Reopen / Delete

```bash
tlc task complete T-0046
tlc task complete T-0046 --note "shipped in v1.2"
tlc task complete T-0046 --no-verify        # bypass state machine (NOT --force)

tlc task reopen T-0046 --note "reason"      # --note required
tlc task delete T-0046 --yes --note "reason"  # --note required (default policy); -y skips prompt
```

---

## Assign / Unassign

```bash
tlc task assign alice T-0046               # assignee FIRST, then IDs
tlc task assign alice T-0041 T-0042        # batch assign
tlc task unassign T-0046 --note "reason"   # --note required
```

---

## Batch Operations

```bash
# Multiple explicit IDs
tlc task complete T-0041 T-0042 T-0043

# Regex pattern
tlc task complete "T-00[12]\d" --no-prompt

# Glob all
tlc task claim "*" --no-prompt

# Batch update
tlc task update T-0041 T-0042 --priority P1 --no-prompt
tlc task delete T-0041 T-0042 --yes --no-prompt --note "obsolete after milestone close"
```

---

## Tags

```bash
tlc tag list                             # all unique tags in project
tlc tag T-0046 feat refactor             # add tags to task
tlc tag feat                             # filter: tasks tagged feat
tlc tag feat bug                         # AND: tagged feat AND bug
tlc tag feat,bug                         # OR:  tagged feat OR bug
tlc tag feat --all-statuses              # include DONE/SKIPPED too
```

---

## Stale Tasks

```bash
tlc task stale                           # list tasks past stale timeout
tlc task stale --run-hooks               # fire stale hooks + record StaleFiredAt
tlc task list --stale                    # same but in standard list view
```

---

## Tracks

Group related tasks into work streams with lifecycle and health tracking.

```bash
# Create tracks
tlc track create "Browser rendering" --type feature
tlc track create "Auth rewrite" --type refactor --id auth-rewrite

# List and inspect
tlc track list                                   # table with progress + state
tlc track list --status active
tlc track show browser-rendering                 # phase breakdown + task list
tlc track summary                                # project health pulse

# Link tasks
tlc task create "Parse HTML" --track browser-rendering --tag phase:1
tlc task update T-0046 --track browser-rendering # link existing task
tlc task update T-0046 --track -                 # unlink task
tlc task list --track browser-rendering          # tasks in this track

# Lifecycle
tlc track update browser-rendering --status completed   # all tasks must be done
tlc track archive browser-rendering
tlc track abandon browser-rendering
tlc track delete browser-rendering               # fails if tasks linked

# Plan integration
tlc track update browser-rendering --add-plan docs/plans/rendering.md
```

Track statuses: `pending` · `active` · `completed` · `abandoned` · `archived`
Types: `feature` · `bug` · `refactor`

Config (`tracks:` section in `.tlc/config.yaml`):
- `stale_threshold: 48h` — when tracks are considered stale
- `health.max_active: 3` — soft cap before overcommit warning
- `health.min_progress_to_start: 50` — avg progress threshold

---

## Recipes

```bash
tlc recipe list                          # recipes on the search path
tlc recipe show <recipe>                 # header, vars, expanded steps
tlc recipe validate <recipe>             # exit 0 valid, 1 with the problem
tlc recipe import <track>                # capture a track as a recipe file
tlc recipe runs [<recipe>]               # the run ledger
tlc recipe diff <track>                  # steps vs. ledger vs. tasks
```

Create tasks from one, then run them:

```bash
tlc track create --recipe <r> --var pr=42        # new track + its tasks
tlc task create --recipe <r> --var pr=42 --track <id>
tlc track execute <track> --agent claude         # run the ready tasks
tlc track execute <track> --recipe <r>           # reconcile, then run
tlc task approve <id> --note "looks good"        # decide a human step
tlc task reject <id> --reason "scope creep"
```

---

## Sync (GitHub etc.)

```bash
tlc sync status                          # push queue + sync state
tlc sync push                            # push local changes to external system
tlc sync pull                            # pull updates from external system
tlc sync config                          # configure sync settings
```

---

## Config

```bash
tlc config list                          # all config values
tlc config get task.stale.default_timeout
tlc config set task.stale.default_timeout 24h
tlc config validate                      # validate current config
```

---

## Output Formats

```bash
tlc task show T-0046 --format json
tlc task show T-0046 --format yaml
tlc task list --format table             # default
tlc task list --format tls               # TLS-style compact
tlc task list --format summary           # status summary
```

---

## URI Scheme

```bash
tlc uri register                         # register tlc:// with OS
tlc uri snippet                          # print OS-specific config snippet
```

---

## Common Tips and Failure Modes

| Symptom | Fix |
|---------|-----|
| `cannot transition from TODO to DONE` | use `claim` first; or `--force` to bypass |
| `--note is required` | add `--note "<reason>"` to reopen/unassign |
| `task delete requires confirmation` | add `--yes` |
| Wrong project targeted | check `tlc config list`; use `-c <config>` |
| Stale tasks not showing | check `task.stale.default_timeout` in config |
| `-d` wiped description | read with `tlc task show` first; `-d` replaces |
| Batch command stalled on prompt | add `--no-prompt` to skip confirmations |
| Track delete fails | Unlink tasks first: `tlc task update <id> --track -` |
| Can't complete track | All linked tasks must be DONE or SKIPPED |
