# Changelog

## Unreleased

### Fixed

- config discovery now uses the OS user config directory for user config lookup
- config writes now target the active local config or the user config path; they no
  longer fall back to `/etc/tlc/config.yaml` when no writable config exists
