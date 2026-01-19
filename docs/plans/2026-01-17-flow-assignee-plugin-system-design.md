# Flow & Assignee Plugin System Design

**Date**: 2026-01-17
**Status**: Design Document
**Related**: [Flows & Assignees](../flows-and-assignees.md), [Plugin Spec](../tlc-plugin-spec-0.1.md)

## Overview

This document describes the plugin system for extending TLC's Flow & Assignee functionality, enabling users to install workflow automation bundles from the community.

## Objective

Create a plugin architecture that allows users to extend TLC with:
- **Flows**: Contextual workflow templates that generate task hierarchies
- **Assignees**: Capability-based task routers with execution or orchestration abilities
- **Executors**: Optional execution backends for automated task completion

Supporting "very different" plugin types - from simple executors to complex orchestrators to result collectors.

## Design Principles

1. **Hybrid distribution** (Local + Git + Registry)
   - Install locally: `tlc plugin install ./my-plugin`
   - Install from Git: `tlc plugin install github.com/user/aws-flows`
   - Install from registry (future): `tlc plugin install @registry/aws-deployment`

2. **Manifest-driven registration** - Single source of truth
   - Plugin declares what it provides (flows, assignees, executors)
   - Namespace isolation prevents ID conflicts
   - Clear dependency declarations

3. **Lazy activation** - Fast startup, load on first use
   - Manifest parsed on install, plugin code loaded on demand
   - First `tlc flow invoke plugin:flow` triggers plugin activation
   - Cached for subsequent uses

4. **Context-aware execution** - Two invocation modes
   - Lightweight: `tlc flow invoke aws:deploy --env staging --image app:v1`
   - Tracked: `tlc flow invoke aws:deploy --for T-100`

5. **Execution spectrum** - Assignees range from simple to complex
   - **Executors**: Wrapper around executor plugin (just execute)
   - **Orchestrators**: Break down work, delegate to other assignees
   - **Collectors**: Gather and aggregate results from sub-tasks (pattern, not type)

---

## High-Level Architecture

### Plugin Components

A workflow bundle plugin can provide:

```
aws-deployment/
├── manifest.yaml           # Plugin metadata and registration
├── flows/                  # Workflow templates
│   ├── deploy-to-eks.yaml
│   └── rollback.yaml
├── assignees/              # Task routers/executors
│   ├── aws-cli-executor.yaml
│   └── deployment-orchestrator.yaml
└── bin/                    # Optional executors
    └── aws-executor
```

### Installation Flow

```
User runs: tlc plugin install github.com/user/aws-flows

1. Fetch    → Download plugin to temp location
2. Validate → Parse manifest.yaml, check schema
3. Resolve  → Verify dependencies (TLC version, system tools)
4. Install  → Copy to ~/.config/tlc/plugins/aws-deployment/
5. Register → Add to plugin registry (lightweight index)
```

### Lazy Loading

```yaml
# ~/.config/tlc/plugin-registry.yaml (lightweight index)
plugins:
  aws-deployment:
    version: 1.0.0
    flows: [flow:aws:deploy-to-eks:1.0, flow:aws:rollback:1.0]
    assignees: [assignee:aws-cli:1.0, assignee:deployment-orchestrator:1.0]
    status: installed
    loaded: false  # Not yet activated
```

On first invocation:
```bash
tlc flow invoke aws:deploy --for T-100
# → Registry lookup: which plugin provides "flow:aws:deploy"?
# → Load aws-deployment plugin (parse YAMLs, validate)
# → Cache in memory for subsequent uses
# → Execute flow
```

---

## Plugin Manifest Structure

Every plugin provides a `manifest.yaml`:

