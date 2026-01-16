# TLC Agent Tool Definition

This document provides the standard tool definition for AI agents to interact with the TLC (Task Line CLI). Agents should use this tool to manage their internal planning and work breakdown.

## Tool: `manage_tlc_task`

Use this tool to create, list, or update tasks in the TLC system. This allows you to track your own work and maintain an audit trail of your progress without polluting external systems like GitHub or Jira.

### Parameters (JSON Schema)

```json
{
  "name": "manage_tlc_task",
  "description": "Create, list, or update tasks in the TLC (Task Line CLI) system for internal planning and tracking.",
  "parameters": {
    "type": "object",
    "properties": {
      "action": {
        "type": "string",
        "enum": ["create", "list", "update", "show", "claim", "unclaim"],
        "description": "The action to perform on tasks."
      },
      "task_id": {
        "type": "string",
        "description": "The unique ID of the task (e.g., 'T-0042'). Required for 'update' and 'show'."
      },
      "title": {
        "type": "string",
        "description": "The title of the task. Required for 'create'."
      },
      "status": {
        "type": "string",
        "enum": ["TODO", "IN_PROGRESS", "DONE", "SKIPPED"],
        "description": "The status of the task."
      },
      "assigned_to": {
        "type": "string",
        "description": "The username of the assignee (e.g., 'engineer-1')."
      },
      "tags": {
        "type": "array",
        "items": { "type": "string" },
        "description": "A list of tags for categorization (e.g., ['infra', 'bug'])."
      },
      "description": {
        "type": "string",
        "description": "A detailed description of the task."
      }
    },
    "required": ["action"]
  }
}
```

### Internal Implementation (Reference)

When an agent calls this tool, the orchestrator should execute the corresponding `tlc` CLI command.

#### Example: Creating a task
**Tool Call:**
```json
{
  "action": "create",
  "title": "Implement authentication middleware",
  "assigned_to": "engineer-1",
  "tags": ["auth", "security"]
}
```
**Command Executed:**
```bash
tlc task create "Implement authentication middleware" --assigned-to engineer-1 --tag auth --tag security
```

#### Example: Marking a task as in-progress
**Tool Call:**
```json
{
  "action": "update",
  "task_id": "T-0042",
  "status": "IN_PROGRESS"
}
```
**Command Executed:**
```bash
tlc task update T-0042 --status IN_PROGRESS
```

#### Example: Listing my pending tasks
**Tool Call:**
```json
{
  "action": "list",
  "status": "TODO",
  "assigned_to": "engineer-1"
}
```
**Command Executed:**
```bash
tlc task list --status TODO --assigned-to engineer-1
```

#### Example: Claiming a task
**Tool Call:**
```json
{
  "action": "claim",
  "task_id": "T-0042"
}
```
**Command Executed:**
```bash
tlc task claim T-0042
```

#### Example: Releasing a task
**Tool Call:**
```json
{
  "action": "unclaim",
  "task_id": "T-0042"
}
```
**Command Executed:**
```bash
tlc task unclaim T-0042
```

## Best Practices for Agents

1.  **Plan First**: Before starting a complex task, use the `create` action to break it down into smaller sub-tasks.
2.  **Stay Updated**: Always move a task to `IN_PROGRESS` when you start working on it, and to `DONE` when finished.
3.  **Use Tags**: Apply domain tags (e.g., `#storage`, `#cli`, `#sync`) to help teammates filter and understand your work.
4.  **Reference IDs**: When committing code or sending messages, refer to the Task IDs (e.g., "Refs: tlc/T-0042") to maintain a clear link between planning and execution.
5.  **Self-Update**: When TLC is updated, refresh your tool knowledge by running `tlc help llm` which auto-detects your environment and outputs format-specific instructions.
