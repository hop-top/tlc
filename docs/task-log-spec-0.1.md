# Task Log Specification (TLS-LOG) v0.1

## Version

- Version: 0.1
- Generated at: 2026-01-15T15:38:01Z

## Summary

Defines the canonical log entry schema and log write policy used across:
- Task CRUD
- Task Execution
- Collaboration
- Flow Orchestration

## Log Entry Schema

Each log entry MUST include:

- `timestamp`: ISO8601 UTC string
- `entity_type`: one of
  - `task`
  - `flow`
  - `step`
- `entity_id`: string identifier (task_id / flow_id / step_id)
- `by`: actor identifier (agent_id / runner_id / system)
- `action`: string action keyword
- `note`: string, human-readable justification/context

Optional fields:

- `meta`: object/map with structured details
- `ref`: provenance/reference pointer (file path, link, or doc section)

## Log Write Policy

### Default Policy

- Policy: **prepend**
- New log entries MUST be placed at the beginning of the log file and any in-task logs array.

### Rationale

Prepending makes the most recent action immediately visible in CLI and quick file views.

### Alternatives

Append-only logs are allowed only if explicitly overridden by the implementation.
If append is used, the system MUST document the override in `docs/README.md`.

## Canonical Log File Name

Recommended:
- `./CHANGELOG`

Implementations MAY also store a per-task embedded log array, but MUST keep ordering consistent.

---

## File Format (Normative)

### Format: JSON Lines (JSONL)

The CHANGELOG MUST be stored in JSON Lines format:

- Each line is a complete, valid JSON object
- Lines are separated by newline (`\n`)
- No commas between lines
- File extension: `.jsonl` or no extension

Example CHANGELOG:
```
{"timestamp":"2025-01-15T12:00:00Z","entity_type":"task","entity_id":"T-0003","by":"codex","action":"DONE","note":"Completed"}
{"timestamp":"2025-01-15T11:30:00Z","entity_type":"task","entity_id":"T-0003","by":"codex","action":"COMMENT","note":"Testing..."}
{"timestamp":"2025-01-15T11:00:00Z","entity_type":"task","entity_id":"T-0003","by":"codex","action":"CLAIMED","note":"Starting work"}
```

### Rationale for JSONL

- **Line-oriented**: Easy to prepend single lines
- **Streamable**: Can process line-by-line without loading entire file
- **Human-readable**: Each line is valid JSON
- **Tool-friendly**: Supported by jq, grep, and log analysis tools
- **Append-safe**: Multiple writers can append without corruption (with locks)

### Prepend Implementation

To efficiently prepend to JSONL:

#### Option 1: Temp File + Rename (Atomic, Recommended)

```javascript
function prependLog(logFile, logEntry) {
  const tempFile = logFile + '.tmp';
  const logLine = JSON.stringify(logEntry) + '\n';

  // Write new entry to temp file
  fs.writeFileSync(tempFile, logLine);

  // Append existing content
  if (fs.existsSync(logFile)) {
    const existingContent = fs.readFileSync(logFile);
    fs.appendFileSync(tempFile, existingContent);
  }

  // Atomic rename
  fs.renameSync(tempFile, logFile);
}
```

**Advantages:**
- Atomic operation (rename is atomic on most filesystems)
- No risk of corruption
- Works across process crashes

**Disadvantages:**
- Requires reading entire file
- Not efficient for large files (>100MB)

#### Option 2: In-Memory Buffer (Fast, Single Process)

```javascript
const logBuffer = []; // Most recent logs in memory
const MAX_BUFFER_SIZE = 1000;

function prependLog(logFile, logEntry) {
  // Add to in-memory buffer
  logBuffer.unshift(logEntry);

  // Flush to disk periodically
  if (logBuffer.length >= MAX_BUFFER_SIZE) {
    flushLogBuffer(logFile);
  }
}

function flushLogBuffer(logFile) {
  const lines = logBuffer.map(e => JSON.stringify(e)).join('\n') + '\n';

  // Prepend buffer to file
  const existingContent = fs.existsSync(logFile) ? fs.readFileSync(logFile, 'utf8') : '';
  fs.writeFileSync(logFile, lines + existingContent);

  logBuffer.length = 0; // Clear buffer
}
```

