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
coordination happens via reference syntax. Workflows that span
multiple agents are defined in `flow` YAML and executed
deterministically with cassette-backed testing.

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
| Flow shims | `cmd/shims/` | Tool wrappers for deterministic flow execution (claude, git, docker, npm, uv, pip, etc.) |
| SQLite DB | `.tlc/db.sqlite` (per project) | State + events; per-project; `~/.local/share/tlc/db.sqlite` for global registry |

## Components

### CLI subcommands (`internal/cli/`)

| Subcommand | Surface |
|---|---|
| `task` | CRUD, claim/unclaim, complete, assign, stale detection, scheduling, reminders, blocked-by relationships |
| `track` | Lifecycle, plan ingestion, cross-track refs, health computation |
| `flow` | Workflow exec with cassette testing; multi-agent dispatch; pause/resume; conditional branching; retry/timeout |
| `project` | Registry, init, import/export, workspace scoping |
| Other | `config`, `auth`, `sync`, `labels`, `assignee`, `agent`, `uri`, `prompt`, `tui`, `workspace`, `tag`, `inbox`, `version`, `doctor`, `upgrade` |

### Core entities (`internal/core/`)

**Task** (`models.go`):
- `id`, `title`, `description`, `status` (TODO / IN_PROGRESS / DONE / SKIPPED), `assignee`, `effort` (XS-XL), `priority` (P0-P3), `tags`, `created_at`, `updated_at`, `archived`
- `project_id`, `track_id`, `due_at`, `remind_at`, `remind_every`, `stale_timeout`, `blocked_reason`, `origin_system`, `last_sync_at`, `meta`

**Track** (`track.go`):
- `id`, `title`, `type` (feature / fix / bug / refactor / chore / ci / docs / style / perf / test / build), `status` (pending / active / completed / abandoned / archived), `assigned_to`, timestamps, `meta`

**Flow** (`flow.go`):
- `AgentRef`, `FlowStatus` (queued / running / succeeded / failed / canceled / paused), `StepType` (task / exec / parallel / branch / join / retry / subflow), `StepStatus`

**LogEntry** (`models.go`):
- task log: `timestamp`, `by`, `action` (incl. SYNC_*), `note`, `meta`

### Storage (`internal/storage/`)

SQLite migrations v1-v7+. Tables: `tasks`, `task_logs`,
`tracks`, `projects`, `flow_runs`, `task_sequences`. Indexes
on status, assigned_to, created_at, origin_system, archived,
project_id, track_id.

Repos: `TaskDomainRepo`, `TrackDomainRepo`, `FlowDomainRepo`,
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

### Flow execution (`internal/core/flow*`, `internal/flowtest/`)

`FlowParser` + `FlowExecutor` with deterministic execution
contract. `FlowTest` sandbox uses cassette recording for
replay testing. Shim binaries (`cmd/shims/`) intercept tool
invocations (claude, git, docker, npm, uv, pip, etc.).
`AgentRegistry` enforces trust policies.

#### Flow step types

| Type | Runner | What it does |
|---|---|---|
| `task` | `SandboxAgentRunner` | Dispatches an LLM agent subprocess (claude, codex, ...) to execute a task per `task-exec-spec-0.1`. Output is the agent's structured task result. |
| `exec` | `ExecAgentRunner` | Runs a literal command (argv array) via `os/exec`. Captures `{exit_code, stdout, stderr, duration_ms, truncated}` as structured step output. **No LLM dispatch** — for deterministic CLI checks (smoke tests, lints, regex/exit-code gates). See `task-flow-spec-0.1-dev.md` and `examples/flows/exec-cli-smoke.yaml`. |
| `parallel` | (control-flow) | Fan-out children with optional `max_concurrency`. Output ordering deterministic by `step_id`. |
| `branch` | (control-flow) | Selects one path from `cases` based on condition expressions. Non-selected steps marked `skipped`. |
| `join` | (control-flow) | Waits for multiple upstream steps; aggregates outputs for a single downstream consumer (e.g. an eva contract assertion). |
| `retry` | (control-flow) | Re-runs a child step up to `max_attempts` with backoff. |
| `subflow` | (control-flow) | Invokes another flow as a nested step (composition). |

The `exec` step type lets flows express deterministic checks (CLI
smoke, branch-name regex, release-please file protection, etc.)
without paying the LLM tax for work that doesn't need judgement.
Cassettes still apply via the existing xrr catchall shim, so replay
remains hermetic.

#### Contract evaluation (eva integration)

Flows can attach an eva contract to any step via a `<stepID>.yaml`
contract file in the flow's `contracts/` directory. After the step
completes, `internal/flowtest/hook.go` POSTs the step output to the
eva gateway at `{EVA_URL}/v1/contract/invoke`.

