# TLC Specifications (Generated)

## Generated

- Generated at: 2026-01-15T15:38:01Z

## Purpose

This directory contains a complementary spec suite for a task system that supports:
- tasks as durable work units
- deterministic execution
- multi-agent or AI-assisted collaboration
- flow orchestration (sequence, branching, parallelism, joins, retries)
- E2E testability from user stories
- a shared glossary to eliminate coordination ambiguity

## Spec Map (Non-Overlapping Ownership)

- `task-crud-spec-0.1.md`
  - Owns: canonical task data model, persistence rules, and statuses-as-values
  - Does NOT own: execution protocol or agent collaboration policies

- `task-exec-spec-0.1.md`
  - Owns: task execution request/response, runtime status, outputs, errors, timeouts
  - Does NOT own: task claiming/ownership or orchestration graphs

- `task-collab-spec-1.0.md`
  - Owns: agent identity, claiming/reassignment, action vocabulary, handoff rules
  - Does NOT own: canonical task schema fields (CRUD) or orchestration semantics (FLOW)

- `task-flow-spec-0.1.md`
  - Owns: orchestration graph semantics (seq/parallel/join/branch/retry/subflow)
  - Does NOT own: agent claiming or base task schema

- `task-log-spec-0.1.md`
  - Owns: log schema and shared write policy (prepend vs append)
  - Referenced by: CRUD, EXEC, COLLAB, FLOW

- `glossary-0.1.md`
  - Owns: canonical definitions for shared terms and states

## Dependency Layering

Recommended dependency direction:

- FLOW -> EXEC
- COLLAB -> CRUD + LOG
- EXEC -> LOG
- CRUD -> LOG
- FLOW -> LOG
- All -> GLOSSARY

This ensures:
- execution remains deterministic
- collaboration remains auditable
- orchestration remains composable
- shared language remains stable
