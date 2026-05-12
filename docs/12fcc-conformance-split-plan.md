# T-1391 12fcc Conformance — Split Plan

Authoritative checklist generated from `docs/strict-validation-baseline.txt`.
89 unique command paths fail strict validation under kit 12fcc-leak.

## Annotation contract (kit/hops/12fcc-leak)

- `kit/side-effect` — required on every runnable leaf.
  - Valid values: `read | write | write-local | write-shared | destructive | destructive-local | destructive-shared | interactive`
  - Source: `go/console/cli/sideeffect.go:30-69` (constants), `:125-134` (validSideEffects)
- `kit/idempotent` — required on every runnable leaf (after auto-apply for known verbs).
  - Valid values: `yes | no | conditional`
  - Source: `go/console/cli/idempotent.go:31-45` (constants), `:78-82` (validIdempotency)
  - Auto-apply: verbs `list/show/get/find/search/info/current/path/paths/doctor/delete/edit/update/use/default/sync/reprocess` → `yes`; `create/add` → `no`. Source: `go/console/cli/idempotent.go:90-110`.
- `kit/top-level-verb` — required on every depth-1 runnable leaf.
  - Valid values: `"true"` (presence-by-string)
  - Source: `go/console/cli/contract.go:41-44` (constant), `shape.go:35-37, 53-55` (set/read), `cli.go:945-951` (check).
- `kit/passthrough` — informational; presence marker for leaves that forward `-- args` to a child process.
  - Valid values: `"true"`
  - Source: `contract.go:49-52`, `shape.go:48-50, 63-65`.
  - PassthroughStrictness=reject hard-errors (`cli.go:930-931`); default is warn.
- `kit/hierarchical` — required on every intermediate non-runnable node at depth ≥ 2 with descendants at depth ≥ 3 when the depth-1 ancestor is not reserved.
  - Source: `contract.go:45-48`, `cli.go:933-942, 956-959`.
- `kit/output-schema` — optional; when present, validator parses it as JSON. Source: `contract.go:27-33`, `cli.go:887-894`.
- `kit/retryable`, `kit/dry-run-rationale`, `kit/examples`, `kit/next-steps`, `kit/format-flag`, `kit/reserves-children`, `kit/exempt-validation` — optional; informational or gated by other Config flags. Source: `contract.go:18-72`.

### Reserved `status` subcommand

The shape validator requires the root to register a child named `status` (presence-by-name; adopter may shadow).
- Source: `cli.go:848-862` (`checkReservedStatus`).

### Other shape rules in play

- `Long:` required on every runnable leaf. Source: `cli.go:867-895`.
- `Short:` required on every runnable leaf and group node (same function — not currently failing).
- Built-in cobra commands and `kit/exempt-validation=true` leaves are skipped. Source: `cli.go:816, 873`.

## Final bucket counts (authoritative)

| Bucket | Count |
|---|---|
| Missing kit/side-effect | 86 |
| Missing kit/idempotent | 61 |
| Missing Long | 48 |
| Missing reserved 'status' subcommand | 1 (root) |
| Missing kit/top-level-verb (depth-1 leaves) | 8 |

Total unique commands across buckets: **89**.

All counts match Agent A's Phase 1 preview exactly. No discrepancies.

## Batch split (3-way parallel)

Each agent annotates its commands end-to-end. Edits are confined to `internal/cli/*.go`.

### Batch C1 — heavy (task / track / tasks deprecated alias / root)

**Verbs**: root + `task`, `tasks`, `track`

**Leaf count**: 28

