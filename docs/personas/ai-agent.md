# Persona P2 — AI Agent

**Description**

An AI agent or autonomous system that integrates with TLC via CLI commands, MCP (Model Context Protocol), or HTTP APIs. This persona claims tasks, transitions them through workflow states, and reports results back to the system. They care about predictable task querying, state transition semantics, atomic operations, and the ability to link work to external execution contexts.

## Primary Stories

- [001 - Task Creation](../stories/001-task-creation.md) (task data available for claiming)
- [002 - Task Listing](../stories/002-task-listing.md) (task data available for querying)
- [003 - Task Claiming](../stories/003-task-claiming.md)
- [004 - Task Management](../stories/004-task-management.md) (update task status to DONE/FAILED)
- [005 - Task Update](../stories/005-task-update.md) (update status; force transitions)
- [007 - Task Reopen](../stories/007-task-reopen.md) (reopen when blocked or regressed)
- [011 - AI Agent Task State Transitions](../stories/011-ai-agent-task-state-transitions.md) (planned)
- [020 - Flow Execution](../stories/020-flow-execution.md) (agent orchestrates flow steps)
- [040 - MCP Integration & Tool Use](../stories/040-mcp-integration-tool-use.md) (planned)
- [042 - CLI Command Integration](../stories/042-cli-command-integration.md) (planned)

## Related Stories

Stories where AI Agent capabilities are leveraged:
