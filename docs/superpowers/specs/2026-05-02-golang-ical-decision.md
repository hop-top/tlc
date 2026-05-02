# golang-ical library decision

Context: `2026-05-02-vtodo-export-sync-design.md` calls for an iCalendar
parser/emitter. First task on the vtodo-export-sync track.

## Decision

Use **`github.com/arran4/golang-ical` v0.3.5**.

## Rationale

- **Most-used Go iCalendar lib**: dominant choice in the Go ecosystem
  for VCALENDAR/VTODO/VEVENT serialisation.
- **RFC 5545 coverage**: VCALENDAR envelope, VTODO/VEVENT/VJOURNAL
  components, `RELATED-TO` with parameters (RELTYPE), RRULE, VALARM,
  ATTENDEE, ORGANIZER, CATEGORIES, X-properties.
- **Active**: tagged v0.3.5 with commits in the last 6 months.
- **Permissive license**: Apache 2.0.
- **Clean API**: `NewCalendar()`, `ParseCalendar(io.Reader)`,
  `Calendar.AddVTodo(uid)`, property accessors via
  `SetProperty/GetProperty`. No reflection, no codegen.

## RFC 9253 caveat

`RELTYPE=DEPENDS-ON` and the temporal types (`FINISHTOSTART` etc.)
are not yet first-class enums in the library. They round-trip
through `RELATED-TO` as raw `RELTYPE` parameter strings — which is
spec-compliant: RFC 5545 §3.2.15 allows arbitrary IANA-registered
or X- values in RELTYPE, and we control both producer and consumer.
Use string constants in our `internal/vtodo` package for the
parameter values; revisit if the upstream lib adds first-class
enums later.

## Pinned version

`github.com/arran4/golang-ical v0.3.5`.

## Alternatives considered

- `github.com/teambition/rrule-go` — RRULE-only, no full iCalendar
  envelope. Could combine with a manual VCALENDAR emitter, but
  doubles the surface area.
- `github.com/laurirad/icalparser` — focused on parsing; no emit
  side. Smaller user base, less battle-tested.
- Hand-roll over byte buffers — RFC 5545 line folding (75 octets,
  CRLF + space continuation) and parameter quoting are non-trivial
  to get right. Reject.

## Sample

```go
import ics "github.com/arran4/golang-ical"

cal := ics.NewCalendar()
cal.SetProductId("-//tlc//vtodo//EN")

todo := cal.AddVTodo("task_01h455vb4pex5vsknk084sn02q@tlc.local")
todo.SetSummary("Replace JWT signer")
todo.SetDescription("Rotate to ES256 across services")
todo.SetCreatedTime(time.Now().UTC())
todo.SetDtStampTime(time.Now().UTC())
todo.SetProperty("RELATED-TO", "auth-rewrite@tlc.local",
    ics.WithParameter("RELTYPE", "PARENT"))

out := cal.Serialize()  // valid .ics text
```

## Out of scope (this decision)

- Choice of CalDAV client library (Phase 3b — separate decision).
- Time-zone handling — start with UTC (`Z` suffix); revisit only if
  a real consumer needs floating local time.