| Command | Missing | Source |
|---|---|---|
| `tlc` | status-sub | `internal/cli/root.go:RootCmd` |
| `tlc task assign` | Long, idempotent, side-effect | `internal/cli/task_lifecycle.go:159` |
| `tlc task claim` | Long, idempotent, side-effect | `internal/cli/task_lifecycle.go:23` |
| `tlc task complete` | Long, idempotent, side-effect | `internal/cli/task_lifecycle.go:296` |
| `tlc task create` | Long, side-effect | `internal/cli/task_create.go:19` |
| `tlc task delete` | Long, side-effect | `internal/cli/task_update.go:374` |
| `tlc task exec` | idempotent, side-effect | `internal/cli/task_exec.go:44` |
| `tlc task graph` | idempotent, side-effect | `internal/cli/task_graph.go:18` |
| `tlc task list` | Long, side-effect | `internal/cli/task_list.go:20` |
| `tlc task remind` | Long, idempotent, side-effect | `internal/cli/task_remind.go:18` |
| `tlc task reopen` | Long, idempotent, side-effect | `internal/cli/task_lifecycle.go:361` |
| `tlc task show` | Long, side-effect | `internal/cli/task_show.go:24` |
| `tlc task stale` | idempotent, side-effect | `internal/cli/task_stale.go:17` |
| `tlc task sync-projection` | idempotent, side-effect | `internal/cli/task_sync_projection.go:18` |
| `tlc task unassign` | Long, idempotent, side-effect | `internal/cli/task_lifecycle.go:227` |
| `tlc task unclaim` | Long, idempotent, side-effect | `internal/cli/task_lifecycle.go:96` |
| `tlc task update` | Long, side-effect | `internal/cli/task_update.go:22` |
| `tlc tasks sync` | side-effect | `internal/cli/tasks_sync.go:34` |
| `tlc track abandon` | Long, idempotent, side-effect | `internal/cli/track_lifecycle.go:21` |
| `tlc track archive` | Long, idempotent, side-effect | `internal/cli/track_lifecycle.go:14` |
| `tlc track create` | Long, side-effect | `internal/cli/track.go:35` |
| `tlc track delete` | Long, side-effect | `internal/cli/task_update.go:374` |
| `tlc track exec` | idempotent, side-effect | `internal/cli/track_exec.go:23` |
| `tlc track graph` | Long, idempotent, side-effect | `internal/cli/track_graph.go:19` |
| `tlc track list` | Long, side-effect | `internal/cli/track_list.go:26` |
| `tlc track show` | Long, side-effect | `internal/cli/track_show.go:17` |
| `tlc track summary` | Long, idempotent, side-effect | `internal/cli/track_summary.go:17` |
| `tlc track update` | Long, side-effect | `internal/cli/track_update.go:15` |

**Destructive commands** (6; exercises T-1392 `--confirm` bridge):
- `tlc task delete`
- `tlc task unassign`
- `tlc task unclaim`
- `tlc track abandon`
- `tlc track archive`
- `tlc track delete`

### Batch C2 — workflow (agent / flow / inbox / project / prompt / sync)

**Verbs**: `agent`, `flow`, `inbox`, `project`, `prompt`, `sync`

**Leaf count**: 29

| Command | Missing | Source |
|---|---|---|
| `tlc agent cancel` | Long, idempotent, side-effect | `internal/cli/agent_async.go:133` |
| `tlc agent list` | side-effect | `internal/cli/agent_list.go:23` |
| `tlc agent register` | idempotent, side-effect | `internal/cli/agent_registry.go:33` |
| `tlc agent registered` | idempotent, side-effect | `internal/cli/agent_registry.go:109` |
| `tlc agent run` | idempotent, side-effect | `internal/cli/agent_run.go:45` |
| `tlc agent show` | side-effect | `internal/cli/agent_registry.go:80` |
| `tlc agent status` | Long, idempotent, side-effect | `internal/cli/agent_async.go:83` |
| `tlc agent watch` | idempotent, side-effect | `internal/cli/agent_async.go:154` |
| `tlc flow approve` | idempotent, side-effect | `internal/cli/flow_human.go:31` |
| `tlc flow cancel` | idempotent, side-effect | `internal/cli/flow_human.go:95` |
| `tlc flow import` | idempotent, side-effect | `internal/cli/flow_import.go:13` |
| `tlc flow invoke` | idempotent, side-effect | `internal/cli/flow.go:302` |
| `tlc flow list` | side-effect | `internal/cli/flow.go:188` |
| `tlc flow reject` | idempotent, side-effect | `internal/cli/flow_human.go:61` |
| `tlc flow run` | idempotent, side-effect | `internal/cli/flow.go:33` |
| `tlc flow status` | idempotent, side-effect | `internal/cli/flow.go:134` |
| `tlc flow test` | idempotent, side-effect | `internal/cli/flow_test_cmd.go:35` |
| `tlc flow validate` | idempotent, side-effect | `internal/cli/flow_validate.go:11` |
| `tlc inbox process` | idempotent, side-effect | `internal/cli/inbox.go:23` |
| `tlc project export` | idempotent, side-effect | `internal/cli/project.go:30` |
| `tlc project import` | idempotent, side-effect | `internal/cli/project.go:101` |
| `tlc project init` | idempotent, side-effect | `internal/cli/init.go:411` |
| `tlc project list` | Long, side-effect | `internal/cli/project.go:171` |
| `tlc project prune` | idempotent, side-effect | `internal/cli/project.go:242` |
| `tlc prompt task` | Long, idempotent, side-effect | `internal/cli/prompt_context.go:28` |
| `tlc sync config` | Long, idempotent, side-effect | `internal/cli/sync.go:532` |
| `tlc sync pull` | Long, idempotent, side-effect | `internal/cli/sync.go:377` |
| `tlc sync push` | Long, idempotent, side-effect | `internal/cli/sync.go:386` |
| `tlc sync status` | Long, idempotent, side-effect | `internal/cli/sync.go:572` |

