# VTODO export, RRULE recurrence, and generic vtodo sync

Status: design — depends on `2026-05-02-typeid-ids-design.md`.

## Problem

tlc has no iCalendar interop. Users can't export tasks to Apple Reminders, Things, Thunderbird, Tasks.org, or any CalDAV server. The current schedule field (`Task.RemindEvery time.Duration`) can express only fixed intervals — not "every weekday", "first Monday of month", or "every 2 weeks until June".

## Goals

1. Emit RFC 5545–compliant VTODO output (`tlc task list --format vtodo`, `tlc track show --format vtodo`).
2. Encode hierarchy via `RELATED-TO` with `RELTYPE=PARENT`/`CHILD` (RFC 5545) and dependencies via `RELTYPE=DEPENDS-ON` (RFC 9253).
3. Replace `RemindEvery` with `RRule` (RFC 5545 §3.3.10) for recurrence.
4. Ship a generic `vtodo-sync` plugin (sibling of `github-sync`) that pulls/pushes against `.ics` files or CalDAV endpoints.

## Non-goals

- Two-way RRULE expansion into individual VTODO occurrences (we emit the rule, let the consumer expand).
- VEVENT, VJOURNAL, VFREEBUSY components.
- Calendar invitations, attendees, scheduling messages (RFC 5546).

## Dependencies

- TypeID identifiers landed (separate spec). Vtodo UID = `<typeid>@<domain>`; URI = `tlc://...` in `URL` property.
- Library: `github.com/arran4/golang-ical` for parse/emit. Vetted as maintained, RFC 5545–compliant, supports `RELATED-TO` and RRULE.

## Phase 1 — VTODO export (`--format vtodo`)

### CLI

```
tlc task list --format vtodo
tlc task show <id> --format vtodo
tlc track show <id> --format vtodo --recursive
tlc task list --format vtodo --output tasks.ics
```

Wire vtodo as another option in `internal/cli/log.go`'s format constants and the renderer dispatch in `task_list.go` / `track_show.go`.

### VTODO mapping

| tlc field | VTODO property |
|---|---|
| `Task.ID` (typeid) | `UID:<id>@<domain>` |
| `Task.Reference` | `URL:<uri>` (e.g. `tlc://project/task_01h...`) |
| `Task.Title` | `SUMMARY` |
| `Task.Description` | `DESCRIPTION` |
| `Task.Status` | `STATUS` (TODO→NEEDS-ACTION, IN_PROGRESS→IN-PROCESS, DONE→COMPLETED, SKIPPED→CANCELLED) |
| `Task.Priority` | `PRIORITY` (P0→1, P1→3, P2→5, P3→7) |
| `Task.AssignedTo` | `ATTENDEE;CN=<name>:mailto:<addr>` (if profile resolves an email; else `X-TLC-ASSIGNEE`) |
| `Task.Tags` | `CATEGORIES:tag1,tag2,...` |
| `Task.CreatedAt` | `CREATED` and `DTSTAMP` |
| `Task.UpdatedAt` | `LAST-MODIFIED` |
| `Task.DueAt` | `DUE` |
| `Task.RemindAt` | `VALARM` block with `TRIGGER` |
| `Task.RRule` (new) | `RRULE` |
| `Task.TrackID` | `RELATED-TO;RELTYPE=PARENT:<track-uid>@<domain>` |
| `Task.Effort` | `X-TLC-EFFORT:XS\|S\|M\|L\|XL` |
| `Task.Archived` | omit row when archived (configurable via `--archived`) |

Track export emits a VTODO with `RELATED-TO;RELTYPE=CHILD` for each member task.

### VCALENDAR envelope

```
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//tlc//vtodo//EN
CALSCALE:GREGORIAN
METHOD:PUBLISH
…components…
END:VCALENDAR
```

### Hierarchy rules

- Track ↔ task only (matches current schema with `Task.TrackID`).
- No `RELTYPE=SIBLING` — siblings are inferable from shared parent and unknown RELTYPEs degrade to PARENT in older clients (per RFC 5545).
- Subtask hierarchy (task ↔ task) deferred until/if `Task.ParentID` is added.

### Dependencies

If `Task.BlockedBy []string` exists (or future): emit `RELATED-TO;RELTYPE=DEPENDS-ON:<blocker-uid>@<domain>` on the blocked task. RFC 9253. Skipped if no blocked-by data is modeled.

## Phase 2 — RRULE replaces `RemindEvery`

### Schema

`Task.RemindEvery *time.Duration` → **deleted**.
`Task.RRule string` → added (empty when not recurring).

Database column rename: `remind_every` → `rrule TEXT`. Existing data is disposable.

### Validation

On `task create`/`task update`:

```go
import ical "github.com/arran4/golang-ical"
_, err := ical.ParseRecurrenceRule(input)  // or equivalent helper
```

Reject malformed RRULE strings at the CLI boundary with a clear error.

### CLI

```
tlc task create --rrule "FREQ=DAILY;INTERVAL=2"
tlc task create --rrule "FREQ=WEEKLY;BYDAY=MO,WE,FR;UNTIL=20260601T000000Z"
```

Removes the `--remind-every <duration>` flag.

