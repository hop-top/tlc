# vstar codec decision

Supersedes [`2026-05-02-golang-ical-decision.md`](2026-05-02-golang-ical-decision.md).

Context: tlc emits/consumes iCalendar via `internal/vtodo` for the
vtodo-export-sync track. The 2026-05-02 ADR picked
`arran4/golang-ical` as the codec. This ADR replaces that pick with
the org's own `hop.top/vstar` (the Go reference implementation; the
module path is `hop.top/vstar`, packages `hop.top/vstar/rrule` etc.).

## Decision

Use **`hop.top/vstar` v0.2+** for VCALENDAR/VTODO/VJOURNAL
serialisation, and **`hop.top/vstar/rrule`** for RFC 5545 §3.3.10
RRULE parsing/validation/iteration. v0.1 shipped the codec; v0.2 added
the RRULE package this migration also depends on (scope: spec-vstar
`specs/v0.1/03-canonicalization.md`, rule 8 and the section "RRULE
parsing scope"), so the floor is v0.2.

Drop `github.com/arran4/golang-ical` from `go.mod`.

## Rationale

- **Org-owned codec.** vstar is the canonical iCalendar+vCard codec
  for agentic systems in the labspace. Using it here drops a
  third-party dep, matches the kit/hdl pattern (consumers use
  internal codecs), and turns tlc into a proving ground for vstar.
- **Spec-aligned scope.** vstar's RRULE package (scoped by spec-vstar
  `specs/v0.1/03-canonicalization.md`, "RRULE parsing scope") is
  RFC-correct on COUNT semantics (DTSTART counts as 1 occurrence).
  arran4's parser was lenient on this; tlc's old impl shadowed the
  difference. Migration nudges tlc closer to RFC.
- **Lower-level API, fewer footguns.** vstar exposes
  `Component{Type, Props, Sub}` + `Property{Name, Params, Value}`
  rather than a per-comp-type wrapper. More verbose to build but
  no convenience methods that silently coerce or escape unexpectedly.
- **Test coverage.** vstar ships with conformance fixtures + fuzz
  tests; the BY-* expansion is enumerated in that same spec section.

## Migration scope

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

## Upstream gaps

`go.mod` requires `hop.top/vstar` directly; there is no replace
directive.

Two upstream gaps surfaced during migration:

- **Generic RRULE parsing** for VTODO/VEVENT recurrence (raised before
  migration, now shipped at v0.2 — see `hop.top/vstar/rrule`).
- **Calendar-level properties beyond VERSION+PRODID**
  (`vstar.Calendar` has no `Props` field; `codec/rfc5545.Encode` hard-
  codes only VERSION+PRODID). tlc workaround: `injectCalendarProps`
  post-processes `Encode` output to insert `CALSCALE:GREGORIAN` +
  `METHOD:PUBLISH` after the PRODID line. Drop the workaround when
  `vstar.Calendar` gains a property bag.

## Alternatives considered (this ADR)

- **Stay on `arran4/golang-ical`.** Rejected: third-party lib for a
  responsibility we now own internally.
- **Migrate encode/decode but keep arran4 for RRULE.** Rejected:
  defeats the dog-food goal; vstar already ships a complete RRULE
  package.

## Out of scope (this decision)

- CalDAV client library (Phase 3b).
- Time-zone handling — UTC (`Z` suffix) remains.
- Calendar-level property emission via vstar API (gated on
  `vstar.Calendar` gaining a `Props` field).
