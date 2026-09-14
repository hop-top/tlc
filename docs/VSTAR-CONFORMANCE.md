# V* conformance — tlc `internal/vtodo`

Self-certification per spec-vstar `specs/v0.1/05-conformance.md`,
section "Self-certification". This is the v0.1 honor-system substitute
for a conformance suite; every claim below is backed by a test named in
the last section.

## Spec revision targeted

`hop-top/spec-vstar` at `a72a5ab5f83a0aae6e62e565edf34c609915bdcd`
(`specs/v0.1/`). Library: `hop.top/vstar` at the version pinned in
`go.mod`.

## Implementation class

**Emitter + Round-trip.** tlc emits VTODO (tasks, tracks), VJOURNAL (log
entries, status transitions as supersession entries), VEVENT (turns,
playthroughs) and VALARM (reminders) and re-imports what it wrote
byte-identically (`TestFixture_RoundTripStability`,
`TestTurn_ClaimedAtRoundTrips`). It also consumes foreign iCalendar
(`Consumer` behaviour), where the V* extras are optional: a missing
`X-VSTAR-HASH` is reported, never fatal.

## What conforms

- Every emitted component carries `UID`, `DTSTAMP` and `X-VSTAR-HASH`
  (spec 02 "Required common properties"), VALARM included. The hash is
  the last step of every builder, computed over the finished component
  with its sub-components (`TestHash_EveryBuilderVerifies`,
  `TestHash_TrackWithoutMembersVerifies`, `TestHash_TrackWithMetaVerifies`).
- Hashes are `sha256:<hex>` over `hop.top/vstar/canonical` bytes (spec
  03); `X-VSTAR-HASH` itself is excluded (rule 7).
- `RRULE` values are emitted verbatim (rule 8); tlc validates the
  FREQ allow-list before storing, never rewrites the wire form.
- On import every component, nested VALARMs included, is re-verified;
  each failure becomes one entry in `ParseResult.Warnings`
  (`TestParse_WarnsOnMissingHash`, `TestParse_WarnsOnTamperedComponent`,
  `TestParse_WarnsOnTamperedAlarm`).
- Extensions live under the `X-TLC-*` namespace (spec 04); see
  `docs/vtodo-sync-spec-0.1.md`.
- Every VTODO and VJOURNAL declares the agentic concept it represents
  (criterion 4) with one `X-TLC-CONCEPT` token: a track and a
  standalone task are `mission`, a task inside a track is
  `assignment`, a log entry is `status` | `decision` | `action` |
  `observation` | `journal` by its action (the rule and its rationale:
  `docs/superpowers/specs/2026-09-14-agentic-concept-mapping-decision.md`).
  VALARM is single-concept and carries none. On import the token is
  the first discriminator for track-vs-task (`assignment` is always a
  task; `mission` defers to `X-TLC-IS-TRACK`, then the UID prefix) and
  is reported per UID in `ParseResult.Concepts`
  (`TestConcept_TrackIsMission`, `TestConcept_StandaloneTaskIsMission`,
  `TestConcept_TrackedTaskIsAssignment`, `TestConcept_JournalSubTypes`,
  `TestConcept_DecisionPrecedesStatus`, `TestConcept_DecodePrecedence`,
  `TestConcept_RoundTripStable`). The token is system-tier
  (`X-TLC-*`); an `X-VSTAR-CONCEPT` is an upstream ask, promotable once
  a second system implements compatible semantics.
- `CATEGORIES` is written one property per tag. RFC 5545 §3.8.1.2 also
  allows one comma-joined property, but the codec escapes every comma
  in a TEXT value, so that shape reaches the wire as
  `CATEGORIES:security\,auth`, one category to any RFC reader. Both
  shapes are accepted on import (`TestCategories_OnePropertyPerTag`,
  `TestCategories_BackwardCompatMixedShapes`).
