# V* conformance — tlc `internal/vtodo`

Self-certification per spec-vstar `specs/v0.1/05-conformance.md`,
section "Self-certification". This is the v0.1 honor-system substitute
for a conformance suite; every claim below is backed by a test named in
the last section.

## Spec revision targeted

`hop-top/spec-vstar` at `4d73c0107f0ea256d3aedae5ded42bb11463c6b9`
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
  a closed turn, `RELATED-TO;RELTYPE=PARENT` to the assignment
  (turn) or the mission (playthrough), hash last
  (`TestTurn_OpenClaimFromTaskRow`, `TestTurn_WindowsFromLog`,
  `TestPlaythrough_Shape`). The mapping and its sources are in
  `docs/vtodo-sync-spec-0.1.md`, "VEVENT: Turns and Playthroughs".

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
  stderr), never dropped.
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

### CHILD back-references

Spec 02 names `RELATED-TO` as the relationship mechanism without
prescribing direction. tlc emits both directions: each member task
carries `RELATED-TO;RELTYPE=PARENT:<track>` and the track carries one
`RELATED-TO;RELTYPE=CHILD:<task>` per member. The CHILD rows make a
track's hash depend on its membership, so adding a task to a track
changes the track's `X-VSTAR-HASH` even though no track field changed.
Deliberate: membership is track content for tlc's purposes.

### VALARM UID scheme

RFC 5545 gives VALARM no UID; spec 02 requires one. tlc derives it from
the parent: the parent UID with `-alarm` inserted before the domain
separator (`task_<id>-alarm@<domain>`). One reminder per task keeps it
unique and stable across exports without stored state.

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

| Fixture | Component UID | `X-VSTAR-HASH` |
|---|---|---|
| `single-task.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:020b85e5ff1b89955052df9617d8b912b5dce0a1c7010f22a8f43f47fe28ab1d` |
| `recurring-rrule.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:eb8ba05c6c89d6f83ca320afe70660c3e30a00c2acbdd98bf70aeab747b89b1a` |
| `track-with-tasks.ics` | `track_01h455vbqkfsn02nk084ksn02q@tlc.local` | `sha256:1336f5df6af964db54fac51a1d78d43030d283ebcc10727e5d8fc1676f30d0c0` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:bba4727ecaeb5e7eb6b124c1b784de1519e6783dd0c2f22a26e1a9b9cd800df2` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0aw@tlc.local` | `sha256:7f0633fde59e28f118e8865e25518373475c84f32c55109721cfa0f5f13fb632` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0ax@tlc.local` | `sha256:19e3c19205e10c776126000554b07e45250ed4875889672b1dd4570290395556` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0az@tlc.local` | `sha256:d1b30190e0c98ac1bb870c266608287afeebe9b51db935fe28779c27fe49adc8` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0aa@tlc.local` | `sha256:70ef6e34c9f03e55130943e2099541d893e3224994547e724f00f3bd606d1770` |
| `with-logs.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:020b85e5ff1b89955052df9617d8b912b5dce0a1c7010f22a8f43f47fe28ab1d` |
| `with-logs.ics` | `journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T143000Z` | `sha256:63232af5c32129151c811defad4b7d3dfa30cd65921697646b29eefc76b05a5d` |
| `with-logs.ics` | `log-task_01h455vb4pex5vsknk084sn02q-PROGRESS-20260502T163000Z@tlc.local` | `sha256:140c79a5a4a06d152d3421e3ff3eb95d4103156fdea7817353e1fe3cd8c65fe7` |
| `with-logs.ics` | `turn-task_01h455vb4pex5vsknk084sn02q-20260502T143000Z@tlc.local` | `sha256:7bd1d3f34f76da79d78529ebea1d8e335257219aed6985593cf7a80b7105a084` |
| `recipe-run.ics` | `track_01h455vbqkfsn02nk084ksn02q@tlc.local` | `sha256:e9c19cf8e596c158db6719835bcf34bfd6f76dd1d02cad42cc02451900cd3f41` |
| `recipe-run.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:e6bc99da99a21b886116112964464381c34b6f1be597f3f5be63f1653ad42202` |
| `recipe-run.ics` | `journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T143000Z` | `sha256:7d79dface6fd31001c197a08726bcf88043d20ca7c8149f9e55f2d756ec59c03` |
| `recipe-run.ics` | `journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T153000Z` | `sha256:8c351916c5dd1cd07a81c0523cf3658688dae5a9a77310876f662c8aa8d6fc33` |
| `recipe-run.ics` | `journal:status:task_01h455vb4pex5vsknk084sn02q@tlc.local:20260502T183000Z` | `sha256:daa122db8b623a43379876afb4124b13948b6b3108354cd063fa5482f7e2e963` |
| `recipe-run.ics` | `turn-task_01h455vb4pex5vsknk084sn02q-20260502T143000Z@tlc.local` | `sha256:5ecaa93f00500c2b431f0072e7f5450ba22be8c48f822613cc9b9c29292e45d8` |
| `recipe-run.ics` | `turn-task_01h455vb4pex5vsknk084sn02q-20260502T183000Z@tlc.local` | `sha256:e9596b9c1104c74b605ac4f681706d95795d8c713632074cd5ed594585a07871` |
| `recipe-run.ics` | `run_01h455vb4pex5vsknk084sn0r1@tlc.local` | `sha256:89383c79e71854f7a463626fb24d570b5a5c5796b8f10a02148dbcdd3ab055f9` |

### Regenerating the golden hashes

Any change to the emitted wire form (a new property, a mapping change)
moves the hashes. Regenerate, then refresh the table:

```sh
go run ./cmd/genfixtures-vtodo
for f in tests/fixtures/vtodo/*.ics; do
  echo "== $f"
  tr -d '\r' < "$f" | awk '/^UID:/{u=$0} /^X-VSTAR-HASH:/{h=$0; getline n; if (n ~ /^ /) h=h substr(n,2); print u " | " h}'
done
go test ./internal/vtodo/ -run 'TestFixture_'
```

The generator writes CRLF; commit the files as written. It refuses to
write a fixture that fails the validation gate (exit 1, findings on
stderr) and prints allowed and warning diagnostics to stderr. The
round-trip and verify tests fail loudly if a fixture and the encoder
disagree. A folded `UID` (the supersession scheme is long) spans two
physical lines; the awk above reads only the first, so join the
continuation by hand or compare on the parsed component.

<!-- added by content-based change detection; belongs under "DTSTAMP is the entity's last-modified instant" -->

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
