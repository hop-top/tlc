# 063 - Hierarchical Config Discovery

**ID**: 063
**Feature**: System Validation
**Persona**: [System](../personas/system.md)
**Related Personas**: All personas
**Priority**: P1

## Story

As a System, I want to discover and merge configuration from the current directory and
all parent directories up to the common ancestor of the current working directory and
the user-global config directory, so that projects nested within a larger organization
can inherit configuration and tasks from parent directories without traversing unrelated
paths.

## Acceptance Scenarios

1. **Given** TLC is starting from `/Users/jadb/projects/team/project-a`, **When** config is loaded, **Then** system checks and merges from this order:
   - `/Users/jadb/projects/team/project-a/.tlc/config.yaml` (current directory)
   - `/Users/jadb/projects/team/.tlc/config.yaml` (parent)
   - `/Users/jadb/projects/.tlc/config.yaml` (grandparent)
   - `/Users/jadb/.tlc/config.yaml` (common-ancestor boundary, if present)
   - `/Users/jadb/.config/tlc/config.yaml` (global user config)
   - System defaults

2. **Given** TLC is starting from nested project `/Users/jadb/org/backend/api/v2`, **When** config is loaded, **Then** system checks all `.tlc/config.yaml` files in hierarchy and merges them with current directory taking highest precedence

3. **Given** multiple config files exist in hierarchy, **When** config values conflict, **Then** child config overrides parent config (closest to working directory wins)

4. **Given** tasks exist in `/Users/jadb/org/.tlc/tasks.md` and `/Users/jadb/org/backend/.tlc/tasks.md`, **When** user runs `tlc task list` from `/Users/jadb/org/backend/api`, **Then** tasks from both parent directories are merged and displayed (deduplicated by ID)

5. **Given** tasks with same ID exist in multiple `.tlc/tasks.md` files in hierarchy, **When** loading tasks, **Then** child directory task takes precedence (closest to working directory wins)

6. **Given** the common ancestor of `cwd` and the user-global config directory is
   `/`, **When** config is loaded, **Then** traversal continues to filesystem root

7. **Given** TLC is running on Windows, **When** config is loaded, **Then** system uses Windows-appropriate paths:
   - User config: `%APPDATA%\tlc\config.yaml`
   - Traversal stops at the common ancestor of `%USERPROFILE%\...cwd...` and
     `%APPDATA%\tlc`
   - Project configs: `.tlc/config.yaml` in each directory

8. **Given** TLC is running on Linux/macOS, **When** config is loaded, **Then** system uses Unix-appropriate paths:
   - User config: `~/.config/tlc/config.yaml` (or `~/.tlc/config.yaml` as fallback)
   - Traversal stops at the common ancestor of `cwd` and `~/.config/tlc`
   - Project configs: `.tlc/config.yaml` in each directory

9. **Given** no `.tlc` directories exist between `cwd` and the computed boundary,
   **When** config is loaded, **Then** system uses global user config and system
   defaults only

10. **Given** user runs `tlc config list`, **When** multiple config files exist in hierarchy, **Then** output shows merged config with source annotations for each value (e.g., "output.format: table (from: project)")

## Notes

**Design Decisions:**

**Traversal Strategy:**
- Start from current working directory
- Walk up directory tree checking for `.tlc/config.yaml` at each level
- Stop when reaching the common ancestor of `cwd` and the user-global config directory
- If that ancestor is `/`, traversal continues to filesystem root

**Merging Semantics:**
- Config values: deeper (closer to cwd) overrides shallower
- Tasks: all tasks in hierarchy are merged and deduplicated
- Task ID conflicts: closer to cwd wins
- Task metadata: merge (child inherits from parent unless overridden)

**Platform Support:**
- macOS: $HOME = `/Users/username`, XDG config at `~/.config/tlc/config.yaml`
- Linux: $HOME = `/home/username`, XDG config at `~/.config/tlc/config.yaml`
- Windows: $USERPROFILE = `C:\Users\username`, APPDATA = `%APPDATA%\tlc\config.yaml`

**Performance Considerations:**
- Cache discovered config paths per session
- Limit traversal depth to prevent infinite loops (e.g., 100 directories max)
- Early exit when the computed boundary is reached

**What's Currently Implemented:**
- ✅ User config: `~/.config/tlc/config.yaml` (hardcoded for Unix)
- ✅ Project config: `.tlc/config.yaml` (single level only)
- ✅ Config merging logic exists in `internal/config/loader.go`

**What's Missing:**
- ❌ Windows path support
- ❌ Task merging from multiple `.tlc/` directories
- ❌ Task deduplication by ID
- ❌ Source annotations in `tlc config list`
- ❌ Traversal depth limiting

## Implementation Plan

**Phase 1: Platform-Agnostic Path Handling**
1. Create `internal/config/paths.go` with platform-specific path resolution
2. Add functions:
   - `GetHomeDir()` - cross-platform home directory detection
   - `GetGlobalConfigPath()` - XDG on Unix, APPDATA on Windows
   - `GetCustomGlobalRoot()` - from TLC_GLOBAL_ROOT env var
   - `NormalizePath()` - normalize paths for comparison

