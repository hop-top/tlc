# State-change notes — design

Status: accepted (T-1177)
Track: `state-change-notes`
Blocks: T-1178 (CLI flags), T-1179 (schema), T-1180 (enforcement), T-1181 (docs)

Contract for downstream tasks. Implementers MUST conform.

## Goals

1. Universal `--note|-n` on every state-changing task subcommand.
2. Per-transition `note_required: true` declarable in workflow YAML.
3. Backwards compatible: omitted `note_required` ⇒ false; existing workflows
   keep working.

## Non-goals

- Editing existing notes (immutable; append-only via new transitions).
- Notes on read-only ops (`show`, `list`).
- Notes on field-only edits without state change (see §6).

## Inventory: today's state

| Subcommand                         | `--note` today | Required today | Records LogEntry |
|------------------------------------|----------------|----------------|------------------|
| `task claim`                       | yes            | no             | yes              |
| `task unclaim`                     | yes            | no             | yes              |
| `task assign`                      | yes            | no             | yes              |
| `task unassign`                    | yes            | yes (hardcoded)| yes              |
| `task complete`                    | yes            | no             | yes              |
| `task reopen`                      | yes            | yes (hardcoded)| yes              |
| `task update --status X`           | NO             | n/a            | yes (hardcoded "Manual update") |
| `task update --priority/--effort/...` | NO          | n/a            | NO (just `UpdateTask`) |
| `task delete`                      | NO             | n/a            | NO (DeleteTask, no log) |

`archive`, `restore`, `skip`, `block`, `unblock` subcommands do not exist
today. `skip` is `update --status SKIPPED`. `block`/`unblock` is
`update --blocked <reason>` / `--unblock`.

## 1. Storage model

**Decision: keep today's storage. Extend by populating the existing
`Note` field on `LogEntry`.** No new table.

Today's persistence:

- `task_logs` table (see `internal/storage/migrations.go` v13):
  `(id, task_id, project_id, timestamp, by, action, note, meta)`.
  One row per state transition. `note` already TEXT NOT NULL.
- Mirrored markdown audit block in `tasks.description` via
  `appendAuditLog` (see `internal/cli/task.go`). Cosmetic mirror —
  source of truth is the `task_logs` row.

The row IS the transition record. `(task_id, action, timestamp)` keys
the transition; `note` lives on the same row. No `transition_event_id`
indirection needed.

Implications for T-1178/T-1180:

- Every state-changing subcommand MUST go through
  `saveTaskWithLog(ctx, cmd, task, logEntry, storage)` (the atomic
  `UpdateTaskWithLog` path) so the note lands in `task_logs`.
- `task delete` currently calls `DeleteTask` directly, no log. T-1180
  MUST emit a final `DELETED` log row before delete (or via a soft-delete
  archive flag — see §6.delete).
- `task update` (non-status fields) currently calls `UpdateTask` with
  no log. When `--note` is supplied, T-1180 MUST emit an `UPDATED` log
  row carrying the note + diff in `meta`.

## 2. Workflow YAML schema

**Decision: migrate `state_machine.rules` from `map[string][]string` to
list-of-records, gaining a third field `note_required`.**

Rationale: the existing map form has no slot for per-transition metadata.
A list-of-records is the only forward-compatible shape. The published
spec doc (`docs/tlc-config-spec-0.1.md` §`task.state_machine`) ALREADY
shows the list form — the Go type drifted. T-1179 lands the type to match
the doc.

New shape:

```yaml
task:
  state_machine:
    rules:
      - from: TODO
        to: IN_PROGRESS
      - from: TODO
        to: SKIPPED
        note_required: true        # mandate reason on cancel
      - from: IN_PROGRESS
        to: DONE
      - from: IN_PROGRESS
        to: TODO
      - from: IN_PROGRESS
        to: SKIPPED
        note_required: true
```

Go types (T-1179):

```go
type WorkflowDefinition struct {
    Rules []TransitionRule `yaml:"rules"`
}

type TransitionRule struct {
    From         string `yaml:"from"`
    To           string `yaml:"to"`
    NoteRequired bool   `yaml:"note_required,omitempty"`
}
```

Migration: `WorkflowDefinition.UnmarshalYAML` MUST accept BOTH legacy
map form and new list form, normalizing to `[]TransitionRule`. Legacy
map ⇒ all `NoteRequired=false`. Drop legacy form in v0.2 (not now).

`WorkflowManager` (`internal/core/workflow.go`) re-keys internally:

```go
type transitionKey struct{ From, To string }
rules        map[string][]string         // KEEP (allow-list, hot path)
noteRequired map[transitionKey]bool      // NEW
```

`ValidateTransition` MUST accept a new `note string` arg (or sibling
method `ValidateNote(from,to,note) error`) — see §4.

## 3. Exit code

**Decision: exit code 4 (conflict).** Justification:

