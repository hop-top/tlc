# Changelog

## Unreleased

### Added

- Root `tlc status` subcommand: read-only project + caller snapshot
  (detected project, caller identity, in-progress/TODO task counts,
  overdue items). Satisfies kit's `checkReservedStatus` shape rule
  and gives users a short health view at the root. (T-1394)
- Kit 12fcc command-surface annotations on every cobra leaf:
  `kit/side-effect` (86 leaves), `kit/idempotent` (61),
  `kit/top-level-verb` (8), plus `Long:` descriptions on 48 leaves
  that previously had none. See
  [`docs/12fcc-conformance-split-plan.md`](docs/12fcc-conformance-split-plan.md)
  for the full contract. (T-1389 — umbrella)
- `--note|-n` flag on `tlc task update` (when `--status` is set) and
  `tlc task delete` (T-1178). Note text is recorded on the
  STATUS_CHANGED / DELETED `task_logs` entry, mirroring the existing
  `tlc task complete --note` pattern.
- `tlc/runtime/policy` adoption: declarative policy YAML at
  `$XDG_CONFIG_HOME/tlc/policies.yaml`, evaluated against state
  changes via kit's policy engine. First boot seeds the file from
  a bundled default; existing user files are never overwritten.
  Default policy: `delete-requires-note`. See
  [`docs/policies.md`](docs/policies.md). (T-1192)

### Changed

- Bumped `hop.top/kit` to the 12fcc-leak branch tip (strict
  command-surface validation: `Root.Validate()` fires at boot and
  rejects unannotated leaves). Boot now fails loud if any leaf is
  missing a required annotation — the new `TestStrictValidationPasses`
  regression test guards this contract in CI. (T-1389)
- Destructive commands (`task delete`, `task unassign`, `task unclaim`,
  `track abandon`, `track archive`, `track delete`, `agent cancel`,
  `flow cancel`, `flow reject`, `project prune`, `alias remove`,
  `auth logout` — 12 in total) now require `--confirm=yes` from kit's
  persistent flag in non-TTY contexts. The
  pre-existing local flags (`--force`, `--yes`, `--no-prompt`) continue
  to work as bridges: when any local skip flag is true the bridge sets
  the inherited `--confirm` to `yes` before the kit gate runs. No
  user-facing CLI change unless scripts run destructive commands
  without any skip flag in a pipe. (T-1392)
- Schema migration v15: `task_logs` rebuilt without
  `ON DELETE CASCADE` on `task_id` so transition notes — including
  delete reasons recorded via `--note` — survive task deletion.
  Migration applies automatically on first run; existing rows are
  preserved. (T-1232)

### Deprecated

- Legacy `aliases:` key in `config.yaml` is now auto-removed after a one-time
  migration to `<config-dir>/aliases.yaml`. The deprecation warning that
  previously fired on every tlc invocation is now a single one-time message;
  on subsequent runs, no warning. No user action required — migration is
  defensive (won't strip the legacy key unless the YAML store has every
  legacy entry first). User-level + project-level configs both covered;
  system config (`/etc/tlc/`) intentionally untouched (root-only). (T-1339)

### Note for operators

- `tlc task delete <id>` without `--note` now exits 4 with a
  `policy "delete-requires-note" denied: ...` error. Update scripts
  to pass `--note "<reason>"`. To restore the pre-T-1192 behavior
  temporarily, edit `$XDG_CONFIG_HOME/tlc/policies.yaml` and remove
  the `delete-requires-note` rule.

### chore
- kit: migrate to role-based hierarchy (`hop.top/kit/go/<role>/<pkg>`).
  All 16 kit packages tlc imports moved from flat to role-based paths
  (e.g. `hop.top/kit/log` → `hop.top/kit/go/console/log`). Replace
  directive in `go.mod` carries the local kit + hdl worktrees until
  upstream tags publish. See `docs/kit-migration-playbook.md` for
  the migration procedure other consumers can reuse. Refs: T-0757,
  T-0794, PR #92.

### feat
- Plan ingestion: two-phase cross-track ref resolution handles
  circular plan references. Phase 1 creates tasks and captures
  any ref whose target track/plan/task is missing in
  `Meta["blocked_by_unresolved"]` instead of hard-failing.
  Phase 2 runs project-wide after every `track update
  --add-plan`, promotes newly resolvable refs into `blocked_by`,
  and rewrites plan.md files whose refs are now all concrete.
  A set of mutually-referencing plans can now be ingested in any
  order without the chicken-and-egg deadlock from T-0434. Hard
  errors (index out of range, title ambiguity) still fail the
  ingest immediately. (T-0435, story 076 scenarios 10-12)
- Plan ingestion: `blocked-by` frontmatter now accepts mixed entries:
  integer indices (intra-track, existing behaviour), concrete
  `"T-NNNN"` task IDs, and `"<track-id>#<N>"` cross-track refs
  (1-based into target track's linked plan). On success the
  source plan.md is rewritten on disk so subsequent ingestions
  read stable `T-NNNN` IDs. (T-0434, story 076)
- Cross-domain NL classifier: track/flow/project prompts now resolve without LLM
  - Keyword tokenizer + vocabulary layer (verbs, nouns, modifiers)
  - Fuzzy noun/verb matching via Levenshtein distance (typo-tolerant, distance ≤2)
  - Confidence scoring: exact match=1.0, distance-1=0.9, distance-2=0.8
  - Aggregate patterns: "count active tracks" → `track list --status active`
  - Pipeline: regex classifier → cross-domain classifier → LLM (LLM only as last resort)

### Added

- **Track registry** — first-class work stream entity grouping tasks with
  lifecycle management, computed health state, and phase progress:
  - `tlc track create/list/show/update/archive/abandon/delete` commands
  - `tlc track summary` project health pulse view
  - Track status machine: pending → active → completed/abandoned → archived
  - Computed state flags: stale, unlinked, blocked, healthy
  - Phase progress from task `phase:N` tags (format: `5/8 (P2/3)`)
  - `--track` flag on `tlc task create/update/list` for linking
  - Auto-transition: pending → active on first linked task claim
  - `--add-plan` flag with frontmatter task extraction and blocked-by
    resolution
  - Configurable plan-extractor command support
  - Cross-project qualified track IDs (`org_repo--track-id`)
  - `--all-projects` flag on `tlc track list`
  - Health thresholds in config: `tracks.stale_threshold`,
    `tracks.health.max_active`, `tracks.health.min_progress_to_start`
  - Overcommit warning when active tracks exceed threshold
- `--blocked <reason>` on `tlc task update`: set blocked reason on a task.
- `--unblock` on `tlc task update`: clear blocked reason.
- `--timeout <duration>` on `tlc task update` and `tlc task create`: set per-task
  stale timeout (e.g. `2h`, `30m`). Resets stale-crossing state on update.
- `effort` field on tasks: set sizing estimate (XS/S/M/L/XL) via
  `tlc task create --effort` or `tlc task update --effort`.
  Shown in `tlc task show`; serialized in JSON/YAML/TLS formats.

### Changed

- `tlc task list` now defaults to showing `IN_PROGRESS` and `TODO` tasks only (instead of all statuses), with `IN_PROGRESS` tasks sorted first. Use `--status` to override.

### Fixed

- config discovery now uses the OS user config directory for user config lookup
- config writes now target the active local config or the user config path; they no
  longer fall back to `/etc/tlc/config.yaml` when no writable config exists
- global TODO ingest now preserves `project_id` metadata and ignores foreign
  same-ID task lines instead of overwriting tasks in the current project
