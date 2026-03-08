# Workspace Model Design

**Date:** 2026-03-08
**Status:** Implemented
**Tasks:** T-0001, T-0002, T-0003, T-0004

## Problem

TLC currently operates on a single project database at a time. Users working across many repos (e.g. 30+ repos under `$HOME/.w/ideacrafterslabs/`) need a way to query tasks across all of them without manually specifying each project.

## Concepts

### Hierarchy

```
wsm workspace (e.g. "wsm")
  └── space (user-defined URI with adapter)
       ├── local dir   → ~/.w/ideacrafterslabs/  (adapter: filesystem)
       ├── github org   → github://hop-top        (adapter: github)
       ├── docs site    → https://docs.example.com (adapter: web)
       └── database     → postgres://host/db       (adapter: sql)
            └── projects (discovered by adapter)
                 └── tasks
```

### Definitions

- **Workspace**: Maps 1:1 to a wsm workspace. Contains one or more spaces. Stored in TLC's global config.
- **Space**: A user-defined URI identifying a source of projects/tasks. The URI scheme determines which adapter handles it. A space can be a local directory, a GitHub org, a documentation site, a database — anything with an adapter.
- **Adapter**: Implements the `SpaceAdapter` interface for a URI scheme. Responsible for discovering projects and providing task access for that space type.
- **Project**: A unit within a space that has its own task collection. For filesystem spaces, this maps to a repo with `.tlc/db.sqlite`. For other adapters, the mapping is adapter-specific (e.g. a GitHub repo, a database schema).

### Key principles

1. **Adapter-driven discovery**: Each space URI is handled by an adapter that knows how to discover projects and access tasks. No single discovery strategy.
2. **wsm-anchored**: Workspaces reference wsm workspace IDs so the two systems stay in sync.
3. **Space association on add**: When creating tasks from workspace context, TLC detects which space/project the task belongs to. If ambiguous, it prompts the user with options.
4. **Local-first**: The filesystem adapter is the primary adapter shipped in phase 1. Other adapters follow as plugins or built-in extensions.

## Global Config Schema

```yaml
# $XDG_CONFIG_HOME/tlc/config.yaml
workspaces:
  - name: default
    wsm_id: "01KHQDJPDMC8WJQ5HH4V49ZJ4V"  # optional, links to wsm
    spaces:
      - uri: "~/.w/ideacrafterslabs"
        adapter: filesystem                  # inferred from URI scheme when omitted
        label: ideacrafterslabs              # display name, defaults to basename/host
      - uri: "~/.w/exo"
        label: exo
      - uri: "github://hop-top"
        adapter: github
        label: hop-top
      - uri: "https://docs.example.com"
        adapter: web
        label: docs
    default: true  # used when no --workspace flag is given
```

The `adapter` field can be omitted when the URI scheme is unambiguous:
- bare paths / `file://` → `filesystem`
- `github://` → `github`
- other schemes → must be explicit

### Config struct addition

```go
// config/config.go
type WorkspaceConfig struct {
    Name    string        `yaml:"name"`
    WsmID   string        `yaml:"wsm_id,omitempty"`
    Spaces  []SpaceConfig `yaml:"spaces"`
    Default bool          `yaml:"default,omitempty"`
}

type SpaceConfig struct {
    URI     string            `yaml:"uri"`
    Adapter string            `yaml:"adapter,omitempty"` // filesystem, github, web, sql, ...
    Label   string            `yaml:"label,omitempty"`
    Options map[string]string `yaml:"options,omitempty"` // adapter-specific config
}

// Added to Config:
type Config struct {
    // ...existing fields...
    Workspaces []WorkspaceConfig `yaml:"workspaces,omitempty"`
}
```

## Project Registry (Global DB)

The global TLC database is the source of truth for known projects. A new `projects` table stores the project registry:

```sql
CREATE TABLE IF NOT EXISTS projects (
    project_id TEXT PRIMARY KEY,
    db_path    TEXT NOT NULL,       -- absolute path to project's db.sqlite
    space_uri  TEXT,                -- space this project belongs to (if known)
    label      TEXT,                -- human-readable name
    registered_at TEXT NOT NULL,    -- ISO 8601
    last_seen_at  TEXT NOT NULL,    -- updated on each tlc command in this project
    status     TEXT NOT NULL DEFAULT 'active'  -- active, stale, orphan
);
```

