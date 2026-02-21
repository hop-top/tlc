# Event, Hook, and Agent System Design

**Date**: 2026-02-19
**Status**: Draft
**Target Version**: 0.2.0

## Problem Statement

TLC lacks a way for external systems and agents to react to task lifecycle events. Users cannot:
- Notify Slack/webhooks when tasks are claimed or completed
- Invoke QA/Judge agents as part of workflows
- Build multi-agent orchestration (A2A) where TLC acts as coordinator
- Block state transitions pending external validation

## Goals

1. **Event emission** - Core actions emit typed events with full context
2. **Hook system** - Declarative configuration for reacting to events
3. **Agent invocation** - Call external systems (HTTP, MCP, webhooks) with templated payloads
4. **Workflow orchestration** - DAG-based multi-step agent pipelines
5. **Pre-commit hooks** - Block transitions for validation (Judge pattern)

## Non-Goals

- Real-time WebSocket streaming (v0.2 is polling/webhook-based)
- Distributed event bus (single-node only)
- Event replay/sourcing (logs serve this purpose)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        TLC Core                              │
├─────────────────────────────────────────────────────────────┤
│  TaskService     FlowExecutor      SyncManager              │
│       │               │                 │                    │
│       └───────────────┴─────────────────┘                    │
│                       │                                      │
│               EventEmitter                                   │
│                       │                                      │
├───────────────────────┴─────────────────────────────────────┤
│                   Event Bus (in-process)                    │
├─────────────┬─────────────┬─────────────┬──────────────────┤
│  HookRunner │ AgentClient │ WorkflowEng │  ChangeLogWriter │
├─────────────┴─────────────┴─────────────┴──────────────────┤
│  Agent Types:                                               │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────┐   │
│  │ HTTP     │ │ MCP      │ │ Webhook  │ │ Plugin RPC   │   │
│  └──────────┘ └──────────┘ └──────────┘ └──────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## Configuration Schema

### Configuration Loading

Config supports modular composition via imports:

```yaml
# ~/.config/tlc/config.yaml

# Import agents from separate files (local or remote)
import:
  - ~/.config/tlc/agents/company-agents.yaml    # Shared company agents
  - ~/.config/tlc/agents/personal-agents.yaml   # Personal overrides
  - https://company.com/tlc/agents-team.yaml    # Remote config (cached)

# Agents defined inline
agents:
  slack:
    type: webhook
    endpoint: "https://hooks.slack.com/services/..."
```

Imported files are merged with inline definitions. Later imports override earlier ones. Inline definitions have highest priority.

### Remote Config

```yaml
# Remote configs support:
# - HTTPS with caching (ETag/Last-Modified)
# - Local file paths (absolute or relative to project)
# - Environment variable expansion in paths
```

### Agents

Agents can be defined inline or in separate config files:

```yaml
# ~/.config/tlc/agents/company-agents.yaml
agents:
  qa-reviewer:
    type: http
    endpoint: "https://qa-agent.company.com/invoke"
    auth:
      type: bearer
      secret: "${QA_AGENT_TOKEN}"
    timeout: 5m
    retry:
      count: 3
      backoff: exponential
    
  judge:
    type: http
    endpoint: "https://judge-agent.company.com/evaluate"
    auth:
      type: header
      name: "X-Judge-Key"
      secret: "${JUDGE_SECRET}"
    
  coder:
    type: mcp
    command: "/usr/local/bin/coder-mcp-server"
    
  # Reference to another config file (nested import)
  codegen:
    import: ~/.config/tlc/agents/codegen-team.yaml
```

### Agent Definition Schema

