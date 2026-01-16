# TLC CLI Specification v0.1

## Overview

This document defines the command-line interface for TLC (Task Line CLI), a multi-agent task orchestration system. The CLI provides commands for task management, flow execution, collaboration, sync operations, and configuration.

## Design Principles

1. **Composable**: Commands follow Unix philosophy (do one thing well)
2. **Scriptable**: Structured output formats (JSON, YAML) for automation
3. **Human-friendly**: Table output and colors for interactive use
4. **Consistent**: Uniform patterns across all commands
5. **Git-aware**: Auto-detect worktrees, branches, and repository context

---

## Global Flags

Available on all commands:

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--config` | `-c` | path | Config file path (default: auto-detect) |
| `--format` | `-f` | enum | Output format: `table`, `json`, `yaml`, `tls` |
| `--no-color` | | bool | Disable colored output |
| `--verbose` | `-v` | bool | Verbose logging |
| `--quiet` | `-q` | bool | Suppress non-essential output |
| `--help` | `-h` | bool | Show help for command |

**Format Options**:
- `table`: Human-readable table (default for interactive)
- `json`: JSON output (default for non-TTY)
- `yaml`: YAML output
- `tls`: Task Line Syntax (see task-line-spec-0.1.md)

---

## Command Structure

```
tlc <command> <subcommand> [arguments] [flags]
```

**Top-level commands**:
- `init` — Initialize TLC in current directory
- `task` — Task operations (CRUD)
- `flow` — Flow execution and management
- `log` — Query audit logs
- `sync` — External system synchronization
- `label` — Label management
- `config` — Configuration management
- `auth` — Authentication with external systems
- `version` — Show version information

---

## `tlc init`

Initialize TLC in the current directory.

### Synopsis

```bash
tlc init [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--storage` | enum | `local` | Storage backend: `local`, `sqlite` |
| `--db-path` | path | `.tlc/db.sqlite` | Database file path |
| `--worktree-dir` | path | `.worktrees` | Git worktree directory |
| `--force` | bool | `false` | Overwrite existing config |

### Behavior

1. Creates `.tlc/` directory
2. Generates `config.yaml` with defaults
3. Initializes storage backend
4. Adds `.worktrees/` to `.gitignore` if Git repo detected
5. Outputs success message with next steps

### Examples

```bash
# Initialize with defaults
tlc init

# Initialize with custom worktree location
tlc init --worktree-dir .work

# Reinitialize (overwrite existing)
tlc init --force
```

---

## `tlc task` — Task Operations

Manage tasks (create, read, update, delete).

### Subcommands

- `create` — Create new task
- `list` — List tasks with filters
- `show` — Show task details
- `update` — Update task fields
- `delete` — Delete task
- `assign` — Assign task to user
- `comment` — Add comment to task
- `claim` — Claim task for work
- `unclaim` — Release claimed task

---

### `tlc task create`

Create a new task.

#### Synopsis

```bash
tlc task create [title] [flags]
```

#### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--title` | `-t` | string | Task title (or use positional arg) |
| `--description` | `-d` | string | Task description |
| `--status` | `-s` | enum | Initial status (default: `TODO`) |
| `--assigned-to` | `-a` | string | Assignee username |
| `--tag` | | string[] | Tags (repeatable) |
| `--reference` | `-r` | string | Reference pointer (default: auto-generate) |
| `--meta` | `-m` | key=value | Metadata (repeatable) |
| `--interactive` | `-i` | bool | Interactive prompt mode |

#### Behavior

1. Validates required fields (title, reference)
2. Generates unique ID (e.g., `T-0042`)
3. Sets created_at timestamp
4. Writes CREATED log entry
5. Returns task ID

#### Examples

```bash
# Simple task
tlc task create "Add API rate limiting"

# With metadata
tlc task create "Add OAuth support" \
  --assigned-to codex \
  --tag auth \
  --tag security \
  --meta priority=high

# Interactive mode
tlc task create --interactive

# Output (JSON)
{
  "id": "T-0042",
  "title": "Add API rate limiting",
  "status": "TODO",
  "reference": "task://T-0042",
  "created_at": "2025-01-16T10:30:00Z"
}
```

---

### `tlc task list`

List tasks with filtering and sorting.

#### Synopsis

```bash
tlc task list [query] [flags]
```

#### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--status` | `-s` | enum[] | Filter by status (repeatable) |
| `--assigned-to` | `-a` | string | Filter by assignee |
| `--tag` | | string[] | Filter by tag (repeatable) |
| `--reference` | `-r` | string | Filter by reference pattern |
| `--meta` | `-m` | key=value | Filter by metadata |
| `--sort-by` | | string | Sort field (default: `created_at`) |
| `--order` | | enum | Sort order: `asc`, `desc` (default: `desc`) |
| `--limit` | `-n` | int | Limit results (default: 100) |
| `--offset` | | int | Skip results (default: 0) |
| `--mine` | | bool | Shortcut for current user's tasks |
| `--blocked` | | bool | Show only blocked tasks |
| `--urgent` | | bool | Show only high-priority tasks |

#### Query Syntax

Positional query argument supports:

```
status:TODO                    # Status filter
@codex                         # Assigned to codex
#auth                          # Tagged with auth
meta.priority=high             # Metadata filter
domain:api                     # Domain filter
status:IN_PROGRESS @me         # AND logic (space)
status:TODO|status:IN_PROGRESS # OR logic (pipe)
!status:DONE                   # NOT logic (exclamation)
```

See `tlc-query-spec-0.1.md` for full syntax.

#### Examples

```bash
# All tasks
tlc task list

# In-progress tasks
tlc task list --status IN_PROGRESS

# My tasks
tlc task list --mine
# or
tlc task list @me

# High-priority auth tasks
tlc task list "#auth meta.priority=high"

# JSON output for scripting
tlc task list --format json --status TODO | jq '.[] | .id'

# Table output (default)
┌──────────┬─────────────────────────┬──────────────┬────────────┐
│ ID       │ Title                   │ Status       │ Assigned   │
├──────────┼─────────────────────────┼──────────────┼────────────┤
│ T-0042   │ Add API rate limiting   │ IN_PROGRESS  │ codex      │
│ T-0041   │ Fix session timeout     │ TODO         │ -          │
│ T-0040   │ Add OAuth support       │ TODO         │ agent      │
└──────────┴─────────────────────────┴──────────────┴────────────┘
```

---

### `tlc task show`

Show detailed task information.

#### Synopsis

```bash
tlc task show <task-id> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--logs` | bool | Include audit logs |
| `--meta` | bool | Show all metadata |

#### Examples

```bash
# Basic details
tlc task show T-0042

# With logs
tlc task show T-0042 --logs

# JSON output
tlc task show T-0042 --format json

# Output
Task: T-0042
Title: Add API rate limiting
Status: IN_PROGRESS
Assigned: codex
Tags: feat, api, security
Reference: task://T-0042
Created: 2025-01-16T10:30:00Z
Updated: 2025-01-16T11:15:00Z

Metadata:
  priority: high
  domain: api
  effort: md
  branch_name: feat/0042-api-rate-limiting

Logs (3):
  2025-01-16T10:30:00Z  CREATED           (system)
  2025-01-16T10:35:00Z  STATUS_CHANGED    TODO → IN_PROGRESS (codex)
  2025-01-16T11:15:00Z  ASSIGNED          @codex (codex)
```

---

### `tlc task update`

Update task fields.

#### Synopsis

```bash
tlc task update <task-id> [flags]
```

#### Flags

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--title` | `-t` | string | New title |
| `--description` | `-d` | string | New description |
| `--status` | `-s` | enum | New status |
| `--assigned-to` | `-a` | string | New assignee |
| `--add-tag` | | string[] | Add tags (repeatable) |
| `--remove-tag` | | string[] | Remove tags (repeatable) |
| `--set-meta` | `-m` | key=value | Set metadata (repeatable) |
| `--unset-meta` | | key | Remove metadata (repeatable) |

#### Behavior

1. Validates status transitions (see task-crud-spec-0.1.md)
2. Updates updated_at timestamp
3. Writes appropriate log entry (STATUS_CHANGED, ASSIGNED, etc.)
4. Returns updated task

#### Examples

```bash
# Change status
tlc task update T-0042 --status DONE

# Update title and add tag
tlc task update T-0042 \
  --title "Add API rate limiting with Redis" \
  --add-tag redis

# Update metadata
tlc task update T-0042 --set-meta priority=critical
```

---

### `tlc task delete`

Delete a task (soft delete by default).

#### Synopsis

```bash
tlc task delete <task-id> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--force` | bool | Hard delete (permanent) |
| `--yes` | bool | Skip confirmation |

#### Examples

```bash
# Soft delete (with confirmation)
tlc task delete T-0042

# Hard delete (skip confirmation)
tlc task delete T-0042 --force --yes
```

---

### `tlc task assign`

Assign task to user.

#### Synopsis

```bash
tlc task assign <task-id> <assignee> [flags]
```

#### Examples

```bash
# Assign to user
tlc task assign T-0042 codex

# Unassign
tlc task assign T-0042 null
```

---

### `tlc task comment`

Add comment to task.

#### Synopsis

```bash
tlc task comment <task-id> <message> [flags]
```

#### Flags

| Flag | Short | Type | Description |
|------|------|------|-------------|
| `--message` | `-m` | string | Comment message (or use positional) |
| `--author` | | string | Comment author (default: current user) |

#### Examples

```bash
# Add comment
tlc task comment T-0042 "Implemented rate limiting with Redis"

# Multi-line comment
tlc task comment T-0042 -m "$(cat <<EOF
Implementation complete:
- Added Redis client
- Configured rate limits
- Added tests
EOF
)"
```

---

### `tlc task claim`

Claim a task for work (collaborative claiming).

#### Synopsis

```bash
tlc task claim <task-id> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--duration` | duration | Claim duration (default: 1h) |
| `--note` | string | Claim note |

#### Behavior

See task-collab-spec-0.1.md for claiming semantics.

#### Examples

```bash
# Claim for 1 hour
tlc task claim T-0042

