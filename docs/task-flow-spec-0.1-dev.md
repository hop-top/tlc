# Task Flow Specification (TFS) v0.1

## Version

- Version: 0.1
- Generated at: 2026-01-15T17:15:00Z

## Summary

Task Flow Spec (TFS) defines orchestration constructs for composing tasks into executable workflows with control flow semantics.

TFS is intended to enable declarative workflow definitions that can be executed by a flow engine.

This spec OWNS:
- flow definition schema
- step type semantics
- control flow execution rules
- data flow between steps
- flow status lifecycle

This spec DOES NOT own:
- task execution protocol (see task-exec-spec-0.1.md)
- agent collaboration rules (see task-collab-spec-1.0.md)
- task persistence (see task-crud-spec-0.1.md)
- log format details (see task-log-spec-0.1.md)

## Design Principles

- Flows MUST be deterministic: same inputs yield same execution order
- Flows MUST be auditable: every decision and state transition is logged
- Flows MUST fail safely: errors propagate predictably
- Flows MUST support composition: flows can contain subflows
- Flows MUST be declarative: execution is driven by structure, not imperative code

---

## Flow Definition Schema (Normative)

A flow is a JSON object with the following structure:

```json
{
  "flow_id": "<string>",
  "name": "<string>",
  "version": "<string>",
  "entry_step": "<step_id>",
  "steps": {
    "<step_id>": { /* step definition */ }
  }
}
```

### Flow Fields

#### Required Fields

- `flow_id` (string): Unique identifier for this flow definition
  - Format: `flow:<name>:<version>`
  - MUST be globally unique within the system
- `name` (string): Human-readable flow name
- `version` (string): Semantic version of this flow definition
- `entry_step` (string): The step_id where flow execution begins
- `steps` (object): Map of step_id to step definitions

#### Optional Fields

- `description` (string): Human-readable flow description
- `inputs` (object): Schema for flow input variables (future)
- `outputs` (object): Schema for flow output variables (future)
- `meta` (object): Arbitrary metadata

### Flow Validation Rules

A flow definition MUST be rejected if:
- `entry_step` does not exist in `steps`
- Any `depends_on` reference points to a non-existent step
- Any `children` reference points to a non-existent step
- The flow contains cycles (except via explicit retry)
- Multiple entry points are declared (only one `entry_step` allowed)

---

## Step Types (Normative)

All steps share a common base schema with type-specific extensions.

### Base Step Schema

Every step MUST include:

```json
{
  "step_id": "<string>",
  "type": "<step_type>",
  "title": "<string>"
}
```

And MAY include:

```json
{
  "depends_on": ["<step_id>", ...],
  "meta": { /* arbitrary metadata */ }
}
```

#### Base Field Definitions

- `step_id` (string, required): Unique identifier within this flow
  - MUST be unique within the flow's `steps` map
  - MUST be a valid key (no spaces, use kebab-case or underscores)
- `type` (string, required): Step type from the vocabulary below
- `title` (string, required): Human-readable step description
- `depends_on` (array of strings, optional): Step IDs that must succeed before this step starts
  - Default: `[]` (no dependencies)
  - If empty, step starts when flow execution reaches it
  - If non-empty, step waits until all dependencies reach `succeeded` state

---

### Step Type: `task`

Executes a single task using the task execution protocol.

#### Schema

```json
{
  "type": "task",
  "step_id": "...",
  "title": "...",
  "task_ref": "<task_reference>"
}
```

#### Fields

- `task_ref` (string, required): Reference to the task to execute
  - Format: `task:<task_id>` or task ID directly
  - The task MUST exist in the task store before flow execution
  - Task execution follows task-exec-spec-0.1.md protocol

#### Execution Semantics

1. Step transitions to `running`
2. Task execution begins via task runner
3. Step monitors task status
4. When task reaches terminal state:
   - Task `DONE` → Step `succeeded`
   - Task `FAILURE` → Step `failed`
   - Task `SKIPPED` → Step `skipped`
5. Step transitions to terminal state

#### Logging

- `STEP_START` when step begins
- `STEP_END` when step completes with outcome

---

### Step Type: `parallel`

Executes multiple child steps concurrently with optional concurrency limits.

#### Schema

```json
{
  "type": "parallel",
  "step_id": "...",
  "title": "...",
  "children": ["<step_id>", ...],
  "max_concurrency": <number>
}
```

#### Fields

- `children` (array of strings, required): Step IDs to execute in parallel
  - MUST contain at least 1 child
  - All children MUST exist in the flow's `steps` map
- `max_concurrency` (number, optional): Maximum concurrent running children
  - Default: unlimited (all children start immediately)
  - If set, at most N children may be `running` simultaneously
  - Remaining children stay `pending` until a slot opens

#### Execution Semantics

1. Parallel step transitions to `running`
2. All child steps become `queued`
3. Children start according to concurrency limit:
   - If `max_concurrency` is unset: all children start immediately
   - If set: start up to N children, queue the rest
4. As children complete, additional children start (if queued)
5. Parallel step outcome:
   - All children `succeeded` → Parallel `succeeded`
   - Any child `failed` → Parallel `failed` (remaining children may be canceled)
   - Parallel canceled → All children `canceled`

#### Determinism

- Start timing is nondeterministic (race conditions allowed)
- Output ordering MUST be deterministic: sorted by `step_id` alphabetically
- Log ordering MUST reflect actual execution timing

