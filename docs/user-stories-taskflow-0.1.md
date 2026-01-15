# User Stories — Task Flow (TFS) v0.1

## Version

- Version: 0.1
- Generated at: 2026-01-15T15:39:49Z

## Purpose

These user stories define end-to-end behavior requirements for Task Flow execution.
They are written to be:
- executable (can be validated by E2E tests)
- deterministic (repeatable outcomes)
- agent-safe (minimize ambiguous interpretation)
- aligned with the Flow + Exec + Log specs

## Scope

This document focuses on:
- flow graph semantics
- step ordering
- branching rules
- parallel execution rules
- join behavior
- orchestration-level retry behavior
- logging/auditing expectations

Out of scope:
- claiming/ownership policies (task-collab)
- persistence details (task-crud)
- runner internals (task-exec beyond required outputs)

## Glossary Anchors (Normative)

- "Flow" is an orchestration graph of steps.
- "Step" is a node in a flow.
- "Run" is one attempt of a flow execution.
- "Attempt" is one execution attempt of a step (especially under retry).
- "Determinism" means the same inputs yield the same step ordering and final outcome.
- "Terminal states" are: succeeded/failed/canceled/skipped.

---

## Epic A — Sequential Execution (Deterministic Ordering)

### Story A1 — Execute a simple sequential flow

- As a user
- I want to define a flow of task steps A -> B -> C
- So that tasks run strictly in order and only progress on success

#### Acceptance Criteria

- The flow has a single `entry_step` that maps to step A
- Step A MUST start before step B
- Step B MUST NOT start until step A reaches terminal success state
- Step C MUST NOT start until step B reaches terminal success state
- If all steps succeed, the flow MUST succeed
- The flow log MUST include:
  - FLOW_START
  - STEP_START/STEP_END for A, B, C (in ordering consistency)
  - FLOW_END

#### Notes

- This is the baseline determinism contract.

---

### Story A2 — Dependency gates block downstream steps

- As a user
- I want a step to depend on multiple prerequisites
- So that execution is blocked until all prerequisites succeed

#### Acceptance Criteria

Given steps:
- A and B are independent
- C depends_on [A, B]

Then:
- C MUST NOT start until both A and B succeed
- If A fails, C MUST never start
- If B fails, C MUST never start
- The flow MUST fail if any required prerequisite fails

---

## Epic B — Parallelism, Concurrency, and Join Semantics

### Story B1 — Run independent checks in parallel

- As a user
- I want to run N independent QA steps in parallel
- So that total runtime is reduced

#### Acceptance Criteria

- The parallel step starts all child steps as soon as dependencies are satisfied
- The parallel container step succeeds only if all children succeed
- The flow log MUST show all STEP_START events for children without requiring ordering guarantees
- The flow MUST preserve deterministic output ordering by `step_id` when summarizing results

#### Determinism Clause

Parallel start timing is inherently nondeterministic.
However, the system MUST produce a stable ordering for:
- step result presentation
- aggregated outputs (if present)
using step_id sorting.

---

### Story B2 — Concurrency limits are respected

- As a user
- I want to cap the number of concurrently running child steps
- So that resource usage remains bounded

#### Acceptance Criteria

Given:
- parallel(children=[s1..s10], max_concurrency=3)

Then:
- at most 3 child steps may be RUNNING simultaneously
- remaining steps stay PENDING until a slot opens
- flow eventually completes if all children can complete

#### Observability

- Logs MUST make it possible to infer concurrency compliance
  (e.g. timestamps, runner events, or explicit concurrency slot logs).

---

### Story B3 — Join waits for multiple branches

- As a user
- I want to join multiple upstream paths
- So that downstream steps start only once all required upstream steps succeed

#### Acceptance Criteria

- A join step with wait_for [X, Y, Z] MUST NOT succeed until all three succeeded
- If any of X/Y/Z fails, the join MUST fail unless overridden by tolerance policy (future)
- Downstream steps MUST NOT start if join fails

---

## Epic C — Branching (Conditional Paths)

### Story C1 — Branch selects the first matching case

- As a user
- I want to branch into different steps based on conditions
- So that the flow can adapt to inputs

#### Acceptance Criteria

- Branch cases MUST be evaluated in the order declared
- The first matching case MUST be selected
- Once a case is chosen, other cases MUST NOT execute
- If no cases match:
  - default_next MUST be used if provided
  - otherwise the flow MUST fail with a clear error

#### Logging

- Branch evaluation MUST emit:
  - BRANCH_EVAL with the selected path
  - and include inputs used in decision under meta (sanitized if needed)

---

### Story C2 — Branch creates a skipped path trace (audit)

- As a user
- I want unselected paths to be visible in audit logs
- So that I can understand why certain work did not run

#### Acceptance Criteria

- Steps not chosen due to branching SHOULD be marked as SKIPPED
- Logs SHOULD include SKIPPED entries with the branch reason
- This behavior MUST be consistent and documented

---

## Epic D — Retry Semantics (Orchestration-Level)

### Story D1 — Retry a failed step up to max attempts

- As a user
- I want to retry a step that fails
- So that transient errors don’t fail the flow immediately

#### Acceptance Criteria

Given:
- retry(child=S, policy.max_attempts=3)

Then:
- attempt 1: execute S
- if failed: attempt 2 executes S again
- if failed: attempt 3 executes S again
- if all attempts fail: retry step fails and flow fails

#### Logging

- Each attempt MUST be loggable and distinguishable by attempt number
- The final outcome MUST include the number of attempts performed

---

### Story D2 — Retry succeeds early and stops immediately

- As a user
- I want retries to stop once success happens
- So that we don’t waste compute

#### Acceptance Criteria

- If attempt 2 succeeds, attempt 3 MUST NOT run
- The retry step MUST succeed immediately after child success
- Downstream dependencies MUST unblock as soon as retry is succeeded

---

### Story D3 — Retry backoff is respected

- As a user
- I want a backoff delay between retry attempts
- So that repeated failures do not overload dependencies

#### Acceptance Criteria

Given:
- backoff_ms = 1000

Then:
- Attempt N+1 MUST NOT begin earlier than 1000ms after attempt N ends (failed)
- Logging MUST make the delay verifiable via timestamps

---

## Epic E — Failure Handling and Safe Stops

### Story E1 — A failing required step fails the flow

- As a user
- I want the flow to fail fast when a required step fails
- So that I get immediate feedback

#### Acceptance Criteria

- If a required step fails, flow MUST end in failed state
- No downstream steps may start after failure is declared
- The error must be surfaced at the flow level with a cause chain

---

### Story E2 — Canceling a flow cancels all pending/running steps

- As a user
- I want to cancel a flow run
- So that I can stop work safely

#### Acceptance Criteria

- Cancel transitions flow to canceled
- Pending steps become canceled
- Running steps are instructed to stop (best effort)
- Logs MUST show cancellation event propagation

---

## Epic F — Idempotency and Re-runs

### Story F1 — Re-running a flow with the same inputs is stable

- As a user
- I want reruns to behave consistently
- So that results are reproducible and debuggable

#### Acceptance Criteria

- With same inputs and same code version:
  - the same steps run in the same structural order
  - branch selections are identical
- Output ordering MUST remain stable even for parallel blocks

---

### Story F2 — A rerun produces a distinct run identifier

- As a user
- I want each run to be separately observable
- So that I can compare attempts and outcomes

#### Acceptance Criteria

- Each run emits a unique run_id (implementation-defined)
- Logs contain run_id or an equivalent correlation identifier
- A rerun MUST NOT overwrite previous run artifacts