**Destructive commands** (4; exercises T-1392 `--confirm` bridge):
- `tlc agent cancel`
- `tlc flow cancel`
- `tlc flow reject`
- `tlc project prune`

### Batch C3 — admin (alias / assignee / auth / config / doctor / init / label / log / schema / tag / tui / upgrade / uri / version / workflow / workspace)

**Verbs**: `alias`, `assignee`, `auth`, `config`, `doctor`, `init`, `label`, `log`, `schema`, `tag`, `tui`, `upgrade`, `uri`, `version`, `workflow`, `workspace`

**Leaf count**: 32

| Command | Missing | Source |
|---|---|---|
| `tlc alias add` | Long, side-effect | `internal/cli/alias.go:45` |
| `tlc alias list` | Long, side-effect | `internal/cli/alias.go:52` |
| `tlc alias remove` | Long, idempotent, side-effect | `internal/cli/alias.go:60` |
| `tlc assignee list` | side-effect | `internal/cli/assignee.go:30` |
| `tlc assignee show` | Long, side-effect | `internal/cli/assignee.go:81` |
| `tlc auth login` | Long, idempotent, side-effect | `internal/cli/auth.go:40` |
| `tlc auth logout` | Long, idempotent, side-effect | `internal/cli/auth.go:100` |
| `tlc auth status` | Long, idempotent, side-effect | `internal/cli/auth.go:114` |
| `tlc config get` | Long, side-effect | `internal/cli/config.go:49` |
| `tlc config interactive` | idempotent, side-effect | `internal/cli/config_interactive.go:276` |
| `tlc config list` | Long, side-effect | `internal/cli/config.go:36` |
| `tlc config set` | Long, idempotent, side-effect | `internal/cli/config.go:63` |
| `tlc config validate` | Long, idempotent, side-effect | `internal/cli/config.go:20` |
| `tlc doctor` | top-level-verb, side-effect | `internal/cli/doctor.go:862` |
| `tlc init` | top-level-verb, idempotent, side-effect | `internal/cli/init.go:411` |
| `tlc label init` | Long, idempotent, side-effect | `internal/cli/labels.go:18` |
| `tlc label list` | Long, side-effect | `internal/cli/labels.go:43` |
| `tlc label templates` | Long, idempotent, side-effect | `internal/cli/labels.go:52` |
| `tlc log` | top-level-verb, idempotent, side-effect | `internal/cli/log.go:39` |
| `tlc schema` | top-level-verb, idempotent, side-effect | `internal/cli/help.go:14` |
| `tlc tag` | top-level-verb | `internal/cli/tag.go:25` |
| `tlc tag list` | Long, side-effect | `internal/cli/tag.go:57` |
| `tlc tui` | top-level-verb, idempotent, side-effect | `internal/cli/tui.go:13` |
| `tlc upgrade` | top-level-verb | `internal/cli/upgrade.go:23` |
| `tlc upgrade preamble` | idempotent, side-effect | `internal/cli/upgrade.go:37` |
| `tlc uri register` | idempotent, side-effect | `internal/cli/cmd_uri_register.go:25` |
| `tlc uri snippet` | idempotent, side-effect | `internal/cli/cmd_uri_register.go:67` |
| `tlc version` | top-level-verb, idempotent, side-effect | `internal/cli/version.go:11` |
| `tlc workflow rules` | Long, idempotent, side-effect | `internal/cli/workflow.go:58` |
| `tlc workflow statuses` | Long, idempotent, side-effect | `internal/cli/workflow.go:16` |
| `tlc workflow validate` | Long, idempotent, side-effect | `internal/cli/workflow.go:103` |
| `tlc workspace list` | Long, side-effect | `internal/cli/workspace.go:28` |