#### Logging

- `STEP_START(parallel)` when parallel container starts
- `STEP_START(<child>)` for each child as it starts
- `STEP_END(<child>)` for each child as it completes
- `STEP_END(parallel)` when parallel container completes

---

### Step Type: `branch`

Selects one execution path from multiple cases based on condition evaluation.

#### Schema

```json
{
  "type": "branch",
  "step_id": "...",
  "title": "...",
  "cases": [
    {
      "when": "<condition_expression>",
      "next": "<step_id>"
    }
  ],
  "default_next": "<step_id>"
}
```

#### Fields

- `cases` (array of objects, required): Ordered list of condition-path pairs
  - Each case has:
    - `when` (string, required): Condition expression to evaluate
    - `next` (string, required): Step to execute if condition matches
  - Cases MUST be evaluated in array order
  - First matching case wins (short-circuit evaluation)
- `default_next` (string, optional): Step to execute if no cases match
  - If omitted and no cases match, branch MUST fail

#### Condition Expression Syntax

Condition expressions are simple comparison expressions:

- `<variable> == "<value>"` (string equality)
- `<variable> == <number>` (numeric equality)
- `<variable> != "<value>"` (inequality)
- Future: `&&`, `||`, `<`, `>`, `contains`, `matches`

Variables are resolved from:
1. Flow inputs (if provided)
2. Upstream step outputs (if available)
3. Environment variables (if allowed by policy)

#### Execution Semantics

1. Branch step transitions to `running`
2. Evaluate `cases` in order:
   - For each case, evaluate `when` condition
   - If true, select that case's `next` step
   - Break evaluation (do not check remaining cases)
3. If no cases match:
   - If `default_next` exists, select it
   - If `default_next` does not exist, branch MUST fail with error `NO_MATCHING_BRANCH`
4. Selected step becomes active
5. Non-selected steps are marked `skipped` (for audit)
6. Branch step transitions to `succeeded`

#### Logging

- `BRANCH_EVAL` with:
  - Selected case index (or "default")
  - Condition expression evaluated
  - Selected `next` step_id
  - Input values used in evaluation (sanitized if sensitive)

---

### Step Type: `join`

Waits for multiple upstream steps to succeed before proceeding.

#### Schema

```json
{
  "type": "join",
  "step_id": "...",
  "title": "...",
  "wait_for": ["<step_id>", ...]
}
```

#### Fields

- `wait_for` (array of strings, required): Step IDs that must succeed
  - MUST contain at least 1 step ID
  - All steps MUST exist in the flow

#### Execution Semantics

1. Join step transitions to `running`
2. Wait for all steps in `wait_for` to reach terminal state
3. Join outcome:
   - All steps in `wait_for` are `succeeded` → Join `succeeded`
   - Any step in `wait_for` is `failed` → Join `failed`
   - Any step in `wait_for` is `canceled` → Join `canceled`
4. Join acts as a synchronization barrier for downstream steps

#### Notes

- Join is primarily a readability construct (equivalent to `depends_on` on downstream step)
- Useful for visualizing fan-in patterns in flow graphs

#### Logging

- `STEP_START(join)` when waiting begins
- `STEP_END(join)` when all dependencies resolved

---

### Step Type: `retry`

Retries a child step up to a maximum number of attempts with optional backoff.

#### Schema

```json
{
  "type": "retry",
  "step_id": "...",
  "title": "...",
  "child": "<step_id>",
  "policy": {
    "max_attempts": <number>,
    "backoff_ms": <number>
  }
}
```

#### Fields

- `child` (string, required): Step ID to retry
  - MUST exist in the flow's `steps` map
- `policy` (object, required): Retry policy configuration
  - `max_attempts` (number, required): Maximum number of execution attempts
    - MUST be >= 1
    - Default: 3 (if policy object exists)
  - `backoff_ms` (number, optional): Delay in milliseconds between attempts
    - Default: 0 (no delay)
    - Delay applies after failed attempt, before next attempt starts

#### Execution Semantics

1. Retry step transitions to `running`
2. Execute child step (attempt 1)
3. If child `succeeded`:
   - Retry step transitions to `succeeded`
   - Stop (do not execute remaining attempts)
4. If child `failed`:
   - If attempts < `max_attempts`:
     - Wait `backoff_ms` milliseconds
     - Execute child step again (attempt N+1)
     - Go to step 3
   - If attempts == `max_attempts`:
     - Retry step transitions to `failed`
     - Include attempt count in error context
5. If child `canceled`:
   - Retry step transitions to `canceled`

#### Logging

- `STEP_START(retry)` when retry container starts
- `STEP_START(<child>, attempt=N)` for each attempt
- `STEP_END(<child>, attempt=N)` for each attempt completion
- `STEP_END(retry, attempts=N)` when retry completes with final outcome

#### Determinism

- Retry timing is deterministic given `backoff_ms`
- Retry count is deterministic given `max_attempts`
- Retry outcome is deterministic given child behavior

---

### Step Type: `subflow`

Executes another flow as a nested step.

#### Schema

```json
{
  "type": "subflow",
  "step_id": "...",
  "title": "...",
  "flow_ref": "<flow_id>",
  "inputs": { /* optional input bindings */ }
}
```

#### Fields

