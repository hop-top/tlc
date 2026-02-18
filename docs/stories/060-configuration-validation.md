# 060 - Configuration Validation

**ID**: 060
**Feature**: System Validation
**Persona**: [System](../personas/system.md)
**Related Personas**: All personas
**Priority**: P1

## Story

As a System, I want to validate TLC configuration at startup, so that invalid configuration is detected early with clear error messages.

## Acceptance Scenarios

1. **Given** TLC is starting up, **When** config file is not provided via `-c` flag, **Then** system checks cascade: `/etc/tlc`, `~/.config/tlc/config.yaml`, `~/.config/tlc/config`
2. **Given** a config file exists at any location, **When** it contains invalid values, **Then** system should fail with a descriptive error (currently validated: `output.format`, `storage.backend`, `project.fallback_mode`, `project.duplicate_id_strategy`, `sync.github.repo` when enabled)
3. **Given** environment variables are set, **When** config is loaded, **Then** environment variables with `TLC_` prefix override config values (e.g., `TLC_OUTPUT_FORMAT` overrides `output.format`)
4. **Given** multiple config files exist, **When** config is loaded, **Then** files are merged with project-local config taking precedence over root configs (`.tlc/config.yaml` → `.tlc.yaml` → root configs)
5. **Given** config validate command is run, **When** validation fails, **Then** clear error message is returned
6. **Given** config file exists in multiple locations (e.g., `~/.config/tlc/config.yaml` AND `.tlc/config.yaml`), **When** config is loaded, **Then** the closest config to current directory (`~/.config/tlc/config.yaml`) is merged first, then further configs (`.tlc/config.yaml`, root configs) are merged, with the project-local config taking final precedence

## Notes

**What's Currently Implemented:**
- Config file locations checked: `-c` flag, `/etc/tlc`, `~/.config/tlc/config.yaml`, `~/.config/tlc/config`, `.tlc/config.yaml`, `.tlc.yaml` (merged cascade)
- Validation implemented: `output.format` (table, json, yaml, tls), `storage.backend` (sqlite, local, postgres), `project.fallback_mode` (auto, detected, prompt), `project.duplicate_id_strategy` (share, unique, prompt), `sync.github.repo` (required when enabled)
- Environment variable override: `TLC_*` prefix with dot-to-underscore replacement (e.g., `TLC_OUTPUT_FORMAT`)
- Config CLI commands: `config validate`, `config list`, `config get`, `config set`

**What's Missing:**
- ❌ **Hierarchical config discovery** — See [063 - Hierarchical Config Discovery](063-hierarchical-config-discovery.md) for nested project config merging (traversing up the path until $HOME or global config root)
- ❌ TOML format support (only YAML currently)
- ❌ Config file readability validation before parsing
- ❌ Config path accessibility checks (write permissions)
- ❌ Config file not found warning vs. default configuration behavior
- ❌ Line number in error messages for syntax errors (YAML parser doesn't provide)
- ❌ Unknown keys validation (YAML parser may silently ignore unknown keys)
- ❌ File path in error messages (validation errors don't include which config file)
- ❌ **Cross-platform path support** — See [063 - Hierarchical Config Discovery](063-hierarchical-config-discovery.md) for Windows vs Unix path handling

## Tests

### Unit
- ✅ `internal/config/config_test.go` — `TestLoadConfig_Merging`, `TestApplyEnvOverrides`, `TestConfig_Validate`
- ❌ `internal/config/validation_test.go` — NOT YET CREATED
  - Needed: `TestConfigFileNotFound`, `TestConfigInvalidSyntax`, `TestConfigPathAccessibility`, `TestConfigUnknownKeys`
- ❌ `internal/cli/config_test.go` — NOT YET CREATED
  - Needed: Config CLI command tests for validation scenarios

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Config cascade and merging | `TestLoadConfig_Merging` | ✅ COVERED |
| 2 | Invalid config values → descriptive error | `TestConfig_Validate` | ⚠️ PARTIAL |
| 3 | Environment variable override precedence | `TestApplyEnvOverrides` | ✅ COVERED |
| 4 | Multiple config file merging | `TestLoadConfig_Merging` | ✅ COVERED |
| 5 | Config validate command returns errors | Not tested | ❌ NOT COVERED |

## TODO

- [ ] Create `internal/config/validation_test.go` with config file validation scenarios
- [ ] Create `internal/cli/config_test.go` for config CLI command tests
- [ ] Add config file readability test (file exists and is readable)
- [ ] Add config file write permission test (for set/get commands)
- [ ] Add config path to error messages (which config file failed)
- [ ] Add TOML format support or document why not supported
- [ ] Consider adding unknown keys validation

## Status

📋 Planned - Story defined, partial test coverage exists
