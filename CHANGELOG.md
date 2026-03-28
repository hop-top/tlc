# Changelog

## Unreleased

### Added

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