```yaml
agents:
  <name>:
    # Required
    type: http | mcp | webhook | plugin
    
    # HTTP/Webhook types
    endpoint: <url>
    auth:
      type: bearer | basic | header | none
      secret: <secret or ${ENV_VAR}>
      # For 'basic':
      username: <username or ${ENV_VAR}>
      # For 'header':
      name: <header-name>
    
    # MCP type
    command: <executable-path>
    args: [<arg1>, <arg2>]
    env:
      KEY: value
    
    # Optional
    timeout: <duration>           # Default: 30s
    retry:
      count: <n>                  # Default: 3
      backoff: linear | exponential
      initial_delay: <duration>   # Default: 1s
      max_delay: <duration>       # Default: 30s
    
    # Import from another file (instead of inline definition)
    import: <path-to-config.yaml>
```

### Hooks

```yaml
hooks:
  - name: "slack-urgent-claims"
    description: "Notify Slack when urgent tasks are claimed"
    events: ["task.claimed"]
    filter:
      tags: ["urgent", "blocked"]
    agent: slack
    template: |
      🔥 {{.Actor}} claimed urgent task {{.Task.ID}}
      > {{.Task.Title}}
    enabled: true
    
  - name: "qa-on-code-complete"
    events: ["task.completed"]
    filter:
      tags: ["code", "implementation"]
    agent: qa-reviewer
    payload: |
      {
        "task_id": "{{.Task.ID}}",
        "title": "{{.Task.Title}}",
        "pr_url": "{{index .Task.Meta \"pr_url\"}}"
      }
    on_response:
      create_task:
        title: "QA Review: {{.Task.ID}}"
        tags: ["qa", "review"]
        meta:
          parent: "{{.Task.ID}}"
          
  - name: "judge-block-done"
    events: ["task.transitioning"]
    filter:
      to_status: "DONE"
    agent: judge
    blocking: true  # Pre-commit - can reject transition
    payload: |
      {
        "task": {{.Task | json}},
        "logs": {{.RecentLogs | json}}
      }
    on_response:
      reject_if: "{{not .Response.approved}}"
      rejection:
        comment: "Judge rejected: {{.Response.reason}}"
        tag: "needs-revision"
```

### Workflows

```yaml
workflows:
  code-review-pipeline:
    description: "Full code review with QA and Judge"
    trigger:
      events: ["task.completed"]
      filter: {tags: ["code"]}
    
    steps:
      - id: create-pr
        agent: coder
        action: "create_pull_request"
        input:
          task: "{{.Trigger.Task}}"
        output_to:
          task.meta.pr_url: "{{.Result.pr_url}}"
        
      - id: qa-review
        after: [create-pr]
        agent: qa-reviewer
        input:
          pr_url: "{{.Steps.create-pr.Result.pr_url}}"
          
      - id: judge-approve
        after: [qa-review]
        agent: judge
        input:
          task: "{{.Trigger.Task}}"
          qa_result: "{{.Steps.qa-review.Result}}"
        on_success: [mark-task-done]
        on_failure: [request-changes]
```

---

## Event Types

### Task Lifecycle

| Event | When | Blocking | Payload |
|-------|------|----------|---------|
| `task.created` | After CreateTask | No | Task |
| `task.updated` | After UpdateTask | No | Task, ChangedFields |
| `task.claimed` | After ClaimTask | No | Task, Actor |
| `task.unclaimed` | After UnclaimTask | No | Task, Actor |
| `task.transitioning` | Before status change | **Yes** | Task, OldStatus, NewStatus |
| `task.status_changed` | After status change | No | Task, OldStatus, NewStatus |
| `task.completed` | Task reaches DONE | No | Task |
| `task.blocked` | Task gets blocked tag | No | Task, BlockedBy |
| `task.unblocked` | Blocker removed | No | Task |

### Sync

| Event | When | Payload |
|-------|------|---------|
| `sync.started` | Before sync operation | System |
| `sync.pulled` | After pull completes | Tasks, Conflicts |
| `sync.pushed` | After push completes | Tasks |
| `sync.conflict` | Conflict detected | Task, Local, Remote |

### Flow