- Relations follow spec 02 "Relationship types (RELTYPE)". Every edge
  is encoded once, on the contained component, as a bare `RELATED-TO`:
  task → track, journal → task, turn → task, playthrough → track.
  `RELTYPE` is omitted when its value is `PARENT` (rule 2), and no
  `CHILD` back-reference is written on the track (rule 1); a track's
  membership is read from its members' edges on import, so the track's
  hash no longer moves when a task joins it. `DEPENDS-ON` (RFC 9253) is
  the one explicit `RELTYPE` tlc writes. Values are read through
  `vstar.RelType`, folded case-insensitively upstream, an absent
  parameter reading as `PARENT` (rule 3; `TestRelations_ParentRoundTrip`,
  `TestRelations_NoChildBackReference`, `TestRelations_DependsOnRoundTrip`,
  `TestRelations_MissingRelTypeDefaultsToParent`,
  `TestRelations_RelTypeIsCaseInsensitive`).
- Status transitions are appended as spec 02 supersession entries
  (`hop.top/vstar/supersession`): `journal:status:` UID, bare
  `RELATED-TO`, `CATEGORIES:status-supersession`,
  `X-VSTAR-EFFECTIVE-STATUS`, hash last; the ledger projects with
  `supersession.Superseded` and drives the imported status of a foreign
  VTODO. Scope and the remaining deviations are under "Known
  deviations".
- Turns and playthroughs are VEVENTs (spec 02 "Core mapping"),
  `X-TLC-CONCEPT:turn` / `playthrough`, every one with `DTSTART`
  (spec 05 §5, VS041 never fires: `TestEvent_NoVS041`), `DTEND` only on
  a closed turn, a bare `RELATED-TO` to the assignment (turn) or the
  mission (playthrough), hash last
  (`TestTurn_OpenClaimFromTaskRow`, `TestTurn_WindowsFromLog`,
  `TestPlaythrough_Shape`). The mapping and its sources are in
  `docs/vtodo-sync-spec-0.1.md`, "VEVENT: Turns and Playthroughs".
- `DATE` values follow spec 03 rule 11. A task or track due on a day
  (`Meta["due_date_only"]`) exports `DUE;VALUE=DATE:YYYYMMDD` through
  `SetDUEDate` and is never promoted to a midnight `DATE-TIME`; on
  import `DUE;VALUE=DATE` is read through `DUEDate` (the parameter
  matched case-insensitively) into `DueAt` = midnight UTC of that day
  plus the flag, so the two forms stay distinct across a round trip.
  An untagged eight-octet `DUE` is a malformed `DATE-TIME` to the
  library and decodes to nothing; the hand-rolled midnight fallback
  is gone. `CREATED`, `LAST-MODIFIED`, `DTSTAMP` and a turn's
  `DTSTART` are `DATE-TIME` only (`TestDateOnly_DueRoundTrip`,
  `TestDateOnly_ForeignLowercaseValueParam`,
  `TestDateOnly_DistinctFromMidnightDateTime`,
  `TestDateOnly_UntaggedDateIsNotPromoted`, `TestDateOnly_TrackRoundTrip`,
  `TestDecode_DateOnlyCreatedIsNotPromoted`).
- `TRIGGER` follows spec 03 rule 12 and "TRIGGER conventions". tlc's
  own reminders (`RemindAt`, `Meta["reminders"]`) are absolute:
  `TRIGGER;VALUE=DATE-TIME:<UTC form #2>`, `VALUE` explicit, no
  `RELATED`, through `helpers.NewAbsoluteAlarm`
  (`TestBuildVCalendar_AlarmCarriesUIDAndDTSTAMP`). The derived auto
  reminder (`Task.AutoRemindAt`, twelve hours before due) is relative:
  `TRIGGER;RELATED=END:-PT12H` through `helpers.NewRelativeAlarm`, the
  duration in its authored units, `RELATED=START` never written
  (`TestAutoRemind_EmittedAsRelativeTrigger`). On import every VALARM
  is resolved through `duration.AlarmTrigger` and `Trigger.Resolve`,
  `RELATED` honoured (`DUE` for `END`, `DTSTART` for `START`, the RFC
  default), so the relative triggers Apple Reminders and Thunderbird
  emit are no longer dropped (`TestRelativeTrigger_ResolvesAgainstDue`,
  `TestRelativeTrigger_ResolvesAgainstDtstart`). All VALARMs are kept,
  not the first (`TestReminders_MultipleAlarmsPreserved`); the wire
  shape of the auto reminder is recognised and derived again rather
  than stored (`TestAutoRemind_DecodesWithoutDoubleStoring`). See
  `docs/vtodo-sync-spec-0.1.md`, "VALARM: Reminders".
