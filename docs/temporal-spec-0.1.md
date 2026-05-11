# Temporal Specification (TLC-TEMPORAL) v0.1

- Status: Draft (v0.1)
- Author: jadb
- Date: 2026-05-11
- Supersedes: none
- Related: `tlc-cli-spec-0.1.md`, `tlc-config-spec-0.1.md`,
  `task-log-spec-0.1.md`, `stories/081-task-scheduling.md`

## 1. Purpose

Codify the storage, parsing, and display contract for every temporal
field tlc persists or renders. The CLI already stores all timestamps as
UTC RFC3339 in SQLite TEXT columns and parses input via `kit/go/core/util`.
This spec freezes that behaviour and binds future temporal work
(T-0904, T-0906, T-0907, T-0908, T-0913) to it.

Authoritative when in conflict with code: this spec. Code drift = bug.

## 2. Scope

Governs every temporal field on tlc entities:

- `Task.DueAt`, `Task.RemindAt`, `Task.RRule`
- `Task.CreatedAt`, `Task.UpdatedAt`
- `Task.LastSyncAt`, `Task.StaleFiredAt`
- `Track.CreatedAt`, `Track.UpdatedAt`
- `Track.DueAt` (once T-0904 ships — same contract applies)
- `task_logs.timestamp`
- `agent_runs.started_at`, `agent_runs.ended_at`
- Every future column whose Go type is `time.Time` or `*time.Time`

Out of scope:

- `flow.timezone` and per-flow cron tick computation — a flow-level
  override on the cron scheduler, not a display tz. See
  `internal/core/cron_scheduler.go` §`Register`. The TZ feeds the cron
  runner's wall-clock interpretation, not the storage or display path
  defined here.
- Wire-format event timestamps on the kit/bus envelope — defined by
  `bus-topics-spec-0.1.md` §4. That spec already requires RFC3339 UTC
  and inherits this one transitively.
- Recurrence rule semantics — `Task.RRule` storage is in scope; RRULE
  evaluation lives in `internal/core/rrule.go` and is governed by
  story 081.

## 3. Design principles

1. **One storage tz.** Every temporal field is UTC at rest.
2. **One parse path.** Every user-input string goes through a kit util
   parser. No ad-hoc `time.Parse` in CLI or core code.
3. **One display path.** Every rendered timestamp goes through a kit
   util formatter, honouring `cfg.UI.Timezone`.
4. **Nil renders empty.** A nil `*time.Time` MUST render as `""` in
   every format. Never `"null"`, `"0001-01-01..."`, or a placeholder.
5. **Forward-compatible.** New temporal columns inherit this spec by
   declaration; no per-field exception list.

## 4. Storage contract

All temporal fields MUST be persisted as UTC RFC3339 strings in SQLite
TEXT columns.

- Format: `time.RFC3339` (Go stdlib reference layout
  `2006-01-02T15:04:05Z07:00`). Implementations MAY emit `RFC3339Nano`
  on write; readers MUST accept both.
- Column type: `TEXT`. Nullable columns use SQL `NULL`; non-nullable
  columns use `TEXT NOT NULL`.
- Encoding location: writes MUST call `.UTC().Format(time.RFC3339)` or
  pass a `time.Time` already in UTC. Local-tz strings, epoch seconds,
  epoch milliseconds, and ISO-without-offset MUST NOT appear on disk.
- Reads MUST parse with `time.Parse(time.RFC3339, …)` (see
  `internal/storage/sqlite.go::parseRFC3339`). Parse failures are
  hard errors — the row is corrupt.

Verifiable invariants in current code:

- `internal/storage/migrations.go` lines 33-34, 123-124, 158-159,
  201-202, 255, 298-299, 327-328, 340-341, 355-356 — every temporal
  column declared `TEXT`.
- `internal/storage/sqlite.go` lines 131, 142, 148, 153 — every write
  path uses `time.RFC3339`.
- `internal/storage/sqlite_track.go`, `internal/storage/agent_run_store.go`
  — same pattern.

## 5. Input parsing contract

All user-input temporal strings MUST be parsed through one of two kit
utilities:

| Direction         | Function                  | Use for                                      |
|-------------------|---------------------------|----------------------------------------------|
| Forward-looking   | `util.ParseUntil`         | `--due`, `--remind-at`, future `--due-by`    |
| Backward-looking  | `util.ParseSince`         | `--created-since`, `--updated-since`, audit  |

Supported input forms are defined and documented at:

- `hop.top/kit/go/core/util/until.go` (`ParseUntilAt` doc comment)
- `hop.top/kit/go/core/util/since.go` (`ParseSinceAt` doc comment)

This spec does not duplicate the list — kit is authoritative. tlc MUST
NOT add format extensions in `internal/cli`; new forms land in kit and
propagate.

Tz resolution at parse time:

- ISO 8601 strings with explicit offset (e.g. `2026-05-01T14:00:00-04:00`)
  → stored offset preserved through conversion to UTC.
- ISO 8601 strings without offset (e.g. `2026-05-01T14:00:00`,
  `2026-05-01`) → interpreted in system local tz, then converted to UTC.
- Relative expressions (`tomorrow`, `in 3d`, `next monday`, `+24h`) →
  computed against `time.Now()` in system local tz, then converted to
  UTC.

Call sites currently in compliance:

- `internal/cli/task_scheduling.go::taskScheduling.parse`
- `internal/cli/task_update.go` lines 279, 292

New flags (T-0908 and later) MUST route through the same primitives.

## 6. Display contract