**`EVA_URL` is required.** Without it, contract evaluation is
**silently skipped** — the step's contract file is found, parsed,
and then dropped on the floor. There is no warning logged. Set
`EVA_URL` (and `EVA_KEY` if your gateway requires auth) in the
environment before running `tlc flow test` if you want contracts
to actually be checked.

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
- Real-time flow monitoring
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
tlc flow {invoke\|test\|validate\|dry-run\|list}
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
flow_runs (id PK) [flow_id, status, started_at, ended_at, results]
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
flow:
  dir: "examples/flows"
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
- [task-flow-spec-0.1](task-flow-spec-0.1.md) — Flow YAML structure, step types, branching
- [task-log-spec-0.1](task-log-spec-0.1.md) — Audit log actions and schema
- [sync-architecture-0.1](sync-architecture-0.1.md) — Bidirectional sync, conflict resolution, meta fields
- [tlc-cli-spec-0.1](tlc-cli-spec-0.1.md) — CLI command reference
- [tlc-tui-spec-0.1](tlc-tui-spec-0.1.md) — TUI keybindings and views
- [tlc-config-spec-0.1](tlc-config-spec-0.1.md) — Configuration hierarchy
- [tlc-uri-spec-0.1](tlc-uri-spec-0.1.md) — URI scheme and resolution
- [task-exec-spec-0.1](task-exec-spec-0.1.md) — Deterministic execution (stdout/stderr capture, timeouts)
- [vtodo-sync-spec-0.1](vtodo-sync-spec-0.1.md) — RFC 5545 iCalendar VTODO export, RRULE recurrence, sync plugin

Plans: [track-registry-design](plans/2026-04-03-track-registry-design.md), [task-prompt-design](plans/2026-04-04-task-prompt-design.md), [flowtest-agent-dispatch](plans/2026-04-02-flowtest-agent-dispatch.md).

## Evolution

### Recent significant changes

- Bus env-based auth (`051d26f`) — replaces hardcoded dev token
- Connect to dpkms cross-process bus hub (`ae206e4`)
- kit/bus typed topics + domain payload structs (`336caac`)
- Due dates / reminders / recurrence / auto-scheduling (`25efd73`) — v0.3 feature
- Short refs for same-project blocked-by (`50ca32b`, T-0632) — fixes slash-split bug
- Cross-project blocked-by in plan parser (`802abb3`, T-0631)
- Flow use cases — 15 stories + fixtures + e2e tests (`b34a4a1`)
- `--add-plan` idempotent with plan_mapping reconciliation (`24e0887`)
- Dependency graph + execution strategy for tracks (`802abb3`)

### Known issues (open)

- `T-0717` — cross-project blocked-by refs reject multi-segment project IDs (split-on-first-slash bug)
- `T-0719` — `tlc project init` produces literal `'unknown'` project_id when no git remote configured

### Troubleshooting

**My flow's contract is not being checked / I changed the contract
and nothing changed.**
`EVA_URL` is probably not set. The hook in `internal/flowtest/hook.go`
silently passes when `EVA_URL` is empty, even if a `<stepID>.yaml`
contract file exists. Confirm with `echo $EVA_URL`. Set it (and
`EVA_KEY` if needed), then re-run. For CI where you don't want to
stand up the gateway, use the standalone `eva run` CLI
(`hop-top/eva` T-0258) once it ships.

**My `step_status` evaluator is rejected by eva.**
`step_status` was an aspirational evaluator that never landed in eva.
Existing fixtures in `examples/flows/fixtures/{conditional,
composition-child}/` were rewritten 2026-04-27 to use `contains`
against the joined step output. Use the same pattern, or wait for
the `status_code` evaluator (`hop-top/eva#flow-exec-evaluators`
T-0257) to ship.

### Known limits

- Task ID sequences per-project; no global counter
- `origin_system` field determines sync eligibility
- Plan extraction one-way by default; rewrite requires explicit `--add-plan`
- Flow cassettes project-scoped; no cross-project orchestration
- Workspace adapter pluggable but default is filesystem-based
- TUI doesn't support multi-project views

## Open questions

1. **Multi-segment project IDs** — `T-0717` documented; resolver currently splits on first `/`. Fix needed for `IdeaCraftersLabs/ocs-aaarrr-lineup` style IDs.
2. **Stale handling defaults** — per-priority schedules in config; what's the right default cadence for P0/P1/P2/P3?
3. **Plan rewrite safety** — `PlanRewrite` updates plan.md on disk; round-trip semantics under concurrent edits?
4. **Cross-track flow orchestration** — flow cassettes are project-scoped; multi-project flows need either flat namespace or cross-DB resolver.
5. **Bus topic naming** — informally `<tool>.<entity>.<action>`; formalise as kit/bus contract or stay convention-only?
