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
tlc task create "title" --due tomorrow   # set due date
tlc task create "title" --due "in 3d" --remind-every 1h  # due + recurring
tlc task create "title" --due friday --no-auto-remind     # suppress 12h auto-remind
tlc task create --recipe <recipe> --var key=val           # recipe steps as tasks (trackless)
tlc task create --recipe <recipe> --for T-0041            # into T-0041's track, T-0041 blocked on the leaves
tlc task create --recipe <recipe> --var k=v --track <id> --task 1-3 --with-deps
tlc task update <id> --due "2025-05-01"  # set/change due date
tlc task update <id> --due -             # clear due date
tlc task update <id> --remind-at "2025-05-01T14:00:00Z"   # one-shot
tlc task update <id> --remind-every 30m  # recurring reminder
tlc task remind                          # show upcoming + overdue
tlc task remind --check                  # exit 1 if overdue (scriptable)
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
tlc track create --recipe <recipe> --var key=val # track + tasks from a recipe (title/type/plan from its track block)
tlc track create "Title" --recipe <recipe> --var k=v --task lint,review --assign
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

Track identifiers — `<id>` above accepts any of these:

```bash
tlc track show L-0002                                # display alias (per-project, stable)
tlc track show l-0002                                # aliases are case-insensitive
tlc track show browser-rendering                     # slug
tlc track show track_01h455vbqkfsn02nk084ksn02q      # durable TypeID
```

`L-NNNN` is the per-project alias, assigned at creation and fixed for the
life of the track. The slug is the readable name and the on-disk directory
under `.tlc/tracks/<slug>/`, which is why tracks keep both. `track_<typeid>`
is the durable identity: unique across projects and never reused, so prefer
it for references that cross a project boundary.

Tasks follow the same split: `T-NNNN` (or a bare number) is the per-project
alias, `task_<typeid>` the durable identity.

New slugs are capped at `tracks.slug_max_len` (default 24) and derived slugs
cut at a word boundary. The cap applies to slugs being created only —
existing longer slugs keep resolving and are never renamed.

Task-track integration:
```bash
tlc task create "..." --track <track-id>         # link on create
tlc task update <id> --track <track-id>          # link existing
tlc task update <id> --track -                   # unlink
tlc task list --track <track-id>                 # filter by track
```

Auto-transition: when a linked task is claimed (→ IN_PROGRESS), a pending
track auto-transitions to active.

### `tlc agent` — Agent Dispatch

Run tasks or tracks via named agent profiles.

```bash
# run an agent against a task or a track
tlc agent run --agent <name> --task <id>
tlc agent run --agent <name> --track <id>

# watch the agent queue; poll every 5s
tlc agent watch [--queue agent] [--interval 5s]

# inspect / cancel a running job
tlc agent status <job-id>
tlc agent cancel <job-id>

# list agent runs (audit records, default) or registered agents
tlc agent list [--status <s>] [--limit N] [--json]
tlc agent list --source config   # agents declared in agents.yaml
```

Shorthand exec (delegates to `agent run` internally):
```bash
tlc task execute <id> --agent <name>
tlc track execute <id> [--agent <name>]
```

Recipe-driven exec: `task execute` creates the recipe's tasks for the task
(the subject) and runs them; `track execute` reconciles the track against
the recipe's run ledger (creates only the missing steps) and then runs it.
```bash
tlc task execute <id> --recipe <recipe> [--var key=val]
tlc track execute <id> --recipe <recipe> [--recreate]
```

`--agent <name>` supplies the agent for agent-kind tasks that name none;
`track execute` also takes `--with-pod`, `--concurrency`, `--permissive`,
`--reclaim`, `--wait --poll`, `--timeout`, `--trust-project` and `--ctxt`.

### `tlc recipe` — Recipe Templates

```bash
tlc recipe list [--source <dir|builtin>]      # recipes in the search path
tlc recipe show <recipe>                      # header, vars, expanded steps
tlc recipe validate <recipe>                  # exit 0 valid, 1 with the problem
tlc recipe import <track|url> [--var --name --version -o --install --force --external-deps]
tlc recipe runs [<recipe>[@<version>]] [--all-projects --track <track>]
tlc recipe diff <track> [--recipe <recipe>]
```

A recipe is referenced by name, by `name@version`, or by file path. The
search path is `recipe.dir` (relative to the project root), then
`.tlc/recipes`, then `~/.config/tlc/recipes`.

Human-kind steps wait for a decision:
```bash
tlc task approve <id> [--by <who> --note "..."]
tlc task reject <id> --reason "<why>" [--by <who>]
```

See [`docs/recipes.md`](docs/recipes.md) for the guide and
[`docs/recipe-spec-0.1.md`](docs/recipe-spec-0.1.md) for the grammar.