| Event | When | Payload |
|-------|------|---------|
| `flow.started` | Flow execution begins | Flow, Run |
| `flow.step_started` | Step begins | Flow, Run, Step |
| `flow.step_completed` | Step finishes | Flow, Run, Step, Result |
| `flow.completed` | All steps done | Flow, Run |
| `flow.failed` | Flow errors | Flow, Run, Error |

### Agent

| Event | When | Payload |
|-------|------|---------|
| `agent.invoked` | Before calling agent | Agent, Input |
| `agent.responded` | Agent returns | Agent, Response |
| `agent.failed` | Agent errors | Agent, Error |

---

## Event Payload Structure

```go
type Event struct {
    ID        string                 `json:"id"`
    Type      string                 `json:"type"`
    Timestamp time.Time              `json:"timestamp"`
    Actor     string                 `json:"actor"`
    ProjectID string                 `json:"project_id,omitempty"`
    
    // Task context (for task.* events)
    Task      *Task                  `json:"task,omitempty"`
    OldStatus TaskStatus             `json:"old_status,omitempty"`
    NewStatus TaskStatus             `json:"new_status,omitempty"`
    
    // Sync context
    System    string                 `json:"system,omitempty"`
    
    // Flow context
    Flow      *Flow                  `json:"flow,omitempty"`
    FlowRun   *FlowRun               `json:"flow_run,omitempty"`
    Step      *FlowStep              `json:"step,omitempty"`
    
    // Agent context
    AgentName string                 `json:"agent_name,omitempty"`
    Response  map[string]any         `json:"response,omitempty"`
    Error     string                 `json:"error,omitempty"`
    
    // Generic metadata
    Meta      map[string]any         `json:"meta,omitempty"`
}
```

---

## Implementation Phases

### Phase 1: Event Foundation (2-3 days)

**Goal**: Core event emission and bus infrastructure

#### Tasks

1. **Define event types and payload** (`internal/core/event.go`)
   - Event struct
   - EventType constants
   - Event constructors for each type

2. **Create EventEmitter interface** (`internal/core/emitter.go`)
   ```go
   type EventEmitter interface {
       Emit(ctx context.Context, event *Event) error
       EmitBlocking(ctx context.Context, event *Event) (*EventResult, error)
   }
   ```

3. **Implement EventBus** (`internal/eventbus/bus.go`)
   - Subscribe/unsubscribe with filters
   - Sync fan-out to subscribers
   - Blocking event support (for pre-commit hooks)

4. **Integrate into TaskService** (`internal/core/service.go`)
   - Emit `task.created`, `task.claimed`, `task.unclaimed`
   - Emit `task.transitioning` (blocking) before status change
   - Emit `task.status_changed` after status change

5. **Tests**
   - Event emission in task lifecycle tests
   - EventBus fan-out tests
   - Blocking event tests

#### Deliverables

- [ ] `internal/core/event.go`
- [ ] `internal/eventbus/bus.go`
- [ ] `internal/eventbus/filter.go`
- [ ] Updated `internal/core/service.go`
- [ ] `internal/eventbus/bus_test.go`

---

### Phase 2: Hook System (2-3 days)

**Goal**: Declarative hooks that react to events

#### Tasks

1. **Hook configuration schema** (`internal/config/hooks.go`)
   ```go
   type HookConfig struct {
       Name        string
       Description string
       Events      []string
       Filter      map[string]any
       Agent       string
       Template    string    // Simple string template
       Payload     string    // JSON template
       Blocking    bool
       OnResponse  *HookResponseAction
       Enabled     bool
   }
   ```

2. **HookRunner** (`internal/hooks/runner.go`)
   - Load hooks from config
   - Match events to hooks based on event type + filter
   - Render templates (text/template with sprig functions)
   - Invoke agent client
   - Handle response actions

3. **Filter matching** (`internal/hooks/filter.go`)
   - Tag matching: `tags: ["urgent"]`
   - Status matching: `to_status: "DONE"`
   - Meta field matching: `meta.priority: ["high"]`
   - CEL or simple DSL for complex filters

