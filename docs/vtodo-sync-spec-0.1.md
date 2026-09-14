# VTODO Sync Specification v0.1

**Status**: Ratified — vtodo-export-sync track 2026-05  
**Date**: 2026-05-02  
**Cross-references**: [identifiers-spec-0.1.md](identifiers-spec-0.1.md), [sync-architecture-0.1.md](sync-architecture-0.1.md), [tlc-plugin-spec-0.1.md](tlc-plugin-spec-0.1.md)

---

## Overview

TLC exports tasks and tracks as RFC 5545–compliant iCalendar VTODO components via the `--format vtodo` CLI flag and the `vtodo-sync` plugin. This enables interop with Apple Reminders, Things, Thunderbird, Tasks.org, CalDAV servers, and other calendar clients.

**Key capabilities**:
- Task ↔ VTODO field mapping with tlc-specific metadata preserved in X-properties
- Hierarchy: tracks and child tasks linked via RFC 5545 `RELATED-TO` with `RELTYPE=PARENT`/`CHILD`
- Dependencies: task blocking relationships via RFC 9253 `RELTYPE=DEPENDS-ON` (v1 support; temporal RELTYPEs deferred)
- Recurring schedules: `Task.RRule` (RFC 5545 §3.3.10) replaces legacy fixed `RemindEvery` duration
- Log entries: opt-in VJOURNAL export via `--include-logs` CLI flag or `include_logs` plugin config
- Round-trip stability: TypeID UIDs survive export → import; foreign UIDs preserved in `Meta["external_uid"]`

---

## VCALENDAR Envelope

All VTODO/VJOURNAL output is wrapped in a VCALENDAR container:

```
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//tlc//vtodo//EN
CALSCALE:GREGORIAN
METHOD:PUBLISH
[VTODO components]
[VJOURNAL components]
END:VCALENDAR
```

**Notes**:
- `VERSION` and `CALSCALE` are RFC 5545 defaults
- `METHOD:PUBLISH` indicates one-way export (not a scheduling request)
- `PRODID` uses `-//tlc//vtodo//EN` by default, configurable via `WithProductID()` option or `output.vtodo.product_id` config key

---

## Task ↔ VTODO Field Mapping