### `tlc schema` — Toolspec Schema

Output the tool definition that AI agents use to interact with TLC.

```bash
tlc schema                        # default JSON output
tlc schema --format json          # explicit JSON
tlc schema --format mcp           # MCP tool definition
tlc schema --format openai        # OpenAI function-calling schema
tlc schema --format anthropic     # Anthropic tool-use schema
```

Supported formats: `json` · `mcp` · `openai` · `anthropic`

### `tlc recipe test` — Deterministic Recipe Testing

Materialize a recipe into a throwaway store and execute it inside a hermetic
sandbox with cassette-backed tool shims.

```bash
# replay every run the recipe has
tlc recipe test examples/recipes/exec-cli-smoke.yaml

# replay one named run
tlc recipe test examples/recipes/exec-cli-smoke.yaml happy-path

# record cassettes (proxy real tool calls; write to fixtures/)
tlc recipe test examples/recipes/exec-cli-smoke.yaml happy-path --record

# force live execution for named tools; replay everything else
tlc recipe test examples/recipes/exec-cli-smoke.yaml happy-path --passthrough wrangler,docker

# keep the sandbox dir after the run for debugging
tlc recipe test examples/recipes/exec-cli-smoke.yaml happy-path --keep-sandbox

# materialize only some steps: ordinals, ranges or ids
tlc recipe test examples/recipes/exec-cli-smoke.yaml happy-path --task 1-2
```

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | Every task ran and every contract passed |
| 1 | A task failed or a contract was violated |
| 2 | Replay found no cassette for a step |
| 3 | The sandbox, the recipe or the fixtures could not be set up |

Runs live under `<recipe dir>/fixtures/<recipe>/<run>/`. Each run has a
`record/` dir (cassettes, one subdirectory per step id), an optional
`contracts/` dir (eva contracts, `<step-id>.yaml`), and a `test.yaml`
manifest declaring `expected_exit`, `vars` and `passthrough`.

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
6. Every new cobra leaf MUST declare kit annotations or boot fails
   via `Root.Validate()`. Minimum:
   `Annotations: map[string]string{"kit/side-effect": "<read|write-local|write-shared|destructive-local|destructive-shared|interactive>", "kit/idempotent": "<yes|no|conditional>"}`,
   plus a `Long:` description and (for depth-1 leaves) `kit/top-level-verb: "true"`.
   Destructive leaves (`destructive-local`/`destructive-shared`) go
   through `installConfirmBridge` in `confirm_bridge_init.go` to honour
   `--confirm=yes` in non-TTY contexts. See
   [`docs/12fcc-conformance-split-plan.md`](docs/12fcc-conformance-split-plan.md)
   for the full contract; `TestStrictValidationPasses` guards it in CI.

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
- all top-level subcommands (`task`, `recipe`, `auth`, `config`, …)
- all `task` and `recipe` subcommands
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

## Build & Test

All builds go through the project `Makefile`. Two top-level workflows:

| Target | Mutating? | Steps | Use |
|--------|-----------|-------|-----|
| `make dev` | yes | fmt, vet, lint, tidy, test | Local developer workflow |
| `make check` | no | fmt-check, vet, lint, tidy-check, test | CI / pre-merge gate |

Always run `make check` before push — it mirrors CI exactly.

Additional targets:
```bash
make build          # compile binary + plugins → bin/
make test           # go test -race ./...
make lint           # golangci-lint
make fmt            # gofmt + goimports (mutating)
make vet            # go vet ./...
make tidy           # go mod tidy && go mod verify (mutating)
make fmt-check      # formatting check (non-mutating; exits 1 if dirty)
make tidy-check     # module tidiness check (non-mutating; exits 1 if dirty)
make coverage       # test with coverage report
make build-shims    # compile the recipe test shim binaries
```

## Configurable Paths

Directory paths for tracks, recipes, assignees, inbox, projection, todo
file, and database are all configurable in `.tlc/config.yaml`. Accessor
methods (`TracksDir()`, `RecipeDir()`, `AssigneesDirectory()`,
`InboxDir()`, `ProjectionDirectory()`, `TodoFilePath()`, `DBFilePath()`)
return the configured value or a default. Never hardcode directory names
like `"tracks"` or `"examples/assignees"` — always use the accessor method
from the relevant config struct. The recipe search path is assembled by
`recipeDirsFromConfig()` in `internal/cli/config_dirs.go` (`recipe.dir`,
then `.tlc/recipes`, then the user config dir); use `recipeLocator()`
rather than scanning directories by hand.

## Required Docs to Keep Updated

- `README.md`
- `CHANGELOG.md`
- `docs/stories/README.md`
- `docs/personas/*.md`
- `docs/global-flags.md` — persistent global flags (`--offline`, `--profile`, `--instance`)