4. **Template rendering** (`internal/hooks/template.go`)
   - Access to Event, Task, Actor, Timestamp
   - JSON helper: `{{.Task | json}}`
   - Index helper: `{{index .Task.Meta "key"}}`
   - Sprig functions (default, required, etc.)

5. **Response actions** (`internal/hooks/actions.go`)
   - `create_task`: Create follow-up task
   - `add_tag`, `remove_tag`: Modify task tags
   - `set_meta`: Update task metadata
   - `comment`: Add log entry
   - `reject`: For blocking hooks, reject with reason

6. **Tests**
   - Hook matching tests
   - Template rendering tests
   - Filter matching tests
   - Integration tests with mock agents

#### Deliverables

- [ ] `internal/config/hooks.go`
- [ ] `internal/hooks/runner.go`
- [ ] `internal/hooks/filter.go`
- [ ] `internal/hooks/template.go`
- [ ] `internal/hooks/actions.go`
- [ ] `internal/hooks/runner_test.go`

---

### Phase 3: Agent Client (2-3 days)

**Goal**: Invoke external agents (HTTP, MCP, Webhook)

#### Tasks

1. **Agent registry** (`internal/agents/registry.go`)
   - Load agent configs
   - Create clients by type

2. **HTTP Agent Client** (`internal/agents/http.go`)
   - POST with auth (bearer, header, basic)
   - Timeout handling
   - Retry with backoff
   - Response parsing

3. **Webhook Agent Client** (`internal/agents/webhook.go`)
   - Simpler HTTP client for webhooks
   - Slack-formatted payloads
   - Success detection (200-299)

4. **MCP Agent Client** (`internal/agents/mcp.go`)
   - Reuse existing MCP infrastructure
   - Map event to MCP tool call
   - Extract response

5. **Agent client interface**
   ```go
   type AgentClient interface {
       Invoke(ctx context.Context, input map[string]any) (*AgentResponse, error)
   }
   
   type AgentResponse struct {
       Success bool
       Data    map[string]any
       Error   string
   }
   ```

6. **Tests**
   - HTTP client with mock server
   - Webhook client tests
   - MCP client tests (integration)

#### Deliverables

- [ ] `internal/agents/registry.go`
- [ ] `internal/agents/http.go`
- [ ] `internal/agents/webhook.go`
- [ ] `internal/agents/mcp.go`
- [ ] `internal/agents/client_test.go`

---

### Phase 4: Pre-Commit Hooks (1-2 days)

**Goal**: Blocking hooks that can reject transitions

#### Tasks

1. **Blocking event emission** (`internal/eventbus/blocking.go`)
   - `EmitBlocking` waits for all blocking subscribers
   - Collects votes/rejections
   - Returns aggregated result

2. **Rejection handling** (`internal/hooks/rejection.go`)
   - `reject_if` condition evaluation
   - Rollback actions on rejection
   - User feedback (why was it rejected?)

3. **Integration with TaskService**
   ```go
   func (s *TaskService) TransitionStatus(...) error {
       // Emit blocking event
       result, err := s.emitter.EmitBlocking(ctx, &Event{
           Type: EventTaskTransitioning,
           ...
       })
       if err != nil || result.Rejected {
           return fmt.Errorf("transition blocked: %s", result.Reason)
       }
       
       // Proceed with transition
       ...
   }
   ```

4. **Tests**
   - Blocking hook acceptance
   - Blocking hook rejection
   - Multiple blocking hooks (all must pass)

#### Deliverables

- [ ] `internal/eventbus/blocking.go`
- [ ] `internal/hooks/rejection.go`
- [ ] Updated `internal/core/service.go`
- [ ] `internal/eventbus/blocking_test.go`

---

### Phase 5: Workflow Engine (3-4 days)

**Goal**: DAG-based multi-step agent orchestration

#### Tasks

