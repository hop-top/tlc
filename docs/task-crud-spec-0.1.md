# Task CRUD Specification (TCS) v0.1

## Version

- Version: 0.1

## Summary

Task CRUD Spec defines the canonical task entity model and permitted mutations.

This spec OWNS:
- task schema fields (canonical)
- persistence invariants and mutation semantics
- task status values and allowed transitions (as stored state)

This spec DOES NOT own:
- execution protocol (see task-exec-spec-0.1.md)
- collaboration/claiming rules (see task-collab-spec-1.0.md)
- flow orchestration semantics (see task-flow-spec-0.1.md)

This spec references:
- identifiers-spec-0.1.md for TypeID format and T-NNNN alias rules
- task-line-spec-0.1.md for line format
- task-log-spec-0.1.md for log schema

---

## Canonical Task Entity

A Task MUST be representable as:

- id: string (stable, unique)
- title: string (human-readable)
- description: string (optional but recommended)
- status: enum
  - TODO
  - IN_PROGRESS
  - DONE
  - SKIPPED
- assigned_to: string or null
- tags: array of strings (optional)
- reference: string (required)
- created_at: ISO8601 UTC (recommended)
- updated_at: ISO8601 UTC (recommended)
- logs: array of log entries (recommended, see task-log-spec-0.1.md)
- meta: object/map (optional)
- archived: boolean (defaults to false)
- stale_timeout: duration in nanoseconds (optional); task is stale if `now − updated_at > stale_timeout`
- blocked_reason: string (optional); human-readable explanation of why the task is blocked
- stale_fired_at: ISO8601 UTC (optional); timestamp when the stale alert was last emitted

---

## Configuration

### Stale Detection

Staleness is computed at read-time: `now − updated_at > effective_timeout`.
No daemon required.

#### Keys

- `task.stale.default_timeout` — duration; project-wide stale threshold; default `6h`
  - Applied when a task's `stale_timeout` is nil
  - Format: Go duration string (`6h`, `30m`, `72h`)
- `task.stale.hooks` — list of hook objects; fired when a task crosses the stale threshold
  - Each entry: `{command: string}`
  - Command is a Go `text/template` string; expanded before execution via `sh -c`

#### Hook Template Variables

| Variable | Type | Description |
|----------|------|-------------|
| `{{.ID}}` | string | Task display alias (e.g. `T-0042`; see identifiers-spec-0.1.md) |
| `{{.Title}}` | string | Task title |
| `{{.AssignedTo}}` | string | Assignee identifier; empty if unassigned |
| `{{.UpdatedAt}}` | time.Time | Last mutation timestamp (UTC) |
| `{{.Timeout}}` | time.Duration | Effective stale threshold |
| `{{.StaleSince}}` | time.Duration | How long past the threshold |

#### Hook Fire Semantics

- Fired once per stale crossing; tracked via `stale_fired_at`
- **Auto-fired on `tlc task list`**: after filtering, before rendering, any task with
  `IsStale() == true` and `StaleFiredAt == nil` triggers `RunStaleHooks` and records
  `StaleFiredAt`; subsequent `task list` calls are no-ops until the crossing resets
- **Row output only**: an aggregate format (`--summary` / `--counters`, however
  spelled) fires no hooks and writes no `StaleFiredAt`. Counts never read that
  field, and `task list` is annotated `kit/side-effect: read` — a hook exec plus
  a write transaction per stale task is not a read
- Crossing resets naturally: any task update advances `UpdatedAt` and sets
  `StaleFiredAt = nil`; staleness is re-evaluated on the next `task list` run
- `tlc task stale --run-hooks` forces re-fire regardless of `stale_fired_at`
- Hook failures are non-fatal; remaining hooks run regardless

#### Example Config (YAML)

```yaml
task:
  stale:
    default_timeout: 6h
    hooks:
      - command: 'echo "stale: {{.ID}} {{.Title}} (since {{.StaleSince}})" >> /tmp/stale.log'
```

---

## Status Semantics (CRUD-Level)

- TODO
  - Task exists and is available to be started.
