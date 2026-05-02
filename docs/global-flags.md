# Global Flags

Persistent flags available on every `tlc` subcommand. Defined in
`internal/cli/root.go` per [`cli-conventions-with-kit.md`][1] §5.

These supplement kit-provided globals (`--quiet`, `--no-color`, `--format`,
`--no-hints`, `--verbose/-V`, `-C/--chdir`) and the local `--config/-c`.

[1]: ../../.ops/docs/cli-conventions-with-kit.md

## Reference

| Flag         | Type   | Default          | Viper key          | Behaviour                                                                                                  |
|--------------|--------|------------------|--------------------|------------------------------------------------------------------------------------------------------------|
| `--offline`  | bool   | `false`          | `runtime.offline`  | Disable all network. Skips upgrade check, blocks `sync push`/`sync pull` (returns usage error), skips extension `InitAll`. |
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
