# Task Collaboration Specification (TCS-COLLAB) v1.0

## Version

- Version: 1.0

## Summary

Task Collaboration Spec defines how multiple actors (humans and AI agents) safely coordinate work on shared tasks.

This spec OWNS:
- actor identity and accountability
- claiming / releasing / reassigning responsibilities
- delegation vs responsibility transfer
- collaboration-safe invariants
- audit and logging requirements for collaboration actions

This spec DOES NOT own:
- canonical task schema fields (see task-crud-spec-0.1.md)
- execution protocol and runner behavior (see task-exec-spec-0.1.md)
- flow orchestration semantics (see task-flow-spec-0.1.md)

This spec references:
- task-log-spec-0.1.md (log schema + ordering)
- glossary-0.1.md (terms)

---

## Actor Identity (Normative)

All actors MUST self-identify using:

- AGENT_ID (string)

This identity MUST be included in collaboration logs.

---

## Ownership vs Delegation vs Transfer

- Ownership
  - One actor is accountable.
  - Reflected by assigned_to (CRUD-level).

- Delegation
  - Owner requests work output from another actor.
  - Does NOT change assigned_to.

- Transfer
  - Ownership moves to another actor.
  - DOES change assigned_to.

---

## Ownership Invariants (Normative)

- A task may have at most one owner at a time.
- Only the current owner may transfer ownership.
- DONE and SKIPPED tasks cannot be claimed or mutated (except COMMENT logs).

---

## Collaboration Action Vocabulary

### Enforceable Actions

- CREATED
- CLAIMED
- RELEASED
- REASSIGNED
- UPDATED
- DONE
- SKIPPED
- MIGRATED
- FAILURE
- RETRY

### Informative Actions

- COMMENT
- BLOCKED
- SPLIT
- MERGED

---

## Claim / Release / Reassign Rules

### Claim

Preconditions:
- Task status MUST be TODO

Effects:
- Emit CLAIMED log
- Transition task to IN_PROGRESS (CRUD-level)
- Set assigned_to = AGENT_ID

### Release

Preconditions:
- Task status MUST be IN_PROGRESS
- Only current owner may release

Effects:
- Emit RELEASED log
- Transition status to TODO
- assigned_to becomes null

### Reassign (Transfer)

Preconditions:
- Task status MUST be IN_PROGRESS
- Only current owner may reassign

Required log format:
- action: REASSIGNED
- note includes:
  - from: <old AGENT_ID>
  - to: <new AGENT_ID>
  - reason: <short justification>

Effects:
- assigned_to updated to new owner

---

## Delegation Protocol (Normative)

Before requesting help, owner MUST:
- emit COMMENT with:
  - what is needed
  - expected output format
  - where results should go

Delegate MUST:
- create a new task referencing the COMMENT
  OR
- respond with a COMMENT including output + references

---

## Failure & Retry Discipline

### FAILURE

FAILURE logs MUST include:
- what failed
- what was attempted
- next action suggestion

### RETRY

RETRY logs MUST include:
- what changed (hypothesis/input/approach)

---

## Lease / TTL (Recommended)

To prevent dead ownership (agent crash), systems SHOULD implement leases.

### Lease Mechanism

A lease grants temporary ownership that automatically expires unless renewed.

#### Lease Parameters

- **Default TTL**: 300 seconds (5 minutes) RECOMMENDED
- **Minimum TTL**: 60 seconds
- **Maximum TTL**: 3600 seconds (1 hour)
- **Renewal Interval**: TTL / 2 (e.g., renew every 2.5 minutes for 5-minute lease)

#### Lease Lifecycle

1. **Acquisition**: When agent CLAIMS a task:
   - Task assigned_to = AGENT_ID
   - Lease granted with TTL
   - Lease expiration timestamp recorded

2. **Renewal**: Active agent periodically renews lease:
   - Renewal MUST occur before expiration
   - Renewal extends expiration by TTL
   - Renewal SHOULD be automatic (heartbeat)
   - Renewal MAY emit COMMENT log (optional for audit)

3. **Expiration**: If lease expires without renewal:
   - Task becomes available for claiming
   - Original owner loses authority
   - System MAY emit COMMENT noting expiration
   - Another agent MAY claim task