1. **Workflow configuration** (`internal/config/workflow.go`)
   ```go
   type WorkflowConfig struct {
       Name        string
       Description string
       Trigger     WorkflowTrigger
       Steps       []WorkflowStep
   }
   
   type WorkflowStep struct {
       ID         string
       After      []string    // Dependencies
       Agent      string
       Action     string
       Input      map[string]any
       OutputTo   map[string]string
       OnSuccess  []string
       OnFailure  []string
   }
   ```

2. **Workflow execution engine** (`internal/workflow/engine.go`)
   - DAG resolution (topological sort)
   - Parallel step execution
   - Step result passing
   - Error handling and rollback

3. **Workflow triggers** (`internal/workflow/trigger.go`)
   - Event-triggered workflows
   - Filter matching (same as hooks)
   - Manual triggering (`tlc workflow run`)

4. **Step execution** (`internal/workflow/step.go`)
   - Template input rendering
   - Agent invocation
   - Output extraction and storage
   - Success/failure handlers

5. **Workflow state persistence** (`internal/workflow/state.go`)
   - Store workflow runs in DB
   - Resume interrupted workflows
   - Query workflow history

6. **CLI commands**
   ```bash
   tlc workflow list
   tlc workflow show <name>
   tlc workflow run <name> [--input key=value]
   tlc workflow runs [--workflow <name>]
   ```

7. **Tests**
   - DAG resolution tests
   - Step execution tests
   - Parallel execution tests
   - Error handling tests
   - Integration tests

#### Deliverables

- [ ] `internal/config/workflow.go`
- [ ] `internal/workflow/engine.go`
- [ ] `internal/workflow/trigger.go`
- [ ] `internal/workflow/step.go`
- [ ] `internal/workflow/state.go`
- [ ] `internal/cli/workflow.go`
- [ ] `internal/workflow/engine_test.go`

---

### Phase 6: Integration & Polish (2 days)

**Goal**: End-to-end integration, docs, examples

#### Tasks

1. **Config loading** (`internal/config/loader.go`)
   - Load agents, hooks, workflows from config.yaml
   - **Import resolution**: Load referenced config files
   - **Remote config**: HTTPS fetch with caching (ETag support)
   - **Merge strategy**: Later imports override, inline wins
   - Validation and error messages
   - Environment variable expansion in paths

2. **Config merge logic** (`internal/config/merge.go`)
   - Deep merge of maps
   - Array append vs replace (configurable)
   - Conflict detection and warnings

3. **Remote config caching** (`internal/config/remote.go`)
   - Cache in `~/.cache/tlc/config/`
   - ETag/Last-Modified validation
   - TTL-based refresh (default: 1 hour)
   - Offline fallback to cached version

4. **Wire everything together**
   - EventBus subscribes HookRunner
   - HookRunner uses AgentRegistry
   - WorkflowEngine uses EventBus + AgentRegistry
   - TaskService emits events

5. **CLI commands for config and agents**
   ```bash
   # Config management
   tlc config validate                    # Validate all config files
   tlc config show                        # Show merged effective config
   tlc config show --agents               # Show effective agent config
   tlc config resolve <path>              # Resolve imports and show source
   
   # Agent management
   tlc agent list                         # List all configured agents
   tlc agent show <name>                  # Show agent configuration
   tlc agent test <name>                  # Test agent connectivity
   tlc agent test <name> --input @stdin   # Test with custom payload
   
   # Hook management
   tlc hook list                          # List all configured hooks
   tlc hook show <name>                   # Show hook configuration
   tlc hook test <name> --event task.claimed   # Dry-run hook matching
   
   # Workflow management
   tlc workflow list
   tlc workflow show <name>
   tlc workflow run <name> [--input key=value]
   tlc workflow runs [--workflow <name>]
   tlc workflow cancel <run-id>
   ```

6. **Example configurations**
   - Slack notification example
   - QA reviewer + Judge pipeline
   - Multi-file config organization example
   - Shared team agents config

