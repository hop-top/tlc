# TLC Configuration Specification v0.1

## Overview

This document defines the configuration system for TLC (Task Line CLI), including config file formats, hierarchy, settings, and environment variable overrides.

## Design Principles

1. **Convention over configuration**: Sensible defaults for most use cases
2. **Hierarchical**: System → User → Project → Environment variables
3. **Git-aware**: Auto-detect repository context and conventions
4. **Extensible**: Support for plugin-specific configuration
5. **Secure**: Credentials stored separately from config files

---

## Configuration Hierarchy

Configuration is loaded and merged in this order (later overrides earlier):

1. **System defaults** (built-in)
2. **System config** (`/etc/tlc/config.yaml`)
3. **User config** (`<os user config dir>/tlc/config.yaml`)
4. **Project config** (`.tlc/config.yaml`)
5. **Environment variables** (`TLC_*`)

### Precedence Example

```yaml
# System default
output.format: table

# User config (<os user config dir>/tlc/config.yaml)
output.format: json

# Project config (.tlc/config.yaml)
output.format: yaml

# Environment variable
TLC_OUTPUT_FORMAT=table

# Final value: table (environment wins)
```

---

## Configuration File Format

### File Location

**User config** (resolved via XDG then OS-native fallback;
see [XDG Base Directory Support](#xdg-base-directory-support)):
- Linux: `~/.config/tlc/config.yaml`
- macOS: `~/Library/Application Support/tlc/config.yaml`
- Windows: `%APPDATA%\tlc\config.yaml`

**Project config**:
- `.tlc/config.yaml` (in project root)

### Format

YAML format with nested keys:

```yaml
# Project settings
project:
  id: my-app
  workspace: personal

# General settings
output:
  format: table
  color: true
  verbose: false

# Task defaults
task:
  default_status: TODO
  id_format: "T-{seq:04d}"
  auto_assign: false

# Git integration
git:
  worktree:
    directory: .worktrees
    auto_create: true
  branch:
    prefix_from_type: true
    zero_pad_issue: 4
  commit:
    auto_generate: true
    template: "{type}: {description} (closes #{issue})"

# External system sync
sync:
  enabled: true
  interval: 5m
  conflict_strategy: prompt

  github:
    enabled: true
    repo: org/repo
    sync_direction: bidirectional
    import_labels: true
    import_milestones: true

  jira:
    enabled: false
    url: https://company.atlassian.net
    project: PROJ

# Storage backend
storage:
  backend: sqlite
  db_path: .tlc/db.sqlite

# Plugins
plugins:
  directory: ~/.config/tlc/plugins
  enabled:
    - github-sync
    - jira-sync

# UI preferences
ui:
  pager: auto
  editor: $EDITOR
  date_format: "2006-01-02 15:04:05"
  timezone: local
```

---

## Configuration Schema

### `output` — Output Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `format` | enum | `table` (TTY), `json` (pipe) | Output format: `table`, `json`, `yaml`, `tls`, `summary` |
| `color` | bool | `true` (TTY), `false` (pipe) | Enable colored output |
| `verbose` | bool | `false` | Verbose logging |
| `quiet` | bool | `false` | Suppress non-essential output |

**Example**:
```yaml
output:
  format: json
  color: false
  verbose: true
```

---

### `project` — Project Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `id` | string | - | Unique project identifier |
| `fallback_mode` | enum | `auto` | Fallback behavior: `auto`, `detected`, `prompt` |
| `duplicate_id_strategy` | enum | `share` | Duplicate ID handling: `share`, `unique`, `prompt` |
| `workspace` | string | - | Workspace name (must match a `workspaces[].name`) |

The `workspace` field is optional. When set, TLC resolves the
named workspace from the `workspaces` list in user config and
uses its spaces for storage and sync operations.

**Example**:
```yaml
project:
  id: my-app
  fallback_mode: auto
  duplicate_id_strategy: share
  workspace: personal
```

---

### `task` — Task Defaults

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `default_status` | enum | `TODO` | Initial status for new tasks |
| `id_format` | string | `T-{seq:04d}` | Task ID format template |
| `auto_assign` | bool | `false` | Auto-assign to current user on create |
| `require_reference` | bool | `true` | Require reference on creation |
| `archive_threshold` | duration | `168h` (7d) | Auto-archive DONE/SKIPPED tasks after this duration |

**ID Format Template Syntax**:
- `{seq}` — Sequential number
- `{seq:04d}` — Zero-padded to 4 digits
- `{date:YYYYMMDD}` — Date prefix
- `{random:8}` — Random alphanumeric (8 chars)

**Examples**:
```yaml
task:
  id_format: "T-{seq:04d}"        # T-0001, T-0042
  archive_threshold: 48h          # Archive after 2 days
```

#### `task.statuses` — Custom Status Definitions

Define custom statuses replacing the 4 defaults. Each entry is a
`StatusDefinition` with the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique status identifier (e.g. `TODO`) |
| `label` | string | no | Human-readable display label |
| `description` | string | no | Short explanation of purpose |
| `is_terminal` | bool | no | Terminal state; no outbound transitions |
| `color` | string | no | ANSI/hex color for TUI/CLI output |
| `role` | enum | no | Semantic role: `initial`, `active`, `completed` |
| `tls_marker` | string | no | TLS bracket marker (e.g. `x`, `~`, `-`) |

Semantic roles drive shortcut commands:

- `initial` — target of `tlc task unclaim`
- `active` — target of `tlc task claim`
- `completed` — target of `tlc task complete`

#### `task.state_machine` — Transition Rules

The `task.state_machine.rules` list defines allowed transitions.
Each rule has `from` and `to` (status names). Unlisted transitions
are rejected unless `--force` is passed.

#### `task.workflows` — Per-Tag Workflow Overrides

Override statuses and rules for tasks matching specific tags.
Each entry under `task.workflows` is keyed by tag name and
contains its own `statuses` and `state_machine` blocks.

**Full Custom Statuses Example**:

```yaml
task:
  default_status: OPEN
  statuses:
    - name: OPEN
      label: Open
      role: initial
      tls_marker: " "
      color: "#888888"
    - name: ACTIVE
      label: Active
      role: active
      tls_marker: "~"
      color: "#00AAFF"
    - name: REVIEW
      label: In Review
      tls_marker: "r"
      color: "#FFAA00"
    - name: MERGED
      label: Merged
      role: completed
      is_terminal: true
      tls_marker: "x"
      color: "#00FF00"
    - name: WONTFIX
      label: Won't Fix
      is_terminal: true
      tls_marker: "-"
      color: "#FF0000"

  state_machine:
    rules:
      - from: OPEN
        to: ACTIVE
      - from: ACTIVE
        to: REVIEW
      - from: REVIEW
        to: MERGED
      - from: REVIEW
        to: ACTIVE
      - from: OPEN
        to: WONTFIX

  workflows:
    hotfix:
      statuses:
        - name: OPEN
          role: initial
          tls_marker: " "
        - name: ACTIVE
          role: active
          tls_marker: "~"
        - name: MERGED
          role: completed
          is_terminal: true
          tls_marker: "x"
      state_machine:
        rules:
          - from: OPEN
            to: ACTIVE
          - from: ACTIVE
            to: MERGED
```

When no custom statuses are configured, TLC uses 4 defaults:

| Name | Role | Terminal | TLS Marker |
|------|------|----------|------------|
| `TODO` | initial | no | ` ` (space) |
| `IN_PROGRESS` | active | no | `~` |
| `DONE` | completed | yes | `x` |
| `SKIPPED` | — | yes | `-` |

#### `task.stale` — Staleness Detection

Controls how long a task can be idle before it is considered stale, and
which shell hooks to fire when the threshold is crossed.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `default_timeout` | duration | `6h` | Project-wide stale threshold; applies to tasks without a per-task `stale_timeout` |
| `hooks` | list | `[]` | Shell commands run when a task crosses the stale threshold |

Each entry in `hooks` has a single `command` field — a shell command that
supports Go `text/template` expansion with the following variables:

| Variable | Type | Description |
|----------|------|-------------|
| `{{.ID}}` | string | Task ID (e.g. `T-0042`) |
| `{{.Title}}` | string | Task title |
| `{{.AssignedTo}}` | string | Assignee profile name |
| `{{.UpdatedAt}}` | time.Time | Last updated timestamp |
| `{{.Timeout}}` | duration | Effective stale threshold |
| `{{.StaleSince}}` | duration | How long the task has been stale |

Hook failures are non-fatal; all hooks run regardless of individual errors.

**Example**:

```yaml
task:
  stale:
    default_timeout: 8h
    hooks:
      - command: 'echo "Task {{.ID}} ({{.Title}}) is stale since {{.StaleSince}}"'
      - command: 'tlc task update {{.ID}} --tag stale'
```

---

### `git` — Git Integration

#### `git.worktree`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `directory` | path | `.worktrees` | Worktree directory relative to repo root |
| `auto_create` | bool | `true` | Auto-create worktrees for parent tasks |
| `auto_remove` | bool | `false` | Auto-remove worktrees on task completion |

**Example**:
```yaml
git:
  worktree:
    directory: .work
    auto_create: true
    auto_remove: false
```

#### `git.branch`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `prefix_from_type` | bool | `true` | Use task type as branch prefix |
| `zero_pad_issue` | int | `4` | Zero-pad issue numbers in branches |
| `separator` | string | `/` | Separator between prefix and name |

**Example**:
```yaml
git:
  branch:
    prefix_from_type: true
    zero_pad_issue: 4
    separator: /

# Generates: feat/0042-api-rate-limiting
```

#### `git.commit`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `auto_generate` | bool | `true` | Auto-generate commit messages from branch |
| `template` | string | `{type}: {description} (closes #{issue})` | Commit message template |
| `co_author` | string | - | Optional co-author trailer appended to commit message |

**Template Variables**:
- `{type}` — Branch/task type (feat, fix, etc.)
- `{description}` — Branch purpose or task title
- `{issue}` — Issue number (without padding)
- `{task_id}` — Full task ID

**Example**:
```yaml
git:
  commit:
    auto_generate: true
    template: "{type}: {description} (closes #{issue})"
```

---

### `sync` — External System Sync

#### Global Sync Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `true` | Enable external system sync |
| `interval` | duration | `5m` | Polling interval (e.g., `5m`, `1h`) |
| `conflict_strategy` | enum | `prompt` | Conflict resolution: `prompt`, `local`, `remote`, `manual` |
| `batch_size` | int | `50` | Max tasks per sync batch |

**Conflict Strategies**:
- `prompt` — Ask user on conflict
- `local` — Keep local changes
- `remote` — Keep remote changes
- `manual` — Mark as conflict, require manual resolution

**Example**:
```yaml
sync:
  enabled: true
  interval: 10m
  conflict_strategy: prompt
```

#### `sync.github` — GitHub Integration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable GitHub sync |
| `repo` | string | - | Repository (org/repo) |
| `sync_direction` | enum | `bidirectional` | Sync mode: `pull`, `push`, `bidirectional` |
| `import_labels` | bool | `true` | Import GitHub labels as tags |
| `import_milestones` | bool | `true` | Import milestones |
| `import_assignees` | bool | `true` | Import assignees |
| `issue_filter` | string | - | JQ filter for issues to import |

**Sync Directions**:
- `pull` — Only pull from GitHub (read-only)
- `push` — Only push to GitHub (write-only)
- `bidirectional` — Two-way sync

**Example**:
```yaml
sync:
  github:
    enabled: true
    repo: myorg/myrepo
    sync_direction: bidirectional
    import_labels: true
    import_milestones: true
    issue_filter: '.labels[] | select(.name | startswith("tlc:"))'
```

#### `sync.jira` — Jira Integration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable Jira sync |
| `url` | string | - | Jira instance URL |
| `project` | string | - | Jira project key |
| `sync_direction` | enum | `bidirectional` | Sync mode |
| `issue_type` | string | `Task` | Default issue type for new issues |
| `import_custom_fields` | bool | `false` | Import custom fields as metadata |

**Example**:
```yaml
sync:
  jira:
    enabled: true
    url: https://company.atlassian.net
    project: PROJ
    sync_direction: pull
    issue_type: Task
```

#### `sync.linear` — Linear Integration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Enable Linear sync |
| `team_id` | string | - | Linear team ID |
| `sync_direction` | enum | `bidirectional` | Sync mode |
| `import_projects` | bool | `true` | Import Linear projects as milestones |

**Example**:
```yaml
sync:
  linear:
    enabled: true
    team_id: team_abc123
    sync_direction: bidirectional
```

---

### `storage` — Storage Backend

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `backend` | enum | `sqlite` | Storage backend: `sqlite`, `postgres`, `local` |
| `db_path` | path | `.tlc/db.sqlite` | Database file path (sqlite only) |
| `connection_string` | string | - | Database connection string (postgres) |

**Two databases**: TLC maintains two distinct SQLite files:
- **Project DB** — `.tlc/db.sqlite` (or `.hop/tlc/db.sqlite` in hop mode); stores tasks,
  logs, flow runs, sequences for a single project.
- **Global DB** — user-level data dir (`<XDG_DATA_HOME>/tlc/db.sqlite`); stores the
  `projects` registry tracking all known projects across clones/worktrees. Created
  automatically on first `tlc init`.

The `storage.db_path` config key refers to the project DB only.

**Example (SQLite)**:
```yaml
storage:
  backend: sqlite
  db_path: .tlc/tasks.db
```

**Example (PostgreSQL)**:
```yaml
storage:
  backend: postgres
  connection_string: postgresql://user:pass@localhost/tlc
```

---

### Project Registry (`projects` table)

Stored in the **Global DB**; added in schema migration v2. Current schema version: **v4**.

```sql
CREATE TABLE projects (
    project_id    TEXT PRIMARY KEY,
    db_path       TEXT NOT NULL,
    space_uri     TEXT,
    label         TEXT,
    registered_at TEXT NOT NULL,
    last_seen_at  TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active'
);
CREATE INDEX idx_projects_space_uri ON projects(space_uri);
CREATE INDEX idx_projects_status    ON projects(status);
```

| Column | Description |
|--------|-------------|
| `project_id` | Unique identifier; from git remote (`owner/repo`) or dir name |
| `db_path` | Absolute path to project's local SQLite DB |
| `space_uri` | Workspace org path; inferred from `$HOME/.w/<org>` layout; empty if CWD |
|             | is not under `$HOME/.w/` |
| `label` | Short name; last `/`-segment of `project_id` |
| `registered_at` | ISO-8601 timestamp of first registration |
| `last_seen_at` | ISO-8601 timestamp of last access or path update |
| `status` | Project state; default `active` |

**Schema migration history**:

| Version | Change |
|---------|--------|
| v1 | Initial schema: `tasks`, `task_logs`, `flow_runs`, `task_sequences` |
| v2 | Added `projects` registry table + indexes |
| v3 | Added `effort TEXT NOT NULL DEFAULT ''` to `tasks` |
| v4 | Added `priority TEXT NOT NULL DEFAULT ''` to `tasks` |

See `internal/storage/migrations.go` for authoritative DDL.

---

### `plugins` — Plugin System

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `directory` | path | `~/.config/tlc/plugins` | Plugin directory |
| `enabled` | string[] | `[]` | List of enabled plugins |
| `auto_discover` | bool | `true` | Auto-discover plugins in directory |

**Example**:
```yaml
plugins:
  directory: ~/.config/tlc/plugins
  auto_discover: true
  enabled:
    - github-sync
    - slack-notify
    - custom-executor
```

Plugin-specific settings:
```yaml
plugins:
  slack-notify:
    webhook_url: https://hooks.slack.com/...
    channel: "#dev"
    notify_on:
      - TASK_COMPLETED
      - FLOW_FAILED
```

---

### `ui` — UI Preferences

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `pager` | string | `auto` | Pager command: `auto`, `less`, `more`, `none` |
| `editor` | string | `$EDITOR` | Editor command |
| `date_format` | string | `2006-01-02 15:04:05` | Date format (Go layout) |
| `timezone` | string | `local` | Timezone: `local`, `UTC`, or IANA name |
| `table_style` | enum | `unicode` | Table style: `unicode`, `ascii`, `simple` |

**Table Styles**:
```
unicode:  ┌───┬───┐
          │ A │ B │
          ├───┼───┤

ascii:    +---+---+
          | A | B |
          +---+---+

simple:   A   B
          ─────────
```

**Example**:
```yaml
ui:
  pager: less
  editor: vim
  date_format: "2006-01-02 15:04"
  timezone: America/New_York
  table_style: unicode
```

---

### `workspaces` — Workspace Definitions

A workspace groups one or more **spaces** (storage locations).
Defined in user config; referenced by project config via
`project.workspace`.

#### `WorkspaceConfig` Fields

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `name` | string | yes | Unique workspace identifier |
| `wsm_id` | string | no | External workspace-manager ID |
| `spaces` | SpaceConfig[] | yes | List of space definitions |
| `default` | bool | no | Mark as default workspace (at most one) |

#### `SpaceConfig` Fields

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `uri` | string | yes | Space location (path or URI) |
| `adapter` | string | no | Storage adapter name; inferred from URI when omitted |
| `label` | string | no | Human-readable label |
| `options` | map[string]string | no | Adapter-specific key-value options |

**Adapter inference** (`InferAdapterFromURI`):

| URI pattern | Inferred adapter |
|-------------|-----------------|
| Bare path (`/home/user/data`) | `filesystem` |
| `file://` scheme | `filesystem` |
| Unknown/other scheme | *(empty -- must be set explicitly)* |

Known adapters: `filesystem`. Unknown adapter names produce a
warning at load time but do not fail validation.

**Validation rules**:
- Workspace `name` must not be empty or whitespace-only
- Space `uri` must not be empty or whitespace-only
- Workspace names must be unique across the list
- At most one workspace may have `default: true`
- When no workspace is marked default, the first entry is used

**Example -- single workspace**:
```yaml
workspaces:
  - name: personal
    default: true
    spaces:
      - uri: ~/Documents/tlc-data
        label: local docs
      - uri: file:///mnt/backup/tlc
        label: backup
        options:
          read_only: "true"
```

**Example -- multi-workspace**:
```yaml
workspaces:
  - name: work
    wsm_id: ws_abc123
    default: true
    spaces:
      - uri: ~/work/tasks
        adapter: filesystem
        label: primary
      - uri: /shared/team/tasks
        label: team shared

  - name: personal
    spaces:
      - uri: ~/personal/tasks
        label: home projects
```

Bind a project to a workspace in `.tlc/config.yaml`:
```yaml
project:
  workspace: work
```

---

## XDG Base Directory Support

TLC honours XDG Base Directory environment variables with
OS-native fallbacks when unset. Each directory function checks
the corresponding XDG variable first; if empty, it falls back
to platform-conventional paths.

### Directory Resolution

| Purpose | XDG variable | macOS fallback | Linux fallback |
|---------|-------------|----------------|----------------|
| Config | `XDG_CONFIG_HOME` | `~/Library/Application Support/tlc` | `~/.config/tlc` |
| Data | `XDG_DATA_HOME` | `~/Library/Application Support/tlc` | `~/.local/share/tlc` |
| Cache | `XDG_CACHE_HOME` | `~/Library/Caches/tlc` | `~/.cache/tlc` |
| State | `XDG_STATE_HOME` | `~/Library/Application Support/tlc/state` | `~/.local/state/tlc` |

**Windows fallbacks** (when XDG variable unset):

| Purpose | Windows path |
|---------|-------------|
| Config | `%APPDATA%\tlc` |
| Data | `%LOCALAPPDATA%\tlc` |
| Cache | `%LOCALAPPDATA%\tlc\cache` |
| State | `%LOCALAPPDATA%\tlc\state` |

When an XDG variable **is** set, TLC appends `/tlc` to its
value regardless of OS. For example:

```bash
export XDG_CONFIG_HOME=$HOME/.myconfig
# Config dir becomes: $HOME/.myconfig/tlc
# Config file:        $HOME/.myconfig/tlc/config.yaml
```

### Directory Purposes

- **Config** -- configuration files (`config.yaml`)
- **Data** -- persistent application data
- **Cache** -- regenerable cached data (created on demand
  with `0750` permissions)
- **State** -- runtime state that persists across restarts
  but is not configuration (created on demand with `0750`
  permissions)

---

## Environment Variables

Environment variables override all config file settings.

### Naming Convention

```
TLC_<SECTION>_<KEY>=value
```

Nested keys use underscores:
```
TLC_GIT_WORKTREE_DIRECTORY=.work
TLC_SYNC_GITHUB_REPO=org/repo
```

### Common Variables

| Variable | Equivalent Config | Example |
|----------|-------------------|---------|
| `TLC_CONFIG` | Config file path | `/custom/config.yaml` |
| `TLC_OUTPUT_FORMAT` | `output.format` | `json` |
| `TLC_NO_COLOR` | `output.color=false` | `1` |
| `TLC_VERBOSE` | `output.verbose` | `true` |
| `TLC_TASK_DEFAULT_STATUS` | `task.default_status` | `IN_PROGRESS` |
| `TLC_GIT_WORKTREE_DIRECTORY` | `git.worktree.directory` | `.work` |
| `TLC_SYNC_ENABLED` | `sync.enabled` | `false` |
| `TLC_SYNC_INTERVAL` | `sync.interval` | `10m` |
| `TLC_STORAGE_BACKEND` | `storage.backend` | `postgres` |

### Credential Variables

| Variable | Description |
|----------|-------------|
| `GITHUB_TOKEN` | GitHub personal access token |
| `JIRA_TOKEN` | Jira API token |
| `JIRA_EMAIL` | Jira account email |
| `LINEAR_API_KEY` | Linear API key |

**Note**: Credentials are NOT stored in config files. Use environment variables or system keychain (see tlc-auth-spec-0.1.md).

---

## Auto-Detection

TLC automatically detects context when config is missing:

### Repository Detection

```bash
# Detects Git repo
cd /path/to/repo
tlc init
# Creates: /path/to/repo/.tlc/config.yaml
```

Project ID fallback chain (first non-empty wins):
1. `git remote get-url origin` — parses `owner/repo` from GitHub/GitLab/Bitbucket URLs
2. `git rev-parse --show-toplevel` — uses repo root basename
3. Ancestor `.git` walk — uses first `.git`-bearing ancestor's basename
4. Falls back to `"unknown"` if all fail

### Project Type Detection

```yaml
# Detects Go binary project (go.mod exists)
task:
  id_format: "T-{seq:04d}"

# Auto-suggests domain labels for Go projects
# (see github-label-convention-0.1.md)
```

### Output Format Detection

```bash
# TTY detected
tlc task list
# Uses: output.format = table

# Pipe detected
tlc task list | jq
# Uses: output.format = json
```

---

## Configuration Commands

See `tlc-cli-spec-0.1.md` for full command reference.

### Get Configuration

```bash
# Get single value
tlc config get output.format

# Get nested value
tlc config get sync.github.repo
```

### Set Configuration

```bash
# Set project-level
tlc config set output.format json

# Set user-level
tlc config set --global ui.editor vim

# Set nested value
tlc config set sync.github.repo myorg/myrepo
```

### List Configuration

```bash
# Show all config (merged)
tlc config list

# Show user config only
tlc config list --global

# Show project config only
tlc config list --project

# JSON output
tlc config list --format json
```

### Edit Configuration

```bash
# Edit project config in $EDITOR
tlc config edit

# Edit user config
tlc config edit --global
```

---

## Validation

### Schema Validation

TLC validates config on load:

```yaml
# Invalid: bad enum value
output:
  format: xml  # Error: format must be table|json|yaml|tls|summary

# Invalid: bad type
sync:
  interval: "not-a-duration"  # Error: interval must be duration (e.g., 5m)

# Invalid: missing required field
sync:
  github:
    enabled: true
    # Error: repo is required when enabled=true
```

### Validation Command

```bash
tlc config validate

# Output
✓ Config valid
  - User config: valid
  - Project config: valid
  - Merged config: valid

# Or with errors
✗ Config invalid
  - sync.github.repo: required when enabled=true
  - task.id_format: invalid template syntax
```

---

## Migration

When upgrading TLC versions, config may need migration.

### Version Detection

```yaml
# Config file header
version: 0.1

# Settings...
```

### Migration Command

```bash
# Check if migration needed
tlc config migrate --dry-run

# Output
Config version: 0.1
TLC version: 0.2
Migration required:
  - Rename: git.worktree_dir → git.worktree.directory
  - Add: sync.conflict_strategy (default: prompt)

# Perform migration
tlc config migrate

# Output
✓ Migrated to version 0.2
  - Renamed 1 key
  - Added 1 default
  - Backed up to .tlc/config.yaml.backup
```

---

## Examples

### Minimal Config (Defaults)

```yaml
version: 0.1

# Everything else uses built-in defaults
```

### Local-Only Development

```yaml
version: 0.1

output:
  format: table
  color: true

task:
  default_status: TODO
  id_format: "T-{seq:04d}"

git:
  worktree:
    directory: .worktrees
    auto_create: true

sync:
  enabled: false

storage:
  backend: sqlite
  db_path: .tlc/db.sqlite
```

### GitHub-Synced Team Project

```yaml
version: 0.1

output:
  format: json

task:
  default_status: TODO
  auto_assign: false

git:
  worktree:
    directory: .worktrees
    auto_create: true
  commit:
    auto_generate: true

sync:
  enabled: true
  interval: 5m
  conflict_strategy: prompt

  github:
    enabled: true
    repo: myorg/myrepo
    sync_direction: bidirectional
    import_labels: true
    import_milestones: true

storage:
  backend: sqlite
  db_path: .tlc/db.sqlite

ui:
  pager: less
  editor: vim
  timezone: UTC
```

### Multi-System Sync

```yaml
version: 0.1

sync:
  enabled: true
  interval: 10m
  conflict_strategy: prompt

  github:
    enabled: true
    repo: myorg/frontend
    sync_direction: bidirectional

  jira:
    enabled: true
    url: https://company.atlassian.net
    project: FRONT
    sync_direction: pull
    issue_type: Story

  linear:
    enabled: true
    team_id: team_abc123
    sync_direction: bidirectional
```

---

## Security Considerations

### Credential Storage

**DO NOT** store credentials in config files:

```yaml
# ❌ WRONG - Never do this
sync:
  github:
    token: ghp_xxxxxxxxxxxxx  # NEVER store tokens in config!
```

**✅ CORRECT** — Use environment variables or system keychain:

```bash
# Option 1: Environment variable
export GITHUB_TOKEN=ghp_xxxxxxxxxxxxx
tlc sync pull github

# Option 2: System keychain (via tlc auth)
tlc auth login github
# Stores token in OS keychain (macOS Keychain, Windows Credential Manager, etc.)
```

### File Permissions

Config files should have restrictive permissions:

```bash
# Recommended permissions
chmod 600 ~/.config/tlc/config.yaml
chmod 700 ~/.config/tlc/
```

### Sensitive Data

Mark sensitive config keys (plugins may add):

```yaml
plugins:
  custom-plugin:
    api_key: "${CUSTOM_API_KEY}"  # Use env var reference
```

---

## References

- [tlc-cli-spec-0.1.md](tlc-cli-spec-0.1.md) — CLI commands
- [tlc-auth-spec-0.1.md](tlc-auth-spec-0.1.md) — Authentication (to be created)
- [tlc-plugin-spec-0.1.md](tlc-plugin-spec-0.1.md) — Plugin system (to be created)
- [sync-architecture-0.1.md](sync-architecture-0.1.md) — Sync architecture
- [git-worktree-convention-0.1.md](git-worktree-convention-0.1.md) — Worktree conventions

---

**Version**: 0.1
**Last Updated**: 2026-03-28
**Status**: Normative
