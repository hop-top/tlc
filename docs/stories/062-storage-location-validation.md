# 062 - Storage Location Validation

**ID**: 062
**Feature**: System Validation
**Persona**: [System](../personas/system.md)
**Related Personas**: All personas
**Priority**: P1

## Story

As a System, I want to validate that the database directory is accessible and created, so that operations fail early with clear error messages.

## Acceptance Scenarios

1. **Given** TLC is initializing storage, **When** storage backend is not `sqlite`, **Then** system returns descriptive error ("unsupported storage backend")
2. **Given** database path is not configured, **When** system initializes storage, **Then** default path is used: `$XDG_DATA_HOME/tlc/db.sqlite` or `~/.local/share/tlc/db.sqlite`
3. **Given** storage backend is `sqlite`, **When** database directory doesn't exist, **Then** system creates directory with permissions `0755` (read/write for owner, read/execute for others)
4. **Given** storage backend is `sqlite`, **When** database directory doesn't exist, **Then** system creates directory with permissions `0755` (read/write for owner, read/execute for others)
5. **Given** storage backend is `sqlite`, **When** database directory creation fails, **Then** system returns descriptive error ("failed to create database directory")
6. **Given** TLC storage backend is `sqlite` and database path is configured, **When** multiple config files exist at different locations, **When** config is loaded, **Then** they are merged with project-local config taking precedence over root configs (same behavior as config: `.tlc/config.yaml` → `.tlc.yaml` → root configs)

## Notes

**What's Currently Implemented:**
- ✅ Storage backend validation: checks for `sqlite` (only supported backend)
- ✅ Database path detection: from config or defaults to XDG path
- ✅ Directory creation: `os.MkdirAll(filepath.Dir(dbPath), 0755)`
- ✅ Error handling: returns descriptive error if directory creation fails
- ✅ Database initialization: `sql.Open("sqlite", path)` with SQLite pragmas (foreign keys, WAL mode, busy timeout)
- ✅ Config file merging behavior from multiple locations: project-local config takes precedence over root configs

**What's Missing:**
- ❌ Log location validation or fallback mechanism
- ❌ Sync state directory validation
- ❌ Cache directory validation
- ❌ Disk space threshold check (100MB minimum)
- ❌ Corrupted state detection (file integrity, schema version)
- ❌ Permission checks for existing directories (read/write access beyond creation)
- ❌ Storage location validation CLI command
- ❌ **Hierarchical task merging** — See [063 - Hierarchical Config Discovery](063-hierarchical-config-discovery.md) for merging tasks from multiple `.tlc/` directories up the path tree

**What's NOT in Current Code:**
- The code does NOT validate log file location
- The code does NOT validate sync state directory
- The code does NOT validate cache directory
- The code does NOT check disk space
- The code does NOT detect corrupted database state
- The code does NOT have a storage validation CLI command

## Tests

### E2E
- planned: `tests/e2e/storage_location_test.go::TestStorage_UnsupportedBackendErrors`
- planned: `tests/e2e/storage_location_test.go::TestStorage_DefaultDBPathFromXDG`
- planned: `tests/e2e/storage_location_test.go::TestStorage_CreatesDirWith0755`
- planned: `tests/e2e/storage_location_test.go::TestStorage_DirCreationFailureReportsError`
- planned: `tests/e2e/storage_location_test.go::TestStorage_HierarchicalConfigMerged`

### Unit
- ❌ `internal/storage/locations_test.go` — NOT YET CREATED (package doesn't exist)
  - Needed: `TestStorageDirCreation`, `TestStoragePermissions`, `TestLogLocationFallback`, `TestSyncStateCorruption`, `TestDiskSpaceCheck`
- ❌ `internal/cli/storage_test.go` — NOT YET CREATED
  - Needed: Storage validation CLI command tests

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test Coverage | Status |
|----------|----------|---|--------|
| 1 | Invalid storage backend → descriptive error | Manual testing only | ⚠️ PARTIAL |
| 2 | Missing db_path → XDG default | Manual testing only | ⚠️ PARTIAL |
| 3 | Non-existent storage dir → create with permissions | Manual testing only | ⚠️ PARTIAL |
| 4 | Directory creation failure → descriptive error | Manual testing only | ⚠️ PARTIAL |

## TODO

- [ ] Create `internal/storage/locations_test.go` with all storage validation scenarios
- [ ] Add storage location tests: tasks directory, log directory, sync state directory, cache directory
- [ ] Verify default paths: `~/.tlc/tasks`, `~/.tlc/logs`, `~/.tlc/sync`, `~/.tlc/cache`
- [ ] Add permission tests: read and write access for current user
- [ ] Add disk space threshold test: 100MB minimum free space
- [ ] Add corrupted state detection tests: file integrity, schema version compatibility
- [ ] Test log location fallback behavior
- [ ] Create `internal/cli/storage_test.go` for storage validation CLI command tests

## Status

📋 Planned - Story defined, basic implementation exists, critical validation missing