6. **Documentation**
   - Update spec docs
   - Configuration reference
   - Agent development guide
   - Workflow authoring guide
   - Config organization best practices

7. **Migration guide**
   - Backward compatibility notes
   - Config schema changes

#### Deliverables

- [ ] `internal/config/loader.go`
- [ ] `internal/config/merge.go`
- [ ] `internal/config/remote.go`
- [ ] `internal/cli/config.go`
- [ ] `internal/cli/agent.go`
- [ ] `internal/cli/hooks.go`
- [ ] `internal/cli/workflow.go`
- [ ] `examples/configs/`
- [ ] `docs/event-hook-system.md`
- [ ] `docs/agent-development.md`
- [ ] `docs/workflow-authoring.md`
- [ ] `docs/config-organization.md`

---

## File Structure

```
internal/
├── core/
│   ├── event.go           # Event types and payloads
│   ├── emitter.go         # EventEmitter interface
│   └── service.go         # Updated with event emission
├── eventbus/
│   ├── bus.go             # EventBus implementation
│   ├── blocking.go        # Blocking event support
│   ├── filter.go          # Event filtering
│   └── bus_test.go
├── hooks/
│   ├── runner.go          # HookRunner
│   ├── filter.go          # Hook filter matching
│   ├── template.go        # Template rendering
│   ├── actions.go         # Response actions
│   ├── rejection.go       # Blocking hook rejection
│   └── runner_test.go
├── agents/
│   ├── registry.go        # Agent registry
│   ├── http.go            # HTTP agent client
│   ├── webhook.go         # Webhook agent client
│   ├── mcp.go             # MCP agent client
│   └── client_test.go
├── workflow/
│   ├── engine.go          # Workflow engine
│   ├── trigger.go         # Event triggers
│   ├── step.go            # Step execution
│   ├── state.go           # State persistence
│   └── engine_test.go
├── config/
│   ├── hooks.go           # Hook config schema
│   ├── workflow.go        # Workflow config schema
│   ├── loader.go          # Config loading with imports
│   ├── merge.go           # Config merge logic
│   ├── remote.go          # Remote config fetching + caching
│   └── loader_test.go
└── cli/
    ├── config.go           # Config validation and inspection
    ├── agent.go            # Agent management and testing
    ├── hooks.go            # Hook management CLI
    └── workflow.go         # Workflow CLI commands

examples/
└── configs/
    ├── minimal.yaml           # Simple Slack webhook
    ├── team-shared/
    │   ├── config.yaml        # Main config with imports
    │   ├── agents.yaml        # Shared team agents
    │   └── workflows.yaml     # Team workflows
    └── a2a-pipeline/
        ├── config.yaml
        ├── agents/
        │   ├── qa.yaml
        │   └── judge.yaml
        └── workflows/
            └── code-review.yaml
```

---

## Dependencies

### New Dependencies

- `github.com/Masterminds/sprig/v3` - Template functions
- No other new dependencies (use stdlib HTTP, existing MCP)

### Existing Dependencies

- `gopkg.in/yaml.v3` - Config parsing
- Existing MCP infrastructure
- Existing storage layer

---

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Blocking hooks cause UI lag | Async by default, blocking only for validation |
| Agent failures break workflows | Retry with backoff, failure handlers |
| Complex filter syntax | Start simple, add CEL later if needed |
| Template security | Sandbox templates, limit functions |
| Config complexity | Good defaults, examples, validation |

---

## Success Criteria

1. ✅ Can configure Slack webhook for task claims
2. ✅ Can invoke HTTP agent on task completion
3. ✅ Judge can block DONE transitions
4. ✅ Workflow can chain QA → Judge → Merge
5. ✅ All events logged to CHANGELOG
6. ✅ Tests cover core paths

---

## Future Enhancements (v0.3+)

- WebSocket streaming for real-time UI
- Event replay for debugging
- Distributed event bus (NATS, Redis)
- Visual workflow editor
- Agent marketplace/registry
- Metrics and observability
