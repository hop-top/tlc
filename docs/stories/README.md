# TLC User Stories

User stories organized by feature area, persona, and priority. Each story defines acceptance criteria and links to test coverage.

## Quick Navigation

### By Feature Area

#### Task Management (001-009)
- [001 - Task Creation](001-task-creation.md) — Create tasks with metadata
- [002 - Task Listing](002-task-listing.md) — Query and filter tasks
- [003 - Task Claiming](003-task-claiming.md) — AI agent task assignment
- [004 - Configurable Statuses](004-configurable-statuses.md) — Custom workflow states

#### External Sync (010-019)
- [010 - GitHub Sync](010-github-sync.md) — Bidirectional GitHub issue sync

#### Flow Orchestration (020-029)
- [020 - Flow Execution](020-flow-execution.md) — Orchestrate multi-task workflows

#### TUI Interface (030-039)
- [030 - TUI Navigation](030-tui-navigation.md) — Terminal UI interaction patterns

#### AI Agent Integration (040-049)
- [040 - MCP Integration & Tool Use](040-mcp-integration-tool-use.md) (planned)
- [042 - CLI Command Integration](042-cli-command-integration.md) (planned)

#### System Validation (060-069)
- [060 - Configuration Validation](060-configuration-validation.md) — Config file validation
- [061 - Environment Setup Verification](061-environment-setup-verification.md) — Environment variable and dependency checks
- [062 - Storage Location Validation](062-storage-location-validation.md) — Storage directory accessibility
- [063 - Hierarchical Config Discovery](063-hierarchical-config-discovery.md) — Nested project config and task merging

#### Specialized Topics
- [user-stories-taskflow-0.1.md](user-stories-taskflow-0.1.md) — Flow execution semantics and edge cases

### By Persona

#### [Solo Developer](../personas/solo-developer.md)
- [001 - Task Creation](001-task-creation.md)
- [002 - Task Listing](002-task-listing.md)
- [004 - Configurable Statuses](004-configurable-statuses.md)
- [020 - Flow Execution](020-flow-execution.md)
- [030 - TUI Navigation](030-tui-navigation.md)

#### [AI Agent](../personas/ai-agent.md)
- [001 - Task Creation](001-task-creation.md) (task data available for claiming)
- [002 - Task Listing](002-task-listing.md) (task data available for querying)
- [003 - Task Claiming](003-task-claiming.md)
- [011 - AI Agent Task State Transitions](011-ai-agent-task-state-transitions.md) (planned)
- [020 - Flow Execution](020-flow-execution.md) (agent orchestrates flow steps)
- [040 - MCP Integration & Tool Use](040-mcp-integration-tool-use.md) (planned)
- [042 - CLI Command Integration](042-cli-command-integration.md) (planned)

#### [Team Lead](../personas/team-lead.md)
- [002 - Task Listing](002-task-listing.md) (filtering and reporting)
- [004 - Configurable Statuses](004-configurable-statuses.md)
- [010 - GitHub Sync](010-github-sync.md)
- [012 - Flow Templates](012-flow-templates.md) (planned)
- [013 - Team Task Assignment](013-team-task-assignment.md) (planned)
- [020 - Flow Execution](020-flow-execution.md) (flow governance)
- [041 - Team Visibility Dashboard](041-team-visibility-dashboard.md) (planned)

#### [Test Engineer](../personas/test-engineer.md)
- [001 - Task Creation](001-task-creation.md) (E2E validation)
- [002 - Task Listing](002-task-listing.md) (query validation)
- [020 - Flow Execution](020-flow-execution.md) (orchestration validation)
- [050 - E2E Test Framework](050-e2e-test-framework.md) (planned)
- [051 - Story-to-Test Traceability](051-story-to-test-traceability.md) (planned)
- [052 - CI Integration & Automated Validation](052-ci-integration-automated-validation.md) (planned)

#### [System](../personas/system.md)
- [001 - Task Creation](001-task-creation.md) (validates task storage before creation)
- [002 - Task Listing](002-task-listing.md) (validates task data location readability)
- [010 - GitHub Sync](010-github-sync.md) (validates external system connectivity)
- [060 - Configuration Validation](060-configuration-validation.md) (planned)
- [061 - Environment Setup Verification](061-environment-setup-verification.md) (planned)
- [062 - Storage Location Validation](062-storage-location-validation.md) (planned)
- [063 - Hierarchical Config Discovery](063-hierarchical-config-discovery.md) (planned)

### By Priority

| P1 (Critical) | P2 (High) | P3+ (Future) |
|---|---|---|
| [001](001-task-creation.md) | [030](030-tui-navigation.md) | [012](012-flow-templates.md) (planned) |
| [002](002-task-listing.md) | [050](050-e2e-test-framework.md) (planned) | [013](013-team-task-assignment.md) (planned) |
| [003](003-task-claiming.md) | | [040](040-mcp-integration-tool-use.md) (planned) | [041](041-team-visibility-dashboard.md) (planned) |
| [010](010-github-sync.md) | | | |
| [020](020-flow-execution.md) | | | [042](042-cli-command-integration.md) (planned) |
| [060](060-configuration-validation.md) | | [061](061-environment-setup-verification.md) (planned) | [062](062-storage-location-validation.md) (planned) |
| [063](063-hierarchical-config-discovery.md) | | | |

## Story Format

Each story file includes:

```markdown
# <Story Title>

**ID**: <number>
**Feature**: <feature area>
**Persona**: [Primary Persona](../personas/solo-developer.md)
**Related Personas**: [Other Personas](...) (optional)
**Priority**: P1-P4

## Story

As a [persona], I want to [capability], so that [benefit].

## Acceptance Scenarios

1. **Given** [precondition], **When** [action], **Then** [outcome]
2. ...

## Tests

### E2E
- `tests/integration/<module>_test.go` — `TestFunctionName`

### Unit
- `internal/<module>/<component>_test.go` — `TestFunctionName`
```

## Traceability

Stories link to:

- **Personas** - Who needs this feature
- **Specifications** - Detailed technical design
- **Tests** - Automated validation (E2E and unit)

Example:
- Story [001 - Task Creation](001-task-creation.md) links to:
  - Persona: [Solo Developer](../personas/solo-developer.md)
  - Spec: [task-crud-spec-0.1.md](../task-crud-spec-0.1.md)
  - Tests: E2E in `tests/integration/task_test.go`, Unit in `internal/core/task_test.go`

## Contributing New Stories

When adding a story:

1. Assign to next available ID in the appropriate feature range
2. Identify primary persona and related personas
3. Set priority relative to existing stories
4. Write acceptance scenarios in Given-When-Then format
5. Reference test locations (create tests concurrently or in follow-up PR)
6. Link to spec documents where applicable
7. Update this README with story index entry

## Status Tracking

- **✅ Complete** - Story written, tests exist
- **🔄 In Progress** - Story drafted, tests being written
- **📋 Planned** - Story title only, detailed work pending

## Related Documentation

- [Personas](../personas/README.md) - User archetypes
- [Specifications](../README.md#spec-map-non-overlapping-ownership) - Technical design documents
- [Development Workflow](../dev-workflow-conventions-0.1.md) - Team conventions