# Claim for 30 minutes
tlc task claim T-0042 --duration 30m

# Claim with note
tlc task claim T-0042 --note "Investigating auth flow"
```

---

### `tlc task unclaim`

Release claimed task.

#### Synopsis

```bash
tlc task unclaim <task-id> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--note` | string | Release note |

#### Examples

```bash
# Release claim
tlc task unclaim T-0042

# Release with note
tlc task unclaim T-0042 --note "Need more context, releasing for others"
```

---

## `tlc flow` — Flow Operations

Execute and manage task flows.

### Subcommands

- `run` — Execute a flow
- `status` — Show flow execution status
- `pause` — Pause running flow
- `resume` — Resume paused flow
- `cancel` — Cancel flow execution
- `list` — List flow definitions

---

### `tlc flow run`

Execute a task flow.

#### Synopsis

```bash
tlc flow run <flow-ref> [flags]
```

#### Flags

| Flag | Short | Type | Description |
|------|------|------|-------------|
| `--input` | `-i` | key=value | Flow inputs (repeatable) |
| `--input-file` | | path | JSON/YAML file with inputs |
| `--detach` | `-d` | bool | Run in background |
| `--watch` | `-w` | bool | Watch execution progress |
| `--dry-run` | | bool | Validate without executing |

#### Examples

```bash
# Run flow
tlc flow run flows/feature-development.yaml