All rendered timestamps MUST flow through:

1. `util.LoadTimezone(cfg.UI.Timezone)` → `*time.Location`
2. `util.FormatInZone(t, loc, layout)` OR a humanizer
   (`util.RelativeTime`, `util.HumanDuration`)

`cfg.UI.Timezone` semantics (already documented in
`tlc-config-spec-0.1.md` line 620):

| Value           | Resolves to        |
|-----------------|--------------------|
| `""` or `local` | `time.Local`       |
| `UTC`           | `time.UTC`         |
| IANA name       | `time.LoadLocation(name)` — invalid name is a config error |

Layout by format:

| Output format | Layout                                            |
|---------------|---------------------------------------------------|
| `json`        | `time.RFC3339` via `FormatInZone(t, loc, RFC3339)`|
| `yaml`        | `time.RFC3339` via `FormatInZone(t, loc, RFC3339)`|
| `tls`         | `2006-01-02` (date only, in display tz)           |
| `table`       | humanized — `RelativeTime` for past, `HumanDuration` for future |
| `summary`     | humanized — same as `table`                       |
| `vtodo`       | UTC RFC5545 (`DTSTAMP`), independent of display tz|

Nil handling: a `nil *time.Time` MUST render as the empty string in
every format. `task remind` and `task list --due` views MUST treat nil
as "no value" and omit the row decoration (no `!` prefix, no relative
phrase).

Note: `LoadTimezone` and `FormatInZone` are new kit primitives (kit
commit `457e582`, 2026-05-11). Existing tlc render paths that currently
call `t.Format(...)` directly are pre-spec; T-0907 covers the migration.

## 7. CLI behaviour

Flag conventions for any flag whose value is a temporal expression:

- Value is parsed with `util.ParseUntil` (forward-looking) or
  `util.ParseSince` (backward-looking) per §5.
- Value is case-insensitive at the keyword layer (`TOMORROW`,
  `Tomorrow`, `tomorrow` all parse). Kit utils already lowercase
  weekday names; CLI MUST NOT add a second pass.
- `--flag ""` and `--flag -` on an `update` command MUST clear the
  field (set the underlying `*time.Time` to nil).
- `--flag <value>` on an `update` command sets the field; passing the
  same value twice is idempotent.

Examples (system tz = `America/New_York`, display tz config varies):

| Input                        | Stored (UTC)              | Displayed (`UTC`)         | Displayed (`Asia/Tokyo`)  |
|------------------------------|---------------------------|---------------------------|---------------------------|
| `--due "2026-05-01"`         | `2026-05-01T04:00:00Z`    | `2026-05-01T04:00:00Z`    | `2026-05-01T13:00:00+09:00` |
| `--due tomorrow` at 10:00 EDT| `2026-05-12T14:00:00Z`    | `in 1 day`                | `in 1 day`                |
| `--remind-at "2026-05-01T14:00:00Z"` | `2026-05-01T14:00:00Z` | `2026-05-01T14:00:00Z` | `2026-05-01T23:00:00+09:00` |
| `--due -` (on update)        | `NULL`                    | `""`                      | `""`                      |

This spec does NOT introduce new flags. Net-new flags land in T-0908
and inherit §5 + §7.

## 8. Migration and backward compatibility

No migration required. Every temporal column shipped to date is already
UTC RFC3339 TEXT (verifiable: `sqlite3 ~/.local/share/tlc/db.sqlite
'SELECT due_at FROM tasks WHERE due_at IS NOT NULL LIMIT 3'` returns
`Z`-suffixed strings). `task_logs.timestamp` and `agent_runs.*` are
likewise UTC RFC3339 from inception.

Future schema migrations adding temporal columns MUST declare them
`TEXT` (nullable or not nullable as appropriate). Migrations MUST NOT
introduce `INTEGER` epoch columns or `REAL` Julian-day columns.

Renaming, retyping, or replacing a temporal column requires a v0.2 spec
bump and a parallel-write window per
`tlc-config-spec-0.1.md` §"Schema evolution".

## 9. Glossary

- **temporal field** — any persisted or rendered value whose Go type
  is `time.Time` or `*time.Time`.
- **absolute time** — a fully-qualified instant unambiguous to the
  second (RFC3339 with offset, or epoch ns). UTC at rest.
- **relative expression** — a user-input string resolved against
  `time.Now()` (e.g. `tomorrow`, `+3d`, `2 weeks ago`).
- **display tz** — the `*time.Location` returned by
  `util.LoadTimezone(cfg.UI.Timezone)`. Affects rendering only.
- **storage tz** — UTC. Always. There is no second storage tz.

## 10. References

- `internal/core/due.go` — current temporal logic on Task
- `internal/cli/task_scheduling.go` — current input parse call sites
- `internal/cli/task_update.go` — update-side parse call sites
- `internal/storage/sqlite.go` — RFC3339 write/read primitives
- `internal/storage/migrations.go` — schema TEXT-column declarations
- `internal/core/cron_scheduler.go` — flow-level tz override (out of scope)
- `hop.top/kit/go/core/util/until.go` — `ParseUntil`
- `hop.top/kit/go/core/util/since.go` — `ParseSince`
- `hop.top/kit/go/core/util/timezone.go` — `LoadTimezone`, `FormatInZone`
- `hop.top/kit/go/core/util/humanize.go` — `RelativeTime`, `HumanDuration`
- `docs/stories/081-task-scheduling.md` — shipped story behind §5/§7
- `docs/tlc-config-spec-0.1.md` §`ui.timezone` (line 620) — display-tz config
