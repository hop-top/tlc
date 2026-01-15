# E2E Test Plan — Task Flow (TFS) v0.1

## Version

- Version: 0.1
- Generated at: 2026-01-15T15:39:49Z

## Purpose

This E2E test plan validates the Task Flow Spec as real system behavior.
It is written to be executable by:
- a human using a CLI runner
- an AI agent operating from the same repository
- CI automation (with minimal adaptation)

## Test Conventions (Normative)

- Each test MUST define:
  - Setup
  - Input Flow
  - Execution Steps
  - Expected Observations
  - Pass/Fail Criteria
- Each test MUST verify:
  - ordering constraints
  - final flow status
  - step statuses
  - required log events (from task-log-spec-0.1.md)

## Required Observability

The system under test MUST expose at least one of:
- a flow run summary output
- a JSON result object for the flow
- a log file (e.g. ./CHANGELOG)

At minimum, we must confirm:
- FLOW_START exists
- FLOW_END exists
- STEP_START/STEP_END exist for executed steps

---

## E2E-01 — Sequential Flow Executes Strictly in Order

### Setup

- Define 3 tasks: A, B, C
  - A: outputs "A_OK"
  - B: outputs "B_OK"
  - C: outputs "C_OK"
- Ensure each task succeeds deterministically.

### Input Flow

- Create flow F:
  - entry_step: A
  - A -> B -> C (via depends_on or seq)

### Execution

- Execute flow F once

### Expected Observations

- Step A transitions:
  - pending -> running -> succeeded
- Step B transitions only after A succeeded
- Step C transitions only after B succeeded
- Flow transitions:
  - queued -> running -> succeeded

### Log Expectations

- FLOW_START appears exactly once for this run
- STEP_START(A) precedes STEP_START(B)
- STEP_START(B) precedes STEP_START(C)
- FLOW_END indicates succeeded

### Pass Criteria

- Ordering is respected
- All steps succeeded
- Flow succeeded

---

## E2E-02 — Dependency Gate Blocks Downstream on Failure

### Setup

- Define tasks:
  - A succeeds
  - B fails deterministically with error.code=E2E_FORCED_FAIL
  - C would succeed if executed

### Input Flow

- A and B are independent
- C depends_on [A, B]

### Execution

- Execute the flow

### Expected Observations

- A succeeds
- B fails
- C never starts (remains pending or becomes skipped/canceled depending on engine)
- Flow fails

### Log Expectations

- STEP_END(B) shows failed
- No STEP_START(C) exists
- FLOW_END indicates failed with cause chain mentioning B

### Pass Criteria

- Failure blocks dependent step
- Downstream does not run
- Flow fails with correct cause

---

## E2E-03 — Parallel Block Runs Children Without Ordering Guarantees

### Setup

- Define tasks:
  - LINT succeeds
  - TYPECHECK succeeds
  - UNIT_TESTS succeeds

### Input Flow

- A parallel step with children [LINT, TYPECHECK, UNIT_TESTS]

### Execution

- Execute flow once

### Expected Observations

- All three children eventually succeed
- Parallel container succeeds
- Flow succeeds

### Log Expectations

- All three STEP_START events exist
- All three STEP_END events exist
- No requirement on ordering of starts/ends across children

### Pass Criteria

- All parallel children ran and succeeded
- Flow succeeded

---

## E2E-04 — max_concurrency Is Enforced

### Setup

- Define 6 tasks T1..T6
- Each task:
  - sleeps for ~2 seconds
  - then succeeds

### Input Flow

- parallel(children=[T1..T6], max_concurrency=2)

### Execution

- Execute flow once

### Expected Observations

- At most 2 tasks are running at any moment
- Total runtime should be approximately:
  - ceil(6/2) * 2s = ~6s (plus overhead)

### Log Expectations

- Use timestamps to infer overlapping RUNNING windows
- (Optional) explicit concurrency slot logs if implementation supports it

### Pass Criteria

- Concurrency limit enforced (no 3 overlapping RUNNING at once)

---

## E2E-05 — Join Waits for All Upstream Branches

### Setup

- Define tasks:
  - X succeeds after 1s
  - Y succeeds after 3s
  - Z succeeds after 2s
  - AFTER_JOIN succeeds

### Input Flow

