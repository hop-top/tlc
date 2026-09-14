# VTODO Sync Specification v0.1

**Status**: Ratified — vtodo-export-sync track 2026-05  
**Date**: 2026-05-02  
**Cross-references**: [identifiers-spec-0.1.md](identifiers-spec-0.1.md), [sync-architecture-0.1.md](sync-architecture-0.1.md), [tlc-plugin-spec-0.1.md](tlc-plugin-spec-0.1.md)

---

## Overview

TLC exports tasks and tracks as RFC 5545–compliant iCalendar VTODO components via the `--format vtodo` CLI flag and the `vtodo-sync` plugin. This enables interop with Apple Reminders, Things, Thunderbird, Tasks.org, CalDAV servers, and other calendar clients.

**Key capabilities**:
- Task ↔ VTODO field mapping with tlc-specific metadata preserved in X-properties
- Hierarchy: a task links to its track via a bare RFC 5545 `RELATED-TO` (`RELTYPE` defaults to `PARENT` and is omitted); no `CHILD` back-reference on the track
- Dependencies: task blocking relationships via RFC 9253 `RELTYPE=DEPENDS-ON` (v1 support; temporal RELTYPEs deferred)
- Recurring schedules: `Task.RRule` (RFC 5545 §3.3.10) replaces legacy fixed `RemindEvery` duration
- Log entries: opt-in VJOURNAL export via `--include-logs` CLI flag or `include_logs` plugin config
- Round-trip stability: TypeID UIDs survive export → import; foreign UIDs preserved in `Meta["external_uid"]`

---

## Vocabulary

