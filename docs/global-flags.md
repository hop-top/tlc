# Global Flags

Persistent flags available on every `tlc` subcommand. `--profile` and
`--instance` are defined in `internal/cli/root.go` per
[`cli-conventions-with-kit.md`][1] §5.

`--offline` is kit-owned: kit registers it as a global and reserves the name,
so `root.go` only binds it. What `root.go` still owns is the viper binding —
`runtime.offline` is this repo's documented config key with three consumers
(below), so the flag is bound to that key rather than migrated to kit's own
`offline` key. A shadow binding kept deliberately, not an oversight.

These supplement the other kit-provided globals (`--quiet`, `--no-color`,
`--format`, `--no-hints`, `--verbose/-V`, `-C/--chdir`) and the local
`--config/-c`.

[1]: ../../.ops/docs/cli-conventions-with-kit.md

## Reference

| Flag         | Type   | Default          | Viper key          | Behaviour                                                                                                  |
|--------------|--------|------------------|--------------------|------------------------------------------------------------------------------------------------------------|
| `--offline`  | bool   | `false`          | `runtime.offline`  | Kit-owned flag, bound here. Disable all network. Skips upgrade check, blocks `sync push`/`sync pull` (returns usage error), skips extension `InitAll`. |
| `--profile`  | string | `$APS_PROFILE`   | `runtime.profile`  | aps profile name. Reserved for future use; flag is accepted today but not yet read by any code path.        |
| `--instance` | string | `tlc`            | `runtime.instance` | Backend instance identifier. Reserved for future use; flag is accepted today but not yet read by any code path. |

## `--offline` call sites

Three places consult `viper.GetBool("runtime.offline")` today:

1. **Upgrade check** — `PersistentPreRunE` in `internal/cli/root.go` skips
   `upgrade.NotifyIfAvailable` when offline.
2. **Sync push/pull** — `runSyncPull` and `SyncPushCmd.RunE` in
   `internal/cli/sync.go` short-circuit with
   `WithCode(fmt.Errorf("sync requires network; --offline is set"), CodeUsage)`.
3. **Extension init** — `PersistentPreRunE` skips `extMgr.InitAll(ctx)` and
   logs `offline: skipping extension init`. Extensions that don't touch the
   network are also skipped (acceptable trade-off until per-extension offline
   capability lands).

## Examples

```bash
tlc --offline upgrade            # no-op (skip auto-check, command itself errors out)
tlc --offline sync push github   # → "sync requires network; --offline is set" (exit 2)
tlc --profile work task list     # accepted; viper.GetString("runtime.profile") == "work"
tlc --instance staging task list # accepted; viper.GetString("runtime.instance") == "staging"
```

## Adding new offline-aware code

When wiring new network-using code paths, gate them with:

```go
if viper.GetBool("runtime.offline") {
    return WithCode(fmt.Errorf("<feature> requires network; --offline is set"), CodeUsage)
}
```

For best-effort code paths (telemetry, optional checks), use a soft skip:

```go
if !viper.GetBool("runtime.offline") {
    // do network thing
}
```

## Structured output: `--json`, `--jq`, `--template`

`task list`, `task show`, `track list` and `track show` emit one record per
entity. `list` and `show` build the same record, so a field present in one
is present in the other; json, yaml and csv all render from that record.
Unset values are `null` (never dropped); collections are `[]` / `{}`.

| Flag | Behaviour |
|------|-----------|
| `--json f1,f2` | Select fields, emitted in the order given. Implies `--format json`. Bare `--json` prints the available fields and exits 2. |
| `--jq <expr>` | jq filter over the JSON document (after selection). Implies `--format json`; strings print raw. |
| `--template <tmpl>` | Go `text/template` over `{.Items, .Cols, .Data}` (kit's contract); `.Items` are maps keyed by JSON field name (after selection). |
| `--format json\|yaml\|csv` | Full record, or the `--json` selection. csv: header row = field names, nested values are compact JSON, arrays of scalars join with `;`. |

`--json`/`--jq` with another explicit `--format`, `--jq` with `--template`,
and `--cols` with a structured format are usage errors. `--group-by` nests
records under `{groups:[{name,tasks}]}`; csv rejects it.

```bash
tlc task list --json display_id,priority,effort,track_id
tlc task list --json display_id,priority --jq '.[] | select(.priority=="P0") | .display_id'
tlc task show T-0046 --json display_id,blocked_by
tlc track list --json display_id,progress --jq '.[] | "\(.display_id) \(.progress.done)/\(.progress.total)"'
tlc task list --template '{{range .Items}}{{.display_id}} {{.title}}{{"\n"}}{{end}}'
```

### Identifiers

`id` is the durable id; `seq` the per-project counter; `display_id` the
alias the human view and table print (`T-NNNN` / `L-NNNN`). For rows whose
stored id is a legacy `T-NNNN` string, `display_id` is that string and may
differ from `seq`. Use `display_id` for round-tripping into `task show`.

### Task fields

`id display_id seq title description status kind assigned_to priority effort
tags track_id due_at remind_at rrule no_auto_remind blocked_by blocked_reason
stale_timeout stale_fired_at reference project_id archived created_at
updated_at origin_system last_sync_at meta spec result attempts claimed_at
run_id step_id step_ordinal`

`blocked_by` entries: `{ref, id, title, status, met, missing?}`.
`stale_timeout` is a Go duration string (`6h0m0s`). Default JSON for
`task show` stays `{"task": <record>, "logs": [...]}`; with `--json`,
`--jq`, `--template` or csv it is the bare record.

### Track fields

`id display_id seq slug title type status state assigned_to project_id
progress due_at created_at updated_at meta plan_mapping`; `track show` adds
`tasks` (task records) and `execution_strategy`. `progress` is
`{done, total, percent, current_phase, total_phases, phases, text}`;
`text` is the table's Progress cell (`0/4 (P1/2)`).

Schema tests (`record_output_test.go`) fail when a table column has no JSON
field or a model field is missing from the record.