- IN_PROGRESS
  - Task is actively being worked on.
- DONE
  - Task is completed and immutable except for COMMENT logs.
- SKIPPED
  - Task is intentionally abandoned and immutable except for COMMENT logs.

---

## Allowed Status Transitions (Normative)

Allowed:

- TODO -> IN_PROGRESS
- TODO -> SKIPPED
- IN_PROGRESS -> DONE
- IN_PROGRESS -> TODO
- IN_PROGRESS -> SKIPPED

Forbidden:

- DONE -> anything
- SKIPPED -> anything
- TODO -> DONE (must pass through IN_PROGRESS for audit clarity)

---

## CRUD Operations (Normative)

### CreateTask

Inputs:
- id
- title
- reference

Optional:
- description
- tags
- assigned_to
- meta

Rules:
- id MUST be unique
- reference MUST be non-empty
- status MUST initialize to TODO unless explicitly created as IN_PROGRESS
- creation MUST emit a CREATED log entry

### ReadTask

Rules:
- Read MUST be non-mutating
- Read MAY filter by:
  - status
  - tags
  - assigned_to
  - substring search on title
  - `--stale` — post-query: keep only tasks where `IsStale()` is true
    (applies project `stale.default_timeout` to tasks with no per-task timeout)
  - `--blocked` — column predicate: `blocked_reason IS NOT NULL AND blocked_reason != ''`
  - `--overdue` — column predicate: `due_at < now AND status NOT IN ('DONE', 'SKIPPED')`.
    One definition, expressed in SQL, shared by `task list --overdue` and
    `tlc status`; a finished task past its due date is not overdue.

Aggregate output (`--summary`, `--counters`, and the same two as
`--format` values or a config `output.format`) counts the whole match set,
never a page of it: `--limit`/`--offset` are dropped before the query, since a
count truncated at the page size is indistinguishable from a real one. Where
every filter is a column predicate the count is a single SQL aggregate;
`--stale`, `--blocked-by`, and qualified track IDs are decided in Go, so those
count a fully-filtered, unpaginated slice instead.

#### Query Operators

Implementations SHOULD support the following query operators:

- **Equality**: `status == "TODO"`, `assigned_to == "codex"`
- **Inequality**: `status != "DONE"`
- **Contains**: `tags contains "bug"`, `title contains "auth"`
- **In**: `status in ["TODO", "IN_PROGRESS"]`
- **Date ranges**: `created_at >= "2025-01-01"`, `updated_at < "2025-02-01"`
- **Null checks**: `assigned_to == null`, `assigned_to != null`

#### Sorting

Implementations SHOULD support sorting by:

- `created_at` (ascending or descending)
- `updated_at` (ascending or descending)
- `status` (by enum order: TODO, IN_PROGRESS, DONE, SKIPPED)
- `id` (lexicographic)
- `title` (lexicographic)

Default sort order: `created_at DESC` (newest first)

#### Pagination

Implementations SHOULD support pagination via:

- **Offset/Limit**: Skip N records, return M records
  - `offset=0, limit=20` returns first 20 tasks
  - `offset=20, limit=20` returns next 20 tasks
- **Cursor-based** (recommended for large datasets):
  - `after=<task_id>` returns tasks created after specified ID
  - Stable under concurrent writes

#### Bulk Read

Implementations MAY support bulk read by IDs:
- Input: array of task IDs
- Output: array of task objects (in ID order)
- Missing IDs: included as `null` or omitted based on policy

### UpdateTask

Allowed updates:
- title
- description
- tags
- assigned_to
- reference (allowed only if original is wrong; MUST be logged)
- meta

Rules:
- Update MUST NOT change id
- If task is DONE or SKIPPED, updates MUST be rejected (except COMMENT logs)
- Update MUST emit UPDATED log entry describing changed fields

Note on assignment commands:
- `tlc task assign <id> <assignee>` is the canonical way to assign a task.
  It sets `assigned_to` without changing status and emits a REASSIGNED log.
