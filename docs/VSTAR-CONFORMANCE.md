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
| `single-task.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:9998f7ca8bdf3b38085b0db400921a678f26773c82b2a6f01b48a7354897ba01` |
| `recurring-rrule.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:f58e81296facddd51e131fbb43c36a4bd47fc39c50aed8562cfaebc34d1a812a` |
| `track-with-tasks.ics` | `track_01h455vbqkfsn02nk084ksn02q@tlc.local` | `sha256:f235a15ee893d1cbf83507630025a3133717a1f39beb7f70899128bcd9a5b301` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:f7c6b44790118afbcae365ab559899cd60d1afa6057b140f5c7b880d96cae52c` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0aw@tlc.local` | `sha256:76d03ab3d85134d0ab690cb1bcf372fba4119cf3a07525eaaa576e9b66187a8d` |
| `track-with-tasks.ics` | `task_01h455vb4pex5vsknk084sn0ax@tlc.local` | `sha256:6e2adfe2b7bdaf58ff64b3eda98d25ed8750f01ad3c5209b0c302585d4f7bb75` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0az@tlc.local` | `sha256:bc023aeeb250754915abbf5fc6208d0b4889e3f057fdca2fe435cbd0a3c8c3a2` |
| `with-dependencies.ics` | `task_01h455vb4pex5vsknk084sn0aa@tlc.local` | `sha256:ac6547f7db4ae3063e70ba126a4f21b3d9d50147e0cdd5b8e429843bdf687c62` |
| `with-logs.ics` | `task_01h455vb4pex5vsknk084sn02q@tlc.local` | `sha256:9998f7ca8bdf3b38085b0db400921a678f26773c82b2a6f01b48a7354897ba01` |
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
