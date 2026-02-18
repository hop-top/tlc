# TLC Documentation

## Updated

- Last updated: 2026-02-17
- Generated at: 2026-01-17T21:40:35Z

## Purpose

This directory contains comprehensive documentation for TLC (Task Line CLI), a task orchestration system that supports:
- tasks as durable work units
- deterministic execution
- multi-agent or AI-assisted collaboration
- flow orchestration (sequence, branching, parallelism, joins, retries)
- workflow automation with capability-based assignment (flows & assignees)
- E2E testability from user stories
- a shared glossary to eliminate coordination ambiguity

## Features & Guides

### System Validation

The System persona (P5) validates configuration and environment at startup to ensure TLC operates correctly.

**Related Stories:**
- [Configuration Validation](stories/060-configuration-validation.md) - Validates config file structure, values, and paths
- [Environment Setup Verification](stories/061-environment-setup-verification.md) - Validates environment variables and external dependencies
- [Storage Location Validation](stories/062-storage-location-validation.md) - Validates storage directory accessibility and disk space

## Requirements & Design

### Personas
- `personas/` - User personas and actor definitions
  - [Personas v0.1](personas/README.md) - Primary user archetypes (Solo Developer, AI Agent, Team Lead, Test Engineer, System)

### User Stories
- `stories/` - User stories organized by feature area
  - [User Stories Index](stories/README.md) - All stories with quick navigation by feature, persona, and priority
  - Example stories: [Task Creation](stories/001-task-creation.md), [Task Listing](stories/002-task-listing.md), [Flow Execution](stories/020-flow-execution.md)

### Design Documents

- [Flows & Assignees Design](plans/2026-01-17-flows-and-assignees-design.md) - Complete design for workflow automation system
- [TUI Refactoring Design](plans/2026-01-16-tui-refactoring-design.md) - Terminal UI architecture

## Spec Map (Non-Overlapping Ownership)

- [Flows & Assignees](flows-and-assignees.md) - Workflow automation and capability-based task assignment
- [Development Setup](development-setup.md) - Development workflow, watch modes, and tooling
- [Editor Setup](editor-setup.md) - IDE/editor integration guides
- [Docker Usage](docker.md) - Container deployment and usage

## Design Documents

- [Flows & Assignees Design](plans/2026-01-17-flows-and-assignees-design.md) - Complete design for workflow automation system
- [TUI Refactoring Design](plans/2026-01-16-tui-refactoring-design.md) - Terminal UI architecture

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