- `tlc task unassign <id>` clears `assigned_to` without changing status.
- These are distinct from `tlc task update <id> --assigned-to <assignee>`,
  which can be combined with other field changes (e.g. status transitions)
  in a single mutation.

### TransitionStatus

Rules:
- Must satisfy Allowed Status Transitions
- Must emit a log entry that describes the transition

Note:
- Collaboration actions like CLAIMED/RELEASED/REASSIGNED are owned by task-collab,
  but the stored state transition rules live here.

### DeleteTask (Optional)

Hard delete is allowed only if:
- explicitly supported by the system
- task has never been started (status TODO)
- deletion emits:
  - action: DELETED
  - note: justification

Recommendation:
- Prefer SKIPPED over deletion for auditability.

---

## Batch Operations

Lifecycle commands accept multiple task identifiers in a single invocation, fanning out the
operation across all resolved tasks.

### ID Forms

| Form | Example | Behaviour |
|------|---------|-----------|
| Exact ID | `T-0042` | Resolves to a single task; error if not found |
| Regex pattern | `T-001\d` | Matches against all tasks in current filtered set |
| Glob `*` | `*` | Matches all tasks in current filtered set |

### Pattern Detection

An argument is treated as a regex/glob (not an exact ID) when it contains any metacharacter
from the set: `[ ( * ? + \ . ^ $ { } | )`.

Exact IDs (e.g. `T-0042`, `0042`) are never passed to the regex engine.

### Confirmation Behaviour

When a pattern resolves to more than one task and `--no-prompt` is absent:

- In TTY context: a confirmation prompt lists the matched tasks and asks `y/N`; decline
  aborts with no mutation.
- In non-TTY context (scripted, pipe): the command exits non-zero requiring explicit consent.

Single-task matches and exact IDs proceed without a prompt.

### `--no-prompt` Flag

Persistent flag on the `task` parent command; applies to all subcommands.

```
tlc task --no-prompt <subcommand> <args>
```

or as a trailing flag:

```
tlc task <subcommand> <args> --no-prompt
```

Skips all confirmation prompts. Required in non-TTY batch scripts.

### Command-Specific Rules

| Command | Signature | Multi-target notes |
|---------|-----------|-------------------|
| `claim` | `claim <task-id\|pattern>...` | All matched tasks claimed |
| `unclaim` | `unclaim <task-id\|pattern>...` | All matched tasks unclaimed |
| `complete` | `complete <task-id\|pattern>...` | All matched tasks completed |
| `reopen` | `reopen <task-id\|pattern>... --note <msg>` | Note appended to each task |
| `assign` | `assign <assignee> <task-id\|pattern>...` | Assignee arg comes first |
| `unassign` | `unassign <task-id\|pattern>... --note <msg>` | Note appended to each task |
| `update` | `update <task-id\|pattern>... [flags]` | `--title` blocked for multi-target |
| `delete` | `delete <task-id\|pattern>... [--yes\|-y]` | Requires `--yes`/`-y` or `--no-prompt` for >1 |

### `assign` Signature Change

`assign` takes assignee as the first positional argument, followed by one or more task IDs
or patterns:

```
tlc task assign <assignee> <task-id|pattern>...
```

### Mixed Valid/Invalid IDs

When multiple IDs are given and some are not found, resolved tasks are processed; each
not-found ID emits an error. Command exits non-zero if any ID failed to resolve.

---

## Invariants (Normative)

- Every task MUST have exactly one id.
- Every task MUST have exactly one status.
- Every task MUST have exactly one reference pointer.
- A task in DONE or SKIPPED MUST NOT be mutated (except COMMENT logs).
- All mutations MUST be logged (see task-log-spec-0.1.md).

---

## Archiving (Normative)

Archiving is a secondary state that hides tasks from default listings without deleting them.

### Archiving Rules

- A task MAY be archived if its status is `DONE` or `SKIPPED`.
- `TODO` and `IN_PROGRESS` tasks SHOULD NOT be archived.
- Archiving MUST NOT change the task `status`.
- Archived tasks MUST NOT appear in `ReadTask` results by default.
- Archiving MUST be reversible (Unarchive).

