# TypeID-based task and track identifiers

Status: design — staged before vtodo work. Existing data is disposable; no migration path required.

## Problem

- `Task.ID` is `T-NNNN`, zero-padded 4 digits, capped at 9999.
- `Track.ID` is a user-supplied slug; serves both as primary key and human label.
- Both forms are workspace-local. Globally unique IDs are required for iCalendar UID export and for any future cross-workspace reconciliation.
- Same `T-NNNN` in two workspaces collides on import.

## Goals

- Durable, globally unique primary key for tasks and tracks.
- Preserve the existing short, typeable forms in CLI/TUI: `T-NNNN+` for tasks, slug for tracks.
- Remove the 9999 ceiling.
- Land before vtodo export so vtodo UIDs can rest on a stable identity.

## Non-goals

- Backwards compatibility with existing rows. Existing DBs/repos are wiped and recreated.
- Cross-workspace task graph reconciliation (separate concern).
- Renaming external references (`GH-123`, `GL-45`) — those continue to live in `Task.Reference`.

## Identity model

Two strings per row:

| Role | Task | Track |
|---|---|---|
| Primary key (`ID` field, durable) | `task_<typeid>` | `track_<typeid>` |
| Display alias (short, human-typed) | `T-<seq>` (auto, ≥4 digits) | user-supplied slug |

### TypeID

Format: `<prefix>_<26-char-base32-encoded-uuidv7>` per the TypeID spec.
- Prefix: `task` for tasks, `track` for tracks.
- Body: uuidv7 → base32 (Crockford). Time-sortable.
- Library: `go.jetify.com/typeid` (or equivalent maintained module — see implementation plan for vetting).

Examples:
- `task_01h455vb4pex5vsknk084sn02q`
- `track_01h455vbqkfsn02nk084ksn02q`

### Display alias

**Tasks:** `T-` + sequence, zero-padded to **at least** 4 digits.
- Sequence is per-workspace, monotonically increasing INTEGER column.
- 1 → `T-0001`, 9999 → `T-9999`, 10000 → `T-10000`, 1234567 → `T-1234567`.
- Computed and stored on insert. Indexed for lookup.

**Tracks:** user-supplied slug (existing `^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$` rule). Stored separately from primary key.

### CLI lookup

CLI accepts either form for any command taking a task/track identifier:

```
tlc task show T-0042
tlc task show task_01h455vb4pex5vsknk084sn02q
tlc track show auth-rewrite
tlc track show track_01h455vbqkfsn02nk084ksn02q
```

Resolution: if the input matches the typeid prefix pattern (`^(task|track)_[0-9a-z]{26}$`), look up by primary key; else look up by alias column.

## Schema

### Tasks

```sql
CREATE TABLE tasks (
    id          TEXT PRIMARY KEY,        -- task_<typeid>
    seq         INTEGER NOT NULL,        -- per-workspace sequence
    title       TEXT NOT NULL,
    -- ... existing columns unchanged ...
    UNIQUE (seq)
);
CREATE INDEX idx_tasks_seq ON tasks(seq);
```

The `Task.ID` Go field becomes the typeid string. A new `Task.Seq int64` field exposes the sequence. The display alias is computed (`fmt.Sprintf("T-%04d", seq)` with natural overflow when seq > 9999).

### Tracks

```sql
CREATE TABLE tracks (
    id          TEXT PRIMARY KEY,        -- track_<typeid>
    slug        TEXT NOT NULL UNIQUE,    -- user-supplied
    title       TEXT NOT NULL,
    -- ... existing columns unchanged ...
);
CREATE INDEX idx_tracks_slug ON tracks(slug);
```

The `Track.ID` field becomes the typeid string. A new `Track.Slug string` field holds the user-supplied alias. Existing `Track.ID` semantics (validation regex, slug-ness) move to `Track.Slug`.

## Generation

### Tasks

`core.NewTask` (or equivalent constructor):

1. Generate typeid → `task_<...>`.
2. SELECT `MAX(seq)+1 FROM tasks` (or use a workspace-level sequence counter row).
3. Persist both.

Sequence allocation must be transactional with the insert to prevent duplicates under concurrent creation. SQLite: `INSERT … ON CONFLICT(seq) DO …` retry, or a dedicated `seq_counters` table with `UPDATE seq_counters SET next = next + 1 RETURNING next` inside the same transaction.

### Tracks

`tlc track create <slug>`:

1. Validate slug (existing rules).
2. Generate typeid → `track_<...>`.
3. INSERT with both, slug-uniqueness enforced by DB.

## Display

Anywhere tlc renders a task ID for humans (TUI tables, log entries, dep_render mermaid nodes, error messages, plan files, recipes), use the alias:

| Code path | Render |
|---|---|
| `formatTLS(task)` | `T-<seq>` |
| `dep_render` mermaid | `T-<seq>` (existing comment becomes accurate again) |
| `task_show` header | `T-<seq>` (typeid) |
| `task list` table | `T-<seq>` |
| Plan files / YAML refs | `T-<seq>` |
| Logs (`LogEntry.TaskID`) | typeid (durable) |
| Vtodo UID | typeid + `@<workspace>` |

The TUI/CLI **input** parser accepts both. The **output** renders alias except where durability matters (logs, sync metadata, vtodo).

## Plan files and recipes

Plan files (`docs/plans/*.md`) and recipes reference tasks by `T-NNNN`. Keep using the alias — it's the human-friendly form. Resolution to typeid happens at parse time via the lookup table.

## RFC 5545 UID (forward reference for vtodo work)

UID is the typeid plus a domain suffix:

```
UID:task_01h455vb4pex5vsknk084sn02q@tlc.local
UID:track_01h455vbqkfsn02nk084ksn02q@tlc.local
```

Domain defaults to `tlc.local`; configurable via `output.vtodo.uid_domain` for users who want `@workspace.example.com`. The domain is a namespace hint for calendar clients, not part of identity — typeid alone is sufficient for uniqueness.

The full tlc URI lives in the separate vtodo `URL` property (RFC 5545 §3.8.4.6), not in UID. Renaming a project can rewrite URLs without breaking UID stability.

## Affected components

- `internal/core/models.go` — `Task.ID`, `Task.Seq`, `Track.ID`, `Track.Slug`.
- `internal/core/track.go` — drop `trackIDRe` from `ID`, move to slug validation.
- `internal/core/service.go` — task creation generates typeid + seq.
- `internal/core/flow_executor.go` — `generateTaskID()` emits typeid; sequence allocated by repository, not in this function.
- `internal/core/plan_*.go` — sequence allocation in plan reconcile/tasks paths.
- `internal/storage/migrations.go` — fresh table DDL with `seq` column and indexes.
- `internal/storage/sqlite.go` — repository inserts allocate seq atomically.
- `internal/cli/task_*.go`, `internal/cli/track_*.go` — input parser accepts both forms; output renders alias.
- `internal/cli/formatter.go` — `formatTLS` uses alias; logs use typeid.
- `internal/tui/views.go`, `commands.go` — same dual-input/alias-output rule.
- `internal/core/dep_render.go` — `mermaidNodeID` uses alias.
- Plugin mappers (`plugins/*-sync/mapper.go`) — outbound writes typeid in metadata so re-import can find the row.

## Lookup helpers

Add to `internal/core`:

```go
// ParseTaskRef accepts "T-0042", "task_01h...", or just "0042".
// Returns the typeid after lookup. Empty string + error if not found.
func ParseTaskRef(ctx context.Context, repo TaskRepository, input string) (string, error)

// ParseTrackRef accepts a slug or "track_01h...".
func ParseTrackRef(ctx context.Context, repo TrackRepository, input string) (string, error)
```

CLI commands call these immediately after parsing flags so the rest of the call stack works in typeids only.

## Concurrency

Sequence allocation must hold a write lock on `seq_counters` (or equivalent) for the duration of the insert transaction. SQLite serializes writers anyway, so the contention surface is limited to throughput, not correctness.

## Testing

- Unit: typeid generation, seq allocation under contention, alias formatting (1, 9999, 10000, 1234567).
- Repository: insert N tasks concurrently, verify seq values are 1..N with no gaps and no duplicates.
- CLI: every task/track command accepts both forms; output renders the alias.
- Round-trip: typeid → alias → lookup → same typeid.

## Out of scope

- Vtodo export/import (separate spec).
- RRULE recurrence (separate spec).
- Migration from existing `T-NNNN` rows (explicitly excluded; data is disposable).

## Open questions

- TypeID library choice — `go.jetify.com/typeid` is the reference impl but verify maintenance status before adopting.
- Whether to expose the workspace UID domain via env var, config, or both. Default `tlc.local` is fine for v1.
