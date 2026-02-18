# Task Flow Specification (TFS) v0.1

## Version

- Version: 0.1
- Generated at: 2025-01-15T18:00:00Z

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
  "step_id": "publish",
  "task_ref": "task:publish",
  "depends_on": ["prepare"],
  "inputs": {
    "document_url": "${prepare.output.document_url}"
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
  "timestamp": "2025-01-15T18:00:00Z",
  "action": "FLOW_START",
  "flow_id": "flow:approval:0.1",
  "run_id": "run:abc123",
  "by": "agent-1",
  "note": "Starting approval workflow"
}
```

#### FLOW_END

```json
{
  "timestamp": "2025-01-15T18:15:00Z",
  "action": "FLOW_END",
  "flow_id": "flow:approval:0.1",
  "run_id": "run:abc123",
  "status": "succeeded",
  "by": "agent-1",
  "note": "Flow completed successfully"
}
```

### Step-Level Events

#### STEP_START

```json
{
  "timestamp": "2025-01-15T18:00:05Z",
  "action": "STEP_START",
  "flow_id": "flow:approval:0.1",
  "run_id": "run:abc123",
  "step_id": "review",
  "by": "agent-1",
  "note": "Starting step: Review"
}
```

#### STEP_END

```json
{
  "timestamp": "2025-01-15T18:05:00Z",
  "action": "STEP_END",
  "flow_id": "flow:approval:0.1",
  "run_id": "run:abc123",
  "step_id": "review",
  "status": "succeeded",
  "by": "agent-1",
  "note": "Step completed: Review"
}
```

#### BRANCH_EVAL

```json
{
  "timestamp": "2025-01-15T18:06:00Z",
  "action": "BRANCH_EVAL",
  "flow_id": "flow:approval:0.1",
  "run_id": "run:abc123",
  "step_id": "priority-branch",
  "selected": "urgent-path",
  "condition": "priority == 'urgent'",
  "by": "agent-1",
  "note": "Branch selected: urgent-path"
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
  "flow_id": "flow:approval:0.1",
  "name": "Document Approval",
  "version": "0.1",
  "entry_step": "parallel-reviews",
  "steps": {
    "parallel-reviews": {
      "step_id": "parallel-reviews",
      "type": "parallel",
      "title": "Concurrent reviews",
      "children": ["legal-review", "technical-review", "editorial-review"],
      "max_concurrency": 3
    },
    "legal-review": {
      "step_id": "legal-review",
      "type": "task",
      "title": "Legal review",
      "task_ref": "task:legal-review"
    },
    "technical-review": {
      "step_id": "technical-review",
      "type": "task",
      "title": "Technical review",
      "task_ref": "task:technical-review"
    },
    "editorial-review": {
      "step_id": "editorial-review",
      "type": "task",
      "title": "Editorial review",
      "task_ref": "task:editorial-review"
    }
  }
}
```

---

## Canonical Examples

### Example 1: Sequential Flow with Dependencies

```json
{
  "flow_id": "flow:report-generation:1.0",
  "name": "Report Generation Pipeline",
  "version": "1.0",
  "entry_step": "gather-data",
  "steps": {
    "gather-data": {
      "step_id": "gather-data",
      "type": "task",
      "title": "Gather source data",
      "task_ref": "task:gather-data"
    },
    "analyze": {
      "step_id": "analyze",
      "type": "task",
      "title": "Analyze data",
      "task_ref": "task:analyze",
      "depends_on": ["gather-data"]
    },
    "generate-report": {
      "step_id": "generate-report",
      "type": "task",
      "title": "Generate final report",
      "task_ref": "task:generate-report",
      "depends_on": ["analyze"]
    }
  }
}
```

### Example 2: Branch with Conditional Path

```json
{
  "flow_id": "flow:approval-routing:1.0",
  "name": "Approval Routing",
  "version": "1.0",
  "entry_step": "initial-review",
  "steps": {
    "initial-review": {
      "step_id": "initial-review",
      "type": "task",
      "title": "Initial review",
      "task_ref": "task:initial-review"
    },
    "priority-branch": {
      "step_id": "priority-branch",
      "type": "branch",
      "title": "Route by priority",
      "depends_on": ["initial-review"],
      "cases": [
        {
          "when": "priority == 'urgent'",
          "next": "urgent-approval"
        },
        {
          "when": "priority == 'high'",
          "next": "expedited-approval"
        }
      ],
      "default_next": "standard-approval"
    },
    "urgent-approval": {
      "step_id": "urgent-approval",
      "type": "task",
      "title": "Urgent approval process",
      "task_ref": "task:urgent-approval"
    },
    "expedited-approval": {
      "step_id": "expedited-approval",
      "type": "task",
      "title": "Expedited approval process",
      "task_ref": "task:expedited-approval"
    },
    "standard-approval": {
      "step_id": "standard-approval",
      "type": "task",
      "title": "Standard approval process",
      "task_ref": "task:standard-approval"
    }
  }
}
```

### Example 3: Retry with Backoff

```json
{
  "flow_id": "flow:notification:1.0",
  "name": "Notification Delivery",
  "version": "1.0",
  "entry_step": "retry-send",
  "steps": {
    "retry-send": {
      "step_id": "retry-send",
      "type": "retry",
      "title": "Send notification with retry",
      "child": "send-notification",
      "policy": {
        "max_attempts": 3,
        "backoff_ms": 1000
      }
    },
    "send-notification": {
      "step_id": "send-notification",
      "type": "task",
      "title": "Send notification",
      "task_ref": "task:send-notification"
    }
  }
}
```

### Example 4: Join Multiple Branches

```json
{
  "flow_id": "flow:data-processing:1.0",
  "name": "Data Processing Pipeline",
  "version": "1.0",
  "entry_step": "acquire-source",
  "steps": {
    "acquire-source": {
      "step_id": "acquire-source",
      "type": "task",
      "title": "Acquire source data",
      "task_ref": "task:acquire"
    },
    "transform-a": {
      "step_id": "transform-a",
      "type": "task",
      "title": "Transform dataset A",
      "task_ref": "task:transform-a",
      "depends_on": ["acquire-source"]
    },
    "transform-b": {
      "step_id": "transform-b",
      "type": "task",
      "title": "Transform dataset B",
      "task_ref": "task:transform-b",
      "depends_on": ["acquire-source"]
    },
    "transform-c": {
      "step_id": "transform-c",
      "type": "task",
      "title": "Transform dataset C",
      "task_ref": "task:transform-c",
      "depends_on": ["acquire-source"]
    },
    "join-all": {
      "step_id": "join-all",
      "type": "join",
      "title": "Wait for all transformations",
      "wait_for": ["transform-a", "transform-b", "transform-c"]
    },
    "consolidate": {
      "step_id": "consolidate",
      "type": "task",
      "title": "Consolidate results",
      "task_ref": "task:consolidate",
      "depends_on": ["join-all"]
    }
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
- stories/user-stories-taskflow-0.1.md — User story requirements
- e2e-taskflow-0.1.md — End-to-end test scenarios
- glossary-0.1.md — Canonical term definitions