### Auto-Archiving

TLC implementations MAY support auto-archiving:
- Tasks are auto-archived if they remain in `DONE` or `SKIPPED` status for longer than a configured `archive_threshold`.
- Auto-archiving typically happens during system startup or sync operations.

---

## Example Task Entity (JSON)

```json
{
  "id": "task_01h455vb4pex5vsknk084sn02q",
  "seq": 2,
  "title": "Fix token refresh race",
  "description": "Observed intermittent auth failures when refresh occurs concurrently.",
  "status": "IN_PROGRESS",
  "assigned_to": "codex",
  "tags": ["auth", "bug"],
  "reference": "docs/rfc/012.md",
  "logs": [
    {
      "timestamp": "2025-01-01T00:00:00Z",
      "entity_type": "task",
      "entity_id": "task_01h455vb4pex5vsknk084sn02q",
      "by": "codex",
      "action": "CLAIMED",
      "note": "Taking ownership"
    }
  ]
}
```

---

## Example TODO Line

- [~] T-0002 Fix token refresh race @codex #auth #bug ref:docs/rfc/012.md
  (Alias T-0002 resolves to task_01h455vb4pex5vsknk084sn02q; see identifiers-spec-0.1.md)

---

## Persistence Format (Normative)

### Storage Options

Implementations MUST choose one of the following persistence strategies:

#### Option 1: File-per-Task (Recommended for Small/Medium Scale)

```
./tasks/
  T-0001.json
  T-0002.json
  T-0003.json
```

**Rules:**
- Each task stored in a separate JSON file
- Filename: `<task_id>.json`
- File content: canonical task entity JSON
- Atomic writes: use temp file + atomic rename
- Concurrent access: file-level locking or optimistic concurrency

**Advantages:**
- Simple implementation
- Easy to version control
- Natural task-level locking
- Human-readable and git-friendly

**Disadvantages:**
- Performance degrades with >10,000 tasks
- Directory listing becomes slow
- No efficient cross-task queries

#### Option 2: Single JSON Array

```
./tasks.json
```

Content:
```json
[
  { "id": "T-0001", ... },
  { "id": "T-0002", ... },
  { "id": "T-0003", ... }
]
```

**Rules:**
- All tasks in one JSON array
- Atomic writes: write entire file atomically
- Concurrent access: file-level locking required
- Updates: read entire file, modify, write back

**Advantages:**
- Single file simplicity
- Easy to backup/transfer
- Simple schema validation

**Disadvantages:**
- Entire file must be read/written for any change
- Not suitable for >1,000 tasks
- Concurrent writes require careful locking
- Merge conflicts in version control

#### Option 3: SQLite Database (Recommended for Production)

```
./tasks.db
```

Schema:
```sql
CREATE TABLE tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT,
  status TEXT NOT NULL CHECK(status IN ('TODO','IN_PROGRESS','DONE','SKIPPED')),
  assigned_to TEXT,
  reference TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  meta TEXT, -- JSON blob
  tags TEXT  -- JSON array or comma-separated
);

CREATE TABLE task_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL,
  timestamp TEXT NOT NULL,
  by TEXT NOT NULL,
  action TEXT NOT NULL,
  note TEXT NOT NULL,
  meta TEXT, -- JSON blob
  FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_assigned_to ON tasks(assigned_to);
CREATE INDEX idx_tasks_created_at ON tasks(created_at);
CREATE INDEX idx_task_logs_task_id ON task_logs(task_id);
```

Schema migrations (additive):
- v3: `effort TEXT NOT NULL DEFAULT ''`
- v4: `priority TEXT NOT NULL DEFAULT ''`
- v5: `stale_timeout INTEGER` (nanoseconds, NULL if unset),
      `blocked_reason TEXT` (NULL if unset),
      `stale_fired_at TEXT` (RFC3339, NULL if unset)

**Rules:**
- ACID transactions for all mutations
- Foreign key constraints enforced
- Indexes for common queries
- Logs stored separately with FK reference