**Advantages:**
- Fast writes (batched)
- Reduces disk I/O

**Disadvantages:**
- Not safe across multiple processes
- Logs lost if process crashes before flush
- Requires shutdown hook to flush on exit

#### Option 3: Circular Buffer with Rotation

For very high-volume logging:

```
./CHANGELOG.0  (current, most recent)
./CHANGELOG.1  (previous rotation)
./CHANGELOG.2  (older)
...
```

**Rules:**
- Append to CHANGELOG.0 until size limit reached (e.g., 10MB)
- When limit reached:
  - Rename CHANGELOG.0 → CHANGELOG.1
  - Rename CHANGELOG.1 → CHANGELOG.2
  - Create new empty CHANGELOG.0
- Recent logs always in CHANGELOG.0 (prepended)

---

## Concurrency Control (Normative)

### Multi-Writer Safety

Multiple agents/processes writing to CHANGELOG simultaneously MUST NOT corrupt the file.

#### Option 1: File Locking (Recommended)

```javascript
const lockfile = require('proper-lockfile');

async function prependLogSafe(logFile, logEntry) {
  // Acquire exclusive lock
  const release = await lockfile.lock(logFile, {
    realpath: false,
    retries: {
      retries: 10,
      minTimeout: 100,
      maxTimeout: 1000
    }
  });

  try {
    // Perform prepend while holding lock
    prependLog(logFile, logEntry);
  } finally {
    // Release lock
    await release();
  }
}
```

**POSIX flock/fcntl:**
```javascript
const fs = require('fs');
const lockFile = require('lockfile');

function prependLogWithFlock(logFile, logEntry) {
  const lockPath = logFile + '.lock';

  // Acquire lock
  lockFile.lockSync(lockPath, { retries: 10, retryWait: 100 });

  try {
    prependLog(logFile, logEntry);
  } finally {
    lockFile.unlockSync(lockPath);
  }
}
```

#### Option 2: Database Transactions (SQLite)

If using SQLite for task storage, logs can also use database:

```sql
BEGIN IMMEDIATE TRANSACTION;
  INSERT INTO logs (timestamp, entity_type, entity_id, by, action, note)
  VALUES (?, ?, ?, ?, ?, ?);
COMMIT;
```

**Advantages:**
- ACID guarantees
- No file locking complexity
- Efficient queries

**Disadvantages:**
- Requires database dependency
- Not as human-readable as JSONL

#### Option 3: Append-Only (Alternative to Prepend)

If prepend is too complex, implementations MAY use append-only with explicit override:

```javascript
function appendLog(logFile, logEntry) {
  const logLine = JSON.stringify(logEntry) + '\n';
  fs.appendFileSync(logFile, logLine, { flag: 'a' });
}
```

**Rules for Append-Only:**
- Document override in `docs/README.md`
- Task logs array MUST still be prepended (newest first)
- Reading CHANGELOG requires reversing order

---

## Log Rotation (Normative)

### Rotation Policy

Implementations SHOULD rotate logs when:
- File size exceeds 10MB (recommended threshold)
- Entry count exceeds 100,000 (alternative threshold)
- Age exceeds 90 days (archival threshold)

### Rotation Strategy

```
./CHANGELOG           (current, 0-10MB)
./CHANGELOG.1.gz      (previous rotation, compressed)
./CHANGELOG.2.gz      (older)
./CHANGELOG.3.gz      (oldest)
```

#### Rotation Algorithm

```
1. When CHANGELOG reaches 10MB:
   a. Compress CHANGELOG → CHANGELOG.1.gz
   b. If CHANGELOG.1.gz exists:
      - Rename CHANGELOG.1.gz → CHANGELOG.2.gz
      - Rename CHANGELOG.2.gz → CHANGELOG.3.gz
      - etc.
   c. Create new empty CHANGELOG
2. Keep last N rotations (e.g., N=5)
3. Delete rotations older than N
```

### Querying Rotated Logs

Recent queries read only current CHANGELOG.
Historical queries may need to search rotated files:

