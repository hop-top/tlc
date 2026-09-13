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

- **Recipe**
  - A versioned YAML template that materializes into a track and its tasks the same way every time: an ordered list of steps with dependencies, a kind per step, retries, gates and due dates, plus the variables a run binds. Recipes only create tasks; they do not own execution semantics.

- **Step**
  - One entry in a recipe's `steps` list. A step is a task step (it becomes a task), an `include` block (another recipe spliced in) or a `repeat` block (a sub-list unrolled a fixed number of times).

- **Kind**
  - What a task step is executed by: `agent` (the default; the named agent, else the recipe's, else `track execute --agent`), `exec` (the executor runs `exec.argv` itself) or `human` (a person decides it with `tlc task approve` / `tlc task reject`).

- **Materialize**
  - Create the tasks a recipe declares, rendering every placeholder and recording the result as a run. `tlc track create --recipe`, `tlc task create --recipe` and `tlc task execute --recipe` all materialize.

- **Run**
  - One materialization of a recipe: which recipe at which version and hash, with which vars, for which subject, into which track, by whom, and which task each step became. Rows live in `recipe_runs` and `recipe_run_tasks`.

- **run_id**
  - The identifier of a recipe run. Carried on every task the run created, alongside `step_id` and `step_ordinal`, so a task's provenance is readable from the task itself.

- **Reconcile**
  - Compare a track against a recipe's run ledger and create only the steps that have no ledger row, in a new run whose `parent_run` is the track's latest run of that recipe. `tlc track execute --recipe` reconciles before it runs.

- **Subject**
  - The task or track a recipe is applied to, named with `--for` and bound as `subject.*` in templates. A task subject is blocked on the run's leaves and completes automatically once they are done.

- **Result**
  - What a finished task reports for downstream `when` conditions, read as `results.<step>.<path>`. An exec task's result is `exit_code`, `stdout`, `stderr`, `duration_ms` and `truncated`; an agent task's result is the `outputs` object its results file reports.

- **Gate**
  - An eva contract evaluated on a task's output before it may reach DONE, declared per step as `gate.contract` with an optional `gate.eva_url`.

- **Ledger**
  - The run ledger: `recipe_runs` plus `recipe_run_tasks`. It, not the task table, is the record of which steps a track already has, and what `tlc recipe runs` and `tlc recipe diff` read.

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

- **Archive**
  - A secondary task state that hides tasks from default listings. Archiving is used to clean up the active workspace without deleting durable work records.

- **Auto-archiving**
  - A system process that automatically archives tasks that have been in a terminal status (DONE, SKIPPED) for longer than a configured threshold.

- **Dependency**
  - A relationship where a task must not start until another reaches a terminal status. Declared per step as `depends_on` and stored on the created task as `blocked_by`.

- **depends_on**
  - An array of step IDs a step waits for. Inside an `include` or `repeat` block, an edge on the block means "after every leaf of the block".

- **Batch**
  - One round of the executor loop: the set of tasks whose blockers are all terminal, dispatched together up to `--concurrency`. Ordering within a batch follows step order.

- **Determinism**
  - The guarantee that the same recipe and the same vars produce the same tasks, the same dependency edges and the same dispatch order.

- **Idempotency**
  - The property that repeating the same operation produces the same effective outcome without harm. Reconciliation relies on it: a step whose ledger row exists is never materialized twice.

- **Condition / when**
  - A single comparison `lhs OP rhs` (`==`, `!=`, `<`, `<=`, `>`, `>=`) evaluated at readiness. The left side reads an upstream `results.<step>.<path>` or a `vars.<name>`. False skips the task; an evaluation error blocks it.

- **Attempts**
  - The count of dispatches a task has had. `retry.max_attempts` bounds it and `retry.backoff` spaces them; once exhausted the task is blocked with the failure as its reason.

- **Include**
  - A step that splices another recipe in, binding its vars through `with`. Included steps get ids `<block>/<child>`; nesting is capped at 8 and cycles are refused.

- **Repeat / until**
  - A block unrolled a fixed number of times, stopping early once a result satisfies `until`. Iterations become `<block>/<n>/<child>`, each chained after the previous iteration's leaves.

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

A recipe step has no status of its own. The task it materialized into carries
the state, and the run ledger says which step that task came from — so a run's
progress is read by listing the tasks of its run, not by a separate status
machine.

### Blocked

Orthogonal to status: a task carries a `blocked_reason` when the executor
could not take it further — a `when` that would not evaluate, a gate that
failed, retries exhausted, or a `tlc task reject`. The status stays where it
was; a blocked task is never dispatched, and everything behind it waits until
someone unblocks or skips it.

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

### Recipe Execution Actions
- **APPROVED** - Human task approved, completing it
- **REJECTED** - Human task rejected, blocking it with the reason
- **RECLAIMED** - Claim taken over from a stale actor and re-dispatched

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

- **Recipe Search Path**
  - Where recipe files are looked up, in order: `recipe.dir` from config (relative to the project root), the project's `.tlc/recipes`, then `~/.config/tlc/recipes`. An unpinned name resolves to the first layer that has it, highest version in that layer; a pinned `name@version` is searched across every layer.

- **Run Ledger Tables**
  - `recipe_runs` holds one row per materialization; `recipe_run_tasks` maps each of its steps to the task it created.

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
  - Task statuses are UPPERCASE to reflect durable state stored in persistence
  - A step has no status of its own; the task it materialized into carries the state

- **Action Capitalization Convention:**
  - All log actions are UPPERCASE for consistency and visibility in logs

- **Format Conventions:**
  - `<name>@<version>` - Pinned recipe reference
  - `tlc://recipe/<name>` - Recipe URI (see identifiers-spec-0.1.md)
  - `task:<task_id>` - Task reference format (or task ID directly)
  - `T-0001` - Task ID format (recommended pattern)

- **Retry:**
  - Declared per step as `retry.max_attempts` and `retry.backoff`, applied by the executor and counted on the task as `attempts` (see task-exec-spec-0.1.md)