# Glossary v0.1

## Summary

This glossary defines canonical terms used across all TLC specs.

## Canonical Terms

- **Artifact**
  - Any durable unit of work or decision record (e.g. task, spec, user story, E2E test plan).

- **Task**
  - A durable work unit that can be persisted, assigned, executed, and closed.

- **Task ID**
  - A stable identifier for a task that does not change during the task lifetime.

- **Spec**
  - A normative document defining expected system behavior and rules.

- **User Story**
  - A functional requirement described from the user's perspective, with testable acceptance criteria.

- **Acceptance Criteria**
  - Concrete, testable conditions that must be satisfied for a story/task to be considered complete.

- **E2E Test**
  - An end-to-end validation of user-visible behavior across multiple steps/components.

- **Flow**
  - An orchestration graph composed of steps that reference tasks or control logic. Flows define the structure and control flow but do not directly own task execution semantics.

- **Flow Definition**
  - A declarative JSON specification defining the structure of a flow, including entry_step, steps map, and metadata.

- **Flow Instance / Flow Run**
  - A single execution of a flow definition with a unique run_id. Multiple runs of the same flow definition are possible.

- **Step**
  - A node inside a flow. Steps can represent a task execution (`task` step), control flow construct (`parallel`, `branch`, `join`, `retry`), or nested flow execution (`subflow`).

- **Step Type**
  - The kind of step: `task`, `parallel`, `branch`, `join`, `retry`, or `subflow`.

- **entry_step**
  - The step_id where flow execution begins. Every flow MUST have exactly one entry_step.

- **task_ref**
  - A reference from a flow step to a task in the task store. Format: `task:<task_id>` or task ID directly.

- **Run**
  - A single execution instance of a task or flow. For flows, synonymous with "flow run" or "flow instance".

- **run_id**
  - A correlation identifier for a specific flow run, used for traceability in logs and outputs. Format: `run:<unique_id>`.

- **Attempt**
  - A single execution attempt of a task or step. Retry creates multiple attempts of the same step/task.

- **Actor**
  - The entity performing actions (human, agent, runner, AI model wrapper).

- **Agent**
  - A type of actor that can claim tasks, execute tasks, or collaborate via requests.

- **Runner**
  - The runtime component that executes tasks and produces execution results.

- **Claim**
  - A collaboration action where an agent becomes responsible for a task.

- **Reference**
  - A justification pointer or provenance pointer for why a task exists.

- **Dependency**
  - A relationship where a task/step must not start until another completes successfully. In flows, expressed via `depends_on` field.

- **depends_on**
  - An array of step IDs that must reach `succeeded` status before the dependent step can start.

- **Sequential Execution**
  - Steps that execute one after another based on dependency order. Not a step type, but a pattern achieved via `depends_on`.

- **Parallel Execution**
  - Multiple steps executing concurrently without enforced ordering. Implemented via `parallel` step type.

- **max_concurrency**
  - Maximum number of child steps that may be in `running` state simultaneously within a `parallel` step.

- **Determinism**
  - The guarantee that the same inputs produce the same structural decisions and outcomes. Flow execution order is deterministic where specified by dependencies.

- **Idempotency**
  - The property that repeating the same operation produces the same effective outcome without harm. Important for retry safety.

- **Join / Barrier**
  - A synchronization point that waits for multiple upstream steps to complete before allowing downstream progress. Implemented via `join` step type with `wait_for` field.

- **wait_for**
  - An array of step IDs that a `join` step must wait for before succeeding. All steps in wait_for must reach terminal success state.

- **Branch**
  - A conditional decision point that selects exactly one downstream execution path based on condition evaluation. Implemented via `branch` step type.

- **Condition / when**
  - An expression evaluated by a `branch` step to select which path to execute. Format: `<variable> == "<value>"`.

- **default_next**
  - The step to execute if no branch cases match. If omitted and no cases match, branch fails.

- **Retry (Execution-level)**
  - Retrying a task attempt within the runner/executor. Owned by task-exec-spec-0.1.md.

- **Retry (Orchestration-level)**
  - Retrying a step or subflow as a control mechanism in the flow engine. Implemented via `retry` step type.

