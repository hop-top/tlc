# Changelog

## Unreleased

### Changed

- `tlc task list` now defaults to showing `IN_PROGRESS` and `TODO` tasks only (instead of all statuses), with `IN_PROGRESS` tasks sorted first. Use `--status` to override.

### Fixed

- config discovery now uses the OS user config directory for user config lookup
- config writes now target the active local config or the user config path; they no
  longer fall back to `/etc/tlc/config.yaml` when no writable config exists
- global TODO ingest now preserves `project_id` metadata and ignores foreign
  same-ID task lines instead of overwriting tasks in the current project