```bash
# Search recent logs
grep "T-0042" CHANGELOG

# Search all logs (including rotated)
zgrep "T-0042" CHANGELOG CHANGELOG.*.gz
```

---

## Embedded Log Synchronization (Normative)

Tasks contain an embedded `logs` array. This MUST stay synchronized with CHANGELOG.

### Synchronization Strategy

#### Option 1: Dual-Write (Immediate Consistency)

```javascript
function logAction(taskId, action, note) {
  const logEntry = {
    timestamp: new Date().toISOString(),
    entity_type: "task",
    entity_id: taskId,
    by: AGENT_ID,
    action: action,
    note: note
  };

  // Write to both locations atomically
  try {
    prependToCHANGELOG(logEntry);
    prependToTaskLogsArray(taskId, logEntry);
  } catch (error) {
    // Rollback on failure
    throw error;
  }
}
```

#### Option 2: CHANGELOG as Source of Truth (Lazy Sync)

```javascript
// CHANGELOG is canonical
function getTaskLogs(taskId) {
  // Read from CHANGELOG, filter by entity_id
  const allLogs = readCHANGELOG();
  return allLogs.filter(log => log.entity_id === taskId);
}

// Task.logs is cached subset (recent N entries)
function cacheRecentLogs(taskId, maxEntries = 10) {
  const allLogs = getTaskLogs(taskId);
  task.logs = allLogs.slice(0, maxEntries);
  saveTask(task);
}
```

**Advantages:**
- Single source of truth (CHANGELOG)
- No synchronization complexity
- Task.logs is just a cache

**Disadvantages:**
- Reading task logs requires CHANGELOG scan
- Slower for frequent log queries

### Recommendation

- Use **Option 1 (Dual-Write)** for small/medium systems (<10k tasks)
- Use **Option 2 (CHANGELOG as Source)** for large systems with efficient log storage (database)

---

## Complete Action Vocabulary (Normative)

All valid action keywords across the TLC system:

### Task CRUD Actions

- **CREATED**: Task created
- **UPDATED**: Task metadata modified
- **DELETED**: Task removed (rare, prefer SKIPPED)

### Task Status Actions

- **CLAIMED**: Task ownership acquired (TODO → IN_PROGRESS)
- **RELEASED**: Task ownership released (IN_PROGRESS → TODO)
- **REASSIGNED**: Task ownership transferred to another agent
- **DONE**: Task completed successfully
- **SKIPPED**: Task intentionally abandoned

### Task Execution Actions

- **EXEC_START**: Task execution started by runner
- **EXEC_END**: Task execution completed by runner
- **EXEC_ATTEMPT**: Single execution attempt (for retry tracking)

### Collaboration Actions

- **COMMENT**: Informational message, no state change
- **BLOCKED**: Work paused due to unmet condition
- **SPLIT**: Task broken into subtasks
- **MERGED**: Multiple tasks combined
- **MIGRATED**: Task moved to external system

### Failure and Retry Actions

- **FAILURE**: Blocking issue encountered
- **RETRY**: Action retried with changes

### Flow Orchestration Actions

- **FLOW_START**: Flow execution started
- **FLOW_END**: Flow execution completed
- **STEP_START**: Flow step started
- **STEP_END**: Flow step completed
- **BRANCH_EVAL**: Branch condition evaluated and path selected

### System Actions

- **SYSTEM_START**: System/agent process started
- **SYSTEM_STOP**: System/agent process stopped
- **LEASE_ACQUIRED**: Lease granted for task
- **LEASE_RENEWED**: Lease extended
- **LEASE_EXPIRED**: Lease expired without renewal
- **LEASE_RELEASED**: Lease explicitly released

### External System Sync Actions

- **SYNC_IMPORTED**: Task imported from external system
- **SYNC_PULLED**: Task updated from origin system (poll)
- **SYNC_PUSHED**: Local changes pushed to origin system
- **SYNC_CONFLICT**: Sync conflict detected requiring resolution
- **SYNC_ERROR**: Sync operation failed

---

## Log Querying (Normative)

Implementations SHOULD provide efficient log querying:

### Query By Entity