### Reminder firing

The existing reminder loop currently uses `RemindEvery` to schedule the next fire. Replace with: parse RRULE, compute `Next(now)` via the library, set `RemindAt`. Document the new behavior in the reminder runbook.

## Phase 3 — `vtodo-sync` plugin

Mirrors `plugins/github-sync` shape — JSON-RPC over stdio, `manifest.yaml`, `mapper.go`, `mapper_test.go`, `main.go`.

### Manifest

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
      description: "file:///path/to/cal.ics or https://caldav.example.com/cal/"
      required: true
    auth:
      type: object
      properties:
        type:
          type: string
          enum: [none, basic, bearer]
        username: {type: string}
        password_ref: {type: string}  # secret reference, not literal
    sync_direction:
      type: string
      enum: [pull, push, bidirectional]
      default: bidirectional

permissions:
  - network.http
  - filesystem.read
  - filesystem.write
  - credential.read
```

### RPC methods

- `sync.pull` — read source, parse VCALENDAR, return a list of normalized task records (using the same `Task` shape the github-sync plugin returns).
- `sync.push` — receive task list, emit VCALENDAR back to the source. CalDAV: PUT each VTODO as a separate calendar object resource (per CalDAV §4.1, sibling VTODOs become separate resources keyed by UID). File: write the whole VCALENDAR atomically.
- `sync.delete` — same pattern as github-sync.

### Mapper

`mapper.go` translates between tlc `Task` and `golang-ical` VTODO using the field map from Phase 1. `mapper_test.go` covers each field round-trip plus a fixture-based golden file test.

### CalDAV vs. file mode

- **File mode** (`source: file://...`) — atomic read/write of one `.ics`. All VTODOs in a single VCALENDAR. Easy to test, no auth.
- **CalDAV mode** (`source: https://...`) — `PROPFIND` to list resources, `GET` per object, `PUT`/`DELETE` for writes. RFC 4791. ETags for optimistic concurrency.

Phase 3a ships file mode; CalDAV is Phase 3b.

## Round-trip stability

UIDs survive export → import. On import:
1. Parse VTODO `UID`. If `<typeid>@<domain>` form, extract typeid.
2. If a tlc row with that ID exists, update; else create.
3. `URL` property → `Task.Reference`.
4. RRULE → `Task.RRule`.
5. `RELATED-TO;RELTYPE=PARENT` → `Task.TrackID` (after resolving the parent UID).

External UIDs (e.g. from Apple Reminders) that don't match tlc typeid pattern: store the UID in `Meta["external_uid"]` and generate a fresh tlc typeid for the new row. Re-export uses the tlc typeid; the `Meta` value preserves the original for round-trip with the source system.

## Affected components

| Path | Change |
|---|---|
| `internal/core/models.go` | drop `RemindEvery`, add `RRule string` |
| `internal/core/scheduling.go` | reminder firing uses RRULE |
| `internal/cli/log.go` | `formatVtodo` constant |
| `internal/cli/task_list.go`, `task_show.go` | dispatch `--format vtodo` |
| `internal/cli/track_show.go` | dispatch `--format vtodo` with task expansion |
| `internal/cli/task_create.go`, `task_update.go` | `--rrule` flag, validation |
| `internal/vtodo/` (new package) | encode/decode VTODO, mapping helpers |
| `plugins/vtodo-sync/` (new) | manifest, main, mapper, tests |
| `docs/vtodo-sync-spec.md` (new) | user-facing docs |

## Testing

### Unit
- `internal/vtodo/encode_test.go` — every field maps correctly; round-trip preserves data.
- `internal/vtodo/decode_test.go` — known-good fixtures parse; malformed RRULE rejected.
- Hierarchy: track + 3 tasks → CALENDAR has 4 VTODOs with PARENT/CHILD wired both ways.

### Golden files
- `tests/fixtures/vtodo/single-task.ics`
- `tests/fixtures/vtodo/track-with-tasks.ics`
- `tests/fixtures/vtodo/recurring-rrule.ics`
- `tests/fixtures/vtodo/with-dependencies.ics`

### Plugin
- `plugins/vtodo-sync/mapper_test.go` — same coverage shape as `github-sync/mapper_test.go`.
- E2E: file-mode round-trip in a temp dir.

### Interop check
- Manual smoke: import generated `.ics` into Apple Reminders and Thunderbird/Tasks.org; verify titles, due dates, recurrence visible.

## Out of scope (now)

- VEVENT/VJOURNAL.
- Subtasks (`Task.ParentID`).
- `RELTYPE=SIBLING` rows.
- Temporal RFC 9253 RELTYPEs (`FINISHTOSTART`, etc.) — add if a real consumer needs scheduling semantics.
- CalDAV mode (Phase 3b).

## Open questions

- VALARM defaults: should `RemindAt` always emit a VALARM, or only when set explicitly? Lean: only when set.
- Time zones: emit `DTSTART`/`DUE` in UTC (`Z` suffix) or floating local? Lean: UTC for portability, since tlc stores `time.Time` in UTC.
- `Meta["external_uid"]` shape — single string vs. map keyed by source? Defer until second non-tlc source materializes.