- Per `~/.ops/docs/cli-conventions-with-kit.md` §8.1, code 4 = "already
  exists / conflict". A missing-required-note is a precondition
  conflict: the workflow declares the transition needs a note; the
  invocation is in conflict with the workflow contract.
- tlc already uses code 4 for `task create --id` collisions
  (`internal/cli/task_create.go`). Same family: caller violated a
  declared invariant.
- Exit 2 is wrong: not a usage error (flags parsed fine).
- Exit 1 is what we have today by accident — generic noise; agents
  cannot script on it.
- Tool-specific 64+ is reserved for genuinely tlc-specific signals
  (none yet); a workflow-rule violation is generic enough for the
  shared bucket.

Wiring: `errNoteRequired` in `internal/cli/errors.go` MUST be wrapped
with `WithCode(err, CodeConflict)` at every call site. Update
`docs/exit-codes.md` "Call sites" table to add the rows.

## 4. Error message format

When the note is missing on a `note_required: true` transition:

```
Error: transition TODO → SKIPPED requires --note
re-run with: tlc task update T-0042 --status SKIPPED --note "<reason>"
```

Two lines. Line 1 names the transition (matches the YAML
authoring vocabulary). Line 2 is a copy-pasteable fix using the actual
invocation (preserve the user's task IDs and flags).

When `--note` is missing on a subcommand that has `--note` always
required (today: `unassign`, `reopen`):

```
Error: --note is required
re-run with: tlc task unassign T-0042 --note "<reason>"
```

Same shape, no transition prefix (it's not transition-specific).

Implementation: extend `errNoteRequired` to take `(from, to, cmdHint)`
optional, OR add a sibling `errTransitionNoteRequired(from, to, cmdHint)`.

## 5. `--note` flag scope across subcommands

| Subcommand                       | `--note` accepted | Notes |
|----------------------------------|-------------------|-------|
| `task claim`                     | yes               | already wired |
| `task unclaim`                   | yes               | already wired |
| `task assign` / `unassign`       | yes               | already wired |
| `task complete`                  | yes               | already wired |
| `task reopen`                    | yes               | already wired |
| `task update --status X`         | yes (T-1178)      | replaces hardcoded "Manual update"; treated as transition |
| `task update --priority/--effort/--title/--description/--track/...` | **yes** (T-1178) | non-status field edit; recorded as `UPDATED` log row |
| `task update` with both `--status` and other fields | yes | single note attached to BOTH the transition log row AND the field-update log row (one note, applied) |
| `task delete`                    | yes (T-1178)      | recorded on the `DELETED` log row before row removal |

**Decision on field-only edits:** `--note` accepted on ANY `update`
invocation, not just status changes. Rationale: notes serve audit/why,
not state-machine bookkeeping. A priority hike from P3→P0 deserves a
reason as much as TODO→SKIPPED. Cost: a new `UPDATED` log row per noted
edit. Acceptable; cheap.

**`note_required` only fires on status transitions.** Field edits are
not modeled in the workflow YAML, so there is no rule to consult. If a
user wants to mandate a reason for priority changes, that's a separate
RFC (out of scope).

**Bulk operations**: `tlc task update T-1 T-2 T-3 --note "..."` — same
note applied to each. Rationale: the typical bulk case is "rolled all
three for the same reason". If the note is required by a transition
rule and any target is in a non-matching state, that target fails with
its own error; siblings still proceed (existing per-task error
collection in the loop).

NOT rejected as "ambiguous". The note is metadata, not a unique
identifier. Reusing it is fine.

**`task delete` behavior**: today `DeleteTask` removes the row.
With `--note`, T-1180 MUST insert a `DELETED` log row first inside
the same tx. Because of `ON DELETE CASCADE` on `task_logs`, the log
will be wiped with the task — UNLESS we change the cascade. Option A:
keep cascade, accept logs disappear (note is captured at the moment
but lost on subsequent reads). Option B (recommended): drop the cascade,
treat `task_logs` as an immutable audit trail.

**Open question for Jad**: cascade behavior on delete. Recommendation:
Option B (preserve logs after delete). Requires migration v15. Flagged
as scope-risk.

## 6. Interaction with `--add-eva`

Distinct primitives. Document side-by-side in the user-facing docs
(T-1181):

| Mechanism      | Type           | Lifetime         | Storage              | Purpose |
|----------------|----------------|------------------|----------------------|---------|
| `--add-eva`    | metadata tag   | persistent on task | `tasks.meta.eva` (jsonb) | ad-hoc, additive labels — survives transitions |
| `--note`       | transition log | append-only      | `task_logs.note`      | the "why" of a single state change |

A note is bound to ONE transition event. Eva annotations live on the
task itself and accumulate across transitions. Setting `--note` does
NOT touch eva; setting `--add-eva` does NOT create a log row. They
coexist on the same `update` invocation.

## 7. Implementation phasing (handoff to T-1178/T-1179/T-1180/T-1181)

T-1179 (schema) — lands first; pure type/migration:
- New `TransitionRule` type, polymorphic UnmarshalYAML.
- `WorkflowManager` carries `noteRequired` map.
- New `WorkflowManager.NoteRequired(from, to TaskStatus) bool`.
- Update `docs/tlc-config-spec-0.1.md` to document `note_required` field
  and confirm list form.
- (Optional) migration v15: drop `task_logs` cascade. Decide based on §5.delete.

T-1178 (CLI flags) — depends on T-1179:
- Add `--note|-n` to `TaskUpdateCmd`, `TaskDeleteCmd`.
- Existing `--note` unchanged on claim/unclaim/assign/unassign/complete/reopen.
- Replace hardcoded `"Manual update"` in `task_update.go` line 81 with
  the user-supplied note (empty string if not given).
- Plumb note into `LogEntry.Note` for the `UPDATED` and `DELETED` actions.

T-1180 (enforcement) — depends on T-1178 + T-1179:
- In `WorkflowManager.ValidateTransition` (or sibling), after the
  allow-list check passes, consult `noteRequired[(from,to)]`. If true
  and note is empty, return new error type `ErrNoteRequired{From, To}`.
- CLI layer maps `ErrNoteRequired` to
  `WithCode(errTransitionNoteRequired(...), CodeConflict)`.
- Apply uniformly across claim/unclaim/assign/unassign/complete/reopen/
  update --status/delete (delete only if delete is modeled in workflow;
  otherwise enforce via dedicated `delete.note_required` config knob —
  see open question).

T-1181 (docs):
- Update `docs/exit-codes.md` "Call sites" table with the new rows.
- Update `docs/tlc-cli-spec-0.1.md` per-subcommand sections.
- Update `docs/task-crud-spec-0.1.md` to list `--note` everywhere.
- Add a "State-change notes" section to `docs/cheatsheet-agent.md` and
  `cheatsheet-human.md`.

## 8. Rejected alternatives

- **New `transition_notes` table keyed on `(task_id, transition_event_id)`**.
  Rejected: `task_logs` rows ARE the transition events. Adding a sibling
  table doubles the write path and the join cost on `task show --logs`.
  Premature normalization.

- **Use eva annotations to carry transition reasons**. Rejected: eva is
  task-level persistent metadata, not transition-event metadata. Conflating
  them loses the timestamp + actor + transition triple that auditors
  expect. Eva `cancelled-by-jad` survives a reopen; a transition note
  belongs to the cancel event only.

- **Exit code 5 (unauthorized)** for missing-required-note. Rejected:
  the user IS authorized; they just provided incomplete input against a
  workflow rule. Misuses the auth code.

- **Exit code 64+** (tool-specific). Rejected: a workflow-rule violation
  is exactly what code 4 (conflict) was designed for. Reserve 64+ for
  semantically tlc-only signals (none today).

- **Reject bulk `--note` as ambiguous**. Rejected: bulk operations with
  a shared reason are the common case. Forcing per-task notes via
  multiple invocations is friction with no audit benefit.

- **Keep `state_machine.rules` map, layer `note_required` as a parallel
  map**:
  ```yaml
  state_machine:
    rules: { TODO: [IN_PROGRESS, SKIPPED] }
    note_required: { "TODO->SKIPPED": true }
  ```
  Rejected: split metadata, fragile string keys, every future
  per-transition attribute (e.g. `requires_role`, `webhook`) duplicates
  the split. List-of-records scales.

- **Inline note in description audit block only, skip `task_logs.note`**.
  Rejected: `task_logs.note` is already populated; the markdown block is
  a derived view. Walking it back would orphan structured queries
  (`tlc task show --logs --format json`).

## 9. Open questions for Jad

1. **`task_logs` cascade on delete** — keep or drop? Recommend drop
   (preserve audit trail). Affects T-1179 migration.
2. **`note_required` on `delete`** — should the workflow YAML express
   it? Today delete isn't a status transition. Options:
   - (a) Add `task.delete.note_required: true` config (separate knob).
   - (b) Model delete as a pseudo-status `DELETED` reachable from any
     state, then it falls under the existing schema.
   - (c) Hardcode: delete always requires `--note` after T-1180.
   Recommend (a). Cheap, explicit, doesn't pollute the state machine.
3. **`note_required` on `priority` / `effort` / `track` changes** — model
   in YAML or out of scope? Recommend out of scope; revisit if asked.

## 10. Scope risks

- **Schema migration to list-of-records is breaking on disk** — any
  user who already wrote a custom `state_machine.rules` in the map form
  must be auto-migrated by `UnmarshalYAML`. T-1179 MUST land the
  polymorphic unmarshal before T-1180 enforces.
- **`task delete` log-and-cascade interaction** — see open question 1.
  If we keep cascade, the captured note disappears on delete. That's a
  silent regression of "notes are durable". Strongly recommend the
  migration alongside T-1180.
- **Existing hardcoded `"Manual update"` note on `task update --status`**
  — replacing it with the user-supplied note changes log content for
  any caller relying on the literal string. Low risk (no known scrapers),
  but mention in the changelog.