**Advantages:**
- Scales to millions of tasks
- Efficient queries, filtering, sorting
- Built-in concurrency control
- Transaction support
- Battle-tested reliability

**Disadvantages:**
- More complex than flat files
- Not as human-readable
- Requires SQLite dependency

#### Option 4: Hybrid (File + Index)

```
./TODO              # Task Line format (see task-line-spec-0.1.md)
./tasks/            # Full task JSON files
  T-0001.json
  T-0002.json
```

**Rules:**
- `./TODO` is the canonical source of truth for task list and status
- `./tasks/<id>.json` stores full metadata (description, logs, etc.)
- Updates must keep both in sync
- Reads prefer TODO file for listings, JSON for details

**Advantages:**
- Human-readable TODO file
- Detailed metadata in JSON
- Compatible with existing TODO workflows
- Git-friendly

**Disadvantages:**
- Synchronization complexity
- Risk of inconsistency
- More complex implementation

### Persistence Requirements (All Options)

Regardless of storage format, implementations MUST:

1. **Atomic Writes**: Task updates must be atomic (all-or-nothing)
2. **Durability**: Writes must be durable before returning success
3. **Consistency**: Task invariants must be enforced (see Invariants section)
4. **Concurrent Access**: Multiple processes must not corrupt data
   - Use file locks (flock/fcntl)
   - Use database transactions
   - Use optimistic concurrency (version field)
5. **Backup Strategy**: Document how to backup/restore tasks
6. **Migration Path**: Document how to migrate between storage formats

### Validation

Before persisting any task, implementations MUST validate:

- `id` is non-empty and unique
- `title` is non-empty (max length: 500 chars recommended)
- `description` length (max: 10,000 chars recommended)
- `status` is one of the four allowed values
- `reference` is non-empty
- `tags` are valid identifiers (alphanumeric + dash/underscore)
- `created_at` and `updated_at` are valid ISO8601 UTC timestamps
- `logs` array contains valid log entries (per task-log-spec-0.1.md)
- `meta` is valid JSON object (if provided)

### Transaction Semantics

For operations that modify both task state AND logs:

- **ATOMIC**: Status transition + log entry MUST succeed or fail together
- **ORDERED**: Log entry timestamp MUST be captured before state change completes
- **ISOLATED**: Concurrent updates to different tasks MUST NOT interfere

Example:
```
BEGIN TRANSACTION
  1. Validate status transition is allowed
  2. Generate log entry with current timestamp
  3. Update task.status
  4. Update task.updated_at
  5. Prepend log entry to task.logs array
  6. Write log entry to CHANGELOG
  7. Commit
END TRANSACTION
```

If ANY step fails, entire transaction MUST be rolled back.

---

## Field Validation Rules

### id

- Format: TypeID `<prefix>_<26-char-base32-uuidv7>` (see identifiers-spec-0.1.md)
- Examples: `task_01h455vb4pex5vsknk084sn02q`, `track_01h455vbqkfsn02nk084ksn02q`
- Display alias (CLI only): `T-NNNN+` for tasks, slug for tracks (see identifiers-spec-0.1.md)
- MUST be stable once created
- MUST be unique across all tasks
- Length: ~38 characters (typical TypeID format)

### title

- MUST be non-empty
- MUST NOT contain newlines
- Length: 1-500 characters (recommended)
- Leading/trailing whitespace SHOULD be trimmed

### description

- MAY be empty
- MAY contain newlines
- Length: 0-10,000 characters (recommended)
- Markdown format RECOMMENDED

### status

- MUST be one of: `TODO`, `IN_PROGRESS`, `DONE`, `SKIPPED`
- Case-sensitive
- No other values allowed

### assigned_to

- MAY be null (unassigned)
- MUST be a valid agent identifier if set
- Format: alphanumeric + dash/underscore
- Examples: `codex`, `claude`, `user-123`, `human`

### tags

- MUST be an array (may be empty)
- Each tag MUST be a string
- Tag format: alphanumeric + dash/underscore
- Tags SHOULD be lowercase for consistency
- Duplicate tags MUST be deduplicated
- Example: `["auth", "bug", "p1"]`