- `flow_ref` (string, required): Flow ID to execute
  - Format: `flow:<name>:<version>`
  - The referenced flow MUST exist in the flow registry
- `inputs` (object, optional): Input variable bindings for the subflow
  - Keys are subflow input parameter names
  - Values are expressions or literals

#### Execution Semantics

1. Subflow step transitions to `running`
2. Resolve `inputs` from current context
3. Instantiate subflow with resolved inputs
4. Execute subflow to completion
5. Subflow outcome:
   - Subflow `succeeded` → Subflow step `succeeded`
   - Subflow `failed` → Subflow step `failed`
   - Subflow `canceled` → Subflow step `canceled`
6. Collect subflow outputs (if defined) and make available to downstream steps

#### Isolation

- Subflows execute in isolated context
- Subflows cannot modify parent flow state
- Subflows cannot access parent flow steps directly

#### Logging

- `STEP_START(subflow)` when subflow begins
- `FLOW_START(<flow_ref>, run_id=...)` for nested flow
- All subflow logs are emitted with nested context
- `FLOW_END(<flow_ref>, run_id=...)` when nested flow completes
- `STEP_END(subflow)` when subflow step completes

---

### Step Type: `exec`

Runs a literal command (argv array) and captures its result as structured
step output. Distinct from `task`: `exec` runs a deterministic external
process; `task` dispatches an LLM agent subprocess via the agent adapter
layer. Use `exec` for regex/CLI checks, smoke commands, lint/test invocations,
and any deterministic shell-level work that does not need an LLM.

#### Schema

```json
{
  "type": "exec",
  "step_id": "...",
  "title": "...",
  "exec": {
    "argv": ["<bin>", "<arg>", ...],
    "env": { "<KEY>": "<value>" },
    "cwd": "<path>",
    "timeout_sec": 60,
    "allow_nonzero_exit": false,
    "stdout_max_bytes": 1048576
  }
}
```

#### Fields

- `argv` (array of strings, required): Command and arguments to execute
  - MUST contain at least one element (the binary)
  - First element is resolved against PATH (or absolute path)
  - No shell expansion; arguments are passed literally
- `env` (object, optional): Extra environment variables for the child process
  - Merged on top of the parent process environment
  - Keys with empty values unset that variable in the child
  - Default: parent environment unmodified
- `cwd` (string, optional): Working directory for the child process
  - Default: flow workdir (sandbox repo dir in flow-test mode)
  - Relative paths are resolved against the flow workdir
- `timeout_sec` (integer, optional): Maximum runtime in seconds
  - Default: `60`
  - On timeout, the process is signalled (SIGTERM, then SIGKILL) and
    the step fails with a timeout error regardless of `allow_nonzero_exit`
  - Set to `0` to use the default; negative values are rejected
- `allow_nonzero_exit` (bool, optional): When `true`, a non-zero exit
  code does not fail the step
  - Default: `false` (non-zero exit fails the step)
  - The exit code is always available in the step output
- `stdout_max_bytes` (integer, optional): Truncate captured stdout
  beyond this limit
  - Default: `1048576` (1 MiB)
  - Truncation MUST set `truncated: true` in step output
  - The same limit applies independently to stderr

#### Output Schema

Every `exec` step emits the following structured output:

```json
{
  "exit_code": <int>,
  "stdout": "<string>",
  "stderr": "<string>",
  "duration_ms": <int>,
  "truncated": <bool>
}
```

- `exit_code` (integer): Process exit code (`0` on success)
- `stdout` (string): Captured stdout, possibly truncated
- `stderr` (string): Captured stderr, possibly truncated
- `duration_ms` (integer): Wall-clock runtime in milliseconds
- `truncated` (bool): `true` if stdout or stderr was truncated

#### Execution Semantics

1. Step transitions to `running`
2. Resolve `argv[0]` against PATH using `cwd` and merged env
3. Spawn child process with merged env, configured cwd, empty stdin
4. Capture stdout and stderr into bounded buffers
5. Wait for the process to exit OR for the timeout to expire
6. Emit structured output as defined above
7. Step outcome:
   - Timeout → step `failed`
   - Exit `0` → step `succeeded`
   - Non-zero exit AND `allow_nonzero_exit: true` → step `succeeded`
   - Non-zero exit AND `allow_nonzero_exit: false` → step `failed`

#### Determinism

- Output is deterministic given identical `argv`, `env`, `cwd`, and
  binary version
- Test harnesses MAY cassette `exec` steps via the cross-runtime
  recorder. Replay returns the recorded `{exit_code, stdout, stderr}`
  without invoking the binary
- `duration_ms` is NOT replayed deterministically; consumers MUST NOT
  assert on its exact value

#### Cassette Integration (Informative)

In `tlc flow test`, `exec` steps run inside the sandbox and are
intercepted by the catchall shim — the same path used for arbitrary
binaries spawned by `task` steps. Replay determinism therefore mirrors
the existing `task` semantics: same `argv` plus same env produces the
same recorded `{exit_code, stdout, stderr}` payload.

#### Logging

- `STEP_START` when step begins, with `argv` and `cwd`
- `STEP_END` when step completes, with `exit_code`, `duration_ms`,
  and `truncated`

---

## Step Status Lifecycle (Normative)

Every step follows this status state machine:

```
pending → queued → running → [succeeded | failed | canceled | skipped]
```

### Status Definitions