- `X-TLC-EFFORT` stays an X-property rather than becoming a
  `DURATION`: an effort is a size from a configured vocabulary
  (`XS`…`XL`), not a span of time, and writing it as `PT4H` would
  assert a number of hours nobody entered.

## Validation gate

Every export path is checked with `hop.top/vstar/validate`, the
semantic validator behind spec 05, through `vtodo.ValidateExport`
(`internal/vtodo/validate_gate.go`).

- `validate.Validate` covers the top-level components and never
  descends into `Sub`. `ValidateExport` therefore also calls
  `validate.ValidateComponent` on every nested component at every
  depth, which is what makes the VALARM `UID`/`DTSTAMP`/`X-VSTAR-HASH`
  claim above checked rather than asserted
  (`TestValidateGate_WalksSubComponents`). A nested diagnostic's path
  is anchored under its parent:
  `VCALENDAR.VTODO[uid=a].VALARM[uid=a-alarm].UID`.
- Where it runs: every decode round trip (`roundTrip`), the
  one-of-everything calendar the hash tests share (`fullCalendar`), one
  build per builder path (`TestValidateGate_EveryBuilderPath`), the
  six golden fixtures (`TestFixture_ValidatesUnderGate`) and
  `cmd/genfixtures-vtodo`, which refuses to write a fixture that fails
  it.
- Errors: any `SeverityError` fails the export, except the codes in
  `vtodo.AllowedErrorCodes`, currently `{VS040}`. The set is pinned by
  `TestValidateGate_AllowedErrorCodesPinned`; every entry is a deviation
  recorded below. An allowed diagnostic is logged (test output, generator
  stderr), never dropped. VS052 (malformed `DURATION`, relative
  `TRIGGER` or `REPEAT`) is not allow-listed: every VALARM tlc emits
  goes through the typed `duration` builders, and the gate's walk into
  `Sub` is what would catch a malformed one (`TestValidateGate_EveryBuilderPath`,
  cases "date-only DUE", "several reminders", "auto reminder suppressed").