```bash
# All logs for task T-0042
grep '"entity_id":"T-0042"' CHANGELOG

# All logs by agent "codex"
grep '"by":"codex"' CHANGELOG

# All FAILURE actions
grep '"action":"FAILURE"' CHANGELOG
```

### Query By Time Range

```javascript
function getLogsByTimeRange(startISO, endISO) {
  const logs = readAllLogs();
  return logs.filter(log =>
    log.timestamp >= startISO &&
    log.timestamp <= endISO
  );
}
```

### Query Recent N Logs

```javascript
function getRecentLogs(n) {
  const logs = readAllLogs();
  return logs.slice(0, n); // CHANGELOG is prepended, so first N are most recent
}
```

### Indexed Queries (Database)

If using SQLite for logs:

```sql
-- All logs for task
SELECT * FROM logs WHERE entity_id = 'T-0042' ORDER BY timestamp DESC;

-- Recent failures
SELECT * FROM logs WHERE action = 'FAILURE' ORDER BY timestamp DESC LIMIT 10;

-- Logs by agent in time range
SELECT * FROM logs
WHERE by = 'codex'
  AND timestamp BETWEEN '2025-01-01T00:00:00Z' AND '2025-01-31T23:59:59Z'
ORDER BY timestamp DESC;
```

---

## Log Compaction (Optional)

For long-running systems, old logs may be compacted:

### Compaction Policy

- Keep all logs for active tasks (TODO, IN_PROGRESS)
- For completed tasks (DONE, SKIPPED):
  - Keep last 30 days of logs in full
  - After 30 days, keep summary only:
    - CREATED entry
    - CLAIMED entry
    - DONE/SKIPPED entry
  - Discard intermediate COMMENT, UPDATED, etc.

### Compaction Example

Before compaction (task completed 60 days ago):
```json
{"timestamp":"2024-11-15T10:00:00Z","entity_type":"task","entity_id":"T-0001","action":"DONE","note":"..."}
{"timestamp":"2024-11-15T09:45:00Z","entity_type":"task","entity_id":"T-0001","action":"COMMENT","note":"..."}
{"timestamp":"2024-11-15T09:30:00Z","entity_type":"task","entity_id":"T-0001","action":"COMMENT","note":"..."}
{"timestamp":"2024-11-15T09:00:00Z","entity_type":"task","entity_id":"T-0001","action":"CLAIMED","note":"..."}
{"timestamp":"2024-11-15T08:00:00Z","entity_type":"task","entity_id":"T-0001","action":"CREATED","note":"..."}
```

After compaction:
```json
{"timestamp":"2024-11-15T10:00:00Z","entity_type":"task","entity_id":"T-0001","action":"DONE","note":"..."}
{"timestamp":"2024-11-15T09:00:00Z","entity_type":"task","entity_id":"T-0001","action":"CLAIMED","note":"..."}
{"timestamp":"2024-11-15T08:00:00Z","entity_type":"task","entity_id":"T-0001","action":"CREATED","note":"..."}
```

---

## Examples

### Example 1: Multi-Entity Log Stream

```jsonl
{"timestamp":"2025-01-15T12:05:00Z","entity_type":"step","entity_id":"deploy","flow_id":"flow:ci:1.0","run_id":"run:abc","by":"system","action":"STEP_END","note":"Deployment succeeded"}
{"timestamp":"2025-01-15T12:04:30Z","entity_type":"step","entity_id":"deploy","flow_id":"flow:ci:1.0","run_id":"run:abc","by":"system","action":"STEP_START","note":"Starting deployment"}
{"timestamp":"2025-01-15T12:04:00Z","entity_type":"task","entity_id":"T-0042","by":"codex","action":"DONE","note":"Build completed"}
{"timestamp":"2025-01-15T12:00:00Z","entity_type":"flow","entity_id":"flow:ci:1.0","run_id":"run:abc","by":"system","action":"FLOW_START","note":"Starting CI pipeline"}
{"timestamp":"2025-01-15T11:55:00Z","entity_type":"task","entity_id":"T-0042","by":"codex","action":"CLAIMED","note":"Starting build"}
```