```yaml
name: aws-deployment
version: 1.0.0
description: AWS EKS deployment automation

# What this plugin provides
provides:
  flows:
    - id: "flow:aws:deploy-to-eks:1.0"
      file: "flows/deploy-to-eks.yaml"
      description: "Deploy containerized app to EKS"

    - id: "flow:aws:rollback:1.0"
      file: "flows/rollback.yaml"
      description: "Rollback failed deployment"

  assignees:
    - id: "assignee:aws-cli:1.0"
      file: "assignees/aws-cli-executor.yaml"
      execution_type: executor
      description: "Execute AWS CLI commands"

    - id: "assignee:deployment-orchestrator:1.0"
      file: "assignees/deployment-orchestrator.yaml"
      execution_type: orchestrator
      description: "Coordinate multi-stage deployments"
      patterns: [delegation, monitoring, rollback]

  executors:
    - id: "executor:aws-cli:1.0"
      entry_point: "./bin/aws-executor"
      protocol: jsonrpc
      capabilities:
        task_types: ["aws-command", "aws-deploy"]

# System dependencies
requires:
  tlc_version: ">=0.2.0"
  system:
    - aws-cli >= 2.0
    - kubectl >= 1.25

# Permissions required
permissions:
  - network.http
  - process.spawn
  - credential.read

# Plugin metadata
author: "AWS Community"
license: "MIT"
repository: "github.com/tlc-plugins/aws-deployment"
```

### Key Manifest Elements

**`provides` section**: Explicit registry of what plugin provides
- Flows: Workflow templates with file paths
- Assignees: Task routers/executors with execution type
- Executors: Optional JSON-RPC executors

**Namespaced IDs**: `flow:aws:*`, `assignee:aws:*` prevents conflicts

**Execution type**: Documents how assignee handles work
- `executor`: Wraps an executor, directly executes tasks
- `orchestrator`: Delegates to other assignees, coordinates work

**Patterns field**: Optional behavioral hints
- `delegation`: Breaks work into subtasks
- `aggregation`: Collects results from subtasks
- `monitoring`: Tracks progress/health
- `reporting`: Generates summaries
- `rollback`: Can reverse operations

**No `type` field**: The `provides` section is self-documenting

---

## Context Flow & Task Hierarchy

### Two Invocation Modes

**Mode 1: Lightweight (inline context)**
```bash
tlc flow invoke aws:deploy --env staging --image myapp:v1.2.3
```
- Flow receives context as key-value pairs
- Creates tasks with interpolated context
- Tasks are independent (no parent)

**Mode 2: Tracked (parent task)**
```bash
# 1. Create parent work item
tlc task create "Deploy myapp v1.2.3 to staging" --id T-100

# 2. Flow operates on parent
tlc flow invoke aws:deploy --for T-100

# 3. Creates hierarchical subtasks
T-100 (parent: Deploy myapp v1.2.3 to staging)
├── T-100-1: Validate deployment config
├── T-100-2: Build and push container image
├── T-100-3: Deploy to EKS cluster
└── T-100-4: Run smoke tests
```

### Context Extraction

**For tracked mode:**
- Flow reads parent task title/description
- Parses context (e.g., "myapp v1.2.3", "staging")
- Can access parent task metadata for structured context
- Subtasks inherit parent's context + flow-specific details

**Task metadata structure:**
```yaml
# In subtask metadata
meta:
  parent_task_id: "T-100"
  flow_id: "flow:aws:deploy:1.0"
  flow_run_id: "run:abc123"
  context:
    environment: "staging"
    image: "myapp"
    version: "v1.2.3"
```

### Fine-Grained Context Control

Context security through **dual-gate model**:

**1. Assignee-level policies (what assignee CAN access)**
```yaml
# In assignee definition
assignee_id: "assignee:deployment-orchestrator:1.0"

context_policy:
  # What context this assignee can read
  reads:
    - parent_task.title
    - parent_task.meta.environment
    - parent_task.meta.image

  # What context it passes to delegated tasks
  delegates:
    - environment
    - image

  # What it collects from subtasks
  collects:
    - test_results.*
    - deployment_status

  # What context it creates/adds
  creates:
    - deployment_id
    - cluster_endpoint
```

**2. Delegation-time filtering (what actually GETS passed)**
```yaml
# In flow step when creating subtask
steps:
  deploy:
    type: task
    task_template:
      title: "Deploy to cluster"
      context:
        inherit: [environment, image]     # Explicit whitelist
        exclude: [aws_secret_key]          # Explicit blacklist
        add:
          deployment_strategy: "rolling"   # New context
```

