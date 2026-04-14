## gstack

- Use `/browse` skill from gstack for ALL web browsing
- NEVER use `mcp__claude-in-chrome__*` tools directly

Available gstack skills:
- `/plan-ceo-review` — CEO-perspective plan review
- `/plan-eng-review` — engineering plan review
- `/plan-design-review` — design plan review
- `/design-consultation` — design consultation
- `/review` — general review
- `/ship` — ship/deploy workflow
- `/browse` — web browsing (use this instead of direct browser tools)
- `/qa` — QA workflow
- `/qa-only` — QA without design review
- `/qa-design-review` — QA + design review
- `/setup-browser-cookies` — configure browser session cookies
- `/retro` — retrospective
- `/document-release` — release documentation

## Required Tools

### `tlc` — Task Management

NEVER use built-in TaskCreate/TaskUpdate/TaskGet/TaskList. Use `tlc`
exclusively.

```bash
tlc task create "title" --tag feat       # create task (tlc assigns ID)
tlc task create "title" --blocked-by T-0041 --blocked-by other-project/T-0007
tlc task list                            # list tasks: IN_PROGRESS first, then TODO (default)
tlc task list --limit 30                 # increase result limit
tlc task list --status DONE              # filter by a specific status
tlc task list --priority P0              # filter by priority (P0, P1, P2, P3; repeatable)
tlc task list --blocked-by T-0041        # show tasks blocked by T-0041 (repeatable)
tlc task show <id>                       # task details
tlc task claim <id>                      # claim + transitions status → IN_PROGRESS
tlc task complete <id>                   # mark done (transitions status → DONE)
tlc task complete <id> --note "..."      # mark done with note
tlc task unclaim <id>                    # release + transitions status → TODO
tlc task reopen <id> --note "..."        # reopen terminal task (--note required)
tlc task assign <assignee> <id|pattern>... # assign to someone (assignee first)
tlc task unassign <id> --note "..."      # remove assignee (--note required)
tlc task update <id> --title "..."       # update title
tlc task update <id> -d "..."            # replace description (reads first; -d overwrites)
tlc task update <id> --assigned-to -     # clear assignee (use "-" or "null")
tlc task update <id> --add-blocked-by T-0041
tlc task update <id> --remove-blocked-by T-0041
tlc task update <id> --clear-blocked-by
tlc task update <id> --status IN_PROGRESS --force  # force status (bypass state machine)
tlc task delete <id>                     # delete task
```

Valid statuses (uppercase): `TODO` · `IN_PROGRESS` · `DONE` · `SKIPPED`

State machine (default):
- `TODO` → `IN_PROGRESS`, `SKIPPED`
- `IN_PROGRESS` → `DONE`, `TODO`, `SKIPPED`
- `DONE` / `SKIPPED` → terminal (immutable unless `--force`)

Conventions:
- NEVER prefix titles with "Task N:" — tlc auto-assigns IDs.
- Use `--tag` (not `--label`) for categorization.
- Use `--limit` on `list` when expecting >10 tasks.
- Run one `tlc` command at a time; avoid long `&&` chains
  (verbose logging can cause runaway output).
- NEVER make changes (plan, code, docs) without a claimed task.
  Claim before starting; complete when finished.
- `update -d` REPLACES description — read current value first, then rewrite.
- On task completion, append session log then complete:
  1. `tlc task show <id>` — read current description
  2. `tlc task update <id> -d "<existing>\n\nSessions:\n- <session-id> (<agent>)"`
  3. `tlc task complete <id>`

### `tlc track` — Track Management

```bash
tlc track create "Title" --type feature         # create track (type required)
tlc track create "Title" --type bug --id slug    # custom ID
tlc track list                                   # list with progress/state
tlc track list --status active --type feature    # filtered
tlc track show <id>                              # detail + phase breakdown
tlc track update <id> --title "..."              # update fields
tlc track update <id> --status completed         # transition (validated)
tlc track update <id> --add-plan plan.md         # link plan + extract tasks
tlc track archive <id>                           # completed/abandoned → archived
tlc track abandon <id>                           # active → abandoned
tlc track delete <id>                            # fails if linked tasks
tlc track summary                                # project health pulse
```

Track statuses: `pending` · `active` · `completed` · `abandoned` · `archived`
Types: `feature` · `bug` · `refactor`

Task-track integration:
```bash
tlc task create "..." --track <track-id>         # link on create
tlc task update <id> --track <track-id>          # link existing
tlc task update <id> --track -                   # unlink
tlc task list --track <track-id>                 # filter by track
```

Auto-transition: when a linked task is claimed (→ IN_PROGRESS), a pending
track auto-transitions to active.

### `tlc flow test` — Deterministic Flow Testing

Run a flow definition through a hermetic sandbox with cassette-backed tool shims.

```bash
# replay all named runs for a flow
tlc flow test examples/flows/pr-review-loop.yaml

# replay one named run
tlc flow test examples/flows/pr-review-loop.yaml happy-path

# record cassettes (proxy real tool calls; write to fixtures/)
tlc flow test examples/flows/pr-review-loop.yaml happy-path --record

# force live execution for named tools; replay everything else
tlc flow test examples/flows/pr-review-loop.yaml happy-path --passthrough wrangler,docker

# keep sandbox dir after run for debugging
tlc flow test examples/flows/pr-review-loop.yaml happy-path --keep-sandbox
```

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | All steps executed; all contracts passed |
| 1 | Step failed or contract violated |
| 2 | Cassette miss in replay mode |
| 3 | Sandbox setup failure |