- **pending**: Step exists but dependencies not yet satisfied
- **queued**: Dependencies satisfied, waiting to start execution
- **running**: Step is actively executing
- **succeeded**: Step completed successfully
- **failed**: Step encountered an error and could not complete
- **canceled**: Step was explicitly canceled before completion
- **skipped**: Step was not executed due to branching or other control flow

### Status Transition Rules

#### Valid Transitions

- `pending → queued`: All dependencies satisfied
- `queued → running`: Execution begins
- `running → succeeded`: Execution completed successfully
- `running → failed`: Execution encountered error
- `running → canceled`: Execution was canceled
- `pending → skipped`: Step bypassed by control flow (branch)
- `queued → canceled`: Step canceled before starting
- `running → skipped`: Step determined unnecessary during execution (rare)

#### Invalid Transitions

- Any terminal state → any other state (terminal states are final)
- `pending → running`: Must transition through `queued`
- `queued → succeeded`: Must transition through `running`

---

## Flow Status Lifecycle (Normative)

Every flow run follows this status state machine:

```
queued → running → [succeeded | failed | canceled]
```

### Flow Status Definitions

- **queued**: Flow instantiated, waiting to start
- **running**: Flow execution in progress
- **succeeded**: All required steps succeeded, flow completed
- **failed**: One or more required steps failed, flow cannot complete
- **canceled**: Flow execution was explicitly canceled

### Flow Status Rules

- Flow starts in `queued` state
- Flow transitions to `running` when `entry_step` starts
- Flow transitions to terminal state when:
  - `succeeded`: All steps reached terminal success states
  - `failed`: Any required step failed
  - `canceled`: Flow received cancel signal

---

## Data Flow (Normative)

### Step Outputs

Steps MAY produce outputs that can be consumed by downstream steps.

#### Output Schema

```json
{
  "step_id": "example",
  "status": "succeeded",
  "output": {
    "key": "value",
    /* arbitrary structured data */
  }
}
```

### Output References

Downstream steps can reference upstream outputs using expressions:

```
${step_id.output.key}
```

Example:
```json
{
  "type": "task",
  "step_id": "deploy",
  "task_ref": "task:deploy",
  "depends_on": ["build"],
  "inputs": {
    "artifact_url": "${build.output.artifact_url}"
  }
}
```

### Output Resolution Rules

- Outputs MUST be resolved at step start time
- Missing outputs MUST cause step to fail with `MISSING_OUTPUT` error
- Circular dependencies MUST be rejected at flow validation time

---

## Error Handling (Normative)

### Error Propagation

Errors propagate through the flow according to these rules:

1. **Step Failure**: When a step fails:
   - Step status → `failed`
   - Step error context is captured
   - Downstream steps (via `depends_on`) → `pending` (never start)
   - Parent flow evaluates if this is a terminal failure

2. **Flow Failure**: Flow fails when:
   - Any required step (not marked optional) fails
   - Branch evaluation fails (no matching case, no default)
   - Missing dependency reference

3. **Error Context**: Every failure MUST include:
   - `error.code`: Machine-readable error code
   - `error.message`: Human-readable error message
   - `error.step_id`: Step that failed (if applicable)
   - `error.cause`: Nested error chain (if applicable)

### Error Codes

Standard error codes defined by TFS:

- `STEP_FAILED`: Generic step failure
- `TASK_EXECUTION_FAILED`: Task execution returned failure
- `NO_MATCHING_BRANCH`: Branch found no matching case and no default
- `MISSING_DEPENDENCY`: Referenced step does not exist
- `MISSING_OUTPUT`: Required output from upstream step not available
- `CYCLE_DETECTED`: Flow graph contains a cycle
- `VALIDATION_FAILED`: Flow definition invalid
- `TIMEOUT`: Step exceeded time limit (future)
- `CANCELED`: Step or flow was canceled

### Cancellation

Cancellation is cooperative:

1. Flow receives cancel signal
2. Flow status → `canceled`
3. All `pending` steps → `canceled` (never start)
4. All `queued` steps → `canceled` (never start)
5. All `running` steps receive cancel signal (best effort stop)
6. Flow waits for running steps to respond (with timeout)
7. Flow completes as `canceled`

---

## Logging Requirements (Normative)

Flow execution MUST emit the following log events to CHANGELOG (see task-log-spec-0.1.md).

### Flow-Level Events

#### FLOW_START

```json
{
  "timestamp": "2026-01-15T17:30:00Z",
  "action": "FLOW_START",
  "flow_id": "flow:qa:0.1",
  "run_id": "run:abc123",
  "by": "AGENT_ID",
  "note": "Starting QA Pipeline"
}
```

#### FLOW_END

```json
{
  "timestamp": "2026-01-15T17:35:00Z",
  "action": "FLOW_END",
  "flow_id": "flow:qa:0.1",
  "run_id": "run:abc123",
  "status": "succeeded",
  "by": "AGENT_ID",
  "note": "Flow completed successfully"
}
```

### Step-Level Events

#### STEP_START

```json
{
  "timestamp": "2026-01-15T17:30:05Z",
  "action": "STEP_START",
  "flow_id": "flow:qa:0.1",
  "run_id": "run:abc123",
  "step_id": "lint",
  "by": "AGENT_ID",
  "note": "Starting step: Lint"
}
```

#### STEP_END