- **Retry Policy**
  - Configuration for retry step including `max_attempts` and `backoff_ms`.

- **Subflow**
  - A nested flow executed as a step within a parent flow. Implemented via `subflow` step type with `flow_ref`.

- **flow_ref**
  - Reference to another flow definition to execute as a subflow. Format: `flow:<name>:<version>`.

- **Lease / TTL**
  - A time-bounded ownership lock that prevents tasks from being stuck due to dead owners. Optional but recommended for multi-agent systems.

- **Lease Expiration**
  - When a lease TTL expires without renewal, allowing another agent to claim the task.

## Shared State Terms

### Task Status (CRUD-level)

Durable task state stored in task persistence (UPPERCASE):

- **TODO** - Task exists and is available to be claimed
- **IN_PROGRESS** - Task is actively being worked on by an assigned owner
- **DONE** - Task completed successfully (terminal, immutable)
- **SKIPPED** - Task intentionally abandoned (terminal, immutable)

### Step Status (FLOW-level)

Transient state of a step within a flow run (lowercase):

- **pending** - Step exists but dependencies not yet satisfied
- **queued** - Dependencies satisfied, waiting to start execution
- **running** - Step is actively executing
- **succeeded** - Step completed successfully (terminal)
- **failed** - Step encountered an error (terminal)
- **canceled** - Step was explicitly canceled (terminal)
- **skipped** - Step was not executed due to branching or control flow (terminal)

### Flow Status (FLOW-level)

Transient state of a flow run (lowercase):

- **queued** - Flow instantiated, waiting to start
- **running** - Flow execution in progress
- **succeeded** - All required steps succeeded (terminal)
- **failed** - One or more required steps failed (terminal)
- **canceled** - Flow execution was explicitly canceled (terminal)

### Execution Status (EXEC-level - DEPRECATED)

Note: These statuses were originally defined for task execution protocol but are superseded by:
- Task statuses (CRUD-level) for durable state
- Step statuses (FLOW-level) for orchestration state

For backward compatibility reference:
- QUEUED → Use `pending` or `queued` at step level
- RUNNING → Use `IN_PROGRESS` at task level, `running` at step level
- SUCCEEDED → Use `DONE` at task level, `succeeded` at step level
- FAILED → Use FAILURE log action + status revert at task level, `failed` at step level
- CANCELED → Use `canceled` at step level

---

## Action Vocabulary

Complete list of valid log action keywords across TLC system (34 actions total):

### Task CRUD Actions
- **CREATED** - Task created
- **UPDATED** - Task metadata modified
- **DELETED** - Task removed (rare, prefer SKIPPED)

### Task Status Actions
- **CLAIMED** - Task ownership acquired (TODO → IN_PROGRESS)
- **RELEASED** - Task ownership released (IN_PROGRESS → TODO)
- **REASSIGNED** - Task ownership transferred to another agent
- **DONE** - Task completed successfully
- **SKIPPED** - Task intentionally abandoned

### Task Execution Actions
- **EXEC_START** - Task execution started by runner
- **EXEC_END** - Task execution completed by runner
- **EXEC_ATTEMPT** - Single execution attempt (for retry tracking)

### Collaboration Actions
- **COMMENT** - Informational message, no state change
- **BLOCKED** - Work paused due to unmet condition
- **SPLIT** - Task broken into subtasks
- **MERGED** - Multiple tasks combined
- **MIGRATED** - Task moved to external system

### Failure and Retry Actions
- **FAILURE** - Blocking issue encountered
- **RETRY** - Action retried with changes

### Flow Orchestration Actions
- **FLOW_START** - Flow execution started
- **FLOW_END** - Flow execution completed
- **STEP_START** - Flow step started
- **STEP_END** - Flow step completed
- **BRANCH_EVAL** - Branch condition evaluated and path selected

### System Actions
- **SYSTEM_START** - System/agent process started
- **SYSTEM_STOP** - System/agent process stopped
- **LEASE_ACQUIRED** - Lease granted for task
- **LEASE_RENEWED** - Lease extended
- **LEASE_EXPIRED** - Lease expired without renewal
- **LEASE_RELEASED** - Lease explicitly released

