# TLC Documentation

## Updated

- Last updated: 2026-02-17
- Generated at: 2026-01-17T21:40:35Z

## Purpose

This directory contains comprehensive documentation for TLC (Task Line CLI), a task orchestration system that supports:
- tasks as durable work units
- deterministic execution
- multi-agent or AI-assisted collaboration
- recipes: versioned templates that materialize into a track and its tasks
- recipe execution with dependency batches, conditions, retries and gates
- capability-based assignment to specialized executors
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
  - Example stories: [Task Creation](stories/001-task-creation.md), [Task Listing](stories/002-task-listing.md), [Task Assignment](stories/008-task-assignment.md)

### Design Documents

- [TUI Refactoring Design](plans/2026-01-16-tui-refactoring-design.md) - Terminal UI architecture

## Features & Guides

- [Recipes & Assignees](recipes.md) - Recipe templates, execution and capability-based assignment
- [Policies](policies.md) - Declarative state-change guards (`--note` enforcement, custom rules) via kit/runtime/policy
- [Development Setup](development-setup.md) - Development workflow, watch modes, and tooling
- [Editor Setup](editor-setup.md) - IDE/editor integration guides
- [Docker Usage](docker.md) - Container deployment and usage

## Design Documents

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
  - Does NOT own: canonical task schema fields (CRUD) or recipe semantics (RECIPE)

- `recipe-spec-0.1.md`
  - Owns: recipe grammar — steps, kinds, `depends_on`, `when`, retries, gates, includes, loops, vars and templates
  - Does NOT own: agent claiming or base task schema

- `task-log-spec-0.1.md`
  - Owns: log schema and shared write policy (prepend vs append)
  - Referenced by: CRUD, EXEC, COLLAB, RECIPE

- `glossary-0.1.md`
  - Owns: canonical definitions for shared terms and states

## Dependency Layering

Recommended dependency direction:

- RECIPE -> EXEC
- COLLAB -> CRUD + LOG
- EXEC -> LOG
- CRUD -> LOG
- RECIPE -> LOG
- All -> GLOSSARY

This ensures:
- execution remains deterministic
- collaboration remains auditable
- orchestration remains composable
- shared language remains stable
