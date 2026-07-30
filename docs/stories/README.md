# TLC User Stories

User stories organized by feature area, persona, and priority. Each story defines acceptance criteria and links to test coverage.

Authoring convention: see [Story → e2e test linkage](../conventions/stories.md).

## Quick Navigation

### By Feature Area

#### Task Management (001-009)
- [001 - Task Creation](001-task-creation.md) — Create tasks with metadata
- [002 - Task Listing](002-task-listing.md) — Query and filter tasks
- [003 - Task Claiming](003-task-claiming.md) — AI agent task assignment
- [004 - Configurable Statuses](004-configurable-statuses.md) — Custom workflow states
- [005 - Task Update](005-task-update.md) — Update title, description, tags, assignee, status
- [006 - Task Deletion](006-task-deletion.md) — Delete with `--yes` confirmation skip
- [007 - Task Reopen](007-task-reopen.md) — Reopen terminal tasks with mandatory `--note`
- [008 - Task Assignment](008-task-assignment.md) — Assign to profile; unassign with `--note`
- [009 - Task Audit Log](009-task-audit-log.md) — `tlc log`; filter by action/actor; paginate

#### External Sync (010-019)
- [010 - GitHub Sync](010-github-sync.md) — Bidirectional GitHub issue sync

#### Flow Orchestration (020-029, 077-079)
- [020 - Flow Execution](020-flow-execution.md) — Orchestrate multi-task workflows
- [077 - Conditional Branching](077-conditional-branching.md) — Input-driven step selection
- [078 - Flow Composition](078-flow-composition.md) — Sub-flow invocation
- [079 - Retry Timeout Error Handling](079-retry-timeout-error-handling.md)
  — Retry, timeout, teardown

#### TUI Interface (030-039)
- [030 - TUI Navigation](030-tui-navigation.md) — Terminal UI interaction patterns

#### AI Agent Integration (040-049)
- [040 - MCP Integration & Tool Use](040-mcp-integration-tool-use.md) (planned)
- [042 - CLI Command Integration](042-cli-command-integration.md) (planned)

#### System Validation (060-069)
- [060 - Configuration Validation](060-configuration-validation.md) (in progress) —
  Config file validation
- [061 - Environment Setup Verification](061-environment-setup-verification.md)
  (in progress) — Environment variable and dependency checks
- [062 - Storage Location Validation](062-storage-location-validation.md) — Storage directory accessibility
- [063 - Hierarchical Config Discovery](063-hierarchical-config-discovery.md)
  (in progress) — Nested project config and task merging

#### Error UX (065-069)
- [065 - Actionable Error Messages](065-actionable-error-messages.md) — Agent-first errors with next steps

#### Track Management (070-079)
- [070 - Track Creation & Lifecycle](070-track-creation-lifecycle.md)
  — Create, update, archive, abandon, delete tracks
- [071 - Track Listing & Detail](071-track-listing-detail.md)
  — Query and inspect tracks
- [072 - Task-Track Integration](072-task-track-integration.md)
  — Link tasks to tracks, auto-transitions
- [073 - Project Health & Plan Linkage](073-project-health-plan-linkage.md)
  — Health metrics, plan extraction

#### HTTP API (090-099)
- [090 - HTTP API Server Lifecycle](090-http-api-server-lifecycle.md)
  — Health check, startup/shutdown, auth-gated shutdown
- [091 - HTTP API Task Operations](091-http-api-task-operations.md)
  — List/create/show/update/claim/complete over HTTP
- [092 - HTTP API Track Operations](092-http-api-track-operations.md)
  — List/create/show over HTTP, 404-vs-422 resolve distinction

#### Specialized Topics
- [user-stories-taskflow-0.1.md](user-stories-taskflow-0.1.md)
  — Flow execution semantics and edge cases

### By Persona

#### [Solo Developer](../personas/solo-developer.md)
- [001 - Task Creation](001-task-creation.md)
- [002 - Task Listing](002-task-listing.md)
- [004 - Configurable Statuses](004-configurable-statuses.md)
- [005 - Task Update](005-task-update.md)
- [006 - Task Deletion](006-task-deletion.md)
- [020 - Flow Execution](020-flow-execution.md)
- [030 - TUI Navigation](030-tui-navigation.md)
- [070 - Track Creation & Lifecycle](070-track-creation-lifecycle.md)
- [071 - Track Listing & Detail](071-track-listing-detail.md)