4. **Release**: When agent RELEASES task explicitly:
   - Lease is revoked immediately
   - Task returns to TODO state
   - No expiration needed

#### Implementation Options

##### Option 1: File-Based Leases

```
./tasks/T-0001.json
./leases/T-0001.lease
```

Lease file content:
```json
{
  "task_id": "T-0001",
  "owner": "codex",
  "acquired_at": "2025-01-15T10:00:00Z",
  "expires_at": "2025-01-15T10:05:00Z",
  "renewed_at": "2025-01-15T10:02:30Z"
}
```

**Rules:**
- Lease file created on CLAIMED
- Lease file updated on renewal
- Lease file deleted on RELEASED
- Stale lease files (expires_at < now) are invalid
- Use file modification time as additional check

##### Option 2: Database Leases

```sql
CREATE TABLE leases (
  task_id TEXT PRIMARY KEY,
  owner TEXT NOT NULL,
  acquired_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  renewed_at TEXT,
  FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE INDEX idx_leases_expires_at ON leases(expires_at);
```

**Rules:**
- Insert row on CLAIMED
- Update expires_at and renewed_at on renewal
- Delete row on RELEASED
- Query expired leases: `WHERE expires_at < current_timestamp`

##### Option 3: In-Memory Leases (Single Process Only)

```javascript
const leases = new Map(); // task_id -> { owner, expiresAt }

function renewLease(taskId, owner, ttl) {
  leases.set(taskId, {
    owner,
    expiresAt: Date.now() + ttl * 1000
  });
}

function isLeaseValid(taskId, owner) {
  const lease = leases.get(taskId);
  return lease &&
         lease.owner === owner &&
         lease.expiresAt > Date.now();
}
```

**Limitations:**
- Only works for single-process systems
- Leases lost on process restart
- Not suitable for multi-agent systems

#### Lease Enforcement

Before allowing an operation, systems SHOULD check:

```
function canPerformOperation(taskId, agentId, operation) {
  // Check task ownership
  if (task.assigned_to !== agentId) {
    return { allowed: false, reason: "Not task owner" };
  }

  // Check lease validity (if leases enabled)
  if (leasesEnabled) {
    const lease = getLease(taskId);
    if (!lease || lease.owner !== agentId) {
      return { allowed: false, reason: "No valid lease" };
    }
    if (lease.expires_at < now()) {
      return { allowed: false, reason: "Lease expired" };
    }
  }

  return { allowed: true };
}
```

#### Lease Expiration Handling

When a lease expires:

1. **Detect**: System identifies expired lease
   - Periodic sweep of lease table
   - Check on attempted operation
   - Background cleanup job

2. **Mark**: Task state updated
   - assigned_to remains unchanged (for audit)
   - Lease record deleted or marked invalid
   - Optional: Add COMMENT noting expiration

3. **Allow Re-claim**: Another agent MAY claim
   - New CLAIMED action allowed
   - MUST emit COMMENT: "Taking over from <old_owner> (lease expired)"
   - Preserves audit trail

#### Deadlock Prevention

To prevent stuck tasks during critical operations:

- **Grace Period**: Allow operation to complete if lease expires mid-execution
  - Operations in progress get 30-second grace period
  - Grace period starts at lease expiration
  - After grace period, operation should be aborted

- **Automatic Rollback**: If operation not completed after grace period
  - Task status rolled back to previous state
  - Partial changes discarded
  - COMMENT log added explaining rollback

- **Idempotency**: Operations SHOULD be idempotent
  - Safe to retry if lease expires mid-execution
  - No duplicate side effects

#### Example: Lease Renewal Heartbeat

```javascript
class LeaseRenewer {
  constructor(taskId, agentId, ttl) {
    this.taskId = taskId;
    this.agentId = agentId;
    this.ttl = ttl;
    this.renewalInterval = (ttl / 2) * 1000; // milliseconds
    this.timer = null;
  }

  start() {
    this.timer = setInterval(() => {
      this.renew();
    }, this.renewalInterval);
  }

  stop() {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
  }

  async renew() {
    try {
      await renewLease(this.taskId, this.agentId, this.ttl);
    } catch (error) {
      console.error(`Failed to renew lease for ${this.taskId}:`, error);
      // Optionally: release task and stop work
    }
  }
}

// Usage:
const task = claimTask("T-0001", "codex");
const renewer = new LeaseRenewer("T-0001", "codex", 300);
renewer.start();

// Do work...

renewer.stop();
releaseTask("T-0001", "codex");
```

---

## Delegation Protocol Details

### Delegation Tracking

When owner delegates work, the delegation SHOULD be trackable:

#### Delegation Log Format

```json
{
  "timestamp": "2025-01-15T11:00:00Z",
  "entity_type": "task",
  "entity_id": "T-0010",
  "by": "codex",
  "action": "COMMENT",
  "note": "Requesting review from @claude. Please check auth implementation in src/auth.ts and provide feedback.",
  "meta": {
    "delegation": {
      "delegate": "claude",
      "task_type": "review",
      "expected_output": "feedback on auth implementation",
      "deadline": "2025-01-15T17:00:00Z"
    }
  }
}
```

#### Delegate Response Options

**Option 1: Comment Response**

Delegate adds COMMENT with results:

```json
{
  "timestamp": "2025-01-15T14:30:00Z",
  "entity_type": "task",
  "entity_id": "T-0010",
  "by": "claude",
  "action": "COMMENT",
  "note": "Review complete. Found 2 issues: 1) Token validation missing, 2) Race condition in refresh. See docs/review/auth-feedback.md",
  "meta": {
    "in_response_to": "2025-01-15T11:00:00Z",
    "review_result": "issues_found",
    "issues_count": 2
  }
}
```

**Option 2: New Task Creation**

Delegate creates new task referencing original:

```json
{
  "id": "T-0011",
  "title": "Auth implementation review",
  "description": "Review of auth implementation for T-0010",
  "status": "DONE",
  "assigned_to": "claude",
  "reference": "T-0010",
  "logs": [
    {
      "timestamp": "2025-01-15T14:30:00Z",
      "entity_type": "task",
      "entity_id": "T-0011",
      "by": "claude",
      "action": "DONE",
      "note": "Review completed. Results in docs/review/auth-feedback.md"
    },
    {
      "timestamp": "2025-01-15T11:05:00Z",
      "entity_type": "task",
      "entity_id": "T-0011",
      "by": "claude",
      "action": "CREATED",
      "note": "Created in response to delegation from T-0010"
    }
  ]
}
```

### Delegation Timeout

If delegate does not respond within expected timeframe:

1. Original owner MAY add COMMENT noting timeout
2. Original owner MAY delegate to different agent
3. Original owner MAY complete work themselves
4. Delegation timeout does NOT block original task

Example:
```json
{
  "timestamp": "2025-01-15T18:00:00Z",
  "entity_type": "task",
  "entity_id": "T-0010",
  "by": "codex",
  "action": "COMMENT",
  "note": "Review request to @claude timed out after 7 hours. Proceeding without external review.",
  "meta": {
    "delegation_status": "timeout",
    "original_request": "2025-01-15T11:00:00Z"
  }
}
```

### Multi-Level Delegation

Delegation MAY be transitive:

```
Task T-0020 owned by human
  └─> Delegates to codex (COMMENT requesting implementation)
      └─> codex creates T-0021, assigns to self
          └─> codex delegates to claude (COMMENT requesting review)
              └─> claude creates T-0022, assigns to self
```

**Rules:**
- Each level MUST be explicitly logged
- Each delegate MAY create new tasks
- Reference chain SHOULD be preserved in task.reference or meta
- Final results MUST propagate back to original owner

### Delegation Queries

Systems SHOULD support querying:

- **Outgoing delegations**: Tasks where I requested help
  - Query: `logs contains COMMENT with meta.delegation.delegate=<agent_id>`
- **Incoming delegations**: Tasks requesting my help
  - Query: `logs contains COMMENT with meta.delegation.delegate=<my_id>`
- **Pending delegations**: Delegations without response
  - Query: Outgoing delegation without corresponding response COMMENT or new task

---

## Example Workflows

### Example 1: Simple Claim → Work → Done

```json
[
  {
    "timestamp": "2025-01-15T10:00:00Z",
    "entity_type": "task",
    "entity_id": "T-0030",
    "by": "codex",
    "action": "CLAIMED",
    "note": "Starting work on authentication bug"
  },
  {
    "timestamp": "2025-01-15T10:45:00Z",
    "entity_type": "task",
    "entity_id": "T-0030",
    "by": "codex",
    "action": "DONE",
    "note": "Fixed token refresh race condition. See commit abc123"
  }
]
```