# With inputs
tlc flow run flows/deploy.yaml \
  --input environment=staging \
  --input version=v1.2.0

# Background execution
tlc flow run flows/long-running.yaml --detach

# Watch progress
tlc flow run flows/build.yaml --watch

# Output (watch mode)
Flow: feature-development
Status: RUNNING

✓ setup-worktree          [COMPLETED] 2.1s
▶ parallel-setup          [RUNNING]   4.3s
  ✓ install-deps          [COMPLETED] 4.2s
  ▶ create-branch         [RUNNING]   0.1s
⋯ implement-feature       [PENDING]
⋯ quality-checks          [PENDING]

Progress: 2/8 steps (25%)
```

---

### `tlc flow status`

Show flow execution status.

#### Synopsis

```bash
tlc flow status <execution-id> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--watch` | bool | Continuous updates |
| `--logs` | bool | Include step logs |

#### Examples

```bash
# Check status
tlc flow status exec-1234

# Watch updates
tlc flow status exec-1234 --watch
```

---

### `tlc flow pause`

Pause running flow.

#### Synopsis

```bash
tlc flow pause <execution-id>
```

---

### `tlc flow resume`

Resume paused flow.

#### Synopsis

```bash
tlc flow resume <execution-id>
```

---

### `tlc flow cancel`

Cancel flow execution.

#### Synopsis

```bash
tlc flow cancel <execution-id> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--force` | bool | Force cancel (kill running steps) |
| `--yes` | bool | Skip confirmation |

---

## `tlc log` — Audit Log Queries

Query task audit logs.

### Synopsis

```bash
tlc log [task-id] [flags]
```

### Flags

| Flag | Short | Type | Description |
|------|------|------|-------------|
| `--action` | | enum[] | Filter by action type (repeatable) |
| `--actor` | | string | Filter by actor |
| `--since` | | duration/date | Show logs since time |
| `--until` | | duration/date | Show logs until time |
| `--limit` | `-n` | int | Limit results (default: 100) |
| `--follow` | `-f` | bool | Follow logs (tail mode) |

### Examples

```bash
# All logs for task
tlc log T-0042

