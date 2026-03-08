# Project Detection in TLC

TLC automatically detects projects using a fallback strategy when no explicit configuration is found. This makes working with multiple clones of the same repository seamless.

## How Project Detection Works

When you run any TLC command, the system follows this detection order:

1. **Explicit Config**: Check for `.tlc/config.yaml` in the current directory or parent directories
2. **Git Remote Fallback**: If no config is found, attempt to detect the project from the git remote URL
3. **Directory Fallback**: If no git remote is found, use the repository directory name

## Fallback Modes

You can configure how TLC handles projects when no explicit configuration is present:

### Auto Mode (default)

Automatically creates a `.tlc/config.yaml` file on first use.

```bash
tlc init --fallback-mode auto
```

Behavior:
- Detects project from git remote
- Creates `.tlc/config.yaml` automatically
- Shows a log message: `Detected project from git remote, created .tlc/config.yaml`

### Detected Mode

Uses inferred project ID without creating a config file.

```bash
tlc init --fallback-mode detected
```

Behavior:
- Detects project from git remote
- Does NOT create a config file
- Project ID is inferred on each command
- Useful for ephemeral clones or CI/CD environments

### Prompt Mode

Interactively asks the user how to proceed on first use.

```bash
tlc init --fallback-mode prompt
```

Behavior:
- Detects project from git remote
- Shows interactive prompt:
  ```
  Detected project 'org/repo' from git remote
  How should TLC handle project detection?

  > Create config file (recommended)
    Use detected mode without config
  ```

## Git Remote Detection

TLC supports automatic project ID detection from the following git hosts:

- **GitHub**: `github.com/user/repo` → `user/repo`
- **GitLab**: `gitlab.com/user/repo` → `user/repo`
- **Bitbucket**: `bitbucket.org/user/repo` → `user/repo`

The detection uses the `origin` remote by default.

## Config File Merging

TLC automatically merges configurations from parent directories:

```
/repo/
  .tlc/config.yaml        ← Project-specific config
  feature-branch/
    .tlc/config.yaml      ← Overrides parent
    current-dir/           ← Working here
```

Config files are merged from root-most to closest, with closer files taking precedence.

## Configuration Reference

Project detection settings in `.tlc/config.yaml`:

```yaml
project:
  id: "user/my-repo"              # Project identifier (auto-detected from git remote)
  fallback_mode: "auto"            # auto, detected, or prompt
  duplicate_id_strategy: "share"     # share, unique, or prompt (for clones)
```

### `project.id`

The unique identifier for your project. Automatically detected from git remote URL, but can be manually set.

### `project.fallback_mode`

Determines behavior when no explicit project config is found:

- `auto` (default): Automatically create config file on first use
- `detected`: Use inferred ID without creating config file
- `prompt`: Ask user interactively

### `project.duplicate_id_strategy`

Determines how to handle duplicate project IDs when running `tlc init` in a clone:

- `share` (default): Use the same project ID (tasks are shared across clones)
- `unique`: Generate a unique project ID (tasks are isolated)
- `prompt`: Ask user interactively

See [Multi-Clone Setup](multi-clone-setup.md) for more details.

## Entry Point Detection

TLC operates in two modes depending on how it was invoked: **standalone**
or **hop**. Mode determines config directory layout and validation.

### Detection Order

`DetectMode()` in `internal/config/mode.go` resolves mode as follows:

1. **`TLC_MODE` env var** -- if set to `hop` or `standalone`, returns
   immediately (case-insensitive)
2. **CWD path contains `.hop/` segment** -- any path component literally
   named `.hop` triggers hop mode
3. **Ancestor directory has `.hop/` child** -- walks from cwd to
   filesystem root; if any ancestor contains a `.hop/` directory,
   returns hop mode
4. **Default** -- standalone

### Config Paths per Mode