```json
{
  "timestamp": "2026-01-15T17:30:15Z",
  "action": "STEP_END",
  "flow_id": "flow:qa:0.1",
  "run_id": "run:abc123",
  "step_id": "lint",
  "status": "succeeded",
  "by": "AGENT_ID",
  "note": "Step completed: Lint"
}
```

#### BRANCH_EVAL

```json
{
  "timestamp": "2026-01-15T17:30:10Z",
  "action": "BRANCH_EVAL",
  "flow_id": "flow:qa:0.1",
  "run_id": "run:abc123",
  "step_id": "environment-branch",
  "selected": "prod-deploy",
  "condition": "env == 'production'",
  "by": "AGENT_ID",
  "note": "Branch selected: prod-deploy"
}
```

### Logging Rules

- All logs MUST be prepended to CHANGELOG (newest first)
- All logs MUST include ISO8601 UTC timestamp
- All logs MUST include `run_id` for correlation
- Logs MUST be emitted synchronously before state transitions complete
- Log ordering MUST reflect actual execution timing

---

## Determinism Guarantees (Normative)

TFS provides the following determinism guarantees:

### Guaranteed Deterministic

- **Sequential ordering**: Steps with `depends_on` always execute in dependency order
- **Branch selection**: Same inputs always select same branch case
- **Retry count**: Same failure pattern always yields same attempt count
- **Output ordering**: Parallel step outputs always sorted by `step_id`

### Allowed Nondeterminism

- **Parallel start timing**: Children of parallel may start in any order
- **Parallel completion timing**: Children may complete in any order
- **Actual durations**: Step execution time may vary

### Determinism Testing

- Flows SHOULD be tested with multiple runs and identical inputs
- Flow outputs SHOULD be compared for equality
- Log ordering SHOULD be ignored (only log presence matters)
- Timestamps SHOULD be ignored (only relative ordering matters)

---

## Integration with Other Specs

### Task Execution (task-exec-spec-0.1.md)

- `task` steps invoke task execution protocol
- Task status maps to step status:
  - Task `DONE` → Step `succeeded`
  - Task `FAILURE` → Step `failed`
  - Task `SKIPPED` → Step `skipped`
- Task outputs become step outputs

### Task CRUD (task-crud-spec-0.1.md)

- `task_ref` must resolve to a task in the task store
- Tasks MAY be created before flow execution
- Tasks MAY be created dynamically during flow execution (future)
- Task status transitions follow task-crud-spec rules

### Collaboration (task-collab-spec-1.0.md)

- Flow execution MAY be performed by any agent
- Flow engine does NOT claim tasks on behalf of steps
- Task steps trigger collaboration protocol when task execution begins
- Agents MAY claim tasks referenced by flow steps

### Logging (task-log-spec-0.1.md)

- Flow events use standard log schema
- Flow events MUST be prepended to CHANGELOG
- Flow events MUST include `run_id` for correlation
- Flow-specific actions: `FLOW_START`, `FLOW_END`, `STEP_START`, `STEP_END`, `BRANCH_EVAL`

---

## Minimal Example (Normative)

```json
{
  "flow_id": "flow:qa:0.1",
  "name": "QA Pipeline",
  "version": "0.1",
  "entry_step": "parallel-checks",
  "steps": {
    "parallel-checks": {
      "step_id": "parallel-checks",
      "type": "parallel",
      "title": "Run QA checks",
      "children": ["lint", "typecheck", "tests"],
      "max_concurrency": 3
    },
    "lint": {
      "step_id": "lint",
      "type": "task",
      "title": "Lint",
      "task_ref": "task:lint"
    },
    "typecheck": {
      "step_id": "typecheck",
      "type": "task",
      "title": "Typecheck",
      "task_ref": "task:typecheck"
    },
    "tests": {
      "step_id": "tests",
      "type": "task",
      "title": "Unit tests",
      "task_ref": "task:unit-tests"
    }
  }
}
```

---

## Canonical Examples

### Example 1: Sequential Flow with Dependencies

```json
{
  "flow_id": "flow:build-deploy:1.0",
  "name": "Build and Deploy",
  "version": "1.0",
  "entry_step": "build",
  "steps": {
    "build": {
      "step_id": "build",
      "type": "task",
      "title": "Build application",
      "task_ref": "task:build"
    },
    "test": {
      "step_id": "test",
      "type": "task",
      "title": "Run tests",
      "task_ref": "task:test",
      "depends_on": ["build"]
    },
    "deploy": {
      "step_id": "deploy",
      "type": "task",
      "title": "Deploy to production",
      "task_ref": "task:deploy",
      "depends_on": ["test"]
    }
  }
}
```

### Example 2: Branch with Conditional Deployment

```json
{
  "flow_id": "flow:conditional-deploy:1.0",
  "name": "Conditional Deployment",
  "version": "1.0",
  "entry_step": "build",
  "steps": {
    "build": {
      "step_id": "build",
      "type": "task",
      "title": "Build",
      "task_ref": "task:build"
    },
    "env-branch": {
      "step_id": "env-branch",
      "type": "branch",
      "title": "Select deployment target",
      "depends_on": ["build"],
      "cases": [
        {
          "when": "env == 'production'",
          "next": "deploy-prod"
        },
        {
          "when": "env == 'staging'",
          "next": "deploy-staging"
        }
      ],
      "default_next": "deploy-dev"
    },
    "deploy-prod": {
      "step_id": "deploy-prod",
      "type": "task",
      "title": "Deploy to production",
      "task_ref": "task:deploy-prod"
    },
    "deploy-staging": {
      "step_id": "deploy-staging",
      "type": "task",
      "title": "Deploy to staging",
      "task_ref": "task:deploy-staging"
    },
    "deploy-dev": {
      "step_id": "deploy-dev",
      "type": "task",
      "title": "Deploy to dev",
      "task_ref": "task:deploy-dev"
    }
  }
}
```

