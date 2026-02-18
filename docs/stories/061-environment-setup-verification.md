# 061 - Environment Setup Verification

**ID**: 061
**Feature**: System Validation
**Persona**: [System](../personas/system.md)
**Related Personas**: All personas
**Priority**: P1

## Story

As a System, I want to verify that environment variables are properly loaded and external dependencies are available, so that failures due to incorrect environment are caught early with helpful error messages.

## Acceptance Scenarios

1. **Given** TLC is starting up, **When** environment variables with `TLC_` prefix are set, **Then** they override config file values (e.g., `TLC_OUTPUT_FORMAT` → `output.format`)
2. **Given** TLC config references an external tool (e.g., editor), **When** the tool is not found, **Then** config uses default value but no error is raised at startup
3. **Given** TLC commands execute, **When** `EDITOR` environment variable is not set, **Then** default editor is used
4. **Given** TLC needs XDG paths for data/config home, **When** environment is set, **Then** correct paths are used; when not set, appropriate defaults are used

6. **Given** TLC requires an external tool (git for sync), **When** that tool is not available in PATH, **Then** system should fail with clear error message including installation instructions
7. **Given** TLC loads config from global location (e.g., `~/.config/tlc/config.yaml`), **When** user has project-local config (`.tlc/config.yaml`), **Then** global config is merged with project-local config, with project-local taking precedence

## Notes

**What's Currently Implemented:**
- ✅ Environment variables: `TLC_CONFIG_PATH` (via `-c` flag), `TLC_OUTPUT_FORMAT` (`output.format`), `TLC_LOG_LEVEL` (via `verbose`/`quiet` flags → `output.verbose`/`output.quiet`), `EDITOR` (default for `ui.editor`)
- ✅ XDG support: `XDG_DATA_HOME` (used for default db path), `XDG_CONFIG_HOME` (used for config directory fallback)
- ✅ Environment variable prefix: `TLC_` with automatic dot-to-underscore replacement
- ✅ Automatic environment variable binding via viper

**What's Missing:**
- ❌ No required environment variable validation (no check that specific env vars are set)
- ❌ No external dependency detection (git, ssh, editor availability)
- ❌ No version requirement validation (e.g., "git >= 2.30")
- ❌ No environment variable value validation (type checking: numeric, path validity)
- ❌ No error messages when dependencies missing with installation instructions
- ❌ No semantic version comparison logic
- ❌ No config file merging behavior from global/user/project directories (currently: `findAllConfigs` in `internal/cli/root.go` stops at filesystem root without checking `$HOME` or `$XDG_CONFIG_HOME`)

## Tests

### Unit
- ❌ `internal/system/env_test.go` — NOT YET CREATED (package doesn't exist)
  - Needed: `TestRequiredEnvVarsMissing`, `TestExternalDependencyMissing`, `TestDependencyVersionMismatch`, `TestEnvVarInvalidValue`
- ❌ `internal/cli/env_test.go` — NOT YET CREATED
  - Needed: Environment validation CLI command tests

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Env vars override config values | Manual testing only | ⚠️ PARTIAL |
| 2 | Missing editor → default value | Manual testing only | ⚠️ PARTIAL |
| 3 | EDITOR env var usage | Manual testing only | ⚠️ PARTIAL |
| 4 | XDG paths with defaults | Manual testing only | ⚠️ PARTIAL |

## TODO

- [ ] Create `internal/system/` package for environment validation
- [ ] Create `internal/system/env_test.go` with all environment validation scenarios
- [ ] Add external dependency checks: `git`, `ssh`, editor availability
- [ ] Add semantic version checking logic for git and other tools
- [ ] Add environment variable type validation (numeric, path format)
- [ ] Add error messages with installation instructions when dependencies missing
- [ ] Create `internal/cli/env_test.go` for env validation CLI command tests

## Status

📋 Planned - Story defined, implementation gaps identified, no test coverage
