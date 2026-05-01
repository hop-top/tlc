---
status: shipped
---

# 078 - Flow Invoke Generates Tasks

**ID**: 078
**Feature**: Flow Orchestration — Task Generation
**Persona**: [Team Lead](../personas/team-lead.md)
**Related Personas**:
  [AI Agent](../personas/ai-agent.md),
  [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As a Team Lead, I want to invoke a flow that generates and assigns
tasks so I can bootstrap sprint work from a template.

## Context

`tlc flow invoke` reads a flow YAML with `task_template` blocks on
each step, extracts tasks from those templates, substitutes input
variables (`--var key=val`), and persists the tasks in the DB. The
flow's `depends_on` graph is reflected in the generated tasks via
`blocked_by` metadata. Each task carries `flow_id`, `flow_run_id`,
and `step_id` in its meta for traceability.

This enables template-driven sprint planning: define a flow once,
invoke it per sprint with different inputs, and get a fully linked
task DAG in the backlog.

## Acceptance Scenarios

1. **Given** a flow with 3 task-template steps and input variable
   `sprint_name`, **When** `tlc flow invoke task-generation.yaml
   --var sprint_name=S42`, **Then** 3 tasks are created in the DB,
   each with `{{sprint_name}}` replaced by `S42` in title and
   description.

2. **Given** the same flow, **When** tasks are created, **Then**
   each task has `meta.flow_id`, `meta.flow_run_id`, and
   `meta.step_id` set correctly.

3. **Given** a flow where step B depends on step A, **When** tasks
   are extracted, **Then** the task for step B has `blocked_by`
   metadata referencing the task for step A (or equivalent flow
   dependency info).

4. **Given** a flow with `config.inputs.sprint_name.required: true`,
   **When** `tlc flow invoke` is called without `--var sprint_name=`,
   **Then** the command fails with an actionable error naming the
   missing input.

## Tests

### E2E
- `internal/cli/flow_invoke_task_gen_e2e_test.go`
  - `TestFlowInvoke_TaskGeneration_HappyPath` (scenarios 1, 2)
  - `TestFlowInvoke_TaskGeneration_MissingInputFails` (scenario 4)

### Unit
- `internal/core/flow_test.go`
  - `TestFlowExecutor_Sequential` (task lifecycle)
- `internal/cli/flow_invoke_var_inputs_e2e_test.go`
  - `TestFlowInvoke_VarInputs_SubstitutesTemplatePlaceholders`
  - `TestFlowInvoke_VarInputs_MissingRequiredFails`

### Flow Test Fixtures
- `examples/flows/task-generation.yaml` — flow definition
- `examples/flows/fixtures/task-generation/happy-path/`

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | 3 tasks created, vars substituted | `TestFlowInvoke_TaskGeneration_HappyPath` | ✅ |
| 2 | meta fields set on tasks | `TestFlowInvoke_TaskGeneration_HappyPath` | ✅ |
| 3 | depends_on reflected in task meta | `TestFlowInvoke_TaskGeneration_HappyPath` | ✅ |
| 4 | missing required input fails | `TestFlowInvoke_TaskGeneration_MissingInputFails` | ✅ |

## Related

- Task: T-0620
- Track: flow-use-cases
