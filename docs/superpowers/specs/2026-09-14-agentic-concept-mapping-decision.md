# agentic concept mapping decision

Context: `internal/vtodo` exports tlc as V\* (iCalendar) per
[`docs/vtodo-sync-spec-0.1.md`](../../vtodo-sync-spec-0.1.md). That
spec maps field → property by shape: a `Task` becomes a VTODO because
a VTODO has the right properties, a `Track` becomes a VTODO for the
same reason. The V\* spec ([hop-top/spec-vstar](https://github.com/hop-top/spec-vstar),
`specs/v0.1/02-component-mapping.md`, "Core mapping") instead maps
*agentic concepts* to components — Mission → VTODO, Assignment →
VTODO, Turn → VEVENT, Playthrough → VEVENT, Action, Observation,
Decision and the rest → VJOURNAL — and conformance criterion 4
(`specs/v0.1/05-conformance.md`) requires "the correct component
types for the agentic concepts it represents". Shape alone cannot
satisfy that: two VTODOs with the same properties are a Mission and an
Assignment, and nothing on the wire says which.

This ADR records which concept each tlc entity *is*, and how the
export declares it. Spec revision read: spec-vstar `4d73c01`. Concept
definitions come from hop-top/agr `GLOSSARY.md` — spec 02 defines
none of the terms in its table.

## Decision

Each exported component declares its agentic concept with one
`X-TLC-CONCEPT:<token>` property. The wire shape does not change;
what changes is that a VTODO says whether it is a `mission` or an
`assignment`, and a VJOURNAL says what kind of journal it is.

| tlc entity | V\* concept (agr glossary) | Component (spec 02) | Declared as | Exported today |
|---|---|---|---|---|
| project | World — "a temporal ledger container" | VCALENDAR | — (the envelope) | yes: one VCALENDAR per export, `X-TLC-PROJECT-ID` per component |
| `Track` | Mission — "a goal or obligation" | VTODO | `X-TLC-CONCEPT:mission` | yes, with `X-TLC-IS-TRACK:TRUE` |
| `Task`, `TrackID` nil | Mission | VTODO | `X-TLC-CONCEPT:mission` | yes; carries no PARENT edge |
| `Task`, `TrackID` set | Assignment — "a scoped unit of work inside a mission" | VTODO | `X-TLC-CONCEPT:assignment` | yes, with `RELATED-TO;RELTYPE=PARENT` → its mission |
| `LogEntry` | Journal, sub-typed (below) | VJOURNAL | `X-TLC-CONCEPT:status` \| `decision` \| `action` \| `observation` \| `journal` | yes, opt-in (`--include-logs`); a `status` entry whose task is exported is a spec 02 supersession entry |
| `RecipeRun` | Playthrough — "a branch/run/path through a mission or world" | VEVENT | `X-TLC-CONCEPT:playthrough` | no — follow-on |
| claim window | Turn — "a bounded execution window" | VEVENT | `X-TLC-CONCEPT:turn` | no — follow-on |
| `Task.AssignedTo`, `Track.AssignedTo`, `LogEntry.By` | Player | VCARD | — (VCARD is unambiguous) | no VCARD; `ATTENDEE` / `X-TLC-ASSIGNEE` / `X-TLC-LOG-BY` carry the name |
| `Task.RRule` | Cadence — "a recurring temporal rhythm, usually RRULE" | `RRULE` property | — (a property, not a component) | yes |
| `Task.RemindAt` | Timeout / escalation | VALARM | — (VALARM is unambiguous) | yes |

Tokens are lowercase, compared case-insensitively on read. One token
per component: a component is exactly one thing.

## Track → Mission, Task → Assignment

- **Track is the goal; tasks are the scoped units inside it.**
  `Track` (`internal/core/track.go`) is "a grouping of related tasks
  toward a deliverable"; `Task.TrackID *string`
  (`internal/core/models.go`) is the membership edge and the *only*
  parent link on a task — there is no `ParentID`, and the sync spec
  lists task ↔ task subtasks as out of scope. So the containment the
  glossary describes (assignment inside mission) is exactly the
  containment tlc stores.
- **Both stay VTODO.** Spec 02 maps Mission and Assignment to the same
  component; the encoder already emits both as VTODO with
  `RELATED-TO;RELTYPE=PARENT` (task → track) and `RELTYPE=CHILD`
  (track → task). Nothing moves; the token is added.
- **Concept is not entity type.** `X-TLC-IS-TRACK` is the decoder's
  track-vs-task discriminator (`internal/vtodo/decode.go`,
  `isTrackComponent`: the marker first, then the `track_` UID prefix
  for foreign UIDs). It stays emitted, unchanged, alongside the token:
  removing an `X-*` property is a breaking change per spec 04
  ("Compatibility rules"), and — more to the point — `mission` alone
  cannot pick a Go type, because a standalone task is a mission too.

## A standalone task is a Mission

A task with `TrackID == nil` is a Mission, not a parentless
Assignment. This is structural, not a new label:

- An Assignment always carries `RELATED-TO;RELTYPE=PARENT` pointing at
  its Mission (encoder: emitted whenever `TrackID` is set). A
  standalone task carries no PARENT edge at all.
- The glossary's Assignment is "inside a mission". A unit of work
  inside nothing is not a scoped unit of anything; it is the goal
  itself, held directly. That is a Mission with no assignments yet.
- The wire already says this. A foreign reader that resolves
  `RELATED-TO` sees one VTODO with a parent and one without; the token
  makes explicit what the edge structure already implies, and keeps
  criterion 4 checkable without graph traversal.

On decode nothing changes: a `mission` VTODO without `X-TLC-IS-TRACK`
is still a `Task`, which is correct — it *is* a standalone task.

## LogEntry → Journal, sub-typed

The glossary's Journal is "a VJOURNAL entry recording an event,
action, observation, decision, score, artifact, memory, risk, or
learning"; spec 02 lists Action, Observation, Decision, Score, Memory,
Artifact, Resource delta and Learning as VJOURNAL, plus the
status-supersession shape ("Status supersession (append-only
ledger)"). tlc's `LogEntry` has one discriminator, `Action`
(`internal/core/models.go`). The sub-type is derived from it at
encode time.

What the code actually writes into `Action` (verified):

- The constants in `internal/core/service.go` (CRUD, status,
  recipe-execution, execution, collaboration, failure/retry, sync,
  assignee groups).
- The **target status name**: `Task.TransitionWithWorkflow`
  (`internal/core/task.go`) writes `Action: string(next)`. Most
  callers keep it (`internal/cli/task_lifecycle.go`,
  `task_lifecycle_block.go`, `task_field_changes.go`,
  `serve_routes_task.go`, `core/service.go` `TransitionTask`,
  `core/track_service.go`); some override it (`CLAIMED`, `RELEASED`,
  `DONE`, `APPROVED`, `SKIPPED`, `RETRY`, `BLOCKED` in the executor
  and human-gate paths). Because the status vocabulary is configurable
  (`task.statuses`), the action vocabulary is **open**.
- `REOPENED`, from the audit subscriber (`internal/events/audit.go`,
  `topicToAction`), and — for a bus event without an entity payload —
  the raw topic string.

Sub-typing rule, first match wins, matched case-insensitively:

| Token | Records | `LogEntry.Action` |
|---|---|---|
| `decision` | a human-gate verdict on the assignment | `APPROVED`, `REJECTED` |
| `status` | a transition of `Task.Status`, or of the readiness gate that decides its effective status — spec 02's supersession shape | any name in the effective status vocabulary (`core.ConfiguredTaskStatusStrings`); `CLAIMED`, `RELEASED`, `DONE`, `SKIPPED`, `RETRY`, `REOPENED`, `BLOCKED`, `UNBLOCKED` |
| `action` | something a player did to the assignment other than move its status | `CREATED`, `UPDATED`, `DELETED`, `REASSIGNED`, `RECLAIMED`, `AUTO_ASSIGNED`, `DELEGATED`, `SPLIT`, `MERGED`, `MIGRATED`, `EXEC_START`, `EXEC_END`, `EXEC_ATTEMPT`, `SYNC_IMPORTED`, `SYNC_PULLED`, `SYNC_PUSHED` |
| `observation` | something noticed; no state change | `COMMENT`, `FAILURE`, `SYNC_CONFLICT`, `SYNC_ERROR` |
| `journal` | umbrella — cannot be classified | raw bus topics from the audit fallback; foreign VJOURNAL imports; `SYSTEM_*` / `LEASE_*` (documented in `docs/glossary-0.1.md`, never emitted by code); anything else |

Notes on the rule:

- `decision` precedes `status` because `APPROVED` is produced on a
  transition path (it completes the task) but what the entry *records*
  is the verdict; the transition is its consequence.
- `BLOCKED`/`UNBLOCKED` are `status` because `BlockedReason` is what
  the executor's readiness classification reads (recipe spec §12.3);
  it is tlc's effective status even though `Task.Status` is untouched.
  This is the same notion spec 02 names `X-VSTAR-EFFECTIVE-STATUS`.
- `RECLAIMED` is `action`, not `status`: the status stays active; the
  owner and `ClaimedAt` change (`internal/core/executor_dispatch.go`,
  `take`).
- The umbrella is required, not a nicety: the vocabulary is open on
  two sides (configured status names, raw topic fallback), and the
  decoder mints `LogEntry` rows from foreign VJOURNALs.
- A `status` journal whose task is in the export is emitted as spec
  02's supersession entry (`supersession.Supersedes`), with
  `X-VSTAR-EFFECTIVE-STATUS` holding the RFC 5545 VTODO STATUS value
  the transition lands the task in, by role: `CLAIMED` → active
  (`IN-PROCESS`), `RELEASED` / `RETRY` / `REOPENED` / `BLOCKED` /
  `UNBLOCKED` → initial (`NEEDS-ACTION`), `DONE` → completed
  (`COMPLETED`), `SKIPPED` → skipped (`CANCELLED`), a status-name
  action by its configured role. Parsing the `Note` text is not an
  acceptable source. Follow-on: a `status` or `decision` journal
  should also carry the resulting tlc status name as
  `X-TLC-STATUS:<name>`, the way VTODOs do today; a `decision` journal
  (`APPROVED` completes the task) carries no effective status yet.

## RecipeRun → Playthrough

`RecipeRun` (`internal/core/recipe_run.go`) is "one materialization
of a recipe: which recipe at which version and content hash, with
which vars, for which subject, into which track". That is a run
through a mission — the glossary's Playthrough — and spec 02 maps
Playthrough to VEVENT, not to a branch. Not exported today; VEVENT is
"out of scope" in the sync spec. This ADR reserves the token and the
mapping. Shape notes for the follow-on, from the fields that exist:

- `CreatedAt` → `DTSTART`. No end field; `DTEND` omitted (RFC 5545
  §3.6.1 permits it) or derived from the run's leaves' terminal
  journals — the same leaves subject completion already computes
  (recipe spec §11).
- `TrackID != ""` → `RELATED-TO` the track VTODO (the mission).
  `TrackID == ""` is a trackless run (`internal/core/recipe_materialize.go`):
  a path through the World whose tasks are standalone missions. The
  glossary allows "through a mission or world"; the mission edge is
  simply absent.
- `ParentRun` → `RELATED-TO` the prior run (reconcile lineage).
  `SubjectType`/`SubjectID` → `RELATED-TO` the subject VTODO.
- Membership: `RecipeRunTask{RunID, StepID, TaskID}` and the mirror
  `Task.RunID`/`StepID`. Which side carries the edge and under which
  RELTYPE is a follow-on design point; `PARENT` on the task side is
  taken by the mission edge.

## Claim window → Turn

The glossary's Turn is "a bounded execution window". In tlc the
per-assignment window is the claim: opened by the compare-and-set that
stamps `claimed_at` (`internal/storage/task_claim.go`,
`core.Executor.take`, `TaskService.ClaimTask`) and logged as `CLAIMED`
or `RECLAIMED`; closed wherever `ClaimedAt` is cleared
(`Executor.complete`, `Executor.fail`, human approve) and logged as
`DONE`, `RELEASED`, `SKIPPED`, `APPROVED`, `REJECTED`, `RETRY` or
`BLOCKED`. `EXEC_START`/`EXEC_END` pairs, when present, bound the
dispatch inside the claim. Spec 02 maps Turn to VEVENT: `DTSTART` from
the opening journal, `DTEND` from the closing one, `RELATED-TO` the
assignment. Not exported today; follow-on. See "Open points" for what
in the decision as stated did not survive contact with the code.

## Assignee → Player, RRule → Cadence

- `AssignedTo` (task and track) and `LogEntry.By` name a Player. Spec
  02 maps Player to VCARD; tlc emits no VCARD, only the name (as
  `ATTENDEE:mailto:` when it is an email, else `X-TLC-ASSIGNEE`, and
  `X-TLC-LOG-BY` on journals). A VCARD export is a follow-on; VCARD is
  a distinct component and needs no concept token.
- `RRule` is a Cadence, which the glossary defines as a property
  ("usually encoded with `RRULE`"). It is a property on the mission or
  assignment, not a component. Nothing to declare.

## Declaration: `X-TLC-CONCEPT`

The spec has no declaration mechanism. Spec 04 ("Namespaces") puts a
single system's extensions in `X-<SYSTEM>-*`, and tlc's slug is `TLC`
(`internal/vtodo/meta.go`, `tlcSystemSlug`). So:

```text
X-TLC-CONCEPT:mission        on a track VTODO or a standalone-task VTODO
X-TLC-CONCEPT:assignment     on a task VTODO with a PARENT edge
X-TLC-CONCEPT:status         on a VJOURNAL (or decision | action | observation | journal)
X-TLC-CONCEPT:playthrough    on a run VEVENT (follow-on)
X-TLC-CONCEPT:turn           on a claim VEVENT (follow-on)
```

- **One property, one token.** Not a list, so no comma handling and no
  ambiguity about which concept "wins".
- **Emitted on every VTODO/VEVENT/VJOURNAL tlc produces**, so a
  consumer never has to infer.
- **Read-side rules.** The token is the first track-vs-task
  discriminator, ahead of `X-TLC-IS-TRACK` and the UID prefix, but only
  `assignment` decides: it names a unit inside a mission, which in tlc
  is always a task, so it wins over a conflicting marker or prefix.
  `mission` covers a track and a standalone task alike and so defers
  to the marker, then the prefix, the ladder a calendar without the
  token walks. The decoder reports every declared token by wire UID in
  `ParseResult.Concepts` and stores none of them on the entity; the
  encoder re-derives the token from `TrackID` and `Action`. The
  property is in `knownXProps` (`internal/vtodo/meta.go`), so the wire
  copy is never parked in `Meta["x_tlc"]` and re-emitted beside the
  encoder's own.
- **Not CATEGORIES.** Two independent reasons. (1) The vstar codec in
  use (`hop.top/vstar v0.0.0-20260526030101-766e6e3a0692`,
  `codec/rfc5545/encoder.go`) lists `CATEGORIES` as TEXT and
  `escapeText` "escapes every comma uniformly" by its own comment;
  `helpers.SetCategories` joins values with `,` before that, so a
  three-tag task reaches the wire as `CATEGORIES:a\,b\,c` — one
  category to any RFC 5545 reader. A concept token appended there
  would be glued to the user's tags. (2) `CATEGORIES` already carries
  `Task.Tags`, which are user-facing labels; a machine token in the
  same list is a namespace collision the RFC gives no way to resolve.
- **Not `X-VSTAR-CONCEPT`.** That tier is cross-system and reached by
  promotion "once two independent systems implement the same extension
  with compatible semantics" (spec 04, "Promotion path"). tlc is the
  first; the ask is recorded below.
- **`X-TLC-IS-TRACK` stays.** Spec 04: "removing an extension is a
  breaking change … promote-then-replace rather than rename". It also
  still does a job the token cannot (entity type).

## Consequences

- Wire shape unchanged; one property added per component. Existing
  consumers are unaffected (spec 04: receivers MUST ignore unknown
  `X-*`).
- Criterion 4 becomes checkable per component instead of by graph
  shape; the token is what `docs/VSTAR-CONFORMANCE.md` (spec 05,
  "Self-certification") points at.
- The journal sub-type is computed at encode from `Action` and the
  configured status vocabulary. It therefore depends on the exporting
  project's config, as `STATUS` already does.
- `docs/vtodo-sync-spec-0.1.md` gains a "Vocabulary" section (entity →
  concept → component) referencing this ADR, and `X-TLC-CONCEPT` has
  its row in that file's `X-TLC-*` registry.
- VEVENT emission (turn, playthrough) and VCARD emission (player) are
  reserved, not delivered.

## Upstream asks (spec-vstar)

1. **Definitions for the concepts in 02.** The "Core mapping" table
   names World, Environment, Player, Group, Mission, Assignment, Turn,
   Playthrough, Action, Observation, Decision, Score, Memory,
   Artifact, Resource delta, Learning, Availability, Timeout /
   escalation and Timezone, and defines none. Criterion 4 in 05 cannot
   be met against an undefined vocabulary. hop-top/agr `GLOSSARY.md`
   has definitions for eight of them; the spec should own them.
2. **`X-VSTAR-CONCEPT`.** A cross-system concept declaration with the
   token vocabulary above. `X-TLC-CONCEPT` is the first implementation
   under the 04 promotion rule; a second (AGR) with compatible
   semantics is what promotion needs.
3. **A vocabulary for `X-VSTAR-EFFECTIVE-STATUS`.** The supersession
   example in 02 shows `COMPLETED`, which reads as the RFC 5545 STATUS
   set, but nothing says so. tlc's status names are user-configurable
   and role-mapped onto the four RFC values on export; it needs to
   know whether to emit the role's RFC value or the system name. Until
   the spec says, tlc emits the role's RFC value (the reading the
   example supports and the one that keeps ledger and VTODO in one
   vocabulary) and projects only those four values on import.