Both must align for context to flow. This provides:
- **Capability declaration**: What assignee can access
- **Least privilege**: Only pass what's needed
- **Security**: Prevent secret leakage

---

## Assignee Execution Models

### The Execution Spectrum

Assignees fall into patterns based on how they handle work:

### 1. Executor Assignees (Simple)

**Purpose**: Wrap existing executor plugins, directly execute tasks

```yaml
assignee_id: "assignee:aws-cli:1.0"
execution_type: executor

executor:
  plugin: "executor:aws-cli:1.0"
  entry_point: "./bin/aws-executor"

context_policy:
  reads: [parent_task.meta.aws_command]
  creates: [execution_output, exit_code]
```

**Behavior:**
- TLC invokes wrapped executor
- Passes task context via JSON-RPC
- Executor runs, returns result
- Task updated with output

### 2. Orchestrator Assignees (Complex)

**Purpose**: Break down work, delegate to specialized assignees

```yaml
assignee_id: "assignee:deployment-orchestrator:1.0"
execution_type: orchestrator
patterns: [delegation]

context_policy:
  reads: [parent_task.*]
  delegates: [environment, image, version]
  creates: [subtask_ids, deployment_plan]
```

**Behavior:**
- Breaks work into subtasks
- Delegates to other assignees
- Monitors subtask progress
- Does NOT execute directly

### 3. Collector Assignees (Aggregator Pattern)

**Purpose**: Gather and aggregate results from multiple subtasks

```yaml
assignee_id: "assignee:test-suite:1.0"
execution_type: orchestrator
patterns: [delegation, aggregation, reporting]

context_policy:
  reads: [parent_task.meta.test_paths]
  delegates: [test_type, test_path]
  collects: [test_results.*, coverage_data.*]
  creates: [aggregated_report, overall_status]
```

**Behavior:**
- Creates subtasks for each test type
- Delegates to test executors
- **Collects** results from all subtasks
- **Aggregates** into summary report
- Updates parent task with final metrics

**Note**: Collectors are a **pattern**, not a distinct type. Any orchestrator can implement collection/aggregation.

### JSON-RPC Interface

**TLC → Assignee (execute request)**
```json
{
  "jsonrpc": "2.0",
  "method": "assignee.execute",
  "params": {
    "task_id": "T-100-1",
    "context": {
      "environment": "staging",
      "image": "myapp:v1.2.3"
    },
    "parent_task_id": "T-100"
  },
  "id": 1
}
```

**Assignee → TLC (delegation request)**
```json
{
  "jsonrpc": "2.0",
  "method": "assignee.delegate",
  "params": {
    "task_id": "T-100-1",
    "subtasks": [
      {
        "title": "Run unit tests",
        "assign_to": "assignee:test-runner:1.0",
        "context": {
          "inherit": ["test_path"],
          "add": {"test_type": "unit"}
        }
      }
    ]
  },
  "id": 2
}
```

**Assignee → TLC (collect results)**
```json
{
  "jsonrpc": "2.0",
  "method": "assignee.collect",
  "params": {
    "task_id": "T-100-1",
    "subtask_ids": ["T-100-1-1", "T-100-1-2"],
    "aggregated_result": {
      "total_tests": 47,
      "passed": 45,
      "failed": 2,
      "coverage": 82.5
    }
  },
  "id": 3
}
```

---

## Integration with Existing Plugin System

### Unified Plugin Architecture

Flow/assignee plugins extend the existing TLC plugin system (from `tlc-plugin-spec-0.1.md`):

**Shared directory:**
```
~/.config/tlc/plugins/
├── github-sync/          # Existing: External sync plugin
├── aws-deployment/       # NEW: Workflow bundle plugin
├── slack-notify/         # Existing: Notification plugin
└── custom-executor/      # Existing: Task executor plugin
```

**Shared mechanisms:**
- Same discovery process (scan `plugins/` directory)
- Same permission model (declare in manifest, prompt on first use)
- Same JSON-RPC protocol for executors
- Same CLI commands: `tlc plugin list`, `tlc plugin install`

**Key difference:**
- Existing plugins: Single-purpose (sync, notify, execute)
- Workflow bundles: Multi-component (flows + assignees + executors together)

**Benefit:** Users learn one plugin system, works for all extension types.

---

## Complete Example: AWS Deployment Plugin

### Directory Structure

```
~/.config/tlc/plugins/aws-deployment/
├── manifest.yaml
├── flows/
│   ├── deploy-to-eks.yaml
│   └── rollback.yaml
├── assignees/
│   ├── aws-cli-executor.yaml
│   └── deployment-orchestrator.yaml
└── bin/
    └── aws-executor
```

### flows/deploy-to-eks.yaml

```yaml
flow_id: "flow:aws:deploy-to-eks:1.0"
name: "Deploy to EKS"
version: "1.0"
description: "Deploy containerized app to AWS EKS cluster"
entry_step: "validate-config"

config:
  category: "deployment"
  triggers:
    - "deploy to aws"
    - "eks deployment"

  procedure: |
    ## EKS Deployment Process

    1. Validate deployment configuration
    2. Build and push container image
    3. Deploy to EKS cluster
    4. Run smoke tests
    5. Monitor rollout

steps:
  validate-config:
    step_id: "validate-config"
    type: "task"
    task_template:
      title: "Validate deployment configuration"
      description: |
        Validate:
        - AWS credentials configured
        - EKS cluster accessible
        - kubectl context set
        - Container registry accessible

      requirements:
        capabilities: ["aws-validation", "config-validation"]
        tools: ["aws-cli", "kubectl"]

      context:
        inherit: [environment, region, cluster_name]

  build-image:
    step_id: "build-image"
    type: "task"
    depends_on: ["validate-config"]
    task_template:
      title: "Build and push container image"
      description: |
        Build container from context:
        - Image: {{image}}
        - Tag: {{version}}
        - Registry: {{registry}}

      requirements:
        capabilities: ["docker-build", "container-registry"]
        tools: ["docker"]

      context:
        inherit: [image, version, registry]
        add:
          build_context: "./app"

  deploy-to-cluster:
    step_id: "deploy-to-cluster"
    type: "task"
    depends_on: ["build-image"]
    task_template:
      title: "Deploy to EKS cluster"
      description: |
        Deploy to cluster:
        - Environment: {{environment}}
        - Cluster: {{cluster_name}}
        - Strategy: rolling update

      requirements:
        capabilities: ["kubernetes-deploy", "eks-deploy"]
        tools: ["kubectl", "aws-cli"]

      context:
        inherit: [environment, cluster_name, image, version]
        exclude: [aws_secret_key]  # Security: don't pass secrets

  smoke-tests:
    step_id: "smoke-tests"
    type: "task"
    depends_on: ["deploy-to-cluster"]
    task_template:
      title: "Run smoke tests"
      description: |
        Run post-deployment smoke tests:
        - Health endpoint check
        - Basic functionality test
        - Performance baseline

      requirements:
        capabilities: ["testing", "smoke-testing"]
        tools: ["curl"]

      context:
        inherit: [environment]
        add:
          test_timeout: "5m"
```

### assignees/deployment-orchestrator.yaml

