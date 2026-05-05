# vstar codec decision

Supersedes [`2026-05-02-golang-ical-decision.md`](2026-05-02-golang-ical-decision.md).

Context: tlc emits/consumes iCalendar via `internal/vtodo` for the
vtodo-export-sync track. The 2026-05-02 ADR picked
`arran4/golang-ical` as the codec. This ADR replaces that pick with
the org's own `hop.top/vstar` (specifically the Go reference impl
at `github.com/hop-top/vstar/go`).

## Decision

Use **`github.com/hop-top/vstar/go` v0.2+** for VCALENDAR/VTODO/
VJOURNAL serialisation, and **`github.com/hop-top/vstar/go/rrule`**
for RFC 5545 §3.3.10 RRULE parsing/validation/iteration. v0.1 shipped
the codec; v0.2 added the RRULE package this migration also depends
on (vstar T-0125, ADR-0009 + amendment), so the floor is v0.2.

Drop `github.com/arran4/golang-ical` from `go.mod`.

## Rationale

- **Org-owned codec.** vstar is the canonical iCalendar+vCard codec
  for agentic systems in the labspace. Using it here drops a
  third-party dep, matches the kit/hdl pattern (consumers use
  internal codecs), and turns tlc into a proving ground for vstar.
- **Spec-aligned scope.** vstar's RRULE package (per ADR-0009) is
  RFC-correct on COUNT semantics (DTSTART counts as 1 occurrence).
  arran4's parser was lenient on this; tlc's old impl shadowed the
  difference. Migration nudges tlc closer to RFC.
- **Lower-level API, fewer footguns.** vstar exposes
  `Component{Type, Props, Sub}` + `Property{Name, Params, Value}`
  rather than a per-comp-type wrapper. More verbose to build but
  no convenience methods that silently coerce or escape unexpectedly.
- **Test coverage.** vstar ships with conformance fixtures + fuzz
  tests; the BY-* expansion is amendment-tracked in vstar ADR-0009.

## Migration scope (T-1230)

| File | Before | After |
|------|--------|-------|
| `internal/vtodo/encode.go` | `*ics.Calendar` + lib helpers | `vstar.Calendar` + raw `Property` literals; new `Serialize` helper |
| `internal/vtodo/decode.go` | `ics.ParseCalendar` + getters | `rfc5545.Parse` + `Component.Get/GetAll` |
| `internal/core/rrule.go` | `ics.ParseRecurrenceRule` + hand-rolled iterator | `rrule.ParseRRule` + `rrule.NextOccurrence` |
| `internal/cli/formatter.go` | `cal.Serialize()` | `vtodo.Serialize(cal)` |
| `internal/vtodo/encode_test.go` etc | `cal.Serialize()` | `mustSerialize(t, cal)` shim |

## Tests changed

`TestNextFireFromRRule_CountTermination` updated. RFC-correct semantics
now apply: `COUNT=1` from now (DTSTART=now) yields no fire strictly
after now (the single occurrence is DTSTART itself). The old behaviour
fired COUNT+1 times because DTSTART wasn't counted.

## Local replace + upstream gaps

`go.mod` carries `replace github.com/hop-top/vstar/go => …labspace…`
until vstar publishes a go-fetchable tag.

Two upstream gaps surfaced and were filed during migration:

- **vstar T-0125** — generic RRULE parsing for VTODO/VEVENT recurrence
  (filed before migration, now shipped at v0.2 — see vstar/go/rrule/).
- **vstar T-0135** — calendar-level properties beyond VERSION+PRODID
  (`vstar.Calendar` has no `Props` field; `codec/rfc5545.Encode` hard-
  codes only VERSION+PRODID). tlc workaround: `injectCalendarProps`
  post-processes `Encode` output to insert `CALSCALE:GREGORIAN` +
  `METHOD:PUBLISH` after the PRODID line. Drop the workaround when
  T-0135 ships.

## Alternatives considered (this ADR)

- **Stay on `arran4/golang-ical`.** Rejected: third-party lib for a
  responsibility we now own internally.
- **Migrate encode/decode but keep arran4 for RRULE.** Rejected:
  defeats the dog-food goal; vstar T-0125 already shipped a complete
  RRULE package.

## Out of scope (this decision)

- CalDAV client library (Phase 3b).
- Time-zone handling — UTC (`Z` suffix) remains.
- Calendar-level property emission via vstar API (gated on T-0135).