# Status changes only
tlc log T-0042 --action STATUS_CHANGED

# Recent logs (last 24 hours)
tlc log --since 24h

# All logs by actor
tlc log --actor codex

# Follow logs (tail)
tlc log --follow

# Output
2025-01-16T10:30:00Z  T-0042  CREATED           (system)
2025-01-16T10:35:00Z  T-0042  STATUS_CHANGED    TODO → IN_PROGRESS (codex)
2025-01-16T11:15:00Z  T-0042  ASSIGNED          @codex (codex)
2025-01-16T12:00:00Z  T-0042  COMMENT           "Started implementation" (codex)
```

---

## `tlc sync` — External System Sync

Synchronize with external task systems.

### Subcommands

- `pull` — Pull updates from external systems
- `push` — Push local changes to external systems
- `status` — Show sync status
- `config` — Configure external systems

---

### `tlc sync pull`

Pull updates from external systems.

#### Synopsis

```bash
tlc sync pull [system] [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--all` | bool | Sync all configured systems |
| `--force` | bool | Force sync (ignore cache) |
| `--dry-run` | bool | Show what would sync |

#### Examples

```bash
# Pull from all systems
tlc sync pull --all

# Pull from GitHub only
tlc sync pull github

# Dry run
tlc sync pull --all --dry-run

# Output
Syncing with GitHub...
  ✓ Pulled 3 new issues
  ✓ Updated 5 existing tasks
  ⚠ 1 conflict detected (T-0042)

Syncing with Jira...
  ✓ Pulled 1 new issue
  ✓ No updates

Summary: 4 new, 5 updated, 1 conflict
```

---

### `tlc sync push`

Push local changes to external systems.

#### Synopsis

```bash
tlc sync push [system] [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--all` | bool | Push to all systems |
| `--task` | string | Push specific task only |

#### Examples

```bash
# Push all changes
tlc sync push --all

# Push to GitHub only
tlc sync push github

# Push specific task
tlc sync push github --task T-0042
```

---

### `tlc sync status`

Show sync status and conflicts.

#### Synopsis

```bash
tlc sync status [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--conflicts-only` | bool | Show only conflicts |

#### Examples

```bash
# All sync status
tlc sync status

# Output
GitHub:
  Status: Connected
  Last sync: 2 mins ago
  Pending: 3 tasks to push
  Conflicts: 1 task

Jira:
  Status: Connected
  Last sync: 15 mins ago
  Pending: None
  Conflicts: None

Conflicts:
  T-0042: Title mismatch (local: "Add API...", remote: "Implement API...")
```

---

### `tlc sync config`

Configure external system integration.

#### Synopsis

```bash
tlc sync config <system> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--enable` | bool | Enable sync for system |
| `--disable` | bool | Disable sync for system |
| `--set` | key=value | Set config value |

#### Examples

```bash
# Configure GitHub
tlc sync config github \
  --set repo=org/repo \
  --set sync_interval=5m \
  --enable

# Disable Jira sync
tlc sync config jira --disable
```

---

## `tlc label` — Label Management

Manage GitHub labels for issues.

### Subcommands

- `init` — Initialize labels from template
- `list` — List available labels
- `create` — Create new label
- `templates` — Show label templates

---

### `tlc label init`

Initialize labels from project-type template.

#### Synopsis

```bash
tlc label init [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--type` | enum | Project type (auto-detect if omitted) |
| `--force` | bool | Recreate existing labels |

#### Examples

```bash
# Auto-detect and initialize
tlc label init

# Output
Detected project type: go-binary
Creating labels:
  ✓ domain:cli
  ✓ domain:core
  ✓ domain:config
  ✓ domain:io
  ✓ priority:high
  ✓ priority:medium
  ✓ feat
  ✓ fix

# Override detection
tlc label init --type python-mvc
```

---

### `tlc label templates`

Show available label templates.

#### Synopsis

```bash
tlc label templates
```

#### Examples

```bash
tlc label templates

# Output
Available templates:
  - go-binary
  - go-socket
  - python-mvc
  - react-frontend
  - microservices
  - generic
```

---

## `tlc config` — Configuration Management

Manage TLC configuration.

### Subcommands

- `get` — Get config value
- `set` — Set config value
- `list` — List all config
- `edit` — Edit config file

---

### `tlc config get`

Get configuration value.

#### Synopsis

```bash
tlc config get <key>
```

#### Examples

```bash
tlc config get worktree.directory
# Output: .worktrees

tlc config get sync.github.repo
# Output: org/repo
```

---

### `tlc config set`

Set configuration value.

#### Synopsis

```bash
tlc config set <key> <value> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--global` | bool | Set in user config (not project) |

#### Examples

```bash
# Project-level
tlc config set worktree.directory .work

# User-level
tlc config set --global output.format json
```

---

### `tlc config list`

List all configuration.

#### Synopsis

```bash
tlc config list [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--global` | bool | Show user config only |
| `--project` | bool | Show project config only |

#### Examples

```bash
# All config (merged)
tlc config list

# User config only
tlc config list --global
```

---

## `tlc auth` — Authentication

Authenticate with external systems.

### Subcommands

- `login` — Login to external system
- `logout` — Logout from external system
- `status` — Show auth status
- `refresh` — Refresh tokens

---

### `tlc auth login`

Login to external system.

#### Synopsis

```bash
tlc auth login <system> [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--token` | string | API token (or use interactive flow) |

#### Examples

```bash
# Interactive login (OAuth)
tlc auth login github

# Token-based login
tlc auth login github --token ghp_xxxxxxxxxxxxx

# Output
Opening browser for GitHub OAuth...
✓ Authenticated as @codex
✓ Token saved to system keychain
```

---

### `tlc auth status`

Show authentication status.

#### Synopsis

```bash
tlc auth status [system]
```

#### Examples

```bash
# All systems
tlc auth status

# Output
GitHub:
  ✓ Authenticated as @codex
  Token expires: 2025-07-16

Jira:
  ✗ Not authenticated
```

---

## `tlc version`

Show version information.

#### Synopsis

```bash
tlc version [flags]
```

#### Flags

| Flag | Type | Description |
|------|------|-------------|
| `--short` | bool | Show version number only |

#### Examples

```bash
tlc version
# Output:
# TLC version 0.1.0
# Built: 2025-01-16
# Commit: abc1234

tlc version --short
# Output: 0.1.0
```

---

## Output Formats

### Table Format (Default for TTY)

```
┌──────────┬─────────────────────────┬──────────────┬────────────┐
│ ID       │ Title                   │ Status       │ Assigned   │
├──────────┼─────────────────────────┼──────────────┼────────────┤
│ T-0042   │ Add API rate limiting   │ IN_PROGRESS  │ codex      │
└──────────┴─────────────────────────┴──────────────┴────────────┘
```

### JSON Format

```json
[
  {
    "id": "T-0042",
    "title": "Add API rate limiting",
    "status": "IN_PROGRESS",
    "assigned_to": "codex",
    "tags": ["feat", "api"],
    "created_at": "2025-01-16T10:30:00Z"
  }
]
```

### YAML Format

```yaml
- id: T-0042
  title: Add API rate limiting
  status: IN_PROGRESS
  assigned_to: codex
  tags:
    - feat
    - api
  created_at: 2025-01-16T10:30:00Z
```

### Task Line Syntax (TLS)

```
[~] T-0042 Add API rate limiting @codex #feat #api prio:P1
```

See `task-line-spec-0.1.md` for full TLS specification.

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TLC_CONFIG` | Config file path | Auto-detect |
| `TLC_FORMAT` | Default output format | `table` (TTY), `json` (non-TTY) |
| `TLC_NO_COLOR` | Disable colors | `false` |
| `TLC_EDITOR` | Editor for interactive editing | `$EDITOR` |
| `TLC_PAGER` | Pager for long output | `$PAGER` |
| `GITHUB_TOKEN` | GitHub API token | - |
| `JIRA_TOKEN` | Jira API token | - |

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid usage (bad arguments) |
| 3 | Not found (task, flow, etc.) |
| 4 | Validation error |
| 5 | Permission denied |
| 6 | Conflict (sync, concurrent edit) |
| 7 | Timeout |
| 8 | Network error |
| 9 | Authentication error |

---

## Shell Completion

TLC supports shell completion for bash, zsh, and fish.

### Installation

```bash
# Bash
tlc completion bash > /etc/bash_completion.d/tlc

# Zsh
tlc completion zsh > /usr/local/share/zsh/site-functions/_tlc

# Fish
tlc completion fish > ~/.config/fish/completions/tlc.fish
```

---

## Implementation Notes

### Recommended Libraries (Go)

**CLI Framework**:
- [Cobra](https://github.com/spf13/cobra) — CLI application framework
- [Viper](https://github.com/spf13/viper) — Configuration management

**Interactive Components** (Charm):
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) — TUI framework (for `tlc tui`)
- [Huh](https://github.com/charmbracelet/huh) — Interactive forms and prompts
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) — Terminal styling
- [Glamour](https://github.com/charmbracelet/glamour) — Markdown rendering
- [Log](https://github.com/charmbracelet/log) — Structured logging

**Output Formatting**:
- [Bubbles Table](https://github.com/charmbracelet/bubbles) — Table component for `--format table`
- Standard library `encoding/json` for JSON output
- [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml) for YAML output

### Interactive Mode Examples

**Using Huh for prompts**:

```go
import (
    "github.com/charmbracelet/huh"
    "github.com/charmbracelet/lipgloss"
)

// Interactive task creation (tlc task create --interactive)
func createTaskInteractive() error {
    var (
        title       string
        description string
        status      string
        assignee    string
        tags        []string
        priority    string
    )

    form := huh.NewForm(
        huh.NewGroup(
            huh.NewInput().
                Title("Task Title").
                Value(&title).
                Validate(func(s string) error {
                    if len(s) == 0 {
                        return fmt.Errorf("title required")
                    }
                    return nil
                }),

            huh.NewText().
                Title("Description").
                Value(&description).
                Lines(5),
        ),

        huh.NewGroup(
            huh.NewSelect[string]().
                Title("Status").
                Options(
                    huh.NewOption("TODO", "TODO"),
                    huh.NewOption("IN_PROGRESS", "IN_PROGRESS"),
                    huh.NewOption("DONE", "DONE"),
                ).
                Value(&status),

            huh.NewInput().
                Title("Assigned To").
                Placeholder("@username").
                Value(&assignee),
        ),

        huh.NewGroup(
            huh.NewMultiSelect[string]().
                Title("Tags").
                Options(
                    huh.NewOption("feat", "feat"),
                    huh.NewOption("fix", "fix"),
                    huh.NewOption("chore", "chore"),
                    huh.NewOption("docs", "docs"),
                ).
                Value(&tags),

            huh.NewSelect[string]().
                Title("Priority").
                Options(
                    huh.NewOption("Low", "low"),
                    huh.NewOption("Medium", "medium"),
                    huh.NewOption("High", "high"),
                    huh.NewOption("Critical", "critical"),
                ).
                Value(&priority),
        ),
    )

    err := form.Run()
    if err != nil {
        return err
    }

    // Create task with collected values
    task := Task{
        Title:       title,
        Description: description,
        Status:      status,
        AssignedTo:  assignee,
        Tags:        tags,
        Meta: map[string]string{
            "priority": priority,
        },
    }

    return createTask(task)
}

// Confirmation prompts
func confirmDelete(taskID string) (bool, error) {
    var confirm bool

    form := huh.NewForm(
        huh.NewGroup(
            huh.NewConfirm().
                Title(fmt.Sprintf("Delete task %s?", taskID)).
                Description("This action cannot be undone.").
                Value(&confirm),
        ),
    )

    err := form.Run()
    return confirm, err
}
```

**Using Lip Gloss for styled output**:

```go
import "github.com/charmbracelet/lipgloss"

var (
    // Brand colors
    primaryColor = lipgloss.Color("#00AAFF")
    successColor = lipgloss.Color("#00FF00")
    warningColor = lipgloss.Color("#FFAA00")
    errorColor   = lipgloss.Color("#FF0000")

    // Styles
    titleStyle = lipgloss.NewStyle().
        Foreground(primaryColor).
        Bold(true).
        MarginBottom(1)

    successStyle = lipgloss.NewStyle().
        Foreground(successColor).
        Bold(true)

    errorStyle = lipgloss.NewStyle().
        Foreground(errorColor).
        Bold(true)

    boxStyle = lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(primaryColor).
        Padding(1, 2)
)

// Pretty print task
func printTask(task Task) {
    fmt.Println(titleStyle.Render(fmt.Sprintf("Task %s", task.ID)))
    fmt.Printf("Title: %s\n", task.Title)
    fmt.Printf("Status: %s\n", formatStatus(task.Status))
    fmt.Printf("Assigned: @%s\n", task.AssignedTo)
    fmt.Printf("Tags: %s\n", strings.Join(task.Tags, ", "))

    if task.Description != "" {
        desc := boxStyle.Render(task.Description)
        fmt.Println("\nDescription:")
        fmt.Println(desc)
    }
}

func formatStatus(status string) string {
    switch status {
    case "TODO":
        return "[ ] TODO"
    case "IN_PROGRESS":
        return lipgloss.NewStyle().Foreground(primaryColor).Render("[●] IN_PROGRESS")
    case "DONE":
        return successStyle.Render("[✓] DONE")
    case "SKIPPED":
        return warningStyle.Render("[-] SKIPPED")
    default:
        return status
    }
}

// Success messages
func printSuccess(message string) {
    fmt.Println(successStyle.Render("✓ " + message))
}

// Error messages
func printError(err error) {
    fmt.Println(errorStyle.Render("✗ Error: " + err.Error()))
}
```

**Using Glamour for markdown descriptions**:

```go
import "github.com/charmbracelet/glamour"

// Render task description as markdown
func printTaskDescription(task Task) error {
    // Auto-detect terminal capabilities
    r, err := glamour.NewTermRenderer(
        glamour.WithAutoStyle(),
        glamour.WithWordWrap(80),
    )
    if err != nil {
        return err
    }

    out, err := r.Render(task.Description)
    if err != nil {
        return err
    }

    fmt.Println(titleStyle.Render("Description:"))
    fmt.Print(out)
    return nil
}
```

**Using Charm Log for structured logging**:

```go
import "github.com/charmbracelet/log"

func setupLogging(verbose bool) {
    logger := log.NewWithOptions(os.Stderr, log.Options{
        ReportTimestamp: true,
        TimeFormat:      "15:04:05",
        Prefix:          "tlc 🚀",
    })

    if verbose {
        logger.SetLevel(log.DebugLevel)
    } else {
        logger.SetLevel(log.InfoLevel)
    }

    log.SetDefault(logger)
}

// Usage
log.Info("Creating task", "id", "T-0042", "title", "Add API rate limiting")
log.Debug("Calling CLI command", "cmd", "tlc task create")
log.Error("Failed to create task", "error", err)
```

### CLI Structure with Cobra

```go
package cmd

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
    Use:   "tlc",
    Short: "Task Line CLI - Multi-agent task orchestration",
    Long:  "TLC provides commands for task management, flow execution, and collaboration.",
}

var taskCmd = &cobra.Command{
    Use:   "task",
    Short: "Task operations",
}

var taskCreateCmd = &cobra.Command{
    Use:   "create [title]",
    Short: "Create new task",
    Args:  cobra.MaximumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        interactive, _ := cmd.Flags().GetBool("interactive")

        if interactive || len(args) == 0 {
            return createTaskInteractive()
        }

        title := args[0]
        // ... create task
        return nil
    },
}

func init() {
    // Global flags
    rootCmd.PersistentFlags().StringP("config", "c", "", "config file")
    rootCmd.PersistentFlags().StringP("format", "f", "table", "output format")
    rootCmd.PersistentFlags().Bool("no-color", false, "disable colors")
    rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")

    viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
    viper.BindPFlag("format", rootCmd.PersistentFlags().Lookup("format"))

    // Task create flags
    taskCreateCmd.Flags().BoolP("interactive", "i", false, "interactive mode")
    taskCreateCmd.Flags().StringP("description", "d", "", "task description")
    taskCreateCmd.Flags().StringP("status", "s", "TODO", "initial status")
    taskCreateCmd.Flags().StringP("assigned-to", "a", "", "assignee")

    // Build command tree
    taskCmd.AddCommand(taskCreateCmd)
    rootCmd.AddCommand(taskCmd)
}

func Execute() error {
    return rootCmd.Execute()
}
```

---

## References

- [task-crud-spec-0.1.md](task-crud-spec-0.1.md) — Task schema and operations
- [task-flow-spec-0.1.md](task-flow-spec-0.1.md) — Flow orchestration
- [task-log-spec-0.1.md](task-log-spec-0.1.md) — Audit logging
- [task-collab-spec-0.1.md](task-collab-spec-0.1.md) — Collaboration semantics
- [tlc-query-spec-0.1.md](tlc-query-spec-0.1.md) — Query syntax (to be created)
- [tlc-config-spec-0.1.md](tlc-config-spec-0.1.md) — Configuration (to be created)
- [tlc-auth-spec-0.1.md](tlc-auth-spec-0.1.md) — Authentication (to be created)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
