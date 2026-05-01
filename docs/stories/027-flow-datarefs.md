# 027 - Flow Data Refs (Cross-Step Variable Passing)

**ID**: 027
**Feature**: Flow Orchestration
**Persona**: [Solo Developer](../personas/solo-developer.md)
**Related Personas**: [Team Lead](../personas/team-lead.md),
[AI Agent](../personas/ai-agent.md)
**Priority**: P1
**Task**: T-0115
**Status**: paper
**Author**: jadb

## Story

As a flow author, I want `${<step-id>.output.<key>}` substitution in flow yaml so a
downstream step consumes upstream output (file path, ctxt object id, JSON field)
without writing glue scripts or env-var smuggling.

## Acceptance Scenarios

1. **Given** step `select-top` emits `output.path: /tmp/draft.md`,
   **When** downstream step `review` references `${select-top.output.path}`,
   **Then** ref resolves at flow-eval time to the literal string before step args
   are passed to the runner.

2. **Given** step output is a JSON object,
   **When** downstream step references `${gen.output.summary}` (subkey),
   **Then** ref resolves to the field value; nested paths supported via dot
   (`${gen.output.meta.author}`).

3. **Given** ref `${select-top.output.missing-key}`,
   **When** flow eval runs,
   **Then** error: `data ref unresolved: select-top.output.missing-key`; message
   lists available keys (`path`, `score`, `metadata`).

4. **Given** ref points at a ctxt object id (`output.ctxt_id`),
   **When** downstream step uses it as input,
   **Then** value passes through as opaque string; type-safe (no coercion);
   downstream runner interprets per its contract.

5. **Given** an `eva` gate step emits `output.passed: true` and
   `output.report_path: /tmp/r.json`,
   **When** downstream step references `${gate.output.passed}` or
   `${gate.output.report_path}`,
   **Then** refs resolve identically to non-gate steps; gate output is just step
   output.

6. **Given** step A references `${B.output.x}` AND step B references
   `${A.output.y}` (cycle),
   **When** flow load/validate runs,
   **Then** load fails: `data ref cycle: A → B → A`; error surfaces before any
   step runs. (Existing `depends_on` DAG check covers the same shape; datarefs
   MUST respect/extend that DAG, not introduce a parallel one.)

7. **Given** step C references `${D.output.x}` but C does NOT declare
   `depends_on: [D]`,
   **When** flow validate runs,
   **Then** error: `data ref to step "D" requires depends_on entry`; load fails.

## Implementation Notes

- Syntax: `${<step-id>.output.<key>[.<subkey>...]}`; escape via `$${...}`.
- Resolution: per-step, just before runner dispatch.
- Validation: at load — missing target step ids OR missing `depends_on` edges fail-fast.
- Step output: each runner declares an `output` map (JSON); recorded in run row
  `step_outputs` blob.
- Cycle detection: extend existing DAG validator; datarefs add edges, stay acyclic.
- No coercion: refs resolve to JSON value; downstream runner converts.

## Tests

### E2E (planned)

- `tests/e2e/flow_datarefs/file_ref_test.go::TestDatarefs_FilePath`
- `tests/e2e/flow_datarefs/json_ref_test.go::TestDatarefs_JSONField`
- `tests/e2e/flow_datarefs/missing_key_test.go::TestDatarefs_MissingKeyError`
- `tests/e2e/flow_datarefs/eva_gate_ref_test.go::TestDatarefs_EvaGateResult`
- `tests/e2e/flow_datarefs/cycle_test.go::TestDatarefs_RespectsDependsOn`

### Unit (planned)

- `internal/core/flow/datarefs_test.go` — parser, resolver, cycle detection, escape.

## Dependencies

- Builds on: 020-flow-execution, 022-sequential-chain-execution.
- Composes with: 025-flow-human-step (human step output usable downstream),
  077-gate-step-eva-contract (eva gate output usable downstream).
- Prereq for: scenario 4a (newsletter-weekly-publish — multi-step pipeline).