### Example 3: Retry with Backoff

```json
{
  "flow_id": "flow:flaky-api:1.0",
  "name": "Flaky API Call",
  "version": "1.0",
  "entry_step": "retry-api",
  "steps": {
    "retry-api": {
      "step_id": "retry-api",
      "type": "retry",
      "title": "Call external API with retry",
      "child": "api-call",
      "policy": {
        "max_attempts": 3,
        "backoff_ms": 1000
      }
    },
    "api-call": {
      "step_id": "api-call",
      "type": "task",
      "title": "External API call",
      "task_ref": "task:api-call"
    }
  }
}
```

### Example 4: Join Multiple Branches

```json
{
  "flow_id": "flow:fan-out-join:1.0",
  "name": "Fan-out and Join",
  "version": "1.0",
  "entry_step": "prepare",
  "steps": {
    "prepare": {
      "step_id": "prepare",
      "type": "task",
      "title": "Prepare data",
      "task_ref": "task:prepare"
    },
    "process-a": {
      "step_id": "process-a",
      "type": "task",
      "title": "Process A",
      "task_ref": "task:process-a",
      "depends_on": ["prepare"]
    },
    "process-b": {
      "step_id": "process-b",
      "type": "task",
      "title": "Process B",
      "task_ref": "task:process-b",
      "depends_on": ["prepare"]
    },
    "process-c": {
      "step_id": "process-c",
      "type": "task",
      "title": "Process C",
      "task_ref": "task:process-c",
      "depends_on": ["prepare"]
    },
    "join-all": {
      "step_id": "join-all",
      "type": "join",
      "title": "Wait for all processing",
      "wait_for": ["process-a", "process-b", "process-c"]
    },
    "finalize": {
      "step_id": "finalize",
      "type": "task",
      "title": "Finalize results",
      "task_ref": "task:finalize",
      "depends_on": ["join-all"]
    }
  }
}
```

### Example 5: Feature Development with Worktree (Development-Specific)

Complete feature development workflow using Git worktrees and branch conventions:

```json
{
  "flow_id": "flow:feature-worktree:1.0",
  "name": "Feature Development with Worktree",
  "version": "1.0",
  "entry_step": "setup-worktree",
  "steps": {
    "setup-worktree": {
      "step_id": "setup-worktree",
      "type": "task",
      "title": "Setup Git worktree",
      "task_ref": "task:setup-worktree",
      "meta": {
        "branch": "feat/0042-api-rate-limiting",
        "worktree_path": ".worktrees/feat-0042-api-rate-limiting",
        "issue_number": 42
      }
    },
    "parallel-setup": {
      "step_id": "parallel-setup",
      "type": "parallel",
      "title": "Parallel setup tasks",
      "children": ["install-deps", "create-branch"],
      "depends_on": ["setup-worktree"],
      "max_concurrency": 2
    },
    "install-deps": {
      "step_id": "install-deps",
      "type": "task",
      "title": "Install dependencies",
      "task_ref": "task:npm-install"
    },
    "create-branch": {
      "step_id": "create-branch",
      "type": "task",
      "title": "Create feature branch",
      "task_ref": "task:git-checkout-branch",
      "meta": {
        "branch_name": "feat/0042-api-rate-limiting"
      }
    },
    "implement-feature": {
      "step_id": "implement-feature",
      "type": "task",
      "title": "Implement rate limiting",
      "task_ref": "task:implement",
      "depends_on": ["parallel-setup"]
    },
    "quality-checks": {
      "step_id": "quality-checks",
      "type": "parallel",
      "title": "Quality checks",
      "children": ["lint", "typecheck", "unit-tests"],
      "depends_on": ["implement-feature"],
      "max_concurrency": 3
    },
    "lint": {
      "step_id": "lint",
      "type": "task",
      "title": "Run linter",
      "task_ref": "task:eslint"
    },
    "typecheck": {
      "step_id": "typecheck",
      "type": "task",
      "title": "Run type checker",
      "task_ref": "task:typescript"
    },
    "unit-tests": {
      "step_id": "unit-tests",
      "type": "task",
      "title": "Run unit tests",
      "task_ref": "task:jest"
    },
    "integration-tests": {
      "step_id": "integration-tests",
      "type": "task",
      "title": "Run integration tests",
      "task_ref": "task:integration-tests",
      "depends_on": ["quality-checks"]
    },
    "commit-changes": {
      "step_id": "commit-changes",
      "type": "task",
      "title": "Commit changes",
      "task_ref": "task:git-commit",
      "depends_on": ["integration-tests"],
      "meta": {
        "commit_template": "feat: implement API rate limiting (closes #42)"
      }
    },
    "cleanup-worktree": {
      "step_id": "cleanup-worktree",
      "type": "task",
      "title": "Cleanup worktree",
      "task_ref": "task:cleanup-worktree",
      "depends_on": ["commit-changes"],
      "meta": {
        "worktree_path": ".worktrees/feat-0042-api-rate-limiting"
      }
    }
  }
}
```