| Mode         | `LocalConfigDir` | `LocalConfigFile` |
|:-------------|:-----------------|:------------------|
| `standalone` | `.tlc`           | `.tlc.yaml`       |
| `hop`        | `.hop/tlc`       | `.hop/tlc.yaml`   |

### `ValidateLocalConfig` Behavior

Walks from `startDir` (or cwd) upward, checking each ancestor for:
- the mode's config directory (`LocalConfigDir`), OR
- the mode's flat config file (`LocalConfigFile`)

If neither is found:
- **Hop mode** -- returns an error (`"hop mode detected but no
  .hop/tlc or .hop/tlc.yaml found..."`)
- **Standalone mode** -- returns `nil` (user/system config may still
  apply)

## Project Reconnection on Init

When `tlc init` runs, it registers or reconnects the project in the
global `projects` table. See `internal/cli/init.go`.

### Flow

1. Detect project ID from git remote (`core.DetectProjectID()`)
2. Open global storage backend
3. Look up `finalProjectID` via `s.LookupProject()`
4. **Existing project found** -- reconnect: call
   `s.UpdateProjectPath()` to update `db_path` to the current clone's
   local DB path. Logs `"Reconnected to existing project"` with
   original `registered_at` date.
5. **No existing project** -- register: call `s.RegisterProject()`
   with inferred `space_uri`, `label`, and local `db_path`. Logs
   `"Registered new project"`.

### Inferred Fields

- **`db_path`** -- `<cwd>/<LocalConfigDir(mode)>/db.sqlite`
- **`space_uri`** -- if cwd is under `$HOME/.w/<org>/`, returns that
  org path (e.g. `~/.w/ideacrafterslabs`); empty otherwise
- **`label`** -- last path segment of the project ID (e.g.
  `org/repo` yields `repo`)

## Global Projects Table

Added in schema migration v2 (`internal/storage/migrations.go`).
Tracks all known projects across clones/worktrees.

### Schema

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
```

### Columns

| Column          | Description                                      |
|:----------------|:-------------------------------------------------|
| `project_id`    | Unique project identifier (from git remote)      |
| `db_path`       | Absolute path to project's local SQLite DB       |
| `space_uri`     | Workspace org path (inferred from `$HOME/.w/`)   |
| `label`         | Short name (last segment of project ID)          |
| `registered_at` | Timestamp of first registration                  |
| `last_seen_at`  | Timestamp of last access/update                  |
| `status`        | Project state; default `active`                  |

### Indexes

- `idx_projects_space_uri` -- on `space_uri`
- `idx_projects_status` -- on `status`

## Common Scenarios

### Working in a Clone Without Config

```bash
cd ~/repo-clone/  # No .tlc directory here
tlc task list       # Automatically detects project from git remote
```

Result: Project ID is detected from git remote, tasks are filtered accordingly.

### Different Fallback Modes for Different Environments

Local development (auto-create config):
```bash
# ~/.tlc/config.yaml
project:
  fallback_mode: auto
```

CI/CD (no config files):
```bash
# .github/workflows/ci.yml
env:
  TLC_PROJECT_FALLBACK_MODE: detected
```

Ephemeral test environments:
```bash
tlc init --fallback-mode prompt
```

## Troubleshooting

### Project Not Detected

If TLC reports "Not in a project":

1. Check that you're in a git repository: `git status`
2. Verify you have a remote configured: `git remote -v`
3. Check the remote URL is supported (GitHub, GitLab, or Bitbucket)

### Wrong Project Detected

If TLC detects the wrong project:

1. Explicitly set the project ID in your config:
   ```yaml
   project:
     id: "my-custom-project-id"
   ```
2. Or use environment variable: `TLC_PROJECT_ID=my-custom-project-id tlc task list`

### Config Not Created in Auto Mode

If auto mode doesn't create a config file:

1. Check write permissions in current directory
2. Verify TLC has access to create `.tlc` directory
3. Check logs for error messages: `tlc task list --verbose`
