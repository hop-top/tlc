# Track Registry Design

## Task List

- [ ] Add `tracks` table to storage layer (schema + migration)
- [ ] Add `track_id` column to tasks table (migration)
- [ ] Define `Track` model in `internal/core/`
- [ ] Implement track status transitions + validations
- [ ] Implement computed state flags (stale, unlinked, blocked, healthy)
- [ ] Implement phase progress computation from task `phase:N` tags
- [ ] Add track repository interface + SQLite implementation
- [ ] Wire track CRUD into service layer
- [ ] `tlc track create` command
- [ ] `tlc track list` command (with progress, state columns)
- [ ] `tlc track show` command (with per-phase breakdown)
- [ ] `tlc track update` command
- [ ] `tlc track archive` / `abandon` / `delete` commands
- [ ] `--track` flag on `tlc task create` / `update` / `list`
- [ ] Auto-transition: pending → active on first task IN_PROGRESS
- [ ] Cross-project qualified ID parsing + `--all-projects` support
- [ ] Artifact folder convention (`tracks/<id>/`)
- [ ] `--add-plan` flag on `tlc track update` (link + optional task extraction)
- [ ] Plan frontmatter task extraction (parse `tasks:` field)
- [ ] Configurable plan-extractor command support
- [ ] Bulk task creation from plan with `blocked-by` index resolution
- [ ] Project health computation from active track load + avg progress
- [ ] Overcommit warning on pending → active transition
- [ ] `tlc track summary` (project pulse view)
- [ ] Health thresholds in `config.yaml` (max-active, min-progress-to-start)

---

## Problem

Tags group tasks by domain. `blocked-by` orders tasks sequentially.
Neither represents a cohesive work stream with its own lifecycle.

A feature like "browser rendering" may span 3 phases, 8 tasks, 2 agents.
Today nothing says "these tasks belong to this initiative, here is overall
progress, here are the artifacts."

Tracks fill that gap: a first-class entity grouping tasks into a work
stream with type, status, computed health state, phase progress, and an
artifact folder.

## Data Model

### Track struct

```go
type Track struct {
    ID         string
    Title      string
    Type       string          // feature, bug, refactor
    Status     TrackStatus     // pending, active, completed, abandoned, archived
    AssignedTo *string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    ProjectID  *string
    Meta       map[string]any
}
```

### Database

New `tracks` table:

```sql
CREATE TABLE tracks (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    type        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    assigned_to TEXT,
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL,
    project_id  TEXT,
    meta        TEXT
);
```

New column on `tasks`:

```sql
ALTER TABLE tasks ADD COLUMN track_id TEXT REFERENCES tracks(id);
```

### Artifact folder

`tracks/<track-id>/` — created on demand, not by `tlc track create`.
Convention: `spec.md`, `plan.md`, `reviews/`.

## Track ID

### Format

Lowercase alphanumeric + hyphens. Min 3 chars, max 64.
Derived from title by default; user can override with `--id`.

### Qualified ID (cross-project)

Format: `{org}_{repo}--{track-id}`

- `_` replaces `/` in project slug (hop-top/tlc → hop-top_tlc)
- `--` separates project from track
- Parsing: split on first `--`; absent `--` means local

Examples:
- Local: `browser-rendering`
- Qualified: `hop-top_tlc--browser-rendering`
- Multi: `hop-top_tlc--browser-rendering,hop-top_aps--browser-rendering`

Uniqueness is per-project. Two projects can share the slug
`browser-rendering`.

## Status (user/system editable)

### Transitions

```
pending → active           (auto: first linked task → IN_PROGRESS)
active → completed         (explicit, validated)
active → abandoned         (explicit)
completed → archived       (explicit)
abandoned → archived       (explicit)
```

### Validations

| Transition | Condition |
|------------|-----------|
| → active | 1+ linked tasks |
| → completed | all linked tasks DONE or SKIPPED |
| → archived | status = completed or abandoned |
| stay pending | enforced while 0 linked tasks |

### Auto-transitions

Only `pending → active`: triggered in the task update/claim code path
when a linked task moves to IN_PROGRESS. All other transitions require
explicit user action.

## State (computed flags, system-only)

Computed at query time. Never stored. A track can carry multiple flags.

| Flag | Condition |
|------|-----------|
| `stale` | status ∈ {pending, active}, 1+ tasks, max(task.updated_at) < now − threshold |
| `unlinked` | 0 linked tasks |
| `blocked` | 1+ tasks, all non-terminal tasks have unresolved blocked_by |
| `healthy` | none of the above (exclusive) |

Stale threshold: configurable in `config.yaml`, default 2 days.

## Phases

Phases derive from task tags. No dedicated model.

- Tasks tagged `phase:1`, `phase:2`, etc.
- Phase count = max phase tag value across linked tasks
- Current phase = lowest phase with incomplete tasks
- Tasks without a `phase:N` tag appear under "Unphased"

Progress format: `5/8 (P2/3)` = 5 of 8 tasks done, phase 2 of 3.

## CLI Surface

### Track commands

```bash
tlc track create "Browser rendering" --type feature [--id slug] [--assigned-to @me]
tlc track list [--status active] [--state stale] [--type feature]
tlc track show <id>
tlc track update <id> [--title "..."] [--status completed] [--assigned-to @me]
tlc track archive <id>
tlc track abandon <id>
tlc track delete <id>          # fails if linked tasks exist
tlc track summary              # project pulse view
```

### Task integration

```bash
tlc task create "..." --track browser-rendering
tlc task update <id> --track browser-rendering
tlc task update <id> --track -                  # unlink
tlc task list --track browser-rendering
```