Every exported component is one of the V\* agentic concepts below. The mapping, its rationale, the journal sub-typing rule and the declaration property are recorded in [2026-09-14-agentic-concept-mapping-decision.md](superpowers/specs/2026-09-14-agentic-concept-mapping-decision.md); this table is the normative summary. Concept definitions: hop-top/agr `GLOSSARY.md`. Concept → component table: [hop-top/spec-vstar](https://github.com/hop-top/spec-vstar) `specs/v0.1/02-component-mapping.md`.

| tlc entity | V\* concept | Component | `X-TLC-CONCEPT` |
|---|---|---|---|
| project | World | VCALENDAR | — (the envelope) |
| `Track` | Mission | VTODO | `mission` |
| `Task` without a track (`TrackID` nil) | Mission | VTODO | `mission` |
| `Task` with a track | Assignment | VTODO | `assignment` |
| `LogEntry` | Journal | VJOURNAL | `status` \| `decision` \| `action` \| `observation` \| `journal` |
| `RecipeRun` | Playthrough | VEVENT | `playthrough` — see [VEVENT: Turns and Playthroughs](#vevent-turns-and-playthroughs) |
| claim window (`ClaimedAt`, or a `CLAIMED`/`RECLAIMED` … `DONE`/`RELEASED`/… journal pair) | Turn | VEVENT | `turn` — see [VEVENT: Turns and Playthroughs](#vevent-turns-and-playthroughs) |
| `AssignedTo`, `LogEntry.By` | Player | VCARD | — (no VCARD export yet) |
| `Task.RRule` | Cadence | `RRULE` property | — (a property, not a component) |
| `Task.RemindAt` | Timeout / escalation | VALARM | — |

A component declares which concept it is with one `X-TLC-CONCEPT` property holding one lowercase token, compared case-insensitively on read. The declaration adds no wire shape: a mission and an assignment are both VTODO. On decode the token is consulted first for track-vs-task: `assignment` always decodes as a task, whatever marker or UID prefix the component also carries; `mission` names a track and a standalone task alike, so it defers to `X-TLC-IS-TRACK`, then to the UID prefix, the same ladder a calendar without the token walks. A standalone task is a mission because it carries no parent `RELATED-TO` edge; an assignment always does. The decoder reports every declared token by wire UID in `ParseResult.Concepts` and never stores it on the entity; the encoder re-derives it from `TrackID` and `Action`.

---

## VCALENDAR Envelope

All output is wrapped in a VCALENDAR container, components in this order:

```
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//tlc//vtodo//EN
CALSCALE:GREGORIAN
METHOD:PUBLISH
[VTODO components: tracks, then tasks]
[VJOURNAL components]
[VEVENT components: turns per task, then playthroughs]
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
| `Task.Status` | `STATUS` | Mapped by the status's configured ROLE (initial→NEEDS-ACTION, active→IN-PROCESS, completed→COMPLETED, skipped→CANCELLED); the configured NAME is preserved in `X-TLC-STATUS` so custom vocabularies round-trip exactly |
| `Task.Priority` | `PRIORITY` | Mapped by configured RANK onto 1–9 (`1 + floor(rank·9/N)`; default P0–P3 → 1/3/5/7); the configured NAME is preserved in `X-TLC-PRIORITY`; empty priority emits no PRIORITY |
| `Task.AssignedTo` | `ATTENDEE` | If the value is an email address: `ATTENDEE:mailto:<addr>` (no `CN`); else `X-TLC-ASSIGNEE:<name>` |
| `Task.Tags` | `CATEGORIES` | Comma-separated list |
| `Task.CreatedAt` | `CREATED` | The entity's creation instant, UTC `YYYYMMDDTHHMMSSZ`; omitted when zero |
| `Task.UpdatedAt` | `LAST-MODIFIED` | The entity's last-modification instant, UTC; omitted when zero. On import, `DTSTAMP` is the fallback when `LAST-MODIFIED` is absent |
| — | `DTSTAMP` | Required on every VTODO (RFC 5545 §3.6.2); the instant it carries is defined in the hashing section |
| `Task.DueAt` | `DUE` | UTC form #2; `DUE;VALUE=DATE:YYYYMMDD` when `Meta["due_date_only"]` is set (see [VALARM: Reminders](#valarm-reminders)); omitted if not set |
| `Task.RemindAt`, `Task.Meta["reminders"]`, `Task.AutoRemindAt()` | `VALARM` | One VALARM per reminder: absolute `TRIGGER;VALUE=DATE-TIME` for `RemindAt` and each entry of `reminders`, relative `TRIGGER;RELATED=END:-PT12H` for the derived auto reminder; see [VALARM: Reminders](#valarm-reminders) |
| `Task.RRule` | `RRULE` | RFC 5545 string (e.g., `FREQ=DAILY;INTERVAL=2`); omitted if empty |
| `Task.TrackID` | `RELATED-TO` | Points to the parent track's UID, bare: RFC 5545 §3.2.15 defaults `RELTYPE` to `PARENT` and spec-vstar 02 requires the parameter be omitted for that value; omitted if no parent |
| `Task.Effort` | `X-TLC-EFFORT` | Value: `XS` \| `S` \| `M` \| `L` \| `XL`; omitted if not set |
| `Task.Meta` (map) | `X-TLC-META` | JSON-serialized; omitted if empty. Derived keys excluded — see [The `X-TLC-*` Extension Namespace](#the-x-tlc--extension-namespace) |
| `Task.Archived` | `X-TLC-ARCHIVED` | `TRUE`, emitted only when archived. Archived tasks excluded from output by default; include via `--archived` flag |

**Time format**: All datetime fields use UTC ISO 8601 (`20260501T143000Z`) with trailing `Z` to indicate UTC. The one date-only field is `DUE` when the entity is due on a day rather than at an instant: `DUE;VALUE=DATE:20260515` (RFC 5545 §3.3.4), never promoted to a midnight `DATE-TIME` (spec-vstar 03 rule 11).

Every `X-TLC-*` property, including those this table does not list (`X-TLC-STATUS`, `X-TLC-PRIORITY`, `X-TLC-PROJECT-ID`, `X-TLC-TASK-SEQ`, …), is specified in the [`X-TLC-*` property registry](#x-tlc--property-registry).

---

## Hierarchy: PARENT Relationships

**Track ↔ Task**:
- When exporting a task with `TrackID` set, emit `RELATED-TO:<track-uid>@<domain>` with no `RELTYPE`: RFC 5545 §3.2.15 defaults it to `PARENT`, and spec-vstar 02 ("Relationship types") requires emitters to omit the parameter for that value, `RELATED-TO:x` and `RELATED-TO;RELTYPE=PARENT:x` naming the same edge but canonicalizing, and so hashing, differently
- A track emits no `RELTYPE=CHILD` back-reference: each edge is encoded once, in the PARENT direction (spec-vstar 02, rule 1). Track membership is derived on import from the member tasks' edges, so a track's `X-VSTAR-HASH` does not move when a task joins it
- On import an explicit `RELTYPE=PARENT` is accepted as the same edge, in any case; `CHILD` is ignored

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
| UID (derived) | `UID` | Synthesized: `log-<task-typeid>-<action>-<utc-stamp>@<domain>` — see below. A status transition whose task is in the export uses the spec-vstar supersession scheme instead, `journal:status:<task-uid>:<utc-stamp>` — see [Status transitions as supersession entries](#status-transitions-as-supersession-entries) |
| `Timestamp` | `CREATED` | The entry's own instant, UTC; omitted when zero. On import `CREATED` is preferred; `DTSTAMP` is only a fallback for producers that emit no `CREATED` |
| — | `DTSTAMP` | Required on every VJOURNAL (RFC 5545 §3.6.3); the instant it carries is defined in the hashing section |
| `By` (user) | `X-TLC-LOG-BY` | Actor name verbatim; omitted if empty. No `ORGANIZER` is emitted |
| `Action` | `X-TLC-LOG-ACTION` | Action string verbatim (e.g., `created`, `status_change`, `comment`); omitted if empty |
| `Note` | `DESCRIPTION`, `SUMMARY` | `DESCRIPTION` carries the full note and `SUMMARY` its first line; both omitted if the note is empty, in which case `SUMMARY` carries the action instead |
| `TaskID` | `RELATED-TO`, `X-TLC-LOG-TASK` | `RELATED-TO` points at the task's UID, bare on both journal shapes (RFC 5545 §3.2.15 defaults `RELTYPE` to `PARENT`; spec-vstar 02 omits the parameter for that value); `X-TLC-LOG-TASK` carries the bare task TypeID and is preferred on import |
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

### Status transitions as supersession entries

A log entry in the `status` class (see [Vocabulary](#vocabulary): the fixed transition constants `CLAIMED`, `RELEASED`, `DONE`, `SKIPPED`, `RETRY`, `REOPENED`, `BLOCKED`, `UNBLOCKED`, or any name in the configured status vocabulary) whose task is in the same export is emitted as a spec-vstar "status supersession" entry, built by `hop.top/vstar/supersession.Supersedes` against the finished task VTODO:

```
BEGIN:VJOURNAL
UID:journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T143000Z
DTSTAMP:20260502T143000Z
RELATED-TO:task_01h455vb4pex5vsknk084sn02q@tlc.local
CATEGORIES:status-supersession
X-VSTAR-EFFECTIVE-STATUS:IN-PROCESS
X-TLC-CONCEPT:status
SUMMARY:starting work on signer rotation
CREATED:20260502T143000Z
X-TLC-LOG-TASK:task_01h455vb4pex5vsknk084sn02q
X-TLC-LOG-ACTION:CLAIMED
X-TLC-LOG-BY:alice
X-VSTAR-HASH:sha256:…
END:VJOURNAL
```

- **Both UID schemes coexist.** Every other journal, and a status transition whose task is *not* in the export (an orphan supersession entry is unprojectable, spec-vstar 05 §4), keeps `log-<task-typeid>-<action>-<utc-stamp>@<domain>`. Both shapes carry the same `X-TLC-*` properties, so a reader that ignores the ledger discipline sees one kind of journal.
- **`X-VSTAR-EFFECTIVE-STATUS`** carries the RFC 5545 VTODO `STATUS` value the transition lands the task in, through the same role mapping `STATUS` uses: a configured name by its role; `CLAIMED` → `IN-PROCESS`; `RELEASED`, `RETRY`, `REOPENED`, `BLOCKED`, `UNBLOCKED` → `NEEDS-ACTION`; `DONE` → `COMPLETED`; `SKIPPED` → `CANCELLED`. The tlc status name is not written here (rationale and the upstream ask: [VSTAR-CONFORMANCE.md](VSTAR-CONFORMANCE.md)).
- **Tasks only.** `LogEntry.TaskID` is the only subject; tracks have no journals and so no ledger.
- **Import.** A task VTODO whose `X-TLC-STATUS` is absent or names a status the current vocabulary lacks takes its status from the latest supersession entry pointing at it (`supersession.Superseded`, latest `DTSTAMP` wins) when that value is one of the four RFC `STATUS` values; otherwise from its own `STATUS`. A VTODO with a usable `X-TLC-STATUS` is a tlc snapshot and keeps it. An imported supersession journal's effective status is kept in `LogEntry.Meta["effective_status"]` when its action cannot reproduce it (a foreign entry with no `X-TLC-LOG-ACTION`, or a disagreeing value), and is re-emitted verbatim; tlc's own entries decode Meta-clean.

---

## VEVENT: Turns and Playthroughs

Two V\* concepts map to VEVENT (spec-vstar 02): a **turn**, one bounded execution window on an assignment, and a **playthrough**, one run of a recipe through a mission. Every VEVENT carries `UID`, `DTSTAMP`, `DTSTART` (spec-vstar 05 §5, VS041), one `X-TLC-CONCEPT` and `X-VSTAR-HASH` last. No `STATUS`: the pinned vstar has no VEVENT status enum, and an open turn is told from a closed one by the absence of `DTEND`. VEVENTs are not decoded into entities; the one thing read back is an open turn's `DTSTART`, which is the wire form of `Task.ClaimedAt`.

### Turn

| Source | VEVENT Property | Notes |
|---|---|---|
| task id + claim instant | `UID` | `turn-<task-typeid>-<DTSTART>@<domain>`; deterministic, the same whether the window came from the log or from `ClaimedAt` |
| claim instant | `DTSTART` | The `CLAIMED` / `RECLAIMED` entry's timestamp, or `Task.ClaimedAt` |
| release instant | `DTEND` | The closing entry's timestamp; absent while the turn is open |
| — | `DTSTAMP` | `DTEND` when closed, `DTSTART` while open (the turn's last modification) |
| the task | `RELATED-TO`, `SUMMARY` | The assignment's UID, bare (`PARENT` by default); `SUMMARY` is the task title |
| the player | `ATTENDEE:mailto:` / `X-TLC-ASSIGNEE` | The opening entry's `By`, or `Task.AssignedTo` for a `ClaimedAt` turn; same email rule as a task |

**Where turns come from.** With `--include-logs` the exported log is the source: `CLAIMED` or `RECLAIMED` opens a window, `DONE`, `RELEASED`, `SKIPPED`, `APPROVED`, `REJECTED`, `RETRY` or `BLOCKED` closes it, a `RECLAIMED` while a window is open closes that window at the reclaim instant and opens the next, a closer with no open window is ignored, and the last window is left open when nothing closed it. Without the log, or when the exported log opens no window, `Task.ClaimedAt` yields the one open turn. The two sources are never combined, so a claim is never emitted twice. A task never claimed emits no turn. Configured status names do not bound a turn: the executor logs the constants above whatever the vocabulary.

**Import.** A VEVENT declaring `X-TLC-CONCEPT:turn` with no `DTEND` sets `ClaimedAt` on the task its parent `RELATED-TO` names (the latest start when several are open). A VEVENT without the token is not a claim, whatever it is related to.

### Playthrough

| `RecipeRun` Field | VEVENT Property | Notes |
|---|---|---|
| `ID` | `UID` | `<run-typeid>@<domain>` (`run_…`), like a task or track |
| `CreatedAt` | `DTSTART`, `DTSTAMP` | The materialisation instant; a run row never changes, so both are the same. No `DTEND`: a run has no end timestamp (RFC 5545 §3.6.1 permits its absence) |
| `RecipeID` | `SUMMARY`, `X-TLC-RECIPE-ID` | Omitted when empty |
| `Version` | `X-TLC-RECIPE-VERSION` | Omitted when empty |
| `Hash` | `X-TLC-RECIPE-HASH` | The recipe content hash the run pinned; omitted when empty |
| `TrackID` | `RELATED-TO` | The mission the run went through, bare (`PARENT` by default); absent for a trackless run (a path through the world) |
| `ProjectID` | `X-TLC-PROJECT-ID` | Omitted when empty |

Runs reach the encoder through `vtodo.WithRecipeRuns`; the CLI passes the run behind every exported task that has a `RunID` (`tlc task list/show --format vtodo`). `Vars`, `Selection`, `DroppedDeps`, `ParentRun`, `SubjectType`/`SubjectID` and `CreatedBy` are not exported yet.

**Assignment → playthrough edge.** A task materialised by a run carries `X-TLC-RUN:<playthrough UID>`. It is an X-property rather than a `RELATED-TO` because no `RELTYPE` means "member of a run": `PARENT` is the mission edge, `CHILD`/`SIBLING` are hierarchy, `DEPENDS-ON` is ordering, and RFC 5545 readers degrade an unknown `RELTYPE` to `PARENT`, which would silently re-parent the task under the run. On import the value decodes to `Task.RunID` when it is a run TypeID under our domain; a foreign run UID names a run this store has no row for and is left alone. `Task.StepID` is not exported yet.

---

## VALARM: Reminders

A task's reminders travel as VALARMs nested in its VTODO. Every VALARM carries `UID`, `DTSTAMP` (the parent's), `ACTION:DISPLAY`, `DESCRIPTION` (the task title, RFC 5545 §3.6.6 requiring one on a DISPLAY alarm), one `TRIGGER` and `X-VSTAR-HASH` last. `TRIGGER` follows spec-vstar 03 "TRIGGER conventions": `VALUE=DATE-TIME` explicit and UTC form #2 on an absolute trigger, no `RELATED` there; a relative trigger keeps its authored duration units (rule 12) and omits `RELATED=START`.

| Reminder | `TRIGGER` | `UID` |
|---|---|---|
| `Task.RemindAt` | `TRIGGER;VALUE=DATE-TIME:<UTC form #2>` | `task_<id>-alarm@<domain>` |
| each entry of `Task.Meta["reminders"]` | `TRIGGER;VALUE=DATE-TIME:<UTC form #2>` | `task_<id>-alarm-<UTC form #2>@<domain>` |
| `Task.AutoRemindAt()` (DUE set, `NoAutoRemind` false) | `TRIGGER;RELATED=END:-PT12H` | `task_<id>-alarm-auto@<domain>` |

Absolute alarms are written first, in time order; the auto reminder last. The auto reminder is the derived "twelve hours before it is due" of `internal/core`; the relative form says exactly that to a foreign reader without pinning an instant that moves whenever `DUE` does, and RFC 5545 §3.8.6.3 anchors `RELATED=END` to a VTODO's `DUE`.

**Import.** Every VALARM is read, not the first: its trigger is parsed with `duration.AlarmTrigger` and resolved with `Trigger.Resolve` against the parent (`DTSTART` for `RELATED=START` or no `RELATED`, the RFC default; `DUE` for `RELATED=END`), so the relative triggers Apple Reminders and Thunderbird emit resolve to an instant. A VALARM with no `TRIGGER`, a malformed one, or one whose anchor the VTODO lacks is skipped. The resolved instants, de-duplicated and sorted, are split: the earliest one still in the future at import time is `RemindAt` (the reminder tlc fires next), falling back to the earliest of all when none is; the rest are `Meta["reminders"]`, RFC 3339 UTC strings in time order, so re-export writes each back as its own VALARM. The auto reminder is recognised by shape (relative, `RELATED=END`, twelve hours by signed length) and stored nowhere: `AutoRemindAt()` derives it from `DUE` again. An absolute trigger that merely falls twelve hours before `DUE` is a reminder someone set and is kept. A relative trigger against a date-only anchor, which the library refuses to resolve, is applied to the day's midnight UTC, the same reading `DueAt` gets.

**Date-only DUE.** `Task.DueAt` and `Track.DueAt` are instants; a task due on a day is flagged with `Meta["due_date_only"] = true` (a Meta key because Meta already round-trips through every store, and the flag is a fact about the wire form). Export writes `DUE;VALUE=DATE:YYYYMMDD` for a flagged entity and a UTC `DATE-TIME` otherwise. Import reads `DUE;VALUE=DATE` (parameter matched case-insensitively) into `DueAt` = midnight UTC of that day and sets the flag; `DUE:YYYYMMDDT000000Z` is a midnight instant, decodes without the flag and re-exports as it was. An untagged eight-octet `DUE` is a malformed `DATE-TIME` and decodes to nothing. `due_date_only` and `reminders` are derived keys: neither is written into `X-TLC-META`, and a copy found there on import is ignored.

**Not on the wire.** `NoAutoRemind`: a task exported without an auto reminder because the flag was set imports with the flag clear. A foreign relative trigger is re-exported as the absolute instant it resolved to.

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
| `priority_source`, `priority_rule` | `X-TLC-PRIORITY-SOURCE`, `X-TLC-PRIORITY-RULE` |
| `effective_status` (log entries) | `X-VSTAR-EFFECTIVE-STATUS` on a supersession journal |
| `last_sync_hash` (tasks) | nothing: the sync layer's local change-detection baseline, meaningless off this store and never exported |
| `due_date_only` (tasks, tracks) | the `VALUE=DATE` parameter on `DUE` |
| `reminders` (tasks) | the task's further VALARMs, one per instant; see [VALARM: Reminders](#valarm-reminders) |

An entity whose Meta holds only derived keys emits no `X-TLC-META` at all.

**Unknown `X-TLC-*` properties are preserved, not dropped.** On import, every `X-TLC-*` property that no typed field consumes is parked in `Meta["x_tlc"]` as a map of full (upper-cased) property name → raw string value, and re-emitted verbatim on the next export. This lets a calendar written by a newer tlc survive a round-trip through an older one without losing state.

**Sub-components are walked explicitly.** `ext.ExtensionsByScope` does not recurse into `Component.Sub`, so `VALARM` (and any future nested component) is scanned separately; its `X-TLC-*` properties merge into the parent entity's `x_tlc` bag.

**Non-tlc extensions are ignored, by design.** Extensions owned by another system (`X-APPLE-SORT-ORDER`), by vstar (`X-VSTAR-*`), or experimental (`X-EXP-*`) are neither imported into tlc `Meta` nor echoed back on export. One exception: `X-VSTAR-EFFECTIVE-STATUS` on a well-formed supersession journal is the ledger's fact about the task and is kept as described under [Status transitions as supersession entries](#status-transitions-as-supersession-entries); `X-VSTAR-HASH` is always recomputed, never adopted. This is the receiver rule of spec-vstar's extension discipline — receivers MUST ignore unknown `X-*` properties — see <https://github.com/hop-top/spec-vstar/blob/main/specs/v0.1/04-extensions.md>. Rationale: tlc does not own those namespaces, so it cannot know their semantics, lifetime, or whether round-tripping a stale copy would contradict the owning system. Adopting them into `Meta` would also let a foreign producer inject arbitrary keys into tlc's own model. Preserving them is a defensible future change, but it requires a separate per-system passthrough store, not tlc's `Meta` map.

### `X-TLC-*` property registry

Every `X-TLC-*` property tlc emits, derived from the encoder, the decoder and the known-property table. "Since" is the spec version, with the month the property first shipped in parentheses; a calendar written before that month simply lacks the property and decodes through the fallback named in its row.

| Property | Carried on | Value form | Source field | Since / notes |
|---|---|---|---|---|
| `X-TLC-EFFORT` | VTODO task | Effort name from the configured vocabulary (default `XS` \| `S` \| `M` \| `L` \| `XL`) | `Task.Effort` | 0.1 (2026-05). Omitted when empty. Import: kept only if the name is in the current vocabulary, else dropped |
| `X-TLC-ASSIGNEE` | VTODO task, VTODO track, VEVENT turn | Assignee name, verbatim | `Task.AssignedTo`, `Track.AssignedTo`, the turn's player (`LogEntry.By` of the opening entry, else `Task.AssignedTo`) | 0.1 (2026-05). Task and turn: emitted only when the value is not an email address (an email goes to `ATTENDEE:mailto:` instead). Track: always used; tracks never emit `ATTENDEE`. Import: wins over `ATTENDEE` when both are present; not read from a turn |
| `X-TLC-PROJECT-ID` | VTODO task, VTODO track, VEVENT playthrough | Project identifier, verbatim | `Task.ProjectID`, `Track.ProjectID`, `RecipeRun.ProjectID` | 0.1 (2026-05). Omitted when unset or empty |
| `X-TLC-TASK-SEQ` | VTODO task | Positive base-10 integer | `Task.Seq` | 0.1 (2026-05). Omitted when `Seq <= 0`. Import: an unparsable value is ignored |
| `X-TLC-TRACK-SLUG` | VTODO track | Slug, verbatim | `Track.Slug` | 0.1 (2026-05). Omitted when empty |
| `X-TLC-TRACK-TYPE` | VTODO track | Track type, verbatim (e.g. `feature`) | `Track.Type` | 0.1 (2026-05). Omitted when empty |
| `X-TLC-IS-TRACK` | VTODO track | `TRUE` | (component-kind marker; no model field) | 0.1 (2026-05). Always emitted on a track, never on a task. Import: `TRUE` (case-insensitive) classifies the VTODO as a track unless `X-TLC-CONCEPT:assignment` is present; without it, a UID body that is a track TypeID is the fallback |
| `X-TLC-CONCEPT` | VTODO task, VTODO track, VJOURNAL, VEVENT | `mission` \| `assignment` on a VTODO; `status` \| `decision` \| `action` \| `observation` \| `journal` on a VJOURNAL; `turn` \| `playthrough` on a VEVENT. Lowercase, one token | (derived: `Task.TrackID`, `LogEntry.Action` against the configured status vocabulary; a track is always `mission`; a VEVENT's token is fixed by its builder) | 0.1 (2026-09). Always emitted, never on `VALARM`. Import: read case-insensitively; `assignment` is the first track-vs-task discriminator (see [Vocabulary](#vocabulary)); `turn` gates the `ClaimedAt` read-back; every token is reported by UID in `ParseResult.Concepts`; not stored on the entity, re-derived on export |
| `X-TLC-RUN` | VTODO task | Playthrough UID (`<run-typeid>@<domain>`) | `Task.RunID` | 0.1 (2026-09). Omitted when empty. The assignment → playthrough edge; an X-property, not a `RELATED-TO`, because no `RELTYPE` means run membership (see [VEVENT: Turns and Playthroughs](#vevent-turns-and-playthroughs)). Import: decoded when the UID body is a run TypeID under our domain, else ignored |
| `X-TLC-RECIPE-ID` | VEVENT playthrough | Recipe identifier, verbatim | `RecipeRun.RecipeID` | 0.1 (2026-09). Omitted when empty; also the VEVENT's `SUMMARY`. Never decoded (a playthrough mints no entity) |
| `X-TLC-RECIPE-VERSION` | VEVENT playthrough | Recipe version, verbatim | `RecipeRun.Version` | 0.1 (2026-09). Omitted when empty. Never decoded |
| `X-TLC-RECIPE-HASH` | VEVENT playthrough | Recipe content hash, verbatim (`sha256:…`) | `RecipeRun.Hash` | 0.1 (2026-09). Omitted when empty. Never decoded |
| `X-TLC-LOG-ACTION` | VJOURNAL | Action string, verbatim | `LogEntry.Action` | 0.1 (2026-05). Omitted when empty |
| `X-TLC-LOG-BY` | VJOURNAL | Actor name, verbatim | `LogEntry.By` | 0.1 (2026-05). Omitted when empty. tlc emits no `ORGANIZER` |
| `X-TLC-LOG-TASK` | VJOURNAL | Bare task TypeID (no `@domain`) | `LogEntry.TaskID` | 0.1 (2026-05). Emitted next to the bare `RELATED-TO`. Import: preferred over `RELATED-TO`, so the TypeID survives even when the calendar carries no matching VTODO |
| `X-TLC-PRIORITY` | VTODO task | Priority name from the configured vocabulary (default `P0`–`P3`) | `Task.Priority` | 0.1 (2026-09). Emitted whenever a priority is set, alongside the rank-derived numeric `PRIORITY`. Import: wins when the name is in the current vocabulary; otherwise nearest-rank fallback from `PRIORITY` |
| `X-TLC-PRIORITY-SOURCE` | VTODO task | `manual` \| `derived` | `Task.Meta["priority_source"]` | 0.1 (2026-09). Omitted when absent. Not a derived key, so it also appears inside `X-TLC-META`; the two agree on export, and on import the `X-TLC-META` copy is applied last |
| `X-TLC-PRIORITY-RULE` | VTODO task | Rule name (e.g. `due-soon`) | `Task.Meta["priority_rule"]` | 0.1 (2026-09). Written only next to a `derived` source. Same dual carriage as `X-TLC-PRIORITY-SOURCE` |
| `X-TLC-STATUS` | VTODO task | Status name from the configured vocabulary, verbatim | `Task.Status` | 0.1 (2026-09). Emitted whenever a status is set, alongside the role-mapped `STATUS`. Import: wins (case-insensitive) when the name is in the current vocabulary; otherwise the role implied by `STATUS` selects that role's default status |
| `X-TLC-TRACK-STATUS` | VTODO track | `pending` \| `active` \| `completed` \| `abandoned` \| `archived` | `Track.Status` | 0.1 (2026-09). Distinguishes `completed` from `archived`, which both map to `STATUS:COMPLETED`. Import: case-insensitive match over the closed set, else derived from `STATUS` |
| `X-TLC-ARCHIVED` | VTODO task | `TRUE` | `Task.Archived` | 0.1 (2026-09). Emitted only when true. Import: `TRUE`, `YES` or `1` (case-insensitive) read as true; anything else, or absence, as false |
| `X-TLC-META` | VTODO task, VTODO track, VJOURNAL | JSON object, keys sorted | `Task.Meta`, `Track.Meta`, `LogEntry.Meta` minus the derived keys | 0.1 (2026-09). Omitted when nothing non-derived remains. Import: derived keys inside the payload are ignored. A value that will not marshal is dropped from the payload rather than failing the export |

No `X-TLC-*` property is emitted on `VALARM`. On import, `X-TLC-*` properties found on a `VALARM` (or any other sub-component) are read as the parent entity's, and unknown ones re-emit on the parent VTODO, not inside the alarm.

Names absent from this table are, by definition, unknown. On import they land in `Meta["x_tlc"]` as described above; on export they are re-emitted from that bag, sorted by name, only if the name still classifies as TLC-owned, is not in this table, and holds a string value. The bag is never written into `X-TLC-META`.

**Conformance posture.** Every property in the registry sits in the `X-<SYSTEM>-*` tier of spec-vstar's extension discipline (<https://github.com/hop-top/spec-vstar/blob/main/specs/v0.1/04-extensions.md>), under the system slug `TLC`, and is stable per that tier: removing one is a breaking change for consumers, so a property is promoted or replaced, never renamed in place. tlc defines no `X-VSTAR-*` or `X-EXP-*` property of its own; the two `X-VSTAR-*` properties in its output, `X-VSTAR-HASH` and `X-VSTAR-EFFECTIVE-STATUS`, are minted by the vstar library (`hashing`, `supersession.Supersedes`) rather than by tlc (see the hashing section and [Status transitions as supersession entries](#status-transitions-as-supersession-entries)). Unknown non-TLC `X-*` properties are ignored on import and never re-emitted, per the receiver rule above. Promotion of any `X-TLC-*` property to `X-VSTAR-*` goes through spec-vstar's promotion path — two independent systems implementing compatible semantics — and is recorded there, not here. tlc's self-certification against the spec is written up in [VSTAR-CONFORMANCE.md](VSTAR-CONFORMANCE.md).

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
Emits track as a VTODO (with X-TLC-IS-TRACK marker) plus all member tasks, each carrying a `RELATED-TO` to the track.

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

- **Free-form VEVENT** components (calendar events that are neither a turn nor a playthrough; the two tlc emits are specified in [VEVENT: Turns and Playthroughs](#vevent-turns-and-playthroughs))
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
...
END:VTODO

BEGIN:VTODO
UID:task_01h455vb4pex5vsknk084sn02q@tlc.local
SUMMARY:Add OAuth2 support
RELATED-TO:track_01h455vbqkfsn02nk084ksn02q@tlc.local
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

Output (VJOURNAL; `DTSTAMP` elided — see the hashing section):
```
BEGIN:VJOURNAL
UID:log-task_01h455vb4pex5vsknk084sn02q-status_change-20260502T143000Z@tlc.local
SUMMARY:Moved to done
DESCRIPTION:Moved to done
CREATED:20260502T143000Z
RELATED-TO:task_01h455vb4pex5vsknk084sn02q@tlc.local
X-TLC-LOG-TASK:task_01h455vb4pex5vsknk084sn02q
X-TLC-LOG-ACTION:status_change
X-TLC-LOG-BY:jad
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
- **VALARM UID** is derived from the parent UID with a suffix before
  the domain separator: `task_<id>-alarm@<domain>` for `RemindAt`,
  `task_<id>-alarm-<UTC form #2>@<domain>` for each further reminder,
  `task_<id>-alarm-auto@<domain>` for the auto reminder (see
  [VALARM: Reminders](#valarm-reminders)).

---

<!-- added by track fidelity; merge these rows into the X-prop registry table and the Track mapping when the registry rewrite lands -->

## Addendum: track fidelity

| Property | Entity | Value | Direction | Notes |
|---|---|---|---|---|
| `X-TLC-TRACK-SEQ` | Track | `Track.Seq` as a decimal integer | export + import | Per-project sequence behind the `L-NNNN` alias; omitted when zero. Registered as a typed property, so it never lands in `Meta["x_tlc"]`. |
| `PERCENT-COMPLETE` | Track | terminal members / all members × 100, rounded | export only | The `tlc track` Progress column. Omitted for a track with no member tasks in the export; `0` when none are done. Derived, never decoded. |
| `DUE` | Track | `Track.DueAt`, UTC form #2, or `VALUE=DATE` when `Track.Meta["due_date_only"]` is set | export + import | Same mapping tasks already had, date-only form included. |

`priority_source` and `priority_rule` are excluded from `X-TLC-META`
(they join `blocked_by`, `external_uid` and `x_tlc` in the derived-key
set): each has a typed property, `X-TLC-PRIORITY-SOURCE` and
`X-TLC-PRIORITY-RULE`, and carrying them in the JSON as well wrote them
twice and let the JSON copy win on import.

---

<!-- added by categories wire form; supersedes the `Task.Tags | CATEGORIES | Comma-separated list` row in the Task mapping -->

## Addendum: CATEGORIES wire form

`Task.Tags` is written as **one `CATEGORIES` property per tag**
(`CATEGORIES:security` / `CATEGORIES:auth`), trimmed, empties dropped,
repeats removed keeping first-seen order, case-sensitive. The
comma-joined single property RFC 5545 §3.8.1.2 also allows is not
emitted: the codec escapes every comma in a TEXT value, so it reaches
the wire as `CATEGORIES:security\,auth`, which any RFC 5545 reader
parses as one category. On import both shapes are accepted, and a file
mixing them decodes to the union in wire order.

---

<!-- added by content-based change detection; fold into "The vtodo-sync Plugin" / a future "Sync semantics" section when the registry rewrite lands -->

## Addendum: change detection

Sync decides "changed since last sync" and "in conflict" by content, not
by comparing `UpdatedAt` with `LastSyncAt`.

**Content hash.** A task's sync content hash is `sha256:<hex>` over the
`hop.top/vstar/canonical` bytes of its exported VTODO (built alone,
through the same encoder `tlc export` uses) after dropping the
properties that are not content: `UID`, `X-TLC-TASK-SEQ`,
`X-TLC-PROJECT-ID` (identity), `DTSTAMP`, `LAST-MODIFIED`, `CREATED`,
`COMPLETED` (instants that move without the task moving; `COMPLETED`
falls back to `UpdatedAt` when the log is not supplied) and
`X-VSTAR-HASH`, at every nesting level (a VALARM loses its own `UID`,
`DTSTAMP` and hash too). It is therefore **not** the `X-VSTAR-HASH` on
the wire: that one covers `DTSTAMP`/`LAST-MODIFIED` and moves with
every `UpdatedAt` bump, including saves a remote never sees (a
stale-timeout firing, an executor claim, run/step bookkeeping).

**Baseline.** A successful push or pull records the content hash under
`Task.Meta["last_sync_hash"]` next to `LastSyncAt`. The key is
scrubbed before hashing, so recording it never feeds back into the
hash. It is not in the encoder's derived-key set yet, so it is emitted
inside `X-TLC-META` by a real export; treat it as sync bookkeeping, not
task content.

**Needs push.** With a baseline recorded, a task needs a push iff its
current content hash differs from the baseline, whatever `UpdatedAt`
says. Only while no baseline exists (first sync, or a task last synced
before hashes were kept) does the old rule apply: `UpdatedAt` more than
1 ms after `LastSyncAt`. `sync push` pre-selects candidates in SQL by
timestamp (a superset, since every content change bumps `updated_at`)
and confirms each by hash.

**Conflict.** `DetectConflict(local, remote)` reports a conflict iff:
the local side moved (content hash differs from the baseline; timestamp
rule, with the detector's 1 s slack, only when no baseline exists), the
remote side moved (its `UpdatedAt`, the remote system's own modification
instant, more than 1 s after `LastSyncAt`; no remote baseline hash is
recorded, because the remote representation drops fields it cannot
store), and the two content views are not equal under
`hop.top/vstar/diff.Component`. Two sides that changed to the same
content are not a conflict. A conflict carries `diff.OfComponent` of the
two views (local on the left) so `--strategy manual` shows property-level
detail; `Description` names the differing properties. The four
strategies are unchanged; `last-write-wins` still compares `UpdatedAt`,
that being its definition (recency), not a change heuristic.

**Determinism.** Exporting an unchanged task under two different export
clocks, or none, yields byte-identical canonical bytes, the same
`X-VSTAR-HASH` and the same serialized document
(`TestExport_DeterministicAcrossClocks`). The one boundary: a task with
neither `UpdatedAt` nor `CreatedAt` takes `DTSTAMP` from the export
clock, so its `X-VSTAR-HASH` follows the clock
(`TestExport_TimestamplessTaskIsClockStamped`); the content hash strips
`DTSTAMP` and is unaffected.