### reference

- MUST be non-empty
- SHOULD be a valid file path, URL, or document pointer
- Examples:
  - `docs/architecture/auth.md`
  - `https://github.com/org/repo/issues/123`
  - `RFC-042 Section 3.2`
- Length: 1-500 characters

### created_at / updated_at

- MUST be ISO8601 UTC format
- Example: `2025-01-01T12:34:56Z`
- `created_at` MUST NOT change after creation
- `updated_at` MUST be updated on every mutation
- `updated_at` MUST be >= `created_at`

### logs

- MUST be an array (may be empty)
- Each entry MUST conform to task-log-spec-0.1.md schema
- Logs MUST be in reverse chronological order (newest first)
- When reading from CHANGELOG, logs MAY be a subset (recent N entries)

### meta

- MAY be null or empty object
- MUST be valid JSON object if provided
- Reserved keys for external system sync (v0.1):
  - `origin_system` (string): External system identifier (e.g., "github", "jira", "linear")
  - `origin_id` (string): Task ID in the origin system
  - `origin_url` (string): Direct URL to task in origin system
  - `last_sync_at` (ISO8601 UTC): Last sync timestamp with origin system
  - `sync_direction` (string): "pull" (read-only from origin), "push" (write-back), or "bidirectional"
- Tasks created via TLC CLI MUST NOT have origin_system set (internal tasks are never synced)
- Tasks with origin_system set are externally-synced tasks that originated from external systems
- Future versions MAY define additional reserved keys
- Examples:
  - Internal task: `{"priority": "high"}`
  - Synced task: `{"origin_system": "github", "origin_id": "123", "origin_url": "https://github.com/org/repo/issues/123", "last_sync_at": "2025-01-15T10:00:00Z", "sync_direction": "bidirectional"}`
  - Custom fields: `{"sprint": "2025-W01", "story_points": 5}`

---

## External System Sync Architecture (Normative)

TLC distinguishes between two types of tasks based on their origin:

### Internal Tasks

**Definition**: Tasks created directly via TLC CLI commands.

**Rules**:
- MUST NOT have `origin_system` field in meta
- MUST NOT be synced to any external system
- Provide full traceability for agent-managed internal work
- Enable tool functions for agents to manage todo items during planning

**Example**:
```json
{
  "id": "T-0100",
  "title": "Refactor authentication module",
  "status": "TODO",
  "reference": "docs/internal/refactor-plan.md",
  "meta": {
    "priority": "medium"
  }
}
```

Implementation note:
- TLC stores `meta.blocked_by` as a string array in JSON/YAML.
- `tlc task create --blocked-by` validates that each referenced blocker exists.
- `tlc task update --add-blocked-by` also validates existence.
- Local blockers may be given as `T-0001`; cross-project blockers use URI-style
  identifiers such as `other-project/T-0007`.

### External Synced Tasks

**Definition**: Tasks that originated from external systems (GitHub, Jira, Linear, etc.).

**Rules**:
- MUST have `origin_system` field set in meta
- MUST include `origin_id` to identify task in origin system
- SHOULD include `origin_url` for direct access
- MUST be kept in sync with origin system via polling (until webhook support)
- Updates to synced tasks MUST be pushed back to origin system based on `sync_direction`

**TLC's Sync Role**:
- TLC is NOT a sync engine between multiple systems
- TLC polls origin systems to fetch additions/updates
- TLC pushes changes back ONLY to the task's origin system
- Each synced task maintains sync with exactly one origin system

**Sync Workflow**:
1. **Poll**: TLC regularly probes integrated systems for new/updated tasks
2. **Import**: New external tasks are created with origin_system metadata
3. **Local Updates**: Changes made to synced tasks in TLC are tracked
4. **Push**: Local changes are pushed back to origin system
5. **Conflict Resolution**: Last-write-wins or manual resolution based on implementation