### `track list` output

```
 ID                  Title               Type      Status   State    Progress     Assignee
─────────────────────────────────────────────────────────────────────────────────────────────
 browser-rendering   Browser rendering   feature   active   stale    5/8 (P2/3)   @me
 auth-rewrite        Auth rewrite        refactor  pending  unlinked 0/0          -
```

### `track show` output

```
Track: browser-rendering
Title: Browser rendering
Type:  feature    Status: active    State: stale    Assignee: @me
Created: 2026-04-03    Updated: 2026-04-03

Progress: 5/8 tasks (62%)

Phase 1 — Setup          3/3 ✓
Phase 2 — Core           2/4
  T-0130  [DONE]        Parse HTML
  T-0131  [DONE]        Render DOM
  T-0132  [IN_PROGRESS] CSS engine
  T-0133  [TODO]        Layout engine
Phase 3 — Polish         0/1
  T-0140  [TODO]        Performance tuning

Unphased
  (none)
```

### Cross-project

```bash
tlc track list --all-projects
tlc task list --track hop-top_tlc--browser-rendering,hop-top_aps--browser-rendering
```

`--all-projects` output adds a Project column, reuses existing project
registry infrastructure.

## Plan Linkage

Plans and tracks have a many-to-many relationship. Plans remain files
in `docs/plans/`; they are not a first-class TLC entity.

### Linking

```bash
tlc track update browser-rendering --add-plan docs/plans/2026-04-03-cached-vectors.md
```

Plan files can declare track associations in frontmatter:

```yaml
---
title: Use cached vectors
tracks: [rnd-improve-performance, search-optimization]
---
```

Links are stored in `Track.Meta["plans"]` (list of file paths) and
in the plan's frontmatter `tracks:` field. Either side can be queried.

### Task extraction from plans

`--add-plan` optionally bulk-creates tasks linked to the target track.
Two extraction modes, tried in order:

1. **Frontmatter** — plan declares a `tasks:` field
2. **Command** — configured extractor receives plan path, returns task list

If neither yields tasks, the plan is linked without task creation.

#### Frontmatter format

All task fields supported. Omitted fields use defaults (status=TODO,
no assignee, etc.). `blocked-by` uses zero-based index references
into the list; TLC resolves them to real task IDs during bulk create.

```yaml
---
title: Use cached vectors
tracks: [rnd-improve-performance]
tasks:
  - title: "Benchmark current vector lookups"
    description: |
      Measure p50/p99 latency for current implementation.
      Include cold start and warm cache scenarios.
      Document results in tracks/rnd-improve-performance/.
    tags: [phase:1, domain:perf]
    effort: S
    priority: P1
    assigned-to: "@me"
    blocked-by: []
  - title: "Implement vector cache layer"
    description: |
      Add an LRU cache in front of the vector store.
      Invalidate on write. Configurable TTL.
    tags: [phase:1, domain:perf]
    effort: M
    priority: P1
    blocked-by: [0]
  - title: "Validate cache hit ratio under load"
    tags: [phase:2, domain:perf]
    effort: S
    priority: P2
    blocked-by: [1]
---
```

#### Command extractor

Configured in `config.yaml`:

```yaml
tracks:
  plan-extractor: "scripts/extract-plan-tasks.sh"
```

TLC calls the command with the plan file path as argument. The command
returns a JSON or YAML task list in the same shape as the frontmatter
`tasks:` field.

#### Scope rules

When a plan lists multiple tracks in its `tracks:` frontmatter,
`--add-plan` creates tasks only for the track it was run on. Other
tracks get the plan link but not the tasks. This avoids duplicate tasks
across tracks.

## Project Health

Tracks running in parallel are a resource allocation signal. Many active
tracks with low progress means the project is spread thin. TLC surfaces
this as warnings — never blocks.

### Metrics

```
active_load  = count(tracks where status = active)
avg_progress = mean(progress%) across active tracks
```

### Configuration

```yaml
tracks:
  stale-threshold: 2d
  health:
    max-active: 3              # soft cap before warnings
    min-progress-to-start: 50  # % avg progress threshold
```

### Warning triggers

| Event | Condition | Warning |
|-------|-----------|---------|
| Task claim triggers track pending → active | active_load ≥ max-active AND track's previous status ∉ {active, stale} | "5 tracks already active (avg 32% progress); consider completing existing work before starting browser-rendering" |
| `tlc track create` | active_load ≥ max-active | softer advisory (track starts pending, no block) |
| `tlc track list` | any active track stale | stale flag visible in State column |

The pending → active warning fires only when a track is **newly
becoming active**. Already-active tracks with new task claims do not
re-trigger the warning.

### `track summary` (project pulse)

```
Project: hop-top/tlc

Active: 5    Pending: 2    Completed: 12    Abandoned: 1

Health: ⚠ overcommitted (5 active, avg 32% progress)

 ID                  Progress   State          Updated
──────────────────────────────────────────────────────────
 browser-rendering   62%        healthy        1h ago
 auth-rewrite        15%        stale,blocked  4d ago
 perf-tuning         28%        stale          3d ago
 api-v2              40%        healthy        6h ago
 mobile-sync         18%        blocked        1d ago
```

### Principles

- **Warnings, not blocks.** User can always proceed.
- **Configurable.** Teams running many parallel tracks raise the cap.
- **Actionable.** Warning names the tracks and suggests action.
- **Passive visibility.** `track list` and `track summary`
  surface health without extra commands.