## Open points

Where the decision as stated did not hold up against the code, or
where the code needs attention before the follow-ons land:

- **`RunID`/`StepID` are not turn fields.** They are set at
  materialization (`internal/core/recipe_materialize.go`), never at
  claim time, and identify which playthrough step an assignment *is*.
  They belong to the Assignment → Playthrough edge, not to Turn. The
  turn is bounded by `ClaimedAt` and the log; `Attempts` counts failed
  turns, it is not one.
- **`Task.ClaimedAt` holds only the open turn.** Closed turns are
  recoverable only from journal pairs. Turn export therefore needs
  `--include-logs` semantics or a store query, not a task-row read.
- **Track missions have no journals.** `LogEntry.TaskID` is the only
  subject, and the audit subscriber drops track topics
  (`internal/events/audit.go`, `isTaskTopic`), so `TRACK_*` action
  strings are never written. A Track's status history is not exported
  as supersession; only standalone-task missions get one.
- **The unblock path logs `BLOCKED`.** `internal/cli/task_lifecycle_block.go`
  writes `core.ActionBlocked` with an "unblocked" note on unblock,
  while `core.ActionUnblocked` exists and is used by
  `core/service.go`. The classifier above marks both `status`, so the
  token is right, but the action is wrong. Fix separately.
- **Trackless runs have no mission edge.** `RecipeRun.TrackID` is a
  string, `""` meaning trackless. The glossary covers it ("through a
  mission or world"); the VEVENT simply has no track `RELATED-TO`.
- **`RecipeRun` has no end timestamp.** `DTEND` is derived or absent.
- **The executor round is also a bounded window** (recipe spec §12.3,
  glossary "Batch"), at playthrough level rather than per assignment.
  Not mapped; Turn stays the per-assignment claim.
- **Sync-spec drift the registry owner should pick up.** The VJOURNAL
  table there says `By` → `ORGANIZER`, `TaskID` → bare `RELATED-TO`,
  `Timestamp` → `DTSTAMP`; the encoder emits `X-TLC-LOG-BY`,
  `RELATED-TO;RELTYPE=PARENT` (equivalent — RFC 5545 §3.2.15 defaults
  RELTYPE to PARENT, which is also the shape 02's supersession example
  uses) and `CREATED`, with `DTSTAMP` = export time. The track example
  shows `X-TLC-IS-TRACK:true`; the encoder writes `TRUE` (decode is
  case-insensitive).

## Alternatives considered

- **Infer concept from shape.** The status quo. A foreign reader can
  tell mission from assignment only by resolving `RELATED-TO`, and
  cannot tell a status journal from a comment at all. Rejected: it is
  what criterion 4 is asking us to stop doing.
- **New component types.** Rejected: spec 05 criterion 5 forbids
  top-level new components.
- **`CATEGORIES:mission`.** Rejected for the two reasons above (codec
  escaping, collision with user tags).
- **`X-VSTAR-CONCEPT` directly.** Rejected: not tlc's tier to mint.
- **`X-EXP-CONCEPT`.** Rejected: spec 04 says senders must not depend
  on receivers honoring it, and removing it later is breaking anyway;
  the system tier is the stable one.
- **Widen `X-TLC-IS-TRACK` to carry the token.** Rejected: its boolean
  semantics are baked into the name and the decoder; repurposing is a
  rename, which 04 treats as removal.

## Out of scope (this decision)

- Emitting VEVENT (turn, playthrough) or VCARD (player).
- The `X-TLC-*` property registry in the sync spec.
- Environment, Group, Score, Memory, Artifact, Resource delta,
  Learning, Availability, Timezone — tlc has no entity for them.
- Changing what `X-TLC-IS-TRACK` means or when it is emitted.
