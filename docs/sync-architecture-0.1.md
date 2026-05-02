# TLC External System Sync Architecture - Summary

## Overview

TLC (Task Line CLI) supports integration with external task management systems (GitHub Issues, Jira, Linear, etc.) while maintaining a clear distinction between internal and external tasks. Task identity is managed via TypeID (see [identifiers-spec-0.1.md](identifiers-spec-0.1.md)).

## Core Principles

1. **Internal tasks are NEVER synced** - Tasks created via TLC CLI remain local, providing full traceability for agent-managed work
2. **TLC is NOT a sync engine** - TLC maintains sync with each task's origin system only, not across multiple systems
3. **Origin system is source of truth** - For synced tasks, the external system where the task originated is authoritative
4. **Polling-based sync** - Until webhook support, TLC regularly polls integrated systems (default: 5 minutes)

## Task Types

### Internal Tasks
- Created directly via TLC CLI commands
- NO `origin_system` field in meta
- NEVER synced to external systems
- Enable agents to manage internal todos with full traceability
- Reference points to internal documentation

### External Synced Tasks
- Originated from external systems (GitHub, Jira, Linear, etc.)
- MUST have `origin_system` field in meta
- Kept in sync with origin system via polling and push-back
- Reference points to origin system URL

## Meta Field Extensions

Reserved meta fields for external system sync (v0.1):

| Field | Type | Description |
|-------|------|-------------|
| `origin_system` | string | External system identifier (e.g., "github", "jira", "linear") |
| `origin_id` | string | Task ID in the origin system |
| `origin_url` | string | Direct URL to task in origin system |
| `last_sync_at` | ISO8601 UTC | Last sync timestamp with origin system |
| `sync_direction` | string | "pull" (read-only), "push" (write-back), or "bidirectional" |

**Rule**: Absence of `origin_system` field indicates an internal task.

## Sync Actions

New log actions for tracking sync operations:

- **SYNC_IMPORTED** - Task imported from external system
- **SYNC_PULLED** - Task updated from origin system (poll)
- **SYNC_PUSHED** - Local changes pushed to origin system
- **SYNC_CONFLICT** - Sync conflict detected requiring resolution
- **SYNC_ERROR** - Sync operation failed

## Change Tracking

TLC identifies tasks that need to be pushed back to their origin systems by comparing the `updated_at` timestamp with the `last_sync_at` timestamp.

A task is considered "dirty" (needing push) if:
1. It has an `origin_system` defined.
2. EITHER `last_sync_at` is null (never synced).
3. OR `updated_at` > `last_sync_at`.

This logic is encapsulated in the `NeedsPush()` method on the Task model. The storage layer provides a optimized query `GetTasksNeedingPush()` to fetch all such tasks efficiently.

## Bidirectional Sync & Conflict Resolution

TLC supports bidirectional synchronization, allowing changes to flow both from external systems to TLC (pull) and from TLC back to external systems (push).

### Conflict Detection

A conflict occurs when both the local task and the remote task have been modified since the last synchronization point (`last_sync_at`).

TLC detects conflicts during the `sync pull` operation:
- Local modification: `local.updated_at > local.last_sync_at`
- Remote modification: `remote.updated_at > local.last_sync_at`

### Resolution Strategies

When a conflict is detected, TLC applies one of the following strategies (configurable via `--strategy` flag):

- **remote-wins** (default): Overwrites local changes with the remote version.
- **local-wins**: Keeps local changes; the task will be pushed to the remote system in the next `sync push`.
- **last-write-wins**: Chooses the version with the most recent `updated_at` timestamp.
- **manual**: Prompts the user interactively to choose between local and remote versions.

### Implementation Notes

- **Extensions**: Sync integrations (GitHub, Jira, Linear) are implemented as
  `ext.Extension` instances registered via `kit/ext.Manager` (see
  `internal/extensions/`). Each provider must include `updated_at` and
  `created_at` in the `Task` objects returned by `sync.pull` to enable
  accurate conflict detection.
