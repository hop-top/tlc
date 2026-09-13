# TLC Personas

User personas for TLC requirements and design.

## Current Personas

- [P1 — Solo Developer](solo-developer.md) - Individual contributor
- [P2 — AI Agent](ai-agent.md) - Autonomous system
- [P3 — Team Lead](team-lead.md) - Project manager
- [P4 — Test Engineer](test-engineer.md) - QA specialist
- [P5 — System](system.md) - Configuration and environment validation

## Purpose

Personas provide user-centered context for:

- **User story development** - Stories are written from specific persona perspectives
- **Feature prioritization** - Features are ranked by impact on primary personas
- **UX design decisions** - Navigation, commands, and workflows are designed for persona needs
- **Test scenario creation** - Acceptance tests are written from persona viewpoints
- **Requirements traceability** - Stories link personas to acceptance criteria and tests

## Persona Overview

| Persona | Role | Focus Areas |
|---------|------|------------|
| [P1 — Solo Developer](solo-developer.md) | Individual contributor | Task CRUD, recipe execution, local workflows |
| [P2 — AI Agent](ai-agent.md) | Autonomous system | CLI commands, task claiming, state transitions, MCP/API integration |
| [P3 — Team Lead](team-lead.md) | Project manager | Multi-project sync, team visibility, governance |
| [P4 — Test Engineer](test-engineer.md) | QA specialist | Test coverage, validation, CI integration |
| [P5 — System](system.md) | Configuration validator | Config paths, storage locations, environment setup, runtime invariants |

## Versioning

Personas follow semantic versioning:

- **v0.1** - Initial personas based on APS archetypes, adapted to TLC domain
- Future versions will add or refine personas based on product feedback

## Related Documentation

- [User Stories](../stories/README.md) - Stories organized by persona and feature
- [Specifications](../task-crud-spec-0.1.md) - Technical specifications
- [Development Workflow](../dev-workflow-conventions-0.1.md) - Team conventions
