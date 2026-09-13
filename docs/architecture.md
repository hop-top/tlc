# Architecture — tlc

Last updated: 2026-04-26
Author: $USER

## Purpose

`hop.top/tlc` is the track and task ledger for cross-project
work coordination. It manages tasks (granular work items) and
tracks (cohesive multi-task initiatives), with first-class
multi-agent assignment, dependency graphs, plan ingestion, and
bidirectional sync to GitHub / Jira / Linear.

## Context

tlc is the operational backbone of the hop-top ecosystem. Every
project keeps tasks in its own `.tlc/db.sqlite`; cross-project
coordination happens via reference syntax. Procedures that span
multiple agents are declared in recipe YAML, materialized into
tracks and tasks, and executed deterministically with
cassette-backed testing.

Cross-references:

- [kit](https://github.com/hop-top/kit/blob/main/docs/architecture.md) — heaviest consumer of kit (domain, bus, ext, cli, llm, tui, sqlstore)
- [hop](https://github.com/hop-top/hop/blob/main/docs/architecture.md) — orchestrator
- [aps](https://github.com/hop-top/aps/blob/main/docs/architecture.md) — assignee resolution (`<name>:<role>:<version>`)
- [wsm](https://github.com/hop-top/wsm/blob/main/docs/architecture.md) — workspace scoping
- [uhp](https://github.com/hop-top/uhp/blob/main/docs/architecture.md) — events propagation

## Containers

| Container | Path | Role |
|---|---|---|
| `tlc` CLI | `cmd/tlc/` | Cobra-based CLI |
| Plugins | `plugins/*/` | Per-feature pluggable binaries (compiled to `plugins/*/bin/`) |
| Recipe shims | `cmd/shims/` | Tool wrappers for deterministic recipe testing (claude, git, docker, npm, uv, pip, etc.) |
| SQLite DB | `.tlc/db.sqlite` (per project) | State + events; per-project; `~/.local/share/tlc/db.sqlite` for global registry |

## Components

### CLI subcommands (`internal/cli/`)

| Subcommand | Surface |
|---|---|
| `task` | CRUD, claim/unclaim, complete, assign, stale detection, scheduling, reminders, blocked-by relationships |
| `track` | Lifecycle, plan ingestion, cross-track refs, health computation |
| `recipe` | List, show, validate, import, diff; run ledger |
| `project` | Registry, init, import/export, workspace scoping |
| Other | `config`, `auth`, `sync`, `labels`, `assignee`, `agent`, `uri`, `prompt`, `tui`, `workspace`, `tag`, `inbox`, `version`, `doctor`, `upgrade` |

### Core entities (`internal/core/`)

**Task** (`models.go`):
- `id`, `title`, `description`, `status` (TODO / IN_PROGRESS / DONE / SKIPPED), `assignee`, `effort` (XS-XL), `priority` (P0-P3), `tags`, `created_at`, `updated_at`, `archived`
- `project_id`, `track_id`, `due_at`, `remind_at`, `remind_every`, `stale_timeout`, `blocked_reason`, `origin_system`, `last_sync_at`, `meta`

**Track** (`track.go`):
- `id`, `title`, `type` (feature / fix / bug / refactor / chore / ci / docs / style / perf / test / build), `status` (pending / active / completed / abandoned / archived), `assigned_to`, timestamps, `meta`

**Recipe** (`recipe.go`):
- `Recipe` (name, version, description, `requires.subject`, `vars`, `agent`, `track`, `steps`), `Step` (id, kind, title, description, `depends_on`, `when`, `assignee`, `due`, `retry`, `gate`, plus `include`/`with` and `repeat`/`until` blocks), `TaskKind` (agent / exec / human)

**RecipeRun** (`recipe_run.go`):
- `RecipeRun`: `id`, `recipe_id`, `version`, `hash`, `vars`, `subject_type`, `subject_id`, `track_id`, `selection`, `dropped_deps`, `parent_run`, `created_by`, `created_at`
- `RecipeRunTask`: `run_id`, `step_id`, `task_id`

**LogEntry** (`models.go`):
- task log: `timestamp`, `by`, `action` (incl. SYNC_*), `note`, `meta`

### Storage (`internal/storage/`)

SQLite migrations v1-v7+. Tables: `tasks`, `task_logs`,
`tracks`, `projects`, `recipe_runs`, `recipe_run_tasks`,
`task_sequences`. Indexes
on status, assigned_to, created_at, origin_system, archived,
project_id, track_id.

Repos: `TaskDomainRepo`, `TrackDomainRepo`, `RecipeRunStore`,
`AgentRunStore`, `JobStore`. Projection system in
`projector.go` for denormalisation.

### Auth (`internal/auth/`)

- GitHub OAuth, Jira OAuth, Linear token-based
- Keychain store (`zalando/go-keyring`) + file fallback

### Config (`internal/config/`)

Layered: env → `.tlc/config.yaml` → `~/.config/tlc/config.yaml`
→ defaults. XDG-compliant.

Configurable types: `ProjectConfig`, `WorkspaceConfig`,
`SpaceConfig`, `TrackConfig`, `TaskConfig`.

### Events (`internal/events/`)

kit/bus integration:

- `AuditSubscriber` persists domain events to `task_logs`
- `BusPublisher` adapter bridges domain events to kit/bus
- Topics for lifecycle state changes
- Recently connected to dpkms cross-process bus hub via
  env-based auth (`051d26f`, `ae206e4`)

### Extensions (`internal/extensions/`)

Pluggable sync providers via kit/ext.Manager:

- GitHub (`github.go`) — Issue ↔ Task
- Jira (`jira.go`) — Issue ↔ Task
- Linear (`linear.go`) — Issue ↔ Task
- VTODO / iCalendar (`vtodo-sync` plugin) — RFC 5545 VTODO ↔ Task (file mode v0.1; CalDAV deferred)

Conflict resolution modes: remote-wins, local-wins,
last-write-wins, manual.

### URI system (`internal/uri/`)

`tlc://` URI scheme:

- Task: `tlc://hop-top/tlc/T-0042`
- Track: `tlc://hop-top/tlc/track/<id>`

Cross-project ref normalisation: `<project-id>#<task-id>` and
`<track-id>#<n>`. Resolver supports both intra-project and
cross-project lookups.

### Recipes and the executor (`internal/core/recipe*`, `internal/core/executor*`, `internal/flowtest/`)

Two halves, in order. **Materialize** turns a recipe into tasks;
**execute** runs the tasks that exist. Nothing runs during
materialization, and the executor never reads a recipe file.

**Materialize** — `recipe_parser.go` parses the YAML,
`recipe_validate.go` checks it, `recipe_expand.go` splices
`include` blocks and unrolls `repeat` blocks into a flat step
list, `recipe_template.go` + `recipe_render.go` bind vars,
`subject.*`, `run.*` and `steps.*` placeholders, `recipe_select.go`
applies `--task` / `--with-deps`, and `recipe_materialize.go`
creates the tasks and writes the run to the ledger. Recipe files
are found by `recipe_locator.go` over the three-layer search path.
`recipe_reconcile.go` compares a track against the ledger so
`tlc track execute --recipe` creates only the steps with no ledger
row. `recipe_capture.go` and `recipe_importer.go` run the inverse:
a track, or a markdown procedure, back out into a recipe file.

**Execute** — `Executor` (`executor.go`) loops until nothing is
ready. `executor_ready.go` recomputes readiness from `blocked_by`
and evaluates each ready task's `when` against upstream
`results.*`; `executor_dispatch.go` claims the task
(`TODO` → `IN_PROGRESS`, stamping the actor and `claimed_at`),
hands it to the dispatcher registered for its kind, and applies
the outcome — done once the eva gate passes, retried up to
`retry.max_attempts` with `retry.backoff`, or blocked with the
failure as its reason once `attempts` is exhausted.
`executor_human.go` applies `human.timeout` / `on_timeout` lazily,
and `executor_subject.go` completes the run's subject once its
leaves are done. Task provenance (`run_id`, `step_id`,
`step_ordinal`) is what tells the executor a task belongs to a
run; `--permissive` lifts that restriction.

#### Task kinds

Dispatchers are registered per kind in `internal/cli/track_execute.go`:

| Kind | Dispatcher | What it does |
|---|---|---|
| `agent` | `agentDispatcher` | Dispatches an LLM agent subprocess (claude, codex, ...) to execute the task per `task-exec-spec-0.1`. The agent for the task is the one named on the step, else the recipe's `agent`, else `--agent`. Output is the agent's structured task result. |
| `exec` | `ExecDispatcher` (`exec_dispatcher.go`, `exec_runner.go`) | Runs a literal command (`exec.argv`) via `os/exec`, honouring `exec.cwd`, `exec.env`, `exec.timeout` and `exec.stdout_max`. Captures `{exit_code, stdout, stderr, duration_ms, truncated}` as the task's result. **No LLM dispatch** — for deterministic CLI checks (smoke tests, lints, regex/exit-code gates). |
| `human` | (never dispatched) | Waits for `tlc task approve` / `tlc task reject`. When only human tasks remain ready, the command lists them and exits 0, or keeps polling with `--wait --poll`. |

With `--with-pod`, agent tasks each run in their own container up
to `--concurrency`, and exec tasks run in a fresh pod too: the
working tree is copied to `/workspace`, `exec.env` is set at
creation and `exec.cwd` resolves under `/workspace`.

The `exec` kind lets a recipe express deterministic checks (CLI
smoke, branch-name regex, release-please file protection, etc.)
without paying the LLM tax for work that doesn't need judgement.
Cassettes still apply via the existing xrr catchall shim, so replay
remains hermetic.

#### Contract evaluation (eva integration)

A step declares a gate with `gate.contract` and an optional
`gate.eva_url`. After the task completes, `internal/flowtest/hook.go`
POSTs its output to the eva gateway at
`{EVA_URL}/v1/contract/invoke`; the task only reaches DONE once the
contract passes.

**`EVA_URL` is required.** Without it, contract evaluation is
**silently skipped** — the contract is found, parsed, and then
dropped on the floor. There is no warning logged. Set `EVA_URL`
(and `EVA_KEY` if your gateway requires auth) in the environment if
you want contracts to actually be checked.

For CI use without standing up the full gateway, the standalone
`eva run --contract foo.yaml --input data.json` CLI is the intended
escape hatch (tracked in `hop-top/eva` T-0258 — not yet shipped).

### Plan management (`internal/core/plan_*`)

`PlanParser` extracts tasks from markdown frontmatter:

```yaml
---
title: "Track Name"
tracks: []
tasks:
  - title: "Task Title"
    description: "..."
    tags: [tag1, tag2]
    effort: "M"
    priority: "P1"
    assigned-to: "name:role:version"
    blocked-by: [0, "T-0001", "other-track#2", "hop-top/c12n#T-0018"]
---
```

`BlockedByRef` variants:

- 0-based intra-track index
- `"T-NNNN"` — concrete task
- `"<track-id>#N"` — cross-track (1-based)
- `"<org/project>#<T-NNNN>"` — cross-project

`PlanExtractor` + `PlanReconcile` make plan ingestion
idempotent. `PlanRewrite` updates refs back to plan.md on disk.

### Natural language (`internal/cli/prompt_*`)

Confidence-gated classifier:

- LLM escalation for low-confidence prompts
- Context builder for enriched task prompts (deps, audit log)
- Tokenizer + fuzzy matcher + vocab

### TUI (`internal/tui/`, `internal/cli/tui.go`)

Bubble Tea v2 dashboard:

- Kanban board view
- Runs view over the recipe run ledger
- 250+ iTerm2 community themes
- Keys: j/k navigate, enter view, n create, c/u claim/unclaim, s cycle status, / search, v cycle views

### Workspace (`internal/workspace/`)

- `WorkspaceConfig` with `wsm_id`
- `SpaceConfig` with URIs and adapters
- Registry for cross-project task queries

## Public surfaces

### CLI

```
tlc task {create\|list\|show\|update\|delete\|claim\|unclaim\|complete\|assign\|unassign\|stale\|remind\|exec\|prompt}
tlc track {create\|list\|show\|update\|archive\|exec\|summary}
tlc recipe {list\|show\|validate\|import\|runs\|diff}
tlc project {init\|list\|import\|export}
tlc {config\|auth\|sync\|init\|tui\|uri\|schema\|version\|doctor\|upgrade}
```

### SQLite schema

```
tasks (project_id, id) [title, description, status, assigned_to,
       tags, effort, priority, created_at, updated_at, archived,
       track_id, due_at, remind_at, remind_every, stale_timeout,
       blocked_reason, origin_system, last_sync_at, meta]
task_logs (project_id, task_id) [timestamp, by, action, note, meta]
tracks (project_id, id) [title, type, status, assigned_to,
        timestamps, meta]
projects (project_id PK) [db_path, space_uri, label, registered_at,
                          last_seen_at, status]
recipe_runs (id PK) [project_id, recipe_id, version, hash, vars,
             subject_type, subject_id, track_id, selection,
             dropped_deps, parent_run, created_by, created_at]
recipe_run_tasks (run_id, step_id PK) [task_id]
task_sequences (project_id PK) [next_id]
```

### Config schema (`config.yaml`)

```yaml
project:
  id: "hop-top/tlc"
  fallback_mode: "auto\|detected\|prompt"
  duplicate_id_strategy: "share\|unique\|prompt"
task:
  todo_file: "todo.txt"
  projection_dir: "tasks"
  stale: { default_timeout: "6h", hooks: [...] }
  scheduling:
    by_priority:
      P0: { due: 24h, remind_every: 2h }
tracks:
  dir: "tracks"
  stale_threshold: "48h"
  health: { max_active: 3, min_progress_to_start: 50 }
recipe:
  dir: "recipes"
  assignees_dir: "examples/assignees"
storage:
  db_path: "db.sqlite"
  inbox: { dir: "inbox" }
```

### URI scheme

`tlc://` URIs identify any task / track:

- Task: `tlc://hop-top/tlc/T-0042`
- Track: `tlc://hop-top/tlc/track/<id>`

Cross-project blocked-by uses `#` separator: `<org/project>#<T-NNNN>`.

### Sync meta fields (extensions to `meta`)

| Field | Purpose |
|---|---|
| `origin_system` | `"github" \| "jira" \| "linear" \| ...` |
| `origin_id` | Native issue ID |
| `origin_url` | Native issue URL |
| `last_sync_at` | RFC3339 timestamp |
| `sync_direction` | `"to_remote" \| "to_local" \| "bidirectional"` |

## Integrations

| Integration | Where | How |
|---|---|---|
| kit (domain, bus, ext, cli, llm, tui, sqlstore) | `go.mod` | Heaviest dependency |
| Sync adapters | `internal/extensions/` | GitHub / Jira / Linear bidirectional |
| Git (project detection) | implicit | Project `id` derived from git remote |
| Agent assignment | `core/assignee_loader.go`, `core/assignment_engine.go` | Capability-based via kit/ext; multi-agent step dispatch |
| Workspace | `internal/workspace/` | wsm_id field; Space URIs for cross-project refs |
| Bus events | kit/bus | Domain events → AuditSubscriber → task_logs |

## Build / test / release

### Makefile

| Target | Runs |
|---|---|
| `make build` | tlc + plugins → `bin/` |
| `make build-shims` | Flowtest shims (claude, git, docker, npm, uv, pip, ...) |
| `make test` | All tests with race detector |
| `make lint` | golangci-lint |
| `make fmt` | gofmt + goimports |
| `make dev` | fmt + vet + lint + tidy + test (pre-commit) |
| `make check` | fmt-check + vet + lint + tidy-check + test (CI gate) |
| `make coverage` | Coverage report |
| `make watch(-lint\|-test)` | Air-based file watcher |

### CI (`.github/workflows/`)

`build.yml` (delegates to kit's build workflow), `coverage.yml`,
`lint.yml`, `release.yml`.

## Architecture decisions

Per-feature specs at `docs/`:

- [task-crud-spec-0.1](task-crud-spec-0.1.md) — CRUD lifecycle, soft delete, claim/unclaim/complete
- [task-line-spec-0.1](task-line-spec-0.1.md) — TLS format `[status] <ID> <Title> @assignee #tag prio:<P>`
- [recipe-spec-0.1](recipe-spec-0.1.md) — Recipe YAML grammar, step kinds, includes, loops
- [task-log-spec-0.1](task-log-spec-0.1.md) — Audit log actions and schema
- [sync-architecture-0.1](sync-architecture-0.1.md) — Bidirectional sync, conflict resolution, meta fields
- [tlc-tui-spec-0.1](tlc-tui-spec-0.1.md) — TUI keybindings and views
- [tlc-config-spec-0.1](tlc-config-spec-0.1.md) — Configuration hierarchy
- [task-exec-spec-0.1](task-exec-spec-0.1.md) — Deterministic execution (stdout/stderr capture, timeouts)
- [vtodo-sync-spec-0.1](vtodo-sync-spec-0.1.md) — RFC 5545 iCalendar VTODO export, RRULE recurrence, sync plugin

Plans: [track-registry-design](plans/2026-04-03-track-registry-design.md), [task-prompt-design](plans/2026-04-04-task-prompt-design.md).

## Evolution

### Recent significant changes

- Bus env-based auth (`051d26f`) — replaces hardcoded dev token
- Connect to dpkms cross-process bus hub (`ae206e4`)
- kit/bus typed topics + domain payload structs (`336caac`)
- Due dates / reminders / recurrence / auto-scheduling (`25efd73`) — v0.3 feature
- Short refs for same-project blocked-by (`50ca32b`, T-0632) — fixes slash-split bug
- Cross-project blocked-by in plan parser (`802abb3`, T-0631)
- `--add-plan` idempotent with plan_mapping reconciliation (`24e0887`)
- Dependency graph + execution strategy for tracks (`802abb3`)

### Known issues (open)

- `T-0717` — cross-project blocked-by refs reject multi-segment project IDs (split-on-first-slash bug)
- `T-0719` — `tlc project init` produces literal `'unknown'` project_id when no git remote configured

### Troubleshooting

**My step's gate is not being checked / I changed the contract
and nothing changed.**
`EVA_URL` is probably not set. The hook in `internal/flowtest/hook.go`
silently passes when `EVA_URL` is empty, even if the step declares a
`gate.contract`. Confirm with `echo $EVA_URL`. Set it (and
`EVA_KEY` if needed), then re-run. For CI where you don't want to
stand up the gateway, use the standalone `eva run` CLI
(`hop-top/eva` T-0258) once it ships.

**My `step_status` evaluator is rejected by eva.**
`step_status` was an aspirational evaluator that never landed in eva.
Existing fixtures were rewritten 2026-04-27 to use `contains`
against the joined step output. Use the same pattern, or wait for
the `status_code` evaluator in `hop-top/eva` to ship.

### Known limits

- Task ID sequences per-project; no global counter
- `origin_system` field determines sync eligibility
- Plan extraction one-way by default; rewrite requires explicit `--add-plan`
- Recipe test cassettes project-scoped; no cross-project execution
- Workspace adapter pluggable but default is filesystem-based
- TUI doesn't support multi-project views

## Open questions

1. **Multi-segment project IDs** — `T-0717` documented; resolver currently splits on first `/`. Fix needed for `IdeaCraftersLabs/ocs-aaarrr-lineup` style IDs.
2. **Stale handling defaults** — per-priority schedules in config; what's the right default cadence for P0/P1/P2/P3?
3. **Plan rewrite safety** — `PlanRewrite` updates plan.md on disk; round-trip semantics under concurrent edits?
4. **Cross-project recipes** — test cassettes are project-scoped; a recipe spanning projects needs either a flat namespace or a cross-DB resolver.
5. **Bus topic naming** — informally `<tool>.<entity>.<action>`; formalise as kit/bus contract or stay convention-only?