Named runs live under `examples/flows/fixtures/<flow-name>/<run-name>/`. Each run
has a `record/` dir (cassettes) and optional `contracts/` dir (eva contracts).
Add a `test.yaml` manifest to declare `expected_exit` or `passthrough` overrides.

### `git hop` — Worktree Management

NEVER run `git worktree`, `git branch`, or `git checkout -b` directly.
All branch/worktree ops go through `git hop`.

```bash
git hop add <branch>                     # create worktree + branch
git hop list                             # list worktrees
git hop remove <branch>                  # remove worktree (interactive confirm)
# NOTE: --force does NOT skip the prompt. Pipe `echo "y" |` if needed.
git hop status                           # show working tree status
git hop prune                            # clean orphans
```

Refuse to work in a non-topic branch. Branch naming:
`{category}/{id}-{short-description}` (e.g. `feat/001-session-lifecycle`).

### `xray` — Code Discovery

NEVER blindly grep/glob large portions of the codebase. Use `xray`
first to understand structure before diving into files.

```bash
xray scan                                # scan + index codebase
xray map                                 # visual codebase map
xray explore                             # interactive exploration
xray explore -s "keyword"                # search code for keyword
xray explore -m "*.go"                   # find files by pattern
xray graph                               # CFG/DFG analysis
```

Read `.xray` files when present in directories for cached context.

## Project Goals

1. Build `tlc` as a deterministic task management system for interactive
   terminal applications (PTY/tmux abstraction).
2. CLI-first; SDK layer for programmatic Go integration.
3. Story-driven delivery via `docs/stories/` and `docs/personas/`.

## Story and Plan Workflow

1. `docs/stories/README.md` is source of truth for requirements.
2. Keep plans in `docs/plans/` in sync with implementation.
3. Behavior changes must update matching story acceptance criteria.

## Coding Expectations

1. Go 1.24.x; module path `github.com/hop-top/tlc`.
2. Keep files <500 LOC; split/refactor as needed.
3. DRY; design patterns; avoid cyclomatic complexity.
4. Deterministic tests; explicit exit-code assertions for CLI.
5. No sleep-based automation in tests or runtime.

## Error Philosophy

All error messages in this codebase must follow the **agent-first actionable** pattern:

- **Name the subject**: include the task ID, project ID, or resource that caused the error.
- **State what went wrong**: one clause, no jargon.
- **Provide next step**: tell the agent exactly what command to run or flag to add.
- **Degrade gracefully**: if a Hop product (wsm, aps, …) is absent, omit its hint — never fail
  because a sibling tool isn't installed.

### Pattern

```
<what failed>: <why>; <next step with exact command>
```

### Examples

| Bad | Good |
|-----|------|
| `task not found` | `task T-0042 not found; run 'tlc task list' to see available tasks` |
| `project not found in registry` | `project "hop-top/tlc" not found; run 'tlc init' in the project root` |
| `invalid transition` | `cannot transition from TODO to DONE: … valid: [IN_PROGRESS SKIPPED]; use --force to bypass` |
| `--note is required` | `--note is required; re-run with: tlc task reopen T-0042 --note "<reason>"` |
| `task delete requires --yes` | `task delete requires confirmation; re-run with: tlc task delete T-0042 --yes` |

### Implementation anchors

- Sentinel error types: `uri.ErrTaskNotFound`, `uri.ErrProjectNotFound`
- Transition error: `core.ErrInvalidTransition` (includes `Allowed []string` field)
- CLI helpers: `internal/cli/errors.go` — `errNoteRequired`, `errDeleteRequiresYes`, …

## Doc Command Validation

CLI examples in docs (AGENTS.md, README.md, etc.) must stay in sync with
the live `tlc` CLI registry. Stale examples mislead agents and break automation.

### How it works

`scripts/validate-doc-commands.sh` probes every documented subcommand and flag
using `--help` mode. Exits non-zero if any check fails.

Covers (46 checks as of 2026-03-28):
- all top-level subcommands (`task`, `flow`, `auth`, `config`, …)
- all `task` and `flow` subcommands
- key flags: `--no-verify`, `--yes`, `--mine`, `--counters`, `--force`, etc.

### Running locally

```bash
just validate-docs               # via justfile recipe
# or directly:
./scripts/validate-doc-commands.sh
```

### Keeping examples in sync

1. After adding/removing CLI commands or flags, run `just validate-docs`.
2. If a check fails, update the doc example to match the current CLI surface.
3. Do NOT edit the script to skip a failing check — fix the doc or the CLI.
4. Add new checks to the script when new subcommands or flags are introduced.

### CI integration

Add to CI pipeline: `just validate-docs`

## Required Docs to Keep Updated

- `README.md`
- `CHANGELOG.md`
- `docs/stories/README.md`
- `docs/personas/*.md`