- Run X, Y, Z in parallel
- join(wait_for=[X, Y, Z])
- AFTER_JOIN depends on join

### Execution

- Execute flow once

### Expected Observations

- join succeeds only after X, Y, Z are succeeded
- AFTER_JOIN starts only after join succeeds
- Flow succeeds

### Log Expectations

- STEP_START(AFTER_JOIN) timestamp must be after STEP_END(Y) (the slowest)
- Flow ends succeeded

### Pass Criteria

- Join is a barrier
- Downstream gated correctly

---

## E2E-06 — Branch Selects First Matching Case

### Setup

- Define input variable:
  - mode = "ci"
- Define tasks:
  - FAST_TESTS succeeds
  - FULL_TESTS succeeds

### Input Flow

- branch cases:
  - when: mode == "dev" -> FAST_TESTS
  - when: mode == "ci"  -> FULL_TESTS
- default_next: FAST_TESTS (should NOT trigger)

### Execution

- Execute flow once with mode=ci

### Expected Observations

- FULL_TESTS runs
- FAST_TESTS does not run
- Flow succeeds

### Log Expectations

- BRANCH_EVAL indicates selected FULL_TESTS
- STEP_START(FULL_TESTS) exists
- No STEP_START(FAST_TESTS)

### Pass Criteria

- Correct branch selection
- Unselected path not executed

---

## E2E-07 — Branch Default Path Used When No Cases Match

### Setup

- mode = "unknown"
- Define tasks:
  - DEFAULT_TASK succeeds

### Input Flow

- branch cases:
  - when: mode == "dev" -> DEV_TASK
  - when: mode == "ci"  -> CI_TASK
- default_next: DEFAULT_TASK

### Execution

- Execute flow once

### Expected Observations

- DEFAULT_TASK runs
- Flow succeeds

### Pass Criteria

- Default branch chosen
- No failure due to unmatched cases

---

## E2E-08 — Branch Fails When No Cases Match and No Default

### Setup

- mode = "unknown"

### Input Flow

- branch cases:
  - when: mode == "dev" -> DEV_TASK
- no default_next

### Execution

- Execute flow once

### Expected Observations

- Branch fails
- Flow fails
- No downstream steps run

### Pass Criteria

- Failure is explicit and auditable

---

## E2E-09 — Retry Succeeds Early and Stops

### Setup

- Define a flaky task FLKY that:
  - fails attempt 1 with error.code=E2E_FLAKY
  - succeeds attempt 2

### Input Flow

- retry(child=FLKY, max_attempts=3)

### Execution

- Execute flow once

### Expected Observations

- attempt 1 fails
- attempt 2 succeeds
- attempt 3 does not run
- Flow succeeds

### Log Expectations

- Two distinct attempts are visible
- Final summary indicates attempts=2

### Pass Criteria

- Retry stops on first success
- Flow succeeds

---

## E2E-10 — Retry Exhaustion Fails the Flow

### Setup

- Define task ALWAYS_FAIL that fails deterministically every time

### Input Flow

- retry(child=ALWAYS_FAIL, max_attempts=3)

### Execution

- Execute flow once

### Expected Observations

- 3 attempts happen
- All fail
- Flow fails

### Pass Criteria

- Exactly max_attempts attempts performed
- Flow fails with a clear cause chain

---

## E2E-11 — Cancellation Propagates Safely

### Setup

- Define 5 long-running tasks (sleep 10s each)

### Input Flow

- parallel(children=[T1..T5])

### Execution

- Start flow
- Cancel within 1-2 seconds

### Expected Observations

- Flow transitions to canceled
- Pending tasks become canceled
- Running tasks stop best-effort

### Pass Criteria

- Cancellation is visible and consistent
- No tasks continue indefinitely

---

## E2E-12 — Determinism: Rerun Produces Stable Decisions and Ordering

### Setup

- Use the same flow as E2E-06 (branch) and E2E-03 (parallel)
- Keep identical inputs for rerun

### Execution

- Run flow twice with identical inputs

### Expected Observations

- Branch chooses the same path in both runs
- Parallel results ordering in summary is stable by step_id
- Both runs complete successfully

### Pass Criteria

- Deterministic behavior under rerun constraints
- Outputs are comparable and auditable