- **Audit Logs**: All conflict resolutions are logged with the `SYNC_CONFLICT` action.
- **Last Sync Timestamp**: The `last_sync_at` field is updated only after a successful pull or push operation, serving as the baseline for future change tracking.

## Sync Workflow

1. **Poll**: TLC regularly probes integrated systems for new/updated tasks
2. **Import**: New external tasks are created with `origin_system` metadata set
3. **Local Updates**: Changes made to synced tasks in TLC are tracked in logs
4. **Push**: Local changes are pushed back to origin system based on `sync_direction`
5. **Conflict Resolution**: Last-write-wins or manual resolution based on implementation

## Sync Plugins

TLC integrates external systems via pluggable sync providers:

| Plugin | Scope | Status |
|---|---|---|
| github-sync | GitHub Issues ↔ Task | v1 shipped |
| jira-sync | Jira Issues ↔ Task | v1 shipped |
| linear-sync | Linear Issues ↔ Task | v1 shipped |
| vtodo-sync | RFC 5545 VTODO ↔ Task (file mode) | v0.1 shipped; CalDAV deferred |

Each plugin implements `sync.pull`, `sync.push`, and optionally `sync.delete` RPC methods. See [tlc-plugin-spec-0.1.md](tlc-plugin-spec-0.1.md) for interface details.

## Implementation Guidelines

### Must Requirements

- Never sync internal tasks to external systems
- Always respect `sync_direction` field
- Maintain origin system as single source of truth for synced tasks
- Log sync errors as FAILURE action with recovery steps

### Should Requirements

- Poll origin systems at configurable intervals (default: 5 minutes)
- Track `last_sync_at` timestamp for each synced task
- Handle sync failures gracefully with retry logic
- Log sync operations using COMMENT or SYNC_* actions
- Provide conflict resolution strategy when local and remote changes conflict

## Updated Specifications

The following specifications have been enhanced to include sync architecture:

1. **task-crud-spec-0.1.md**
   - Meta field validation with reserved sync keys
   - External System Sync Architecture section (normative)
   - Internal vs External task examples

2. **task-log-spec-0.1.md**
   - 5 new sync action types added to action vocabulary
   - Sync lifecycle logging examples
   - Conflict resolution logging example

3. **glossary-0.1.md**
   - External System Sync Terms section
   - Definitions for all sync-related concepts
   - Sync actions in action vocabulary

## Use Cases

### Agent Tool Function
TLC provides a tool function for agents to call during planning:
- Agents can create internal tasks for their work breakdown
- All internal tasks leave a trace in TLC
- Team members can discover agent planning and work items
- No external system pollution with agent internal todos

### External Task Management
TLC integrates with existing workflows:
- Import tasks from GitHub, Jira, Linear, etc.
- Work on synced tasks within TLC interface
- Changes automatically pushed back to origin system
- Single source of truth maintained per task

## Example Task Comparison

**Internal Task**:
```json
{
  "id": "T-0200",
  "title": "Refactor authentication module",
  "reference": "docs/architecture/auth-refactor.md",
  "meta": {
    "priority": "medium"
  }
  // NO origin_system field
}
```

**External Synced Task**:
```json
{
  "id": "T-0201",
  "title": "Fix login bug on mobile devices",
  "reference": "https://github.com/org/repo/issues/456",
  "meta": {
    "origin_system": "github",
    "origin_id": "456",
    "origin_url": "https://github.com/org/repo/issues/456",
    "last_sync_at": "2025-01-15T10:00:00Z",
    "sync_direction": "bidirectional"
  }
}
```

## Next Steps

Future enhancements may include:
- Webhook support for real-time sync instead of polling
- Conflict resolution UI
- Sync status dashboard
- Multi-system sync coordination (with explicit user configuration)
- Sync performance metrics and monitoring

---

## References

- task-crud-spec-0.1.md — Task CRUD with sync meta fields
- task-log-spec-0.1.md — Sync action vocabulary
- glossary-0.1.md — Sync terminology definitions

---

**Version**: 0.1
**Last Updated**: 2025-01-15