### `tlc init` and project reconnection

When `tlc init` runs in a directory:

1. Detect `project_id` from git remote (existing `DetectProjectID()` chain)
2. Query global DB: `SELECT * FROM projects WHERE project_id = ?`
3. **Match found:**
   - Project exists with tasks. Update `db_path` to current location, set `last_seen_at`, set `status = 'active'`
   - Copy/link the existing project DB if the old path is gone
   - Print: `Reconnected to existing project "hop-top/tlc" (23 tasks, 5 open)`
4. **No match:**
   - Insert new row into `projects` table
   - Create fresh `.tlc/db.sqlite`
   - Print: `Initialized new project "hop-top/tlc"`

This means a repo can be deleted and re-cloned anywhere, and `tlc init` seamlessly reconnects it to its task history — the `project_id` (from git remote) is the stable identifier, the `db_path` is just the current location.

### Path staleness

On each TLC command that touches a project DB, update `last_seen_at`. A background check can mark projects as `stale` (path no longer exists) or `orphan` (path exists but `.tlc/` gone). `tlc doctor` reports these.

## Space Adapter Interface

The adapter is the core abstraction. Each adapter knows how to discover projects and provide task access for its URI scheme. For filesystem spaces, the adapter queries the global `projects` table filtered by `space_uri` rather than scanning the filesystem.

```go
// internal/workspace/adapter.go
package workspace

// RegisteredProject represents a project known to the global registry.
type RegisteredProject struct {
    ID         string // project_id from global DB
    DBPath     string // absolute path to db.sqlite
    SpaceURI   string
    Label      string
    Status     string // active, stale, orphan
}

// ProjectSource provides task access for a project.
// Concrete types are adapter-specific.
type ProjectSource interface {
    // OpenReadOnly returns a storage handle for querying tasks.
    OpenReadOnly() (core.TaskReader, error)
    // OpenReadWrite returns a storage handle for creating/updating tasks.
    OpenReadWrite() (core.TaskStorage, error)
}

// SpaceAdapter discovers projects within a space.
type SpaceAdapter interface {
    // Schemes returns the URI schemes this adapter handles (e.g. ["file", ""], ["github"]).
    Schemes() []string
    // Discover returns projects belonging to this space.
    // For filesystem adapter: queries global projects table filtered by space_uri.
    // For API adapters: calls external API.
    Discover(space SpaceConfig) ([]RegisteredProject, error)
    // Contains reports whether a local path falls within this space.
    // Returns the matching project or nil. Non-local adapters return nil.
    Contains(space SpaceConfig, localPath string) *RegisteredProject
}
```

### Built-in: Filesystem Adapter

The filesystem adapter handles bare paths and `file://` URIs. Instead of scanning the filesystem for `.tlc/` directories, it queries the global `projects` table:

```sql
SELECT * FROM projects WHERE space_uri = ? AND status = 'active'
```

This is fast (indexed lookup), avoids filesystem scanning, and works even when projects are temporarily offline.

**How projects get into the registry:**
- `tlc init` in a directory → registers the project with its `space_uri` derived from the path
- Space URI is inferred: if the project is at `~/.w/ideacrafterslabs/tlc/`, the space URI is `~/.w/ideacrafterslabs`

### Future Adapters (examples)

| Adapter | Scheme | Discovers | Source |
|---------|--------|-----------|--------|
| `filesystem` | `file://`, bare paths | Global `projects` table by `space_uri` | SQLite direct |
| `github` | `github://` | Repos in org via API | GitHub Issues API |
| `linear` | `linear://` | Teams/projects via API | Linear API |
| `jira` | `jira://` | Projects via API | Jira API |
| `web` | `https://` | Crawled/indexed content | Read-only task extraction |

Adapters beyond `filesystem` are not in scope for phase 1 but the interface is designed to accommodate them.

### Adapter Registry

