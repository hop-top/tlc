---
status: shipped-no-e2e
---

# US-NL-004 - Context Dump

**ID**: US-NL-004
**Feature**: NL Prompt
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI Agent, I want to run `tlc prompt task T-0042` and
receive an enriched markdown context dump with dependencies,
track info, and audit history, so that I can feed structured
task context into LLM prompts without scraping show output.

## Acceptance Scenarios

1. **Given** task T-0042 exists with deps and track,
   **When** I run `tlc prompt task T-0042`,
   **Then** output is markdown containing:
   - Task fields (title, status, assignee, priority, tags)
   - Track name, status, and current phase
   - Blocked-by summaries (ID, title, status)
   - Blocking summaries (ID, title, status)
   - Recent audit log entries

2. **Given** task T-0042 exists,
   **When** I run `tlc task prompt T-0042 --json`,
   **Then** output is a flat JSON object with fields:
   `id`, `title`, `status`, `description`, `assigned_to`,
   `tags`, `priority`, `effort`, `blocked_by`, `blocking`,
   `track`, `recent_activity`.
   Empty/nil fields are omitted (`omitempty`).

3. **Given** task T-9999 does not exist,
   **When** I run `tlc prompt task T-9999`,
   **Then** error:
   `task T-9999 not found; run 'tlc task list' to see
   available tasks`

4. **Given** task T-0042 has no dependencies or track,
   **When** I run `tlc prompt task T-0042`,
   **Then** dependency and track sections are omitted
   (not empty placeholders).

5. **Given** task T-0042 has audit entries in description,
   **When** I run `tlc prompt task T-0042`,
   **Then** markdown `## Recent Activity` section lists
   entries with timestamp, actor, and action.

## Tests

### E2E
- `tests/integration/task_prompt_test.go`
  - `TestContextDump_Markdown`
  - `TestContextDump_JSON`
  - `TestContextDump_NotFound`

### Unit
- `internal/cli/prompt_context_test.go`
  - `TestContextDump_FullMarkdown`
  - `TestContextDump_JSONFormat`
  - `TestContextDump_NoDeps`
  - `TestContextDump_TaskNotFound`