| tlc Field | VTODO Property | Notes |
|---|---|---|
| `Task.ID` (TypeID) | `UID` | Format: `<typeid>@<domain>`; default domain `tlc.local`, configurable |
| `Task.Reference` | `URL` | tlc URI (e.g., `tlc://project/task_01h...`); separate from UID for stability |
| `Task.Title` | `SUMMARY` | — |
| `Task.Description` | `DESCRIPTION` | — |
| `Task.Status` | `STATUS` | TODO→NEEDS-ACTION, IN_PROGRESS→IN-PROCESS, DONE→COMPLETED, SKIPPED→CANCELLED |
| `Task.Priority` | `PRIORITY` | P0→1, P1→3, P2→5, P3→7 (RFC 5545: 1=high, 3=medium, 5=low, 7=lowest) |
| `Task.AssignedTo` | `ATTENDEE` | If email resolvable: `ATTENDEE;CN=<name>:mailto:<addr>`; else `X-TLC-ASSIGNEE:<name>` |
| `Task.Tags` | `CATEGORIES` | Comma-separated list |
| `Task.CreatedAt` | `CREATED`, `DTSTAMP` | Both set to creation time; format: UTC `YYYYMMDDTHHMMSSZ` |
| `Task.UpdatedAt` | `LAST-MODIFIED` | UTC format |
| `Task.DueAt` | `DUE` | UTC format; omitted if not set |
| `Task.RemindAt` | `VALARM` | TRIGGER set to absolute datetime; omitted if RemindAt not set |
| `Task.RRule` | `RRULE` | RFC 5545 string (e.g., `FREQ=DAILY;INTERVAL=2`); omitted if empty |
| `Task.TrackID` | `RELATED-TO;RELTYPE=PARENT` | Points to parent track's UID; omitted if no parent |
| `Task.Effort` | `X-TLC-EFFORT` | Value: `XS` \| `S` \| `M` \| `L` \| `XL`; omitted if not set |
| `Task.Meta` (map) | `X-TLC-META` | JSON-serialized; omitted if empty. Derived keys excluded — see [The `X-TLC-*` Extension Namespace](#the-x-tlc--extension-namespace) |
| `Task.Archived` | (omitted) | Archived tasks excluded from output by default; include via `--archived` flag |

**Time format**: All datetime fields use UTC ISO 8601 (`20260501T143000Z`) with trailing `Z` to indicate UTC.

---

## Hierarchy: PARENT/CHILD Relationships

**Track ↔ Task**:
- When exporting a task with `TrackID` set, emit `RELATED-TO;RELTYPE=PARENT:<track-uid>@<domain>`
- When exporting a track, emit one `RELATED-TO;RELTYPE=CHILD:<task-uid>@<domain>` per member task
- Both directions present in a single export for clarity; clients that consume one direction should ignore the inverse

**Subtask hierarchy (Task ↔ Task)**: Currently out of scope; requires `Task.ParentID` field in tlc schema.

**No SIBLING relations**: Tasks with the same parent are siblings but not explicitly marked with `RELTYPE=SIBLING`. Reason: RFC 5545 readers degrade unknown RELTYPEs to PARENT; explicit SIBLING relations would silently corrupt in older clients.

---

## Dependencies: DEPENDS-ON (RFC 9253)

`RELTYPE=DEPENDS-ON` (RFC 9253) lives **on the blocked task**, pointing to the blocker UID.

**Semantics**: If Task A is blocked by Task B, then A's VTODO emits:
```
RELATED-TO;RELTYPE=DEPENDS-ON:<B-uid>@<domain>
```

Multiple blockers are represented as separate `RELATED-TO` rows.

**Inverse inference**: On import, readers may infer inverse relationships ("X blocks Y") by scanning for all `DEPENDS-ON` references pointing at X. Do NOT invent custom `RELTYPE=BLOCKS` or `BLOCKED-BY` — RFC 5545–era clients treat unknown RELTYPEs as PARENT, silently corrupting hierarchy.

**Temporal RELTYPEs** (FINISHTOSTART, FINISHTOFINISH, STARTTOSTART, STARTTOFINISH from RFC 9253): Out of scope for v1. May be added in v2 if a real consumer requires scheduling semantics.

---

## VJOURNAL: Log Entries (Opt-in)

Log entries naturally map to VJOURNAL components, linked to their task via `RELATED-TO`:

| LogEntry Field | VJOURNAL Property | Notes |
|---|---|---|
| UID (derived) | `UID` | Synthesized: `log-<task-typeid>-<action>-<utc-stamp>@<domain>` — see below |
| `Timestamp` | `DTSTAMP` | UTC format |
| `By` (user) | `ORGANIZER` | If email resolvable: `ORGANIZER:mailto:<addr>`; else `X-TLC-LOG-BY:<name>` |
| `Action` | `SUMMARY` | Abbreviated action string (e.g., `created`, `status_change`, `comment`) |
| `Note` | `DESCRIPTION` | Full note text; omitted if empty |
| `TaskID` | `RELATED-TO` | Points to parent task's UID (no RELTYPE; implicit parent) |
| `Meta` | `X-TLC-META` | JSON-serialized; omitted if empty. See [The `X-TLC-*` Extension Namespace](#the-x-tlc--extension-namespace) |

**Opt-in**: VJOURNAL export is disabled by default. Enable via:
- CLI: `--include-logs` flag on `tlc task list/show` and `tlc track show`
- Plugin: `include_logs: true` in `vtodo-sync` config

**Rationale**: Log volume can dwarf task volume; most calendar clients don't render VJOURNAL. Opt-in keeps exports lean by default.

**UID scheme**: `log-<task-typeid>-<action>-<utc-stamp>@<domain>`, e.g.
`log-task_01h455vb4pex5vsknk084sn02q-status_change-20260502T143000Z@tlc.local`.

A log entry has no identity of its own to put in the UID: `LogEntry.ID` is
a per-database autoincrement integer, not a TypeID, so it is neither
stable across databases nor meaningful to a calendar client. The UID is
therefore *derived* from the fields that do identify the entry — its
task, its action, and its timestamp — which makes it deterministic:
re-exporting the same log entry yields the same UID, which is what a
calendar client needs to recognize a component it has already seen.

The composite is unique in practice rather than by construction. Two log
entries on the same task, with the same action, in the same second would
collide; nothing in the schema forbids that, and no collision has been
observed. Giving `LogEntry` a real TypeID would remove the caveat and let
the UID be `log_<typeid>@<domain>`, but that is a schema change and is
out of scope for v0.1.

**Round-trip**: Foreign VJOURNAL imports (non-tlc UIDs) generate fresh tlc log rows with the original UID stored in `LogEntry.Meta["external_uid"]` for round-trip stability.

---

## RRULE: RFC 5545 Recurrence Rules

`Task.RRule` replaces legacy `RemindEvery` duration field. Stored as an RFC 5545 RRULE string; empty string means no recurrence.

**Format**: `FREQ=<freq>[;INTERVAL=<n>][;BYDAY=<days>][;UNTIL=<datetime>|COUNT=<n>]`

**Supported FREQ values** (v1):
- `HOURLY` — Every hour
- `DAILY` — Every day
- `WEEKLY` — Every week
- `MONTHLY` — Every month

**Supported modifiers**:
- `INTERVAL=<n>` — Repeat every N units (default: 1)
- `BYDAY=MO,WE,FR` — Restrict to specific weekdays
- `UNTIL=<datetime>` — Stop on or before date (UTC, e.g., `20260601T000000Z`)
- `COUNT=<n>` — Stop after N occurrences
- `BYMONTHDAY=<n>` — Restrict to specific day of month

**Examples**:
```
FREQ=DAILY
  Every day.

FREQ=DAILY;INTERVAL=2
  Every other day.

FREQ=WEEKLY;BYDAY=MO,WE,FR
  Every Monday, Wednesday, Friday.

FREQ=MONTHLY;BYMONTHDAY=1
  First day of every month.

FREQ=DAILY;UNTIL=20260601T000000Z
  Every day until June 1, 2026.

FREQ=DAILY;COUNT=5
  Five times, then stop.
```

**Validation**: Malformed RRULE rejected at task create/update time via `core.ValidateRRule()`. Errors reported with parsing details.

**Reminder firing**: Stored RRULE string is parsed by the reminder scheduler, which computes the next fire time via `core.NextFireFromRRule()` and sets `RemindAt`. Supports up to 10,000 iterations to find next occurrence; bails with error if exceeded.

**Time zone**: Rules are stored in UTC and computed in UTC. No floating local time support in v1; use UTC UNTIL dates.

---

## The `X-TLC-*` Extension Namespace

`X-TLC-*` is a System-scoped extension namespace (RFC 5545 §3.8.8.2) owned by tlc. Classification is delegated to `hop.top/vstar/ext`: `ext.ScopeOf` reports `ScopeSystem` and `ext.SystemName` reports owner slug `TLC`. tlc never hand-maintains a name list on the read path.

**`Meta` serialization**: `Task.Meta`, `Track.Meta` and `LogEntry.Meta` each serialize to a **single** `X-TLC-META` property whose value is a JSON object. One property per key (`X-TLC-META-<KEY>`) is explicitly NOT used: Meta values are arbitrary JSON, and a flat property family cannot carry numbers, booleans, arrays or nested maps without inventing a type-tagging convention on top of RFC 5545 TEXT.

Consequence of the JSON round-trip: values return with `encoding/json`'s `interface{}` types. Numbers decode as `float64`, never as the Go integer that was written. Consumers comparing Meta across an export/import boundary must expect that widening.

**Derived keys are excluded** from `X-TLC-META`, because decode reconstructs them from other wire constructs and emitting them twice would break round-trip stability:

| Key | Reconstructed from |
|---|---|
| `blocked_by` | `RELATED-TO;RELTYPE=DEPENDS-ON` rows |
| `external_uid` | the `UID` property |
| `x_tlc` | re-emitted as real `X-TLC-*` properties (below) |

An entity whose Meta holds only derived keys emits no `X-TLC-META` at all.

**Unknown `X-TLC-*` properties are preserved, not dropped.** On import, every `X-TLC-*` property that no typed field consumes is parked in `Meta["x_tlc"]` as a map of full (upper-cased) property name → raw string value, and re-emitted verbatim on the next export. This lets a calendar written by a newer tlc survive a round-trip through an older one without losing state.

**Sub-components are walked explicitly.** `ext.ExtensionsByScope` does not recurse into `Component.Sub`, so `VALARM` (and any future nested component) is scanned separately; its `X-TLC-*` properties merge into the parent entity's `x_tlc` bag.

**Non-tlc extensions are ignored, by design.** Extensions owned by another system (`X-APPLE-SORT-ORDER`), by vstar (`X-VSTAR-*`), or experimental (`X-EXP-*`) are neither imported into tlc `Meta` nor echoed back on export. Rationale: tlc does not own those namespaces, so it cannot know their semantics, lifetime, or whether round-tripping a stale copy would contradict the owning system. Adopting them into `Meta` would also let a foreign producer inject arbitrary keys into tlc's own model. Preserving them is a defensible future change, but it requires a separate per-system passthrough store, not tlc's `Meta` map.

---

## UID Format and Stability

**TypeID UID**: `<typeid>@<domain>`

Example: `task_01h455vb4pex5vsknk084sn02q@tlc.local`

**Domain**: Configurable via `WithUIDDomain()` option or `output.vtodo.uid_domain` config key; defaults to `tlc.local`.

**Rationale for separate URL property**: UIDs should be stable across renames and reorganizations. `Task.Reference` (a tlc URI) is stored separately in the `URL` property to preserve the connection to tlc even if task metadata changes.

**Round-trip stability**:
- On export: UID = `<typeid>@<domain>`
- On import: If UID matches tlc TypeID pattern, extract typeid and update/create that row
- On import (foreign UID): If UID doesn't match tlc pattern (e.g., from Apple Reminders), generate fresh tlc TypeID and store original UID in `Meta["external_uid"]`

---

## The `vtodo-sync` Plugin

Pluggable sync provider (sibling of `github-sync`) for bidirectional sync with `.ics` files or CalDAV endpoints. Implements the standard plugin interface (JSON-RPC over stdio).

**Manifest**: `plugins/vtodo-sync/manifest.yaml`

```yaml
name: vtodo-sync
version: 0.1.0
type: external-sync
description: iCalendar VTODO synchronization (file or CalDAV)
capabilities:
  - sync.pull
  - sync.push
  - sync.bidirectional
entry_point: ./bin/vtodo-sync
config_schema:
  type: object
  properties:
    source:
      type: string
      description: "file:///path/to/cal.ics"
      required: true
    sync_direction:
      type: string
      enum: [pull, push, bidirectional]
      default: bidirectional
    include_logs:
      type: boolean
      default: false
permissions:
  - filesystem.read
  - filesystem.write
```

**RPC methods**:
- `sync.pull` — Read `.ics` file, parse VCALENDAR, return list of normalized task records
- `sync.push` — Receive task list, encode to VCALENDAR, write `.ics` file atomically
- `sync.delete` — Remove task from source (not yet fully specified; see tlc-plugin-spec-0.1.md)

**File mode** (`source: file://...`):
- Atomic read/write of a single `.ics` file
- All VTODOs stored in one VCALENDAR
- No auth required
- Phase 3a (shipped)

**CalDAV mode** (`source: https://...`):
- RFC 4791 WebDAV protocol
- Each VTODO is a separate calendar object resource
- PROPFIND for listing, GET per object, PUT/DELETE for writes
- ETags for optimistic concurrency
- Phase 3b (deferred)

---

## CLI Examples

**Export task list as VTODO**:
```bash
tlc task list --format vtodo
```
Outputs VCALENDAR with all tasks (or filtered by status, assignee, etc.).

**Export single task as .ics file**:
```bash
tlc task show T-0042 --format vtodo --output task.ics
```

**Export track and all members as VTODO**:
```bash
tlc track show auth-rewrite --format vtodo --recursive
```
Emits track as a VTODO (with X-TLC-IS-TRACK marker) plus all member tasks with `RELATED-TO RELTYPE=CHILD`.

**Include log entries**:
```bash
tlc task list --format vtodo --include-logs
```
Adds VJOURNAL components for every log action (create, update, comment, etc.).

**Export archived tasks**:
```bash
tlc task list --format vtodo --archived
```
By default, archived tasks are omitted.

---

## Out of Scope (v1)

- **VEVENT** components (calendar events; separate from tasks)
- **Subtask hierarchy** (`Task.ParentID` field not yet in schema)
- **`RELTYPE=SIBLING`** (siblings inferred from shared parent)
- **`RELTYPE=BLOCKS` / `BLOCKED-BY`** (non-standard; cause silent corruption in RFC 5545 readers)
- **Temporal RFC 9253 RELTYPEs** (`FINISHTOSTART`, `FINISHTOFINISH`, `STARTTOSTART`, `STARTTOFINISH`)
- **CalDAV mode** (Phase 3b; file mode ships first)
- **Attendee scheduling** (RFC 5546; send invites to colleagues)
- **VALARM customization** (only TRIGGER-based alarms; no email/SMS actions)

---

## Configuration

Config lives in `<project>/.tlc/config.yaml`:

```yaml
output:
  vtodo:
    product_id: "-//MyOrg//Calendar//EN"  # Optional; default: -//tlc//vtodo//EN
    uid_domain: "example.com"              # Optional; default: tlc.local
```

Both keys apply to every `--format vtodo` export. `uid_domain` is also
the domain the decoder treats as its own: a UID under any other domain
is foreign, and importing it mints a fresh tlc ID rather than claiming
the existing one. Change `uid_domain` after exporting and the previous
exports read back as foreign.

The `vtodo-sync` plugin runs as a separate process and does not read
this file. It takes the same two settings from the environment it
inherits from tlc:

```
TLC_VTODO_PRODUCT_ID
TLC_VTODO_UID_DOMAIN
```

CLI flags override config:
- `--format vtodo` — Output format
- `--include-logs` — Include VJOURNAL entries (default: off)
- `--output <file>` — Write to file (default: stdout)

---

## Examples

### Single Task

Input: Task `T-0042` with title "Implement RRULE parsing", status IN_PROGRESS, due 2026-05-15.

Output (excerpt):
```
BEGIN:VTODO
UID:task_01h455vb4pex5vsknk084sn02q@tlc.local
URL:tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn02q
SUMMARY:Implement RRULE parsing
STATUS:IN-PROCESS
DUE:20260515T000000Z
LAST-MODIFIED:20260502T143000Z
CREATED:20260501T100000Z
PRIORITY:3
CATEGORIES:feature,parser
END:VTODO
```

### Task with Recurrence

Input: Task with `RRule: "FREQ=WEEKLY;BYDAY=MO,WE,FR"`.

Output (excerpt):
```
BEGIN:VTODO
UID:task_01h455vb4pex5vsknk084sn02q@tlc.local
SUMMARY:Weekly standup
RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR
LAST-MODIFIED:20260502T143000Z
...
END:VTODO
```

### Track with Child Tasks

Input: Track "auth-rewrite" with 3 member tasks.

Output:
```
BEGIN:VTODO
UID:track_01h455vbqkfsn02nk084ksn02q@tlc.local
SUMMARY:auth-rewrite
X-TLC-IS-TRACK:true
RELATED-TO;RELTYPE=CHILD:task_01h455vb4pex5vsknk084sn02q@tlc.local
RELATED-TO;RELTYPE=CHILD:task_01h455vb4pex5vsknk084sn02r@tlc.local
RELATED-TO;RELTYPE=CHILD:task_01h455vb4pex5vsknk084sn02s@tlc.local
...
END:VTODO

BEGIN:VTODO
UID:task_01h455vb4pex5vsknk084sn02q@tlc.local
SUMMARY:Add OAuth2 support
RELATED-TO;RELTYPE=PARENT:track_01h455vbqkfsn02nk084ksn02q@tlc.local
...
END:VTODO
…(two more tasks)…
```

### Task with Dependency

Input: Task A (blocked by Task B).

Output (on Task A's VTODO):
```
BEGIN:VTODO
UID:task_01h455vb4pex5vsknk084sn02q@tlc.local
SUMMARY:Task A
RELATED-TO;RELTYPE=DEPENDS-ON:task_01h455vb4pex5vsknk084sn02r@tlc.local
...
END:VTODO
```

### Log Entry Round-trip

Input: LogEntry for task with timestamp 2026-05-02 14:30:00, by jad, action "status_change", note "Moved to done".

Output (VJOURNAL):
```
BEGIN:VJOURNAL
UID:log-task_01h455vb4pex5vsknk084sn02q-status_change-20260502T143000Z@tlc.local
DTSTAMP:20260502T143000Z
ORGANIZER:mailto:jad@example.com
SUMMARY:status_change
DESCRIPTION:Moved to done
RELATED-TO:task_01h455vb4pex5vsknk084sn02q@tlc.local
X-TLC-LOG-ACTION:status_change
END:VJOURNAL
```

---

## See Also

- [identifiers-spec-0.1.md](identifiers-spec-0.1.md) — TypeID format and display aliases
- [sync-architecture-0.1.md](sync-architecture-0.1.md) — TLC sync principles and plugin architecture
- [tlc-plugin-spec-0.1.md](tlc-plugin-spec-0.1.md) — Plugin interface and manifest schema
- RFC 5545 — iCalendar Specification (VTODO, VJOURNAL, RRULE, RELATED-TO)
- RFC 9253 — iCalendar Recurrence Relations for Task Management (DEPENDS-ON)

---

**Version**: 0.1  
**Last Updated**: 2026-05-02

---

<!-- added by hash-every-component / self-certification; fold into the sections above when the X-prop registry rewrite merges -->

## Addendum: integrity and timestamps

- **Every component is hashed.** Each VTODO, VJOURNAL and VALARM
  carries `UID`, `DTSTAMP` and `X-VSTAR-HASH` (spec-vstar 02). The hash
  is the last property of the component and covers the finished
  component, sub-components included. On import every component is
  re-verified; a missing or mismatching hash is reported in
  `ParseResult.Warnings` (the `vtodo-sync` plugin prints them to stderr)
  and never blocks the import.
- **`DTSTAMP` is the last-modified instant, not export time.** Task and
  track: `UpdatedAt` (then `CreatedAt`, then the export clock for an
  entity with no timestamp). Log entry: its `Timestamp`. VALARM: its
  parent's stamp. The rows above that say `DTSTAMP` is creation or
  export time are superseded by this. Rationale and the RFC 5545
  deviation: `VSTAR-CONFORMANCE.md`.
- **VALARM UID** is derived from the parent UID with `-alarm` before the
  domain separator: `task_<id>-alarm@<domain>`.