```go
// internal/workspace/registry.go

// Registry maps adapter names and URI schemes to adapter implementations.
type Registry struct {
    adapters map[string]SpaceAdapter // keyed by adapter name
    schemes  map[string]string       // scheme → adapter name
}

// Resolve returns the adapter for a space config.
// Uses explicit adapter field first, then falls back to URI scheme.
func (r *Registry) Resolve(space SpaceConfig) (SpaceAdapter, error)
```

### `current` symlink and CWD tracking

Hop-mode repos use a `current` symlink (e.g. `tlc/current -> hops/feat/T-0008-profile-mapping`) to point to the active worktree. Tools that resolve this symlink to an absolute path at startup will not see updates when the symlink changes.

**Known limitation:** Changing the `current` symlink does not automatically update the CWD of terminals, IDEs, or other tools already using the resolved path. This is an OS-level behavior, not something TLC can solve alone.

**Mitigation strategies (future):**
- File-watch based notification (fsnotify on the symlink)
- Shell hook that re-evaluates `current` on each prompt (PROMPT_COMMAND / precmd)
- IDE plugin that watches for symlink changes
- wsm event integration: `wsm` emits events on workspace switches that tools can subscribe to

For TLC's workspace queries, this is not an issue — the global `projects` table stores the `db_path` at the stable repo root, independent of which worktree is active.

### Workspace-level Discovery

```go
// internal/workspace/workspace.go
package workspace

// ListProjects returns all projects across all spaces in a workspace.
// For filesystem spaces, this queries the global projects table.
// For API spaces, this calls the adapter's Discover method.
func ListProjects(ws config.WorkspaceConfig, reg *Registry) ([]RegisteredProject, error)
```

## Cross-Workspace Query

### Architecture

```
tlc task list --workspace default
    │
    ├── 1. Load workspace config from global config
    ├── 2. ListProjects() → query global projects table by space_uri
    ├── 3. For each active project:
    │       ├── Open db_path via ProjectSource (read-only)
    │       ├── Run query
    │       └── Collect results
    ├── 4. Merge results, apply sort/limit
    └── 5. Render with project column
```

### Query struct changes

```go
// core/query.go
type Query struct {
    // ...existing fields...
    AllProjects bool
    Workspace   string   // workspace name to query across
    Spaces      []string // filter to specific spaces within workspace
}
```

### Multi-source executor

```go
// internal/workspace/executor.go
package workspace

// QueryAcross opens each discovered project via its adapter's ProjectSource,
// runs the query, and merges results. Sources are opened read-only.
func QueryAcross(projects []DiscoveredProject, q core.Query) ([]core.Task, error)
```

Each project source is opened read-only. Results are merged in-memory and sorted according to the query's sort parameters. The executor is adapter-agnostic — it works through the `TaskReader` interface returned by `ProjectSource.OpenReadOnly()`.

### CLI changes

```
tlc task list --workspace default        # all tasks across workspace
tlc task list --workspace default --space ideacrafterslabs  # filter to one space
tlc task list                            # current project only (unchanged)
tlc task list --all-projects             # all projects in current DB (unchanged)
```

When `--workspace` is provided, the output table gains a `Project` column showing the project_id (or a short label derived from the space + repo name).

## Task Creation from Workspace Context

### Space detection flow

```
tlc task create "Fix auth bug" --workspace default
    │
    ├── 1. Is cwd inside a known space?
    │       └── Yes → use that space + project
    ├── 2. Does task title/ref match a known project pattern?
    │       └── Yes → use that project
    └── 3. Ambiguous → prompt user
            ┌─────────────────────────────────┐
            │ Which project for this task?     │
            │                                  │
            │ > ideacrafterslabs/tlc           │
            │   ideacrafterslabs/aps           │
            │   ideacrafterslabs/wsm           │
            │   [new project]                  │
            └─────────────────────────────────┘
```

### Implementation

```go
// internal/workspace/resolve.go

// ResolveTargetProject determines which project a task should be
// created in when operating at workspace scope.
// Returns the registered project, or prompts the user.
func ResolveTargetProject(ws WorkspaceConfig, cwd string) (*RegisteredProject, error)
```