```yaml
assignee_id: "assignee:deployment-orchestrator:1.0"
name: "Deployment Orchestrator"
version: "1.0"
description: "Orchestrates multi-stage deployments with rollback capability"

capabilities:
  task_types:
    - "deployment-coordination"
    - "rollback-management"

  tools:
    - "kubectl"
    - "aws-cli"

  domains:
    - "kubernetes"
    - "aws"
    - "deployment-strategies"

execution_type: orchestrator
patterns: [delegation, monitoring, rollback]

context_policy:
  reads:
    - parent_task.title
    - parent_task.meta.environment
    - parent_task.meta.image
    - parent_task.meta.version

  delegates:
    - environment
    - image
    - version

  creates:
    - deployment_id
    - rollout_status
    - health_check_results

instructions: |
  You are a Deployment Orchestrator responsible for coordinating
  multi-stage deployments to Kubernetes clusters.

  ## Responsibilities

  1. **Pre-deployment validation**
     - Verify cluster health
     - Check resource availability
     - Validate configuration

  2. **Deployment coordination**
     - Create deployment subtasks
     - Assign to appropriate executors
     - Monitor progress

  3. **Health monitoring**
     - Track pod rollout
     - Monitor application health
     - Collect metrics

  4. **Rollback handling**
     - Detect deployment failures
     - Trigger automatic rollback if needed
     - Preserve deployment history

delegation:
  handoff_conditions:
    - when: "deployment_failed"
      delegate_to: "assignee:rollback-coordinator:1.0"

    - when: "security_scan_needed"
      delegate_to: "assignee:security-scanner:1.0"

  unblocks:
    - "deployment"
    - "testing"
    - "monitoring"
```

### Usage Example

```bash
# 1. Install plugin
tlc plugin install github.com/tlc-plugins/aws-deployment@v1.0.0

# 2. Create deployment task
tlc task create "Deploy myapp v2.1.0 to production" \
  --id T-200 \
  --meta environment=production \
  --meta image=myapp \
  --meta version=v2.1.0 \
  --meta cluster_name=prod-eks-01 \
  --meta region=us-east-1

# 3. Invoke flow for that task
tlc flow invoke aws:deploy-to-eks --for T-200

# Output:
# Invoking flow: Deploy to EKS (ID: flow:aws:deploy-to-eks:1.0)
# Run ID: run:1768640123
#
# ✓ Created task T-200-1: Validate deployment configuration (assigned to assignee:aws-cli:1.0)
# ✓ Created task T-200-2: Build and push container image (assigned to assignee:aws-cli:1.0)
# ✓ Created task T-200-3: Deploy to EKS cluster (assigned to assignee:deployment-orchestrator:1.0)
# ✓ Created task T-200-4: Run smoke tests (assigned to assignee:test-runner:1.0)
#
# Flow flow:aws:deploy-to-eks:1.0 invoked successfully. Created 4 tasks.

# 4. View task hierarchy
tlc task show T-200

# Output shows:
# Task: T-200
# Title: Deploy myapp v2.1.0 to production
# Status: IN_PROGRESS
# Subtasks:
#   T-200-1: DONE
#   T-200-2: DONE
#   T-200-3: IN_PROGRESS
#   T-200-4: TODO
```

---

## Implementation Roadmap

### Phase 1: Core Plugin Infrastructure

**Goal**: Basic plugin discovery and loading

- [ ] Extend PluginManifest to support `provides` section
- [ ] Implement plugin registry (lightweight index)
- [ ] Add lazy loading mechanism
- [ ] Support local installation: `tlc plugin install ./path`
- [ ] Update `tlc plugin list` to show workflow bundles

**Files to create/modify:**
- `internal/core/plugin.go` - Add `provides` to PluginManifest
- `internal/core/plugin_registry.go` - New: Registry index
- `internal/core/plugin_loader.go` - New: Lazy loading
- `internal/cli/plugin.go` - Update install/list commands

### Phase 2: Context System

**Goal**: Support context passing and hierarchy

- [ ] Add context_policy to Assignee struct
- [ ] Implement context inheritance/filtering
- [ ] Support parent-child task relationships
- [ ] Add task metadata for flow/run tracking

**Files to create/modify:**
- `internal/core/assignee.go` - Add ContextPolicy
- `internal/core/task.go` - Add parent_task_id field
- `internal/core/context.go` - New: Context filtering logic
- `internal/core/flow_executor.go` - Update task generation with context

### Phase 3: Assignee Execution

**Goal**: Enable assignee execution via JSON-RPC

- [ ] Implement assignee execution interface
- [ ] Add delegation mechanism (assignee → TLC → create subtasks)
- [ ] Add collection mechanism (gather subtask results)
- [ ] Support executor wrapping (execution_type: executor)