### Example 2: Lease Tracking in Logs

```jsonl
{"timestamp":"2025-01-15T10:10:00Z","entity_type":"task","entity_id":"T-0050","by":"system","action":"LEASE_EXPIRED","note":"Lease expired for agent codex-1"}
{"timestamp":"2025-01-15T10:05:00Z","entity_type":"task","entity_id":"T-0050","by":"codex-1","action":"LEASE_RENEWED","note":"Lease extended for 5 minutes"}
{"timestamp":"2025-01-15T10:00:00Z","entity_type":"task","entity_id":"T-0050","by":"codex-1","action":"LEASE_ACQUIRED","note":"Lease acquired for 5 minutes","meta":{"expires_at":"2025-01-15T10:05:00Z"}}
{"timestamp":"2025-01-15T10:00:00Z","entity_type":"task","entity_id":"T-0050","by":"codex-1","action":"CLAIMED","note":"Starting work"}
```

### Example 3: Delegation Thread

```jsonl
{"timestamp":"2025-01-15T14:30:00Z","entity_type":"task","entity_id":"T-0060","by":"claude","action":"COMMENT","note":"Review complete. See T-0061 for findings.","meta":{"in_response_to":"2025-01-15T11:00:00Z"}}
{"timestamp":"2025-01-15T11:00:00Z","entity_type":"task","entity_id":"T-0060","by":"codex","action":"COMMENT","note":"@claude please review auth implementation","meta":{"delegation":{"delegate":"claude","task_type":"review"}}}
{"timestamp":"2025-01-15T10:30:00Z","entity_type":"task","entity_id":"T-0060","by":"codex","action":"CLAIMED","note":"Implementing auth"}
```

### Example 4: External System Sync Lifecycle

```jsonl
{"timestamp":"2025-01-15T16:10:00Z","entity_type":"task","entity_id":"T-0070","by":"sync-agent","action":"SYNC_PUSHED","note":"Pushed status update to GitHub issue #456","meta":{"origin_system":"github","origin_id":"456","changes":["status"]}}
{"timestamp":"2025-01-15T16:05:00Z","entity_type":"task","entity_id":"T-0070","by":"codex","action":"DONE","note":"Fixed authentication bug"}
{"timestamp":"2025-01-15T15:30:00Z","entity_type":"task","entity_id":"T-0070","by":"codex","action":"CLAIMED","note":"Starting work on GitHub issue"}
{"timestamp":"2025-01-15T15:25:00Z","entity_type":"task","entity_id":"T-0070","by":"sync-agent","action":"SYNC_PULLED","note":"Updated title and description from GitHub","meta":{"origin_system":"github","origin_id":"456","changes":["title","description"]}}
{"timestamp":"2025-01-15T15:00:00Z","entity_type":"task","entity_id":"T-0070","by":"sync-agent","action":"SYNC_IMPORTED","note":"Imported from GitHub issue #456","meta":{"origin_system":"github","origin_id":"456","origin_url":"https://github.com/org/repo/issues/456","sync_direction":"bidirectional"}}
```

### Example 5: Sync Conflict Resolution

```jsonl
{"timestamp":"2025-01-15T17:20:00Z","entity_type":"task","entity_id":"T-0080","by":"human","action":"COMMENT","note":"Resolved conflict: keeping local status, updated description from GitHub"}
{"timestamp":"2025-01-15T17:15:00Z","entity_type":"task","entity_id":"T-0080","by":"sync-agent","action":"SYNC_CONFLICT","note":"Conflict detected: task modified locally and in GitHub","meta":{"origin_system":"github","local_updated_at":"2025-01-15T17:10:00Z","remote_updated_at":"2025-01-15T17:12:00Z","conflicting_fields":["status","description"]}}
{"timestamp":"2025-01-15T17:10:00Z","entity_type":"task","entity_id":"T-0080","by":"codex","action":"DONE","note":"Completed task locally"}
{"timestamp":"2025-01-15T16:00:00Z","entity_type":"task","entity_id":"T-0080","by":"sync-agent","action":"SYNC_IMPORTED","note":"Imported from GitHub issue #789"}
```

