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
- `CATEGORIES` is written one property per tag. RFC 5545 §3.8.1.2 also
  allows one comma-joined property, but the codec escapes every comma
  in a TEXT value, so that shape reaches the wire as
  `CATEGORIES:security\,auth`, one category to any RFC reader. Both
  shapes are accepted on import (`TestCategories_OnePropertyPerTag`,
  `TestCategories_BackwardCompatMixedShapes`).

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

## Test artifacts

The golden documents are `tests/fixtures/vtodo/*.ics` (CRLF, stored
verbatim via `.gitattributes`). `TestFixture_HashesVerify` re-verifies
every component of every fixture; `TestFixture_RoundTripStability`
proves byte-identical re-emission.

| Fixture | Component UID | `X-VSTAR-HASH` |
|---|---|---|
| `single-task.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:b5e12576d9503b1d17d4ec138b2e4f6a602a77adeec00f1e5a1ae89f99df0067` |
| `recurring-rrule.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:86b46808ac74160e25a57070182f1e61b2323534682641227af948587bdb75f3` |
| `track-with-tasks.ics` | `track_01h455vbqkfsn02nk084ksn02q@tlc.local` | `sha256:28b04785cefafd1cb14de98b89741ed6817621fcee941ea5999d07e645362f5c` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:ce0a7079d9e5bc11f71cf0aba92d4101ba794b14ebcd510747c11d44d20996fe` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0aw@tlc.local` | `sha256:1429814ae7199227563a6bb0019def929bbf19d1ee90b48a865852c0b58d67fb` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0ax@tlc.local` | `sha256:327d85f29ecc64158b5b90932824451cb09d8e6609d8bb48de8054433279b334` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0az@tlc.local` | `sha256:c702770d9ab281ba5fbea5e68fce1d6045e3f72c2e0fd0c831c32cdca5fe6061` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0aa@tlc.local` | `sha256:3aebe770175a4ecb11258f04df2dc0c1c8aa7f9ea180921ef5c5de3cb0e8ecbe` |
| `with-logs.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:b5e12576d9503b1d17d4ec138b2e4f6a602a77adeec00f1e5a1ae89f99df0067` |
| `with-logs.ics` | `log-task_01h455vb4pex5vsknk084sn02q-CLAIMED-20260502T143000Z@tlc.local` | `sha256:073b2ed39a194130c663b93b7d0fde145bffbd4741a39bbbd15209954144e072` |
| `with-logs.ics` | `log-task_01h455vb4pex5vsknk084sn02q-PROGRESS-20260502T163000Z@tlc.local` | `sha256:68f03b1720322eb2f6fdacfb4cd0d4e934827f3c8f6aa04822bebd2fd8fb3f6c` |

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