**Destructive commands** (2; exercises T-1392 `--confirm` bridge):
- `tlc alias remove`
- `tlc auth logout`

## Side-effect picker reference (agent guidance)

When choosing `kit/side-effect` for a leaf:

| Verb pattern | Suggested value |
|---|---|
| `list / show / get / find / status / graph / summary / templates / rules / statuses / registered / version / doctor / validate (read-only) / preamble / snippet` | `read` |
| `create / add / register / claim / assign / complete / reopen / update / edit / approve / push / pull / sync / sync-projection / import / export / set / init / remind / stale` | `write-local` (or `write-shared` if effect crosses upstream) |
| `delete / remove / abandon / archive / prune / unassign / unclaim / reject / cancel / logout` | `destructive-local` (or `destructive-shared`) |
| `tui / interactive / run / exec / watch / invoke / test / process / log / login` | `interactive` |

When choosing `kit/idempotent`:

- Kit auto-applies `yes` for: `list/show/get/find/search/info/current/path/paths/doctor/delete/edit/update/use/default/sync/reprocess`.
- Kit auto-applies `no` for: `create/add`.
- Anything not auto-applied still needs an explicit value. Examples:
  - `register / login / approve / reject / cancel / push / pull / import / export / preamble / snippet / set / invoke / run / test / process / watch / interactive / tui / status / validate / rules / statuses / graph / summary / remind / reopen / unassign / unclaim / abandon / archive / init / log` → pick `yes` if re-running with same inputs converges; `no` otherwise.

## Root `status` subcommand fix (C1 owns)

C1 must add a `tlc status` subcommand. Simplest viable shape: a runnable leaf at depth-1 with:
- `Use: "status"`, `Short`, `Long`
- `kit/side-effect = read`, `kit/idempotent = yes`, `kit/top-level-verb = true`
- Body: report current workspace, claimed tasks, sync state (or stub for now). Keep it minimal — presence-by-name is the only validator check.

## Verification (downstream agents run after each batch lands)

```
/usr/bin/env go build -buildvcs=false -o tlc ./cmd/tlc
./tlc --help 2>&1 | head -3
```

A clean exit and missing `cli validation failed:` on stderr means the batch's commands are no longer in violation. Each batch should reduce the baseline-error byte count by the count of commands it annotated.

## Discrepancies vs Agent A's preview

None. All five bucket counts (86 / 61 / 48 / 1 / 8) and the command lists within each bucket match Agent A's preview verbatim.

## Ready to dispatch?

Yes. C1/C2/C3 may proceed in parallel: they touch disjoint files (task_*/track_*/root.go vs agent_*/flow_*/sync_*/project.go/prompt_*/inbox.go vs the admin set). The only cross-batch concern is each batch will independently rebuild `./tlc` and run `--help` — coordinate that they merge into the umbrella branch sequentially (not in parallel) so the build stays clean.


## Phase 4 — `--confirm` policy bridge (T-1392)

Kit 12fcc-leak auto-enforces a `--confirm` gate on every command tagged `kit/side-effect=destructive*`. In a non-TTY context (CI, pipes, scripts) the gate refuses unless `--confirm=yes` is set; in TTY mode the default is `prompt` (kit prints `[y/N]` to stderr and reads stdin).

