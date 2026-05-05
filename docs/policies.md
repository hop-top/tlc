# Tlc policies

Tlc uses kit's runtime policy engine (`hop.top/kit/go/runtime/policy`)
to gate state changes. Policies are declarative YAML rules,
compiled once at boot, and evaluated synchronously on every state
change. A denial vetoes the change before any row mutation.

For engine semantics, expression vocabulary, deny-overrides
composition, and the canonical worked examples see kit ADR-0008
(`docs/adr/0008-kit-runtime-policy-engine.md` in the kit repo).
This doc covers tlc-specific adoption only.

## Default policy

Tlc ships exactly one rule:

```yaml
policies:
  - name: delete-requires-note
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || context.note != ""'
    effect: allow
    otherwise: deny
    message: "deleting a task requires --note explaining why"
```

Effect: `tlc task delete <id>` without `--note` is rejected with
exit 4.

```text
$ tlc task delete T-0042 --yes
Error: policy "delete-requires-note" denied: deleting a task requires --note explaining why
$ echo $?
4
```

```text
$ tlc task delete T-0042 --yes --note "duplicate of T-0099"
Deleted task T-0042
```

Non-destructive transitions are not gated by default — adopters
opt in by adding rules to their own file.

## Where the policy file lives

| Resolution order | Source |
|------------------|--------|
| 1 | `$TLC_POLICY_FILE` env var (used by tests + CI) |
| 2 | `$XDG_CONFIG_HOME/tlc/policies.yaml` (default `~/.config/tlc/policies.yaml`) |

On first boot, tlc seeds option 2 from the bundled default if it
is missing or empty. **Existing non-empty user files are never
clobbered.** Edit the file in place to extend or override.

If both seeding fails (read-only home, etc.) and `$TLC_POLICY_FILE`
is unset, tlc falls back to the embedded default — enforcement
still applies. The error surfaces the next time the user tries to
edit and the file isn't there.

## Anatomy of a policy

```yaml
policies:
  - name: <unique-string>          # for error messages + logs
    on: <kit-topic>                # see "Available topics" below
    when: <CEL expression>         # evaluated per matching event
    effect: allow | deny           # outcome when `when` is true
    otherwise: allow | deny        # outcome when `when` is false
    message: <string>              # surfaced via PolicyDeniedError
```

Both `effect` and `otherwise` are required — no implicit defaults.
This forces every policy to state both match-outcome and
no-match-outcome explicitly.

When multiple policies match a single event, kit applies
**deny-overrides**: ANY policy resolving to `deny` wins, and the
first denying policy's message is surfaced. See ADR-0008 §8.

## Available topics

Tlc currently subscribes one topic:

| Topic | When it fires | Tlc commands that publish |
|-------|--------------|---------------------------|
| `kit.runtime.entity.pre_persisted` | After validation, before repo write | `tlc task delete` (T-1192) |

`kit.runtime.state.pre_transitioned` and
`kit.runtime.entity.pre_validated` are valid kit topics but tlc
does not publish them yet. Writing a policy on those topics is
not an error — it loads, compiles, never matches, never fires.
Tlc tracks topic-coverage expansion as a follow-up to T-1192.

## Available context attributes

The CEL `when` expression sees four bindings (see ADR-0008 §4):