### External System Sync Actions
- **SYNC_IMPORTED** - Task imported from external system
- **SYNC_PULLED** - Task updated from origin system (poll)
- **SYNC_PUSHED** - Local changes pushed to origin system
- **SYNC_CONFLICT** - Sync conflict detected requiring resolution
- **SYNC_ERROR** - Sync operation failed

---

## Storage and Persistence Terms

- **CHANGELOG**
  - The canonical log file storing all system events in reverse chronological order (newest first). Format: JSON Lines (JSONL).

- **TODO File**
  - Optional human-readable file containing tasks in Task Line format (see task-line-spec-0.1.md).

- **Task Store**
  - The persistence layer for task entities. May be file-per-task, JSON array, SQLite database, or hybrid.

- **Flow Registry**
  - Storage location for flow definitions, referenced by flow_ref in subflow steps.

- **Prepend**
  - Write policy for CHANGELOG where new log entries are added to the beginning of the file.

- **Log Rotation**
  - Process of archiving large log files and starting fresh log file to maintain performance.

---

## Query and Filter Terms

- **Query Operator**
  - Comparison or filter operation: equality (`==`), inequality (`!=`), contains, in, null checks, date ranges.

- **Pagination**
  - Mechanism to retrieve large result sets in chunks: offset/limit or cursor-based.

- **Bulk Operation**
  - Operation affecting multiple tasks/entities atomically or in batch.

---

## Collaboration Terms

- **Delegation**
  - Owner requests work output from another actor without transferring ownership. Does NOT change assigned_to.

- **Transfer / Handoff**
  - Ownership moves to another actor via REASSIGNED action. DOES change assigned_to.

- **Lease Renewal**
  - Active agent extends lease expiration before TTL expires. Typically done at TTL/2 interval.

- **Lease Takeover**
  - Another agent claims a task whose lease has expired, preserving audit trail.

---

## External System Sync Terms

- **Internal Task**
  - Task created directly via TLC CLI. Never synced to external systems. Used for agent-managed internal work with full traceability.

- **External Synced Task**
  - Task that originated from an external system (GitHub, Jira, Linear, etc.). Kept in sync with origin system via polling and push-back.

- **Origin System**
  - The external system where a synced task originated. TLC maintains sync with this single source of truth.

- **origin_system**
  - Meta field identifying the external system (e.g., "github", "jira", "linear"). Absence indicates internal task.

- **origin_id**
  - Task identifier in the origin system (e.g., GitHub issue number).

- **origin_url**
  - Direct URL to task in origin system for human access.

- **last_sync_at**
  - ISO8601 timestamp of last successful sync with origin system.

- **sync_direction**
  - Sync policy: "pull" (read-only), "push" (write-back), or "bidirectional" (both).

- **Sync Polling**
  - TLC regularly probes integrated systems for new/updated tasks. Default interval: 5 minutes (until webhook support).

- **Sync Push-Back**
  - Local changes to synced tasks are pushed back to origin system based on sync_direction.

- **Sync Role**
  - TLC is NOT a sync engine between systems. It only maintains sync with each task's origin system, not across systems.

---

## Notes

- **Status Capitalization Convention:**
  - Task statuses are UPPERCASE to reflect durable CRUD state stored in persistence
  - Step/Flow statuses are lowercase to reflect transient orchestration run state
  - This convention helps distinguish between persistent task state and ephemeral execution state

- **Action Capitalization Convention:**
  - All log actions are UPPERCASE for consistency and visibility in logs

- **Format Conventions:**
  - `flow:<name>:<version>` - Flow ID format
  - `task:<task_id>` - Task reference format (or task ID directly)
  - `run:<unique_id>` - Flow run ID format
  - `T-0001` - Task ID format (recommended pattern)

- **Retry Levels:**
  - Execution-level retry is owned by runners (task-exec-spec-0.1.md)
  - Orchestration-level retry is owned by flow engine (task-flow-spec-0.1.md)
  - Both levels can coexist but must be clearly distinguished to avoid confusion