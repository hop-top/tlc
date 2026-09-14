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
entries) and VALARM (reminders) and re-imports what it wrote
byte-identically (`TestFixture_RoundTripStability`). It also consumes
foreign iCalendar (`Consumer` behaviour), where the V* extras are
optional: a missing `X-VSTAR-HASH` is reported, never fatal.

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
  five golden fixtures (`TestFixture_ValidatesUnderGate`) and
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
cancelled tasks and every undated track (`track-with-tasks.ics`);
nothing else fires.

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

### In-place STATUS mutation on re-export

Spec 02 "Status supersession" and conformance point 6 want an
append-only ledger: state changes as superseding VJOURNAL entries. tlc
re-exports a task with its current `STATUS` (and, for a completed-role
status, `COMPLETED` + `PERCENT-COMPLETE`) written on the VTODO itself,
so a re-export overwrites the previous state rather than appending a
supersession record. tlc's own log entries are exported as VJOURNALs
(`--include-logs`) and do describe each transition, but they do not use
`X-VSTAR-EFFECTIVE-STATUS`. Deviation stands until supersession lands
in `hop.top/vstar` and tlc adopts it.

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
and satisfy the second route. A cancelled task trips the rule when
undated, the rule having no route for `CANCELLED`. So does an undated
track in any status, a completed one included, because the track
builder writes `STATUS:COMPLETED` without `COMPLETED`. A consumer that
needs strict spec 05 §5 must read VS040 on a tlc document as "undated",
not "malformed".

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
| `with-logs.ics` | `log-task_01h455vb4pex5vsknk084sn02q-CLAIMED-20260502T143000Z@tlc.local` | `sha256:4d79ed354448d38f4211e8a084fe527932fefafa371a94102385522de8bfc4e7` |
| `with-logs.ics` | `log-task_01h455vb4pex5vsknk084sn02q-PROGRESS-20260502T163000Z@tlc.local` | `sha256:140c79a5a4a06d152d3421e3ff3eb95d4103156fdea7817353e1fe3cd8c65fe7` |

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
disagree.

The generator writes CRLF; commit the files as written. The round-trip
and verify tests fail loudly if a fixture and the encoder disagree.

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