### Example 6: Multi-Task Development with Worktree Isolation

Parent task with multiple child tasks, each in their own worktree:

```json
{
  "flow_id": "flow:refactor-auth:1.0",
  "name": "Authentication Module Refactor",
  "version": "1.0",
  "entry_step": "parent-worktree",
  "steps": {
    "parent-worktree": {
      "step_id": "parent-worktree",
      "type": "task",
      "title": "Setup parent worktree",
      "task_ref": "task:setup-worktree",
      "meta": {
        "branch": "chore/0089-refactor-auth-module",
        "worktree_path": ".worktrees/chore-0089-refactor-auth-module"
      }
    },
    "child-tasks": {
      "step_id": "child-tasks",
      "type": "parallel",
      "title": "Refactor components in parallel",
      "children": ["refactor-login", "refactor-session", "refactor-permissions"],
      "depends_on": ["parent-worktree"],
      "max_concurrency": 2
    },
    "refactor-login": {
      "step_id": "refactor-login",
      "type": "subflow",
      "title": "Refactor login component",
      "flow_ref": "flow:feature-worktree:1.0",
      "inputs": {
        "branch": "chore/0089-refactor-login",
        "worktree_path": ".worktrees/chore-0089-refactor-login",
        "parent_task": "T-0089"
      }
    },
    "refactor-session": {
      "step_id": "refactor-session",
      "type": "subflow",
      "title": "Refactor session management",
      "flow_ref": "flow:feature-worktree:1.0",
      "inputs": {
        "branch": "chore/0089-refactor-session",
        "worktree_path": ".worktrees/chore-0089-refactor-session",
        "parent_task": "T-0089"
      }
    },
    "refactor-permissions": {
      "step_id": "refactor-permissions",
      "type": "subflow",
      "title": "Refactor permissions check",
      "flow_ref": "flow:feature-worktree:1.0",
      "inputs": {
        "branch": "chore/0089-refactor-permissions",
        "worktree_path": ".worktrees/chore-0089-refactor-permissions",
        "parent_task": "T-0089"
      }
    },
    "merge-branches": {
      "step_id": "merge-branches",
      "type": "task",
      "title": "Merge child branches into parent",
      "task_ref": "task:git-merge-branches",
      "depends_on": ["child-tasks"],
      "meta": {
        "target_branch": "chore/0089-refactor-auth-module",
        "source_branches": [
          "chore/0089-refactor-login",
          "chore/0089-refactor-session",
          "chore/0089-refactor-permissions"
        ]
      }
    },
    "final-tests": {
      "step_id": "final-tests",
      "type": "task",
      "title": "Run full test suite",
      "task_ref": "task:run-all-tests",
      "depends_on": ["merge-branches"]
    },
    "cleanup-all": {
      "step_id": "cleanup-all",
      "type": "parallel",
      "title": "Cleanup all worktrees",
      "children": ["cleanup-parent", "cleanup-children"],
      "depends_on": ["final-tests"]
    },
    "cleanup-parent": {
      "step_id": "cleanup-parent",
      "type": "task",
      "title": "Cleanup parent worktree",
      "task_ref": "task:cleanup-worktree",
      "meta": {
        "worktree_path": ".worktrees/chore-0089-refactor-auth-module"
      }
    },
    "cleanup-children": {
      "step_id": "cleanup-children",
      "type": "task",
      "title": "Cleanup child worktrees",
      "task_ref": "task:cleanup-multiple-worktrees",
      "meta": {
        "worktree_paths": [
          ".worktrees/chore-0089-refactor-login",
          ".worktrees/chore-0089-refactor-session",
          ".worktrees/chore-0089-refactor-permissions"
        ]
      }
    }
  }
}
```

---

## Software Development Workflow Conventions

### Git Worktree Usage (Normative for Development Tasks)

When executing development tasks (parent tasks and child tasks with significant scope), implementations MUST use Git worktrees for isolation.

#### Worktree Directory Structure

```
$PWD/.worktrees/           # Git-ignored worktree directory
├── feat-0013-add-auth/    # Feature branch worktree
├── fix-0042-login-bug/    # Bug fix branch worktree
└── chore-0005-refactor/   # Chore branch worktree
```

#### Worktree Rules

- Worktree directory MUST be `$PWD/.worktrees/`
- Worktree directory MUST be in `.gitignore`
- Create `.worktrees/` directory if it doesn't exist
- Each task with code changes SHOULD get its own worktree
- Parent tasks MUST always use worktrees
- Child tasks SHOULD use worktrees if scope is non-trivial
- Worktree name SHOULD match branch name for consistency

#### Worktree Lifecycle

1. **Setup**: Create worktree when task begins
   ```bash
   mkdir -p .worktrees
   git worktree add .worktrees/feat-0013-add-auth feat/0013-add-auth
   cd .worktrees/feat-0013-add-auth
   ```

2. **Development**: All work happens in worktree
   - Install dependencies
   - Make code changes
   - Run tests
   - Commit changes

3. **Cleanup**: Remove worktree after task completes
   ```bash
   cd $PWD
   git worktree remove .worktrees/feat-0013-add-auth
   ```