**Phase 2: Directory Traversal**
1. Create `internal/config/discovery.go` with hierarchical discovery
2. Add function:
   - `DiscoverConfigHierarchy(cwd string) ([]string, error)`
   - Traverses up from cwd, collects all `.tlc/config.yaml` paths
   - Stops at home directory or custom global root
   - Limits traversal depth (max 100)

**Phase 3: Task Merging**
1. Create `internal/config/tasks.go` with hierarchical task loading
2. Add functions:
   - `LoadTasksFromHierarchy(cwd string) ([]Task, error)`
   - Loads tasks from each `.tlc/tasks.md` in hierarchy
   - Deduplicates by ID (closest wins)
   - Merges task metadata (child inherits from parent unless overridden)

**Phase 4: Config Merging**
1. Update `LoadConfig()` in `internal/config/loader.go`
2. Use `DiscoverConfigHierarchy()` to get all config paths
3. Merge configs in order (system → global → parents → cwd)
4. Add source tracking for `tlc config list`

**Phase 5: CLI Enhancements**
1. Update `tlc config list` to show source annotations
2. Add `tlc config hierarchy` command to show discovered paths
3. Update `tlc task list` to use `LoadTasksFromHierarchy()`

## Tests

### Unit

**Path Handling (`internal/config/paths_test.go`):**
- ❌ NOT YET CREATED
  - Needed: `TestGetHomeDir_Unix`, `TestGetHomeDir_Windows`
  - Needed: `TestGetGlobalConfigPath_Unix`, `TestGetGlobalConfigPath_Windows`
  - Needed: `TestGetCustomGlobalRoot`, `TestNormalizePath`

**Discovery (`internal/config/discovery_test.go`):**
- ❌ NOT YET CREATED
  - Needed: `TestDiscoverConfigHierarchy_NestedProject`
  - Needed: `TestDiscoverConfigHierarchy_StopsAtHome`
  - Needed: `TestDiscoverConfigHierarchy_CustomGlobalRoot`
  - Needed: `TestDiscoverConfigHierarchy_NoTlcDirs`
  - Needed: `TestDiscoverConfigHierarchy_DepthLimit`

**Task Merging (`internal/config/tasks_test.go`):**
- ❌ NOT YET CREATED
  - Needed: `TestLoadTasksFromHierarchy_Merge`
  - Needed: `TestLoadTasksFromHierarchy_Deduplicate`
  - Needed: `TestLoadTasksFromHierarchy_Inheritance`
  - Needed: `TestLoadTasksFromHierarchy_MetadataOverride`

**Config Merging (`internal/config/loader_test.go`):**
- ✅ EXISTS but needs updates
  - Update: `TestLoadConfig_Merging` to test hierarchical configs
  - Add: `TestLoadConfig_SourceTracking`
  - Add: `TestLoadConfig_PlatformPaths`

### Integration

**E2E Scenarios (`tests/integration/config_hierarchy_test.go`):**
- ❌ NOT YET CREATED
  - Needed: `TestConfigHierarchy_NestedProject`
  - Needed: `TestConfigHierarchy_TasksMerge`
  - Needed: `TestConfigHierarchy_CustomGlobalRoot`
  - Needed: `TestConfigHierarchy_WindowsPaths` (mocked)
  - Needed: `TestConfigHierarchy_LinuxPaths` (mocked)

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Nested config discovery and merging | ❌ NOT CREATED | ❌ NOT COVERED |
| 2 | Deep nesting traversal | ❌ NOT CREATED | ❌ NOT COVERED |
| 3 | Config value precedence (child > parent) | ❌ NOT CREATED | ❌ NOT COVERED |
| 4 | Tasks merge from parent directories | ❌ NOT CREATED | ❌ NOT COVERED |
| 5 | Task deduplication (closest wins) | ❌ NOT CREATED | ❌ NOT COVERED |
| 6 | Custom global config root | ❌ NOT CREATED | ❌ NOT COVERED |
| 7 | Windows path support | ❌ NOT CREATED | ❌ NOT COVERED |
| 8 | Unix path support (macOS/Linux) | ❌ NOT CREATED | ❌ NOT COVERED |
| 9 | No .tlc dirs in hierarchy | ❌ NOT CREATED | ❌ NOT COVERED |
| 10 | Source annotations in config list | ❌ NOT CREATED | ❌ NOT COVERED |

## TODO

- [ ] Create `internal/config/paths.go` with platform-agnostic path handling
- [ ] Create `internal/config/discovery.go` with directory traversal logic
- [ ] Create `internal/config/tasks.go` with hierarchical task loading
- [ ] Update `internal/config/loader.go` to use hierarchical discovery
- [ ] Add unit tests for path handling (Unix and Windows)
- [ ] Add unit tests for directory traversal
- [ ] Add unit tests for task merging and deduplication
- [ ] Update existing config tests to cover hierarchical scenarios
- [ ] Create integration tests for full hierarchy scenarios
- [ ] Update `tlc config list` CLI command with source annotations
- [ ] Add `tlc config hierarchy` CLI command
- [ ] Update `tlc task list` CLI command to use hierarchical task loading
- [ ] Document custom global config root (TLC_GLOBAL_ROOT env var)
- [ ] Document Windows vs Unix path differences
- [ ] Document task inheritance and merging semantics
- [ ] Add config source tracking to `Config` struct

## Status

📋 Planned - Story defined, implementation pending