| Binding | Type | Notes for tlc |
|---------|------|---------------|
| `payload.Op` | string | `"delete"` for `tlc task delete` |
| `payload.EntityID` | string | The task id (e.g. `"T-0042"`) |
| `payload.Phase` | string | Always `"pre_persisted"` for tlc today |
| `payload.from` / `payload.to` | string | Set on `kit.runtime.state.pre_transitioned` (not yet published by tlc) |
| `payload.force` | bool | Set on state transitions (not yet published by tlc) |
| `principal.id` | string | Resolved from kit's `DefaultPrincipalResolver` (today: `$USER`). Aps profile lookup is a follow-up — see policy.go `tlcPrincipalResolver` |
| `principal.role` | string | From `KIT_POLICY_ROLE` env var. Empty when unset |
| `context.note` | string | Value passed via `--note|-n` on `tlc task delete` (other commands plumb it but the default policy doesn't read it) |
| `resource.kind`, `resource.fields` | dyn | Per-event payload reflection — see ADR-0008 §4 |

Use `has(context.note)` to check presence; tlc always sets the key,
so an empty value reads as `""` not `unset`.

## Examples

### Block deletion of in-progress tasks

```yaml
policies:
  - name: delete-not-in-progress
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || resource.fields.status != "IN_PROGRESS"'
    effect: allow
    otherwise: deny
    message: "in-progress tasks cannot be deleted; complete or skip first"
```

### Require admin role to delete

```yaml
policies:
  - name: delete-admin-only
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || principal.role == "admin"'
    effect: allow
    otherwise: deny
    message: "only role:admin may delete tasks"
```

Set the role per-shell:

```bash
export KIT_POLICY_ROLE=admin
tlc task delete T-0042 --yes --note "manual cleanup"
```

### Compose with the default

Tlc applies deny-overrides across all matching policies. With both
the default `delete-requires-note` and the `delete-admin-only` rule
above active, deletes need both `--note` AND `role:admin`. Order
in the file does not matter; the first denying policy's message is
surfaced.

### Note required minimum length

```yaml
policies:
  - name: delete-note-min-length
    on: kit.runtime.entity.pre_persisted
    when: 'payload.Op != "delete" || size(context.note) >= 20'
    effect: allow
    otherwise: deny
    message: "delete note must be ≥20 chars; explain why this row goes away"
```

## Authoring custom policies

Kit's `e2e/` directory is the canonical reference. Each test file
is one runnable user story; lift the YAML and CEL idioms from
there:

- `kit/go/runtime/policy/e2e/README.md` — story index + reading order
- `kit/go/runtime/policy/e2e/story_delete_requires_note_test.go` — the
  story tlc's default rule was modeled on
- `kit/go/runtime/policy/e2e/story_admin_only_cancel_test.go` —
  role-gated transitions
- `kit/go/runtime/policy/e2e/story_deny_overrides_compose_test.go` —
  multi-policy semantics

Compile-time validation:

- All CEL programs compile at boot. A broken expression fails the
  CLI loud, before any command runs.
- Topic strings are not yet validated against `bus.ValidateTopic`
  (kit ADR-0008 open question 3); a typo'd `on:` field loads,
  compiles, never matches, and silently never fires.

## Exit codes

| Code | When |
|------|------|
| 4 | Policy denied — `PolicyDeniedError` mapped to `output.CodeConflict`, exit 4 |

See [tlc-cli-spec-0.1.md](tlc-cli-spec-0.1.md#exit-codes) for the
full table.

The policy denial is distinguishable from local validation
errors by the message prefix:

```text
Error: policy "<name>" denied: <message>
```

## Operator notes

- The default rule changes behavior for `tlc task delete`. Old
  scripts that called `tlc task delete <id> --yes` exit 4 after
  upgrade. Update them to pass `--note "<reason>"`.
- The note is recorded against the `task_logs` row before the
  delete and survives row removal (schema migration v15, T-1232).
  Querying historical deletions: `SELECT note FROM task_logs WHERE
  action = 'DELETED' AND task_id = '<id>'`.
- To restore pre-T-1192 behavior temporarily, edit
  `$XDG_CONFIG_HOME/tlc/policies.yaml` and remove the
  `delete-requires-note` entry. Tlc does not re-seed once a user
  file exists, so the change persists across upgrades.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `policy: load <path>: ...` at boot | Malformed YAML | Validate the file; tlc fails loud rather than ignore |
| `policy: build engine: ...` at boot | Broken CEL in `when` | Compare to ADR-0008 §3 examples and the kit e2e stories |
| `policy "<name>" denied: ...` on every command | A new rule's `otherwise: deny` fires for unrelated events | Tighten the `when` clause so non-target events match and `effect: allow` |
| Custom rule never fires | Topic typo, or tlc doesn't publish that topic yet | See "Available topics" — tlc only publishes `kit.runtime.entity.pre_persisted` today |
| Policy fires but message is missing | `message:` field absent | Add `message: "..."` — kit falls back to a generic string otherwise |

## See also

- [tlc-cli-spec-0.1.md](tlc-cli-spec-0.1.md) — CLI reference,
  including `--note` flag plumbing and the exit-code table
- [task-log-spec-0.1.md](task-log-spec-0.1.md) — log schema, where
  the delete note lands
- kit ADR-0008 — engine design, full vocabulary, alternatives
- `internal/config/policies_default.yaml` — the bundled default