The function:
1. Checks if `cwd` falls under a registered project's `db_path` → returns that project
2. Checks if `cwd` falls under any space URI → narrows candidates to that space
3. Falls back to prompting with `charmbracelet/huh` select, listing registered projects grouped by space

## Local Config (T-0002)

Project-level config can optionally declare workspace membership:

```yaml
# .tlc/config.yaml
project:
  id: hop-top/tlc
  workspace: default  # optional, hints which workspace this project belongs to
```

This field is informational — the global `projects` table is authoritative. The field is useful for:
- Showing workspace context in `tlc doctor`
- `tlc init` uses it to set the `space_uri` when registering

## Implementation Plan

### Phase 1: Project Registry + Config (T-0001, T-0002)

1. Add `projects` table migration to `storage/migrations.go`
2. Add `RegisterProject()`, `LookupProject()`, `ListProjectsBySpace()`, `UpdateProjectPath()` to storage layer
3. Update `tlc init` to register/reconnect projects in global DB
4. Add `WorkspaceConfig` and `SpaceConfig` to `config/config.go`
5. Add `workspaces` section support in global config validation
6. Add `workspace` field to `ProjectConfig` (local config)
7. Add `tlc workspace list` command showing configured workspaces and registered projects

### Phase 2: Adapter Interface + Query (T-0003, T-0004)

8. Create `internal/workspace/adapter.go` with `SpaceAdapter`, `RegisteredProject`, `ProjectSource` interfaces
9. Create `internal/workspace/registry.go` with adapter registry and scheme resolution
10. Create `internal/workspace/fs_adapter.go` — filesystem adapter (queries global `projects` table)
11. Define `core.TaskReader` interface (read-only subset of `TaskStorage`)
12. Create `internal/workspace/executor.go` with `QueryAcross()`
13. Add `--workspace` and `--space` flags to `tlc task list`
14. Add `Project` column to table output when in workspace mode
15. Implement merged sorting and pagination

### Phase 3: Task Creation + Lifecycle

16. Create `internal/workspace/resolve.go` with `ResolveTargetProject()`
17. Wire into `tlc task create --workspace` flow
18. Space detection from cwd (delegates to adapter's `Contains()`)
19. Update `last_seen_at` on each project-level TLC command
20. Add staleness detection to `tlc doctor`

## Cache Strategy

The global `projects` table eliminates the need for filesystem scanning caches. The table itself is the cache — always up to date because `tlc init` and each project command maintain it.

For future API-based adapters, cache results per-space at `$XDG_CACHE_HOME/tlc/space-<hash>.json` with adapter-specific TTL. The `--no-cache` flag bypasses any adapter cache.

## Edge Cases

| Case | Behavior |
|------|----------|
| No adapter for URI scheme | Error: "no adapter for scheme X" |
| Adapter discovery fails (network, auth) | Warn, skip space, continue with others |
| Registered project `db_path` no longer exists | Mark `status = 'stale'`, skip in queries, report in `tlc doctor` |
| DB file locked (filesystem) | Open read-only, skip if still fails |
| No projects found in workspace | Show "No projects found" message |
| Duplicate project_id across spaces | Shouldn't happen (PK), but handle gracefully |
| `tlc init` in re-cloned repo | Match by `project_id`, update `db_path`, reconnect tasks |
| `tlc init` same project_id, different space | Update `space_uri` and `db_path` |
| wsm workspace archived | Warn but still allow querying |
| Adapter not installed/available | Error with install hint |

## Non-Goals (Phase 1)

- **Syncing data between databases**: Each project source is authoritative for its own data.
- **Automatic workspace creation from wsm**: User explicitly configures workspaces in tlc config.
- **Non-filesystem adapters**: Only the `filesystem` adapter ships in phase 1. The interface supports future adapters (github, linear, jira, web, sql) but they are out of scope.
- **Space CRUD commands**: Spaces are managed by editing config. No `tlc workspace add-space` commands in phase 1.
- **Automatic CWD refresh on symlink change**: The `current` symlink limitation is documented but solving it is outside TLC's scope (requires shell/IDE integration).
- **Adapter plugin system**: Phase 1 adapters are built-in. A plugin mechanism for third-party adapters is future work.