**Example**:
```json
{
  "id": "T-0200",
  "title": "Fix authentication bug",
  "status": "IN_PROGRESS",
  "reference": "https://github.com/org/repo/issues/456",
  "meta": {
    "origin_system": "github",
    "origin_id": "456",
    "origin_url": "https://github.com/org/repo/issues/456",
    "last_sync_at": "2025-01-15T14:30:00Z",
    "sync_direction": "bidirectional"
  }
}
```

### Sync Implementation Guidelines

Implementations SHOULD:
- Poll origin systems at configurable intervals (default: 5 minutes)
- Track `last_sync_at` timestamp for each synced task
- Handle sync failures gracefully with retry logic
- Log sync operations using COMMENT action
- Provide conflict resolution strategy when local and remote changes conflict

Implementations MUST:
- Never sync internal tasks to external systems
- Always respect `sync_direction` field
- Maintain origin system as single source of truth for synced tasks
- Log sync errors as FAILURE action with recovery steps

---

## Examples

### Example 1: UPDATED Log Entry

Task before:
```json
{
  "id": "T-0005",
  "title": "Add login endpoint",
  "description": "Create POST /auth/login",
  "status": "IN_PROGRESS",
  "assigned_to": "codex"
}
```

Task after updating description:
```json
{
  "id": "T-0005",
  "title": "Add login endpoint",
  "description": "Create POST /auth/login with rate limiting",
  "status": "IN_PROGRESS",
  "assigned_to": "codex",
  "logs": [
    {
      "timestamp": "2025-01-15T10:30:00Z",
      "entity_type": "task",
      "entity_id": "T-0005",
      "by": "codex",
      "action": "UPDATED",
      "note": "Updated description to include rate limiting requirement"
    }
  ]
}
```

### Example 2: SKIPPED Transition

```json
{
  "id": "T-0008",
  "title": "Migrate to new database",
  "status": "SKIPPED",
  "logs": [
    {
      "timestamp": "2025-01-20T15:00:00Z",
      "entity_type": "task",
      "entity_id": "T-0008",
      "by": "human",
      "action": "SKIPPED",
      "note": "Decision made to keep current database. See docs/adr/015.md"
    },
    {
      "timestamp": "2025-01-10T09:00:00Z",
      "entity_type": "task",
      "entity_id": "T-0008",
      "by": "codex",
      "action": "CREATED",
      "note": "Initial task creation"
    }
  ]
}
```

### Example 3: Invalid Transition (Rejected)

Attempt:
```
Task T-0010 with status=TODO
Transition: TODO -> DONE (skipping IN_PROGRESS)
```

Result:
```
Error: Invalid status transition
- Current status: TODO
- Requested status: DONE
- Reason: Tasks must pass through IN_PROGRESS before DONE
- Allowed transitions from TODO: IN_PROGRESS, SKIPPED
```

### Example 4: Using meta Field

```json
{
  "id": "T-0012",
  "title": "Performance optimization",
  "status": "IN_PROGRESS",
  "meta": {
    "priority": "P1",
    "sprint": "2025-W03",
    "story_points": 8,
    "blocked_by": ["T-0011"],
    "external_links": [
      "https://company.atlassian.net/browse/PERF-42"
    ]
  }
}
```

### Example 5: Internal vs External Tasks

**Internal Task** (created via TLC CLI):
```json
{
  "id": "T-0200",
  "title": "Refactor authentication module",
  "description": "Split auth logic into separate services for better testability",
  "status": "TODO",
  "assigned_to": null,
  "tags": ["refactor", "auth"],
  "reference": "docs/architecture/auth-refactor.md",
  "created_at": "2025-01-15T10:00:00Z",
  "updated_at": "2025-01-15T10:00:00Z",
  "logs": [
    {
      "timestamp": "2025-01-15T10:00:00Z",
      "entity_type": "task",
      "entity_id": "T-0200",
      "by": "codex",
      "action": "CREATED",
      "note": "Created during sprint planning"
    }
  ],
  "meta": {
    "priority": "medium",
    "estimated_hours": 8
  }
}
```