- Warnings (`SeverityWarning`: VS020 unknown property, VS050 RRULE
  feature outside the vstar evaluator's scope) are logged and never
  fail (`TestValidateGate_WarningsDoNotBlock`). tlc's own extensions
  live under `X-TLC-*` and its RRULEs are emitted verbatim, so a warning
  here is a user's RRULE, not tlc's wire form.

At the pinned library version no builder path and no fixture produces
a blocking diagnostic. VS040 fires on undated open tasks, undated
cancelled tasks and every undated track that is not completed or
archived (`track-with-tasks.ics`); nothing else fires.

## Known deviations

### DTSTAMP is the entity's last-modified instant

RFC 5545 §3.8.7.2 defines `DTSTAMP` as the moment the calendar object
was created, i.e. export time. Spec 03 hashes `DTSTAMP` (only
`X-VSTAR-HASH` is excluded), so an export-time stamp would give
unchanged content a fresh hash on every export and make the hash
useless as a change detector or ETag.

tlc therefore writes `DTSTAMP` = the entity's last modification: a
task's or track's `UpdatedAt` (then `CreatedAt`, then the export clock
for an entity carrying no timestamp), a log entry's own `Timestamp`,
and for a VALARM its parent's stamp. `LAST-MODIFIED` carries the same
instant. Consequence: two exports of unchanged data are byte-identical
(`TestBuildVCalendar_DTSTAMPStableAcrossExports`); a consumer wanting
export time must look at the file, not the components.

### Refinement: what the hash is an ETag for

The DTSTAMP deviation above makes `X-VSTAR-HASH` stable across exports
of unchanged data, so it is a valid ETag for "same bytes". It is not
invariant to a modification-only touch: `DTSTAMP` and `LAST-MODIFIED`
are `UpdatedAt`, and tlc bumps `UpdatedAt` on saves that change nothing
a remote sees (a stale-timeout firing, an executor claim). Sync change
and conflict detection therefore hash a content view of the component
with `UID`, `DTSTAMP`, `LAST-MODIFIED`, `CREATED`, `COMPLETED`,
`X-TLC-TASK-SEQ`, `X-TLC-PROJECT-ID` and `X-VSTAR-HASH` removed at every
level; that hash is recorded at each sync as `Meta["last_sync_hash"]`.
See `docs/vtodo-sync-spec-0.1.md`, "Addendum: change detection". Both
hashes are `sha256:<hex>` over `hop.top/vstar/canonical` bytes; only
their inputs differ. Boundary: a component whose entity carries no
timestamp at all still stamps the export clock, so its `X-VSTAR-HASH`
follows the clock (`TestExport_TimestamplessTaskIsClockStamped`).

### Status transitions are supersession entries; the VTODO still carries its current STATUS

Spec 02 "Status supersession" and conformance point 6 want an
append-only ledger: state changes as superseding VJOURNAL entries. tlc
adopts the encoding through `hop.top/vstar/supersession`: every log
entry in the `status` class (`X-TLC-CONCEPT:status`; the fixed
transition constants and any configured status name) whose task is in
the same export is emitted by `supersession.Supersedes` against the
finished task component, so it carries `UID
journal:status:<task uid>:<t>`, `DTSTAMP` = the log instant, a bare
`RELATED-TO` (RFC 5545 §3.2.15 default, the spec example's shape),
`CATEGORIES:status-supersession` and `X-VSTAR-EFFECTIVE-STATUS`, hashed
last by `finalize` after tlc's own journal properties are added
(`TestSupersession_StatusTransitionShape`,
`TestSupersession_HashLastAfterSupersedes`). `supersession.Superseded`
projects tlc's ledger, latest `DTSTAMP` wins
(`TestSupersession_LatestWinsOnOwnLedger`). On import a VTODO with no
usable `X-TLC-STATUS` (a foreign ledger, or a vocabulary this config no
longer has) takes its status from the latest supersession entry rather
than from its own `STATUS` (`TestSupersession_ForeignLedgerSetsStatus`),
and an imported supersession journal keeps its effective status
through re-export (`TestSupersession_ReExportPreservesEffectiveStatus`).

What still deviates:

- **The VTODO is a snapshot, not an original.** tlc re-exports a task
  with its current `STATUS` (and, for a completed-role status,
  `COMPLETED` + `PERCENT-COMPLETE`) written on the VTODO itself, so
  a re-export overwrites the previous VTODO rather than leaving the
  original untouched. The ledger beside it is complete history; the
  VTODO is the projection tlc's own decoder trusts when
  `X-TLC-STATUS` is present (`TestSupersession_OwnStatusNameBeatsLedger`).
- **Two journal UID schemes coexist.** Supersession entries use the
  spec's `journal:status:<target>:<t>`; every other journal keeps
  `log-<task>-<action>-<t>@<domain>`. A status transition whose task is
  not in the export also keeps the latter: a supersession entry with
  no target in the same calendar is an orphan (spec 05 §4, VS031) and
  unprojectable, so it travels as history
  (`TestSupersession_TargetAbsentKeepsPlainShape`).
- **Tasks only.** `LogEntry.TaskID` is the only journal subject; a
  track has no log, so a track mission's status history is never a
  ledger. `COMPLETED` on a completed or archived track is its last
  modification, there being no completing entry to read it from.
- **Decision journals carry no effective status.** `APPROVED` and
  `REJECTED` record a verdict (`decision` class) even though the
  approval completes the task; they keep the plain shape
  (`TestSupersession_DecisionIsNotALedgerEntry`). Carrying the
  resulting status on them is a follow-on.

### Turns are reconstructed; the run edge is an X-property

Spec 02 maps Turn and Playthrough to VEVENT and leaves the rest to the
system. What tlc does, and where it stops short:

- **A turn's bounds come from the log, not from a turn record.** tlc
  stores only the open claim (`Task.ClaimedAt`); closed turns exist as
  `CLAIMED`/`RECLAIMED` … `DONE`/`RELEASED`/`SKIPPED`/`APPROVED`/
  `REJECTED`/`RETRY`/`BLOCKED` pairs in the task log. With
  `--include-logs` the log is the source and yields every turn; without
  it `ClaimedAt` yields the open one. The two are never combined
  (`TestTurn_LogWinsOverClaimedAt`, `TestTurn_ClaimedAtIsTheFallback`).
  So a document without its log shows at most one turn per task, and a
  turn's `DTEND` is only as exact as the closing entry's timestamp.
- **No VEVENT `STATUS`.** The pinned vstar has no VEVENT status enum;
  open versus closed is carried by the presence of `DTEND`.
- **A playthrough has no `DTEND`.** `RecipeRun` records no end; RFC
  5545 §3.6.1 permits its absence. Deriving one from the run's leaves'
  terminal journals is a follow-on.
- **The assignment → playthrough edge is `X-TLC-RUN`, not
  `RELATED-TO`.** No RELTYPE means "member of a run": `PARENT` is the
  mission edge, `CHILD`/`SIBLING` are hierarchy, `DEPENDS-ON` is
  ordering, and an RFC 5545 reader degrades an unknown RELTYPE to
  `PARENT`, which would re-parent the task under the run
  (`TestPlaythrough_TaskEdge`). Upstream ask: a RELTYPE (or an
  `X-VSTAR-*` relation) for membership in a playthrough. `StepID`,
  `ParentRun` (reconcile lineage) and the subject edge are not exported
  yet.
- **Turns and playthroughs are not decoded into entities.** The only
  read-back is an open turn's `DTSTART` into `ClaimedAt`
  (`TestTurn_ClaimedAtRoundTrips`); a closed turn is history the log
  already holds, and a run row is never minted from a foreign calendar.

### `X-VSTAR-EFFECTIVE-STATUS` vocabulary

The spec defines no vocabulary for the property; its example shows
`COMPLETED`. tlc writes the RFC 5545 VTODO `STATUS` set
(`NEEDS-ACTION` | `IN-PROCESS` | `COMPLETED` | `CANCELLED`), through
the same role projection the task builder uses for `STATUS`: a
configured status name by its role, `CLAIMED` → `IN-PROCESS`,
`RELEASED` / `RETRY` / `REOPENED` / `UNBLOCKED` / `BLOCKED` →
`NEEDS-ACTION`, `DONE` → `COMPLETED`, `SKIPPED` → `CANCELLED`
(`TestSupersession_EffectiveStatusVocabulary`). The tlc status name is
not written there; it is lossy for foreign readers and would make the
ledger and the VTODO speak different languages. On import only these
four values are projected; any other vocabulary is opaque and the
VTODO's own `STATUS` decides
(`TestSupersession_UnknownVocabularyFallsThrough`). Upstream ask: the
spec should say which vocabulary the property carries; a system name
(`X-TLC-STATUS` beside it) is the follow-on if it says "any".

### VALARM UID scheme

RFC 5545 gives VALARM no UID; spec 02 requires one. tlc derives every
alarm UID from the parent by inserting a suffix before the domain
separator, so no state is stored: `task_<id>-alarm@<domain>` for the
`RemindAt` reminder, `task_<id>-alarm-<UTC form #2>@<domain>` for each
further absolute reminder in `Meta["reminders"]` (instants are unique
within a task), and `task_<id>-alarm-auto@<domain>` for the derived
auto reminder. A foreign parent UID without `@` takes the suffix at
its end.

### Reminders are resolved to instants on import

Every VALARM is kept, but the model holds instants: a foreign relative
trigger (`TRIGGER:-PT15M`) is resolved against its anchor on import
and re-exported as an absolute `VALUE=DATE-TIME` trigger, so the
authored duration does not survive a round trip through tlc. The one
relative shape tlc recognises and reproduces is its own auto reminder
(`RELATED=END`, twelve hours, compared by signed length so `-P0DT12H`
reads the same). `RemindAt` is the earliest reminder still in the
future at import time, falling back to the earliest of all when none
is; the rest are `Meta["reminders"]`. A relative trigger against a
date-only anchor, which the library refuses to resolve, is applied to
the day's midnight UTC, the same reading `DueAt` gets. `NoAutoRemind`
is not on the wire: a task exported without an auto reminder because
the flag was set imports with the flag clear (as before this change;
the flag was never exported).

### Undated VTODOs carry no DUE (VS040)

Spec 05 §5 requires a VTODO to carry `DUE`, or `STATUS=COMPLETED`
together with `COMPLETED`. A tlc task or track has no due date unless
someone set one, and an open item without one is the normal case, not
an error. Synthesising a `DUE` from scheduling defaults would export a
date the user never entered, so tlc emits the component without `DUE`
and the validation gate allow-lists the resulting VS040
(`vtodo.AllowedErrorCodes`, `TestValidateGate_UndatedOpenTaskIsAllowedVS040`).

Completed-role tasks are unaffected: they go through `helpers.Complete`
and satisfy the second route. So are completed and archived tracks: the
track builder pairs `STATUS:COMPLETED` with `COMPLETED` (the track's
last modification, a track having no completing log entry;
`TestTrack_CompletedCarriesCompleted`). A cancelled task trips the rule
when undated, the rule having no route for `CANCELLED`; so does an
undated track in any other status. A consumer that needs strict spec 05
§5 must read VS040 on a tlc document as "undated", not "malformed".

## Test artifacts

The golden documents are `tests/fixtures/vtodo/*.ics` (CRLF, stored
verbatim via `.gitattributes`). `TestFixture_HashesVerify` re-verifies
every component of every fixture; `TestFixture_RoundTripStability`
proves byte-identical re-emission.

Every dated task now carries its auto reminder as a nested VALARM
(`TRIGGER;RELATED=END:-PT12H`), so every task VTODO hash moved with
this change; track, journal, turn and playthrough hashes did not. The
VALARM rows are the nested components' own hashes.

| Fixture | Component UID | `X-VSTAR-HASH` |
|---|---|---|
| `single-task.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:06121082115c2816c96b47818760bf0e0bf336c2d8f1bd53a89259eaa3a18016` |
| `single-task.ics` | `task_01h455vb4pex5vsknk084sn02q-alarm-auto@tlc.local` | `sha256:6e0915804164dd58af0d2c142bd0728525a6882da48fa9aa8752f4b4910ee6eb` |
| `recurring-rrule.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:353cc6b39f1445bc222fa31c02c3011b27803dabf54dd63d9688c5d8c1e28635` |
| `recurring-rrule.ics` | `task_01h455vb4pex5vsknk084sn02q-alarm-auto@tlc.local` | `sha256:6e0915804164dd58af0d2c142bd0728525a6882da48fa9aa8752f4b4910ee6eb` |
| `track-with-tasks.ics` | `track_01h455vbqkfsn02nk084ksn02q@tlc.local` | `sha256:47e12299df1fa71afb9f694c1a2d1dab6255c3f86aaf9d28916626bda30bc323` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:2d11aa76cc532c94503da488d53f3ed0f2b1185a2c48c3fc6e1bd177911f6e84` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn02q-alarm-auto@tlc.local` | `sha256:6e0915804164dd58af0d2c142bd0728525a6882da48fa9aa8752f4b4910ee6eb` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0aw@tlc.local` | `sha256:f5bb1d508545a50ac472893075a5da600fb4685d1e88be219a1ae1f1e9e592e9` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0aw-alarm-auto@tlc.local` | `sha256:332fccf229157c0f2063b153a6fdc482213f2a278301e624fc4bf43c030c8145` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0ax@tlc.local` | `sha256:9033ef37be1e0b77dabf0ad9dcfdcffff4a1a09ca248e993adf5d79ab7f43e6e` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0ax-alarm-auto@tlc.local` | `sha256:b100b711f0b2fe057000d244a769cb63882f3921150f54a967848a340dbab1b8` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0az@tlc.local` | `sha256:f9a3af4285281d10744f457d8281bf73f60369dcd27175dce83e2f212c063c0d` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0az-alarm-auto@tlc.local` | `sha256:b06b99396004b515adfef9b23a459a75d56aa937e7f0971fde6464f8183ea241` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0aa@tlc.local` | `sha256:291da43df94c4870f0f96f38ba03b443028807cbf868905116a840b22af5ed3a` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0aa-alarm-auto@tlc.local` | `sha256:0c39ccc94666fd4a022eba5ac3aafbd3cfa74281b4ff6de78e685c810d8254d1` |
| `with-logs.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:06121082115c2816c96b47818760bf0e0bf336c2d8f1bd53a89259eaa3a18016` |
| `with-logs.ics` | `task_01h455vb4pex5vsknk084sn02q-alarm-auto@tlc.local` | `sha256:6e0915804164dd58af0d2c142bd0728525a6882da48fa9aa8752f4b4910ee6eb` |
| `with-logs.ics` | `journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T143000Z` | `sha256:63232af5c32129151c811defad4b7d3dfa30cd65921697646b29eefc76b05a5d` |
| `with-logs.ics` | `log-task_01h455vb4pex5vsknk084sn02q-PROGRESS-20260502T163000Z@tlc.local` | `sha256:73acc64829b5caa498dd52037cc5130f3afcc971b06783dde189c61866314e2f` |
| `with-logs.ics` | `turn-task_01h455vb4pex5vsknk084sn02q-20260502T143000Z@tlc.local` | `sha256:c062e6644313a96da0377f63438b5336564585a7ce7f8cac4737b60274dade09` |
| `recipe-run.ics` | `track_01h455vbqkfsn02nk084ksn02q@tlc.local` | `sha256:30b82a1dd6436c79ebda245c12b52b86da468b82e898270fbddfedc0ed99a08e` |
| `recipe-run.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:7444fbd9401fae9a16675646c588ed8b305918cbd02c9269917aaab85b368cee` |
| `recipe-run.ics` | `task_01h455vb4pex5vsknk084sn02q-alarm-auto@tlc.local` | `sha256:6e0915804164dd58af0d2c142bd0728525a6882da48fa9aa8752f4b4910ee6eb` |
| `recipe-run.ics` | `journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T143000Z` | `sha256:7d79dface6fd31001c197a08726bcf88043d20ca7c8149f9e55f2d756ec59c03` |
| `recipe-run.ics` | `journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T153000Z` | `sha256:8c351916c5dd1cd07a81c0523cf3658688dae5a9a77310876f662c8aa8d6fc33` |
| `recipe-run.ics` | `journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T183000Z` | `sha256:daa122db8b623a43379876afb4124b13948b6b3108354cd063fa5482f7e2e963` |
| `recipe-run.ics` | `turn-task_01h455vb4pex5vsknk084sn02q-20260502T143000Z@tlc.local` | `sha256:f13d75e7d73f402391f8b519cd76dc385b886fb162bb5a8f9012fe6a8999c74c` |
| `recipe-run.ics` | `turn-task_01h455vb4pex5vsknk084sn02q-20260502T183000Z@tlc.local` | `sha256:b25b1751d177cab6020c4533dce090cf20911c4b9a52ae2e3e9d00aa0f378d67` |
| `recipe-run.ics` | `run_01h455vb4pex5vsknk084sn0r1@tlc.local` | `sha256:a1d21318374d2205fc7f371587380c519673d9b2c1f57d1109804689e0f7d330` |

### Regenerating the golden hashes

Any change to the emitted wire form (a new property, a mapping change)
moves the hashes. Regenerate, then refresh the table:

```sh
go run ./cmd/genfixtures-vtodo
for f in tests/fixtures/vtodo/*.ics; do
  echo "== $f"
  tr -d '\r' < "$f" \
    | awk '/^ /{buf=buf substr($0,2); next} {if (buf!="") print buf; buf=$0} END{print buf}' \
    | awk '/^UID:/{u=substr($0,5)} /^X-VSTAR-HASH:/{print u " | " substr($0,14)}'
done
go test ./internal/vtodo/ -run 'TestFixture_'
```

The generator writes CRLF; commit the files as written. It refuses to
write a fixture that fails the validation gate (exit 1, findings on
stderr) and prints allowed and warning diagnostics to stderr. The
round-trip and verify tests fail loudly if a fixture and the encoder
disagree. The first awk unfolds RFC 5545 continuation lines (a
supersession `UID` and every hash fold), so nested VALARM rows come
out under their own `UID`.

<!-- added by content-based change detection; belongs under "DTSTAMP is the entity's last-modified instant" -->