Contract source: `kit/hops/12fcc-leak/go/console/cli/policy_runE.go:84-359`.

- Flag registered on the root `PersistentFlags()` by `kitcli.New`: `policy_runE.go:21` + `cli.go:486-489`.
- Accepted values: `auto | yes | no | prompt`. Empty defaults to `prompt` on a TTY, `no` otherwise. Invalid values normalise to `prompt`.
- TTY detection via `github.com/mattn/go-isatty` on `cmd.InOrStdin()` (`policy_runE.go:77-82`).
- Auto-enforcement: kit wraps every leaf's `RunE` via `wrapRunESubtree` → `wrapPolicyRunE` (`error_render.go:113-137`). The gate calls `gateConfirm` only when `kit/side-effect` is one of `destructive`, `destructive-local`, `destructive-shared` (or when a YAML policy lifted `require_confirm`). Commands that use cobra's `Run` instead of `RunE` are NOT wrapped — fix by converting to `RunE`.
- Exit code on refusal: 5 (`output.UnauthorizedError` → `CodeUnauthorized`).

### Strategy: hybrid (C)

Local destructive-skip flags (`--yes`, `--no-prompt`, `-y`, `--force`) are kept as silent aliases. A `PreRunE` flips the persistent `--confirm=yes` whenever any of the named local booleans is true. This keeps existing scripts working without deprecation noise.

Helper: `internal/cli/confirm_bridge.go` — `installConfirmBridge(cmd, names...)`. Wiring: `internal/cli/confirm_bridge_init.go`.

### 12-command bridge table

| Command | Side-effect | Local flag(s) | Bridge action |
|---|---|---|---|
| `task delete` | destructive-local | `--yes` / `-y`, `--no-prompt` (persistent on `task`) | `installConfirmBridge(TaskDeleteCmd, "yes", "no-prompt")` |
| `task unassign` | destructive-local | `--no-prompt` (inherited) | `installConfirmBridge(TaskUnassignCmd, "no-prompt")` |
| `task unclaim` | destructive-local | `--no-prompt` (inherited) | `installConfirmBridge(TaskUnclaimCmd, "no-prompt")` |
| `track abandon` | destructive-local | `--no-prompt` | `installConfirmBridge(trackAbandonCmd, "no-prompt")` |
| `track archive` | destructive-local | none | kit gate only |
| `track delete` | destructive-local | none | kit gate only |
| `agent cancel` | destructive-local | none | kit gate only |
| `flow cancel` | destructive-local | none | kit gate only |
| `flow reject` | destructive-local | none | kit gate only |
| `project prune` | destructive-local | `--yes` / `-y` | `installConfirmBridge(ProjectPruneCmd, "yes")` |
| `alias remove` | destructive-local | none | kit gate only |
| `auth logout` | destructive-local | none | kit gate only (also: converted `Run` → `RunE` so the wrapper installs) |

### Operator-facing behaviour changes

- **CI / scripts that already pass `--yes` / `--no-prompt` / `-y`**: continue to work unchanged. The bridge translates them.
- **CI / scripts that invoke a destructive without any skip flag**: now refused with exit 5 instead of running. Add `--confirm=yes` (preferred) or the legacy local flag.
- **Interactive shells (TTY)**: behaviour unchanged — kit's central prompt and tlc's existing local prompts both run; the local prompt is what users see today because it fires first inside `RunE`. The kit prompt is reached only if the local flag bypasses tlc's prompt without setting `--confirm`, which the bridge prevents.
- **`tlc auth logout`**: previously bypassed the gate by using `Run`; now uses `RunE` and is gated like its peers. Side effect: `log.Fatal` replaced with returned error, so the exit code is now driven through kit's error envelope rather than `log.Fatal`'s hardcoded `1`.

### Tests

- Unit: `internal/cli/confirm_bridge_test.go` — synthetic cobra root, three subtests.
- E2E: `internal/cli/confirm_bridge_e2e_test.go` — exec'd tlc binary in a non-TTY pipe, three representative destructives (`task delete`, `flow cancel`, `auth logout`).

Run with: `go test -run "TestConfirm" ./internal/cli/...`