### Branch Naming Convention (Normative)

Branch names MUST follow this format:

```
<type>/<issue-id>-<purpose>
```

#### Components

- **type**: Conventional commit type (see below)
- **issue-id**: Zero-padded 4-digit issue number (e.g., `0013`, `0042`, `0001`)
- **purpose**: Hyphen-separated description of changes

#### Valid Types

- `feat` - New feature
- `fix` - Bug fix
- `chore` - Maintenance/refactoring
- `docs` - Documentation changes
- `test` - Test additions/changes
- `perf` - Performance improvements
- `refactor` - Code refactoring
- `style` - Code style changes
- `ci` - CI/CD changes
- `build` - Build system changes

#### Branch Name Examples

```
feat/0013-add-user-authentication
fix/0042-resolve-login-timeout
chore/0005-refactor-auth-module
docs/0023-update-api-documentation
test/0018-add-integration-tests
```

#### Issue Number Handling

- If task has associated issue: Include zero-padded issue number
  - Example: Issue #13 → `feat/0013-add-authentication`
  - Example: Issue #456 → `fix/0456-memory-leak`
- If task has no issue: Omit issue number entirely
  - Example: `feat/add-authentication`
  - Example: `fix/memory-leak`
- Never use placeholder numbers like `0000` or `9999`

#### Commit Message Generation

When squashing commits, the issue number tells the generator to:
- Extract type from branch name → commit type
- Append " (closes #N)" to commit title if issue number present

Examples:
- Branch `feat/0013-add-auth` → Commit: `feat: add authentication (closes #13)`
- Branch `fix/0042-login-bug` → Commit: `fix: resolve login timeout (closes #42)`
- Branch `feat/add-auth` → Commit: `feat: add authentication`

### Development Task Flow Pattern

Typical flow for feature development with worktrees:

```json
{
  "flow_id": "flow:feature-dev:1.0",
  "name": "Feature Development",
  "version": "1.0",
  "entry_step": "create-worktree",
  "steps": {
    "create-worktree": {
      "step_id": "create-worktree",
      "type": "task",
      "title": "Create Git worktree",
      "task_ref": "task:create-worktree",
      "meta": {
        "branch": "feat/0013-add-auth",
        "worktree_path": ".worktrees/feat-0013-add-auth"
      }
    },
    "install-deps": {
      "step_id": "install-deps",
      "type": "task",
      "title": "Install dependencies",
      "task_ref": "task:install-deps",
      "depends_on": ["create-worktree"]
    },
    "implement": {
      "step_id": "implement",
      "type": "task",
      "title": "Implement feature",
      "task_ref": "task:implement",
      "depends_on": ["install-deps"]
    },
    "test": {
      "step_id": "test",
      "type": "task",
      "title": "Run tests",
      "task_ref": "task:test",
      "depends_on": ["implement"]
    },
    "commit": {
      "step_id": "commit",
      "type": "task",
      "title": "Commit changes",
      "task_ref": "task:commit",
      "depends_on": ["test"]
    },
    "cleanup-worktree": {
      "step_id": "cleanup-worktree",
      "type": "task",
      "title": "Remove worktree",
      "task_ref": "task:cleanup-worktree",
      "depends_on": ["commit"]
    }
  }
}
```

### Worktree Task Metadata

Development tasks SHOULD include worktree metadata:

```json
{
  "id": "T-0013",
  "title": "Add user authentication",
  "status": "IN_PROGRESS",
  "meta": {
    "issue_number": 13,
    "branch_name": "feat/0013-add-user-authentication",
    "worktree_path": ".worktrees/feat-0013-add-user-authentication",
    "commit_type": "feat"
  }
}
```

---

## Future Extensions

The following features are explicitly out of scope for v0.1 but MAY be added in future versions:

- **Dynamic step generation**: Creating steps at runtime
- **Conditional dependencies**: `depends_on` with conditions
- **Loops**: Iterating over collections
- **Timeouts**: Step and flow-level timeout enforcement
- **Optional steps**: Steps that may fail without failing the flow
- **Step outputs schema**: Typed output validation
- **Flow inputs schema**: Typed input validation
- **Step templates**: Reusable step definitions
- **Flow composition**: Importing steps from other flows
- **Partial retry**: Resuming failed flows from checkpoint
- **Human-in-the-loop**: Approval gates and manual steps

---

## Conformance

An implementation conforms to TFS v0.1 if:

1. It accepts all valid flow definitions per this spec
2. It rejects all invalid flow definitions per validation rules
3. It executes flows according to step type semantics
4. It emits all required log events
5. It respects status lifecycle rules
6. It provides determinism guarantees as specified
7. It propagates errors per error handling rules
8. It integrates correctly with task-exec, task-crud, task-collab, and task-log specs

---

## References

- task-exec-spec-0.1.md — Task execution protocol
- task-crud-spec-0.1.md — Task data model and persistence
- task-collab-spec-1.0.md — Multi-agent collaboration
- task-log-spec-0.1.md — Log schema and write policy
- dev-workflow-conventions-0.1.md — Git worktree and branch naming conventions
- sync-architecture-0.1.md — External system sync architecture
- stories/user-stories-taskflow-0.1.md — User story requirements
- e2e-taskflow-0.1.md — End-to-end test scenarios
- glossary-0.1.md — Canonical term definitions