**External Synced Task** (imported from GitHub):
```json
{
  "id": "T-0201",
  "title": "Fix login bug on mobile devices",
  "description": "Users report authentication failures on iOS Safari",
  "status": "IN_PROGRESS",
  "assigned_to": "codex",
  "tags": ["bug", "auth", "mobile"],
  "reference": "https://github.com/org/repo/issues/456",
  "created_at": "2025-01-15T09:00:00Z",
  "updated_at": "2025-01-15T11:30:00Z",
  "logs": [
    {
      "timestamp": "2025-01-15T11:30:00Z",
      "entity_type": "task",
      "entity_id": "T-0201",
      "by": "codex",
      "action": "CLAIMED",
      "note": "Starting investigation"
    },
    {
      "timestamp": "2025-01-15T10:00:00Z",
      "entity_type": "task",
      "entity_id": "T-0201",
      "by": "sync-agent",
      "action": "SYNC_PULLED",
      "note": "Updated description with user reports"
    },
    {
      "timestamp": "2025-01-15T09:00:00Z",
      "entity_type": "task",
      "entity_id": "T-0201",
      "by": "sync-agent",
      "action": "SYNC_IMPORTED",
      "note": "Imported from GitHub issue #456"
    }
  ],
  "meta": {
    "origin_system": "github",
    "origin_id": "456",
    "origin_url": "https://github.com/org/repo/issues/456",
    "last_sync_at": "2025-01-15T10:00:00Z",
    "sync_direction": "bidirectional",
    "priority": "high"
  }
}
```

**Key Differences**:
- Internal task has NO `origin_system` in meta
- External task has sync-related meta fields
- External task logs include SYNC_* actions
- External task reference points to origin system URL
- Internal task reference points to internal documentation

---

## Stale Hook Runner

### Overview

`RunStaleHooks(task *Task, hooks []config.StaleHook) error` executes shell
commands when a task crosses the stale threshold. Hook commands are Go
`text/template` strings expanded against `StaleHookData` before being
passed to `sh -c`.

Always returns `nil`. Hook failures emit a warning to stdout and continue;
all hooks run regardless of individual failures.

### Template Variables

| Variable | Type | Description |
|---|---|---|
| `{{.ID}}` | string | Task display alias (e.g. `T-0094`; see identifiers-spec-0.1.md) |
| `{{.Title}}` | string | Task title |
| `{{.AssignedTo}}` | string | Assignee handle (empty string if unassigned) |
| `{{.UpdatedAt}}` | time.Time | Last update timestamp (UTC) |
| `{{.Timeout}}` | time.Duration | Effective stale threshold |
| `{{.StaleSince}}` | time.Duration | How long task has been past threshold (0 if not stale) |

### Configuration

In `.tlc/config.yaml`:

```yaml
task:
  stale:
    default_timeout: 6h         # fallback when task has no per-task timeout
    hooks:
      - command: 'echo "stale: {{.ID}} ({{.Title}})" >> /tmp/stale.log'
      - command: 'notify-send "Stale task" "{{.ID}} stale for {{.StaleSince}}"'
```

- `default_timeout` — project-wide stale threshold; default `6h` if unset.
- `hooks` — list of hook entries; each entry has a single `command` field.
- Per-task `stale_timeout` overrides `default_timeout` when set.

### Failure Semantics

- Template parse error → warning + skip hook; continue.
- Template execute error → warning + skip hook; continue.
- Shell command non-zero exit → warning + continue; next hook still runs.
- `RunStaleHooks` itself NEVER returns a non-nil error.

### Hook Firing Lifecycle

- `stale_fired_at` records when hooks last fired for a task.
- Auto-fired during `tlc task list` row output — once per stale crossing (guarded by
  `StaleFiredAt == nil`). Aggregate formats (`--summary` / `--counters`) skip the
  bookkeeping entirely: no hook fires and no `stale_fired_at` is written.
- Cleared on any `tlc task update` — `StaleFiredAt` set to nil; stale clock resets with `updated_at`.
- `tlc task stale --run-hooks` re-fires hooks for all currently-stale tasks regardless of
  `stale_fired_at`.
