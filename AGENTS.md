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
tlc task show <id>                       # task details
tlc task claim <id>                      # claim + transitions status → IN_PROGRESS
tlc task complete <id>                   # mark done (transitions status → DONE)
tlc task complete <id> --note "..."      # mark done with note
tlc task unclaim <id>                    # release + transitions status → TODO
tlc task reopen <id> --note "..."        # reopen terminal task (--note required)
tlc task assign <id> <assignee>          # assign to someone else
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

## Required Docs to Keep Updated

- `README.md`
- `CHANGELOG.md`
- `docs/stories/README.md`
- `docs/personas/*.md`