### Example 2: Human-to-AI Delegation

Human creates task and delegates:

```json
[
  {
    "timestamp": "2025-01-15T09:00:00Z",
    "entity_type": "task",
    "entity_id": "T-0040",
    "by": "human",
    "action": "CREATED",
    "note": "Need API rate limiting implementation"
  },
  {
    "timestamp": "2025-01-15T09:01:00Z",
    "entity_type": "task",
    "entity_id": "T-0040",
    "by": "human",
    "action": "COMMENT",
    "note": "@codex Please implement rate limiting for /api/* endpoints using token bucket algorithm. Limit: 100 req/min per user.",
    "meta": {
      "delegation": {
        "delegate": "codex",
        "task_type": "implementation"
      }
    }
  }
]
```

AI responds by claiming task:

```json
[
  {
    "timestamp": "2025-01-15T09:05:00Z",
    "entity_type": "task",
    "entity_id": "T-0040",
    "by": "codex",
    "action": "CLAIMED",
    "note": "Taking ownership to implement rate limiting"
  },
  {
    "timestamp": "2025-01-15T10:30:00Z",
    "entity_type": "task",
    "entity_id": "T-0040",
    "by": "codex",
    "action": "DONE",
    "note": "Rate limiting implemented using token bucket. Added middleware in src/middleware/rate-limit.ts. Tests passing."
  }
]
```

### Example 3: AI-to-AI Task Handoff

Agent encounters blocker and transfers:

```json
[
  {
    "timestamp": "2025-01-15T11:00:00Z",
    "entity_type": "task",
    "entity_id": "T-0050",
    "by": "codex",
    "action": "CLAIMED",
    "note": "Starting database migration"
  },
  {
    "timestamp": "2025-01-15T11:30:00Z",
    "entity_type": "task",
    "entity_id": "T-0050",
    "by": "codex",
    "action": "FAILURE",
    "note": "Cannot proceed: missing production database credentials. Need security team approval."
  },
  {
    "timestamp": "2025-01-15T11:31:00Z",
    "entity_type": "task",
    "entity_id": "T-0050",
    "by": "codex",
    "action": "REASSIGNED",
    "note": "from: codex, to: human, reason: requires production database access approval"
  }
]
```

### Example 4: Lease Expiration and Recovery

Agent crashes, lease expires, another agent takes over:

```json
[
  {
    "timestamp": "2025-01-15T12:00:00Z",
    "entity_type": "task",
    "entity_id": "T-0060",
    "by": "codex-instance-1",
    "action": "CLAIMED",
    "note": "Starting deployment verification"
  },
  {
    "timestamp": "2025-01-15T12:02:00Z",
    "entity_type": "task",
    "entity_id": "T-0060",
    "by": "codex-instance-1",
    "action": "COMMENT",
    "note": "Running smoke tests..."
  },
  // codex-instance-1 crashes, lease expires after 5 minutes
  {
    "timestamp": "2025-01-15T12:10:00Z",
    "entity_type": "task",
    "entity_id": "T-0060",
    "by": "system",
    "action": "COMMENT",
    "note": "Lease expired for codex-instance-1 (no renewal for 5 minutes). Task available for claiming."
  },
  {
    "timestamp": "2025-01-15T12:11:00Z",
    "entity_type": "task",
    "entity_id": "T-0060",
    "by": "codex-instance-2",
    "action": "COMMENT",
    "note": "Taking over from codex-instance-1 (lease expired). Resuming verification from last checkpoint."
  },
  {
    "timestamp": "2025-01-15T12:11:01Z",
    "entity_type": "task",
    "entity_id": "T-0060",
    "by": "codex-instance-2",
    "action": "CLAIMED",
    "note": "Resuming deployment verification"
  },
  {
    "timestamp": "2025-01-15T12:20:00Z",
    "entity_type": "task",
    "entity_id": "T-0060",
    "by": "codex-instance-2",
    "action": "DONE",
    "note": "Deployment verified successfully. All services healthy."
  }
]
```