#### [AI Agent](../personas/ai-agent.md)
- [001 - Task Creation](001-task-creation.md) (task data available for claiming)
- [002 - Task Listing](002-task-listing.md) (task data available for querying)
- [003 - Task Claiming](003-task-claiming.md)
- [005 - Task Update](005-task-update.md) (update task status to DONE/FAILED)
- [007 - Task Reopen](007-task-reopen.md) (reopen when blocked or regressed)
- [011 - AI Agent Task State Transitions](011-ai-agent-task-state-transitions.md) (planned)
- [065 - Actionable Error Messages](065-actionable-error-messages.md)
- [066 - Stale Timeout and Blocked Reason Flags](066-stale-blocked-flags.md)
- [020 - Flow Execution](020-flow-execution.md) (agent orchestrates flow steps)
- [040 - MCP Integration & Tool Use](040-mcp-integration-tool-use.md) (planned)
- [042 - CLI Command Integration](042-cli-command-integration.md) (planned)
- [070 - Track Creation & Lifecycle](070-track-creation-lifecycle.md)
- [071 - Track Listing & Detail](071-track-listing-detail.md)
- [072 - Task-Track Integration](072-task-track-integration.md)
- [090 - HTTP API Server Lifecycle](090-http-api-server-lifecycle.md)
- [091 - HTTP API Task Operations](091-http-api-task-operations.md)
- [092 - HTTP API Track Operations](092-http-api-track-operations.md)

#### [Team Lead](../personas/team-lead.md)
- [002 - Task Listing](002-task-listing.md) (filtering and reporting)
- [004 - Configurable Statuses](004-configurable-statuses.md)
- [007 - Task Reopen](007-task-reopen.md)
- [008 - Task Assignment](008-task-assignment.md)
- [009 - Task Audit Log](009-task-audit-log.md)
- [010 - GitHub Sync](010-github-sync.md)
- [012 - Flow Templates](012-flow-templates.md) (planned)
- [013 - Team Task Assignment](013-team-task-assignment.md) (planned)
- [020 - Flow Execution](020-flow-execution.md) (flow governance)
- [041 - Team Visibility Dashboard](041-team-visibility-dashboard.md) (planned)
- [070 - Track Creation & Lifecycle](070-track-creation-lifecycle.md)
- [071 - Track Listing & Detail](071-track-listing-detail.md)
- [072 - Task-Track Integration](072-task-track-integration.md)
- [073 - Project Health & Plan Linkage](073-project-health-plan-linkage.md)

#### [Test Engineer](../personas/test-engineer.md)
- [001 - Task Creation](001-task-creation.md) (E2E validation)
- [002 - Task Listing](002-task-listing.md) (query validation)
- [009 - Task Audit Log](009-task-audit-log.md) (audit log validation)
- [020 - Flow Execution](020-flow-execution.md) (orchestration validation)
- [050 - E2E Test Framework](050-e2e-test-framework.md) (planned)
- [051 - Story-to-Test Traceability](051-story-to-test-traceability.md) (planned)
- [052 - CI Integration & Automated Validation](052-ci-integration-automated-validation.md) (planned)

#### [System](../personas/system.md)
- [001 - Task Creation](001-task-creation.md) (validates task storage before creation)
- [002 - Task Listing](002-task-listing.md) (validates task data location readability)
- [010 - GitHub Sync](010-github-sync.md) (validates external system connectivity)
- [060 - Configuration Validation](060-configuration-validation.md) (in progress)
- [061 - Environment Setup Verification](061-environment-setup-verification.md)
  (in progress)
- [062 - Storage Location Validation](062-storage-location-validation.md) (planned)
- [063 - Hierarchical Config Discovery](063-hierarchical-config-discovery.md)
  (in progress)

### By Priority

| P1 (Critical) | P2 (High) | P3+ (Future) |
|---|---|---|
| [001](001-task-creation.md) | [006](006-task-deletion.md) | [012](012-flow-templates.md) (planned) |
| [002](002-task-listing.md) | [030](030-tui-navigation.md) | [013](013-team-task-assignment.md) (planned) |
| [003](003-task-claiming.md) | [050](050-e2e-test-framework.md) (planned) | [040](040-mcp-integration-tool-use.md) (planned) |
| [005](005-task-update.md) | | [041](041-team-visibility-dashboard.md) (planned) |
| [007](007-task-reopen.md) | | [042](042-cli-command-integration.md) (planned) |
| [008](008-task-assignment.md) | | |
| [009](009-task-audit-log.md) | | |
| [010](010-github-sync.md) | | |
| [020](020-flow-execution.md) | | |
| [060](060-configuration-validation.md) | | [061](061-environment-setup-verification.md) (planned) |
| [062](062-storage-location-validation.md) | | |
| [063](063-hierarchical-config-discovery.md) | | |
| [065](065-actionable-error-messages.md) | | |
| [070](070-track-creation-lifecycle.md) | | |

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