**Files to create/modify:**
- `internal/core/assignee_executor.go` - New: Execution interface
- `internal/core/assignee_delegation.go` - New: Delegation logic
- `internal/core/assignee_collection.go` - New: Result aggregation
- `internal/cli/flow.go` - Update invoke to trigger execution

### Phase 4: Git-Based Installation

**Goal**: Install plugins from Git repositories

- [ ] Implement Git clone/download
- [ ] Support tags/branches/commits
- [ ] Add version resolution
- [ ] Cache downloaded plugins

**Files to create/modify:**
- `internal/core/plugin_installer.go` - New: Git installation
- `internal/core/plugin_cache.go` - New: Plugin caching

### Phase 5: Registry Support (Future)

**Goal**: Central plugin registry

- [ ] Define registry API spec
- [ ] Implement registry client
- [ ] Add search functionality
- [ ] Support versioning/updates

**Files to create/modify:**
- `internal/core/plugin_registry_client.go` - New: Registry API
- `docs/plugin-registry-spec-0.1.md` - New: Registry specification

---

## Security Considerations

### Permission Model

Plugins inherit the existing permission model from `tlc-plugin-spec-0.1.md`:

```yaml
# In manifest
permissions:
  - network.http          # Make HTTP requests
  - process.spawn         # Spawn subprocesses (for executors)
  - credential.read       # Read credentials from keychain
  - filesystem.read       # Read files
  - filesystem.write      # Write files
```

First activation prompts user:
```
Plugin "aws-deployment" requests permissions:
  - network.http (Make HTTP requests to AWS APIs)
  - process.spawn (Run aws-cli and kubectl commands)
  - credential.read (Read AWS credentials from keychain)

Grant permissions? (y/n):
```

### Context Isolation

**Dual-gate model prevents context leakage:**

1. **Assignee capability** - What assignee CAN read/delegate
2. **Delegation filtering** - What actually GETS passed

Both must align. If assignee tries to access context it doesn't have permission for, TLC denies the operation.

**Example security violation:**
```yaml
# Assignee declares it can read environment
context_policy:
  reads: [parent_task.meta.environment]

# Flow tries to pass AWS secret key
context:
  inherit: [environment, aws_secret_key]  # ❌ DENIED

# TLC blocks: assignee doesn't have permission for aws_secret_key
```

### Credential Isolation

Assignees cannot access credentials for other plugins:
- `assignee:aws-cli:1.0` can read `aws-deployment:credentials`
- `assignee:aws-cli:1.0` CANNOT read `github-sync:token`

---

## Open Questions

1. **Flow versioning**: How do we handle breaking changes in flow schemas?
   - Proposed: Semantic versioning in flow IDs (`flow:aws:deploy:1.0` → `flow:aws:deploy:2.0`)

2. **Assignee discovery**: Should assignees be auto-discovered or manually registered?
   - Proposed: Auto-discovered from `provides.assignees` in manifest

3. **Result aggregation format**: What's the standard format for collector results?
   - Proposed: JSON schema in task metadata, plugin-specific

4. **Circular delegation**: How do we prevent assignee A → B → A loops?
   - Proposed: Track delegation chain, enforce max depth (e.g., 10)

5. **Plugin updates**: How do we handle updating installed plugins?
   - Proposed: `tlc plugin update <name>` pulls latest compatible version

---

## Success Criteria

- [ ] Users can install workflow plugins from Git
- [ ] Flows can be invoked with context (both modes)
- [ ] Assignees can execute, orchestrate, or collect
- [ ] Context flows securely through delegation chain
- [ ] Plugin system feels natural extension of TLC
- [ ] Documentation and examples are clear
- [ ] Community can contribute plugins easily

---

## References

- [Flows & Assignees Documentation](../flows-and-assignees.md)
- [TLC Plugin Specification](../tlc-plugin-spec-0.1.md)
- [Flows & Assignees Design](2026-01-17-flows-and-assignees-design.md)

---

**Version**: 0.1
**Status**: Design Document
**Next Steps**: Review design, create implementation plan
