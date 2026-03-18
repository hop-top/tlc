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
tlc task list                            # list tasks: IN_PROGRESS first, then TODO (default)
tlc task list --limit 30                 # increase result limit
tlc task list --status DONE              # filter by a specific status
tlc task show <id>                       # task details
tlc task claim <id>                      # claim for work
tlc task update <id> --status done       # mark complete
tlc task unclaim <id>                    # release task
tlc task delete <id>                     # delete task
```

Conventions:
- NEVER prefix titles with "Task N:" — tlc auto-assigns IDs.
- Use `--tag` (not `--label`) for categorization.
- Use `--limit` on `list` when expecting >10 tasks.
- Run one `tlc` command at a time; avoid long `&&` chains
  (verbose logging can cause runaway output).
- NEVER make changes (plan, code, docs) without a claimed task.
  Claim before starting; mark DONE when finished.
- On task completion, append session log to task description:
  `tlc task update <id> -d "... \n\nSessions:\n- <session-id> (<agent>)"`

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

## Required Docs to Keep Updated

- `README.md`
- `CHANGELOG.md`
- `docs/stories/README.md`
- `docs/personas/*.md`
