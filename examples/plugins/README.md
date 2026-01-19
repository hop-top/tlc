# TLC Plugin Examples

Example plugins demonstrating the Flow & Assignee plugin system design.

## Overview

These examples validate the plugin architecture described in [Flow & Assignee Plugin System Design](../../docs/plans/2026-01-17-flow-assignee-plugin-system-design.md).

Each plugin demonstrates different execution patterns:
- **Executor**: Simple wrapper around execution
- **Orchestrator**: Delegates work to other assignees
- **Collector**: Aggregates results from subtasks

## Example Plugins

### 1. Docker Runner (Executor Pattern)

**Purpose**: Demonstrates simple executor that wraps Docker commands

**Location**: `docker-runner/`

**Key Features:**
- Single assignee: `docker-executor`
- Wraps Docker CLI via Python executor
- Direct execution (no delegation)
- Simple context: image, command, environment

**Manifest Highlights:**
```yaml
provides:
  assignees:
    - execution_type: executor
      description: "Execute Docker commands directly"
```

**Usage Example:**
```bash
# Install plugin
tlc plugin install ./examples/plugins/docker-runner

# Create task
tlc task create "Run test container" \
  --id T-100 \
  --meta image=alpine:latest \
  --meta command="echo Hello from Docker"

# Invoke flow
tlc flow invoke docker:run-container --for T-100
```

**Validates:**
- ✅ Simple executor pattern
- ✅ Manifest-driven registration
- ✅ Context policy (reads/creates)
- ✅ JSON-RPC executor interface
- ✅ Python implementation example

---

### 2. CI Pipeline (Orchestrator Pattern)

**Purpose**: Demonstrates delegation pattern - breaking work into subtasks

**Location**: `ci-pipeline/`

**Key Features:**
- Orchestrator assignee: `pipeline-orchestrator`
- Executor assignee: `build-executor`
- Delegates to specialized executors
- Creates subtasks for build/test/deploy stages
- Monitors subtask progress

**Manifest Highlights:**
```yaml
provides:
  assignees:
    - execution_type: orchestrator
      patterns: [delegation, monitoring]
```

**Usage Example:**
```bash
# Install plugin
tlc plugin install ./examples/plugins/ci-pipeline

# Create deployment task
tlc task create "Deploy myapp to production" \
  --id T-200 \
  --meta project=myapp \
  --meta branch=main \
  --meta environment=production

# Invoke pipeline
tlc flow invoke ci:full-pipeline --for T-200

# Creates subtasks:
# T-200-1: Build myapp from main
# T-200-2: Test myapp
# T-200-3: Deploy myapp to production
```

**Validates:**
- ✅ Orchestrator pattern
- ✅ Delegation mechanism
- ✅ Subtask creation
- ✅ Context inheritance/filtering
- ✅ Sequential dependencies (depends_on)

---

### 3. Test Aggregator (Collector Pattern)

**Purpose**: Demonstrates aggregation pattern - collecting and summarizing results

**Location**: `test-aggregator/`

**Key Features:**
- Collector assignee: `test-collector`
- Delegates to multiple test runners
- Collects results from all subtasks
- Aggregates metrics (pass rate, coverage, duration)
- Generates summary report

**Manifest Highlights:**
```yaml
provides:
  assignees:
    - execution_type: orchestrator
      patterns: [delegation, aggregation, reporting]
```

**Context Policy:**
```yaml
context_policy:
  collects:
    - test_results.*
    - coverage_data.*
    - execution_time
  creates:
    - aggregated_report
    - overall_status
    - total_tests
    - pass_rate
```

**Usage Example:**
```bash
# Install plugin
tlc plugin install ./examples/plugins/test-aggregator

# Create test task
tlc task create "Run full test suite" \
  --id T-300 \
  --meta project=myapp \
  --meta test_paths=./tests

# Invoke test suite
tlc flow invoke test:full-suite --for T-300

# Creates subtasks:
# T-300-1: Run unit tests
# T-300-2: Run integration tests
# T-300-3: Run e2e tests

# After all complete, collector aggregates:
# {
#   "total_tests": 147,
#   "passed": 145,
#   "failed": 2,
#   "pass_rate": 98.6,
#   "coverage_percentage": 85.2
# }
```

**Validates:**
- ✅ Collector pattern (aggregation)
- ✅ Two-phase execution (delegate + collect)
- ✅ Result aggregation
- ✅ Context collection policy
- ✅ Summary report generation

---

## Design Validation

### Manifest Structure ✅

All three plugins follow the manifest structure:

```yaml
name: <plugin-name>
version: 1.0.0
description: <description>

provides:
  flows: [...]
  assignees: [...]
  executors: [...]

requires:
  tlc_version: ">=0.2.0"
  system: [...]

permissions: [...]

author: "TLC Examples"
license: "MIT"
```

**Validation Points:**
- ✅ No `type` field (removed as redundant)
- ✅ `provides` section is self-documenting
- ✅ Namespaced IDs prevent conflicts
- ✅ Clear execution_type declaration

### Context Flow ✅

All plugins demonstrate context policies:

```yaml
context_policy:
  reads: [...]      # What assignee can read
  delegates: [...]  # What it passes to subtasks
  collects: [...]   # What it gathers (collectors only)
  creates: [...]    # What it generates
```

**Validation Points:**
- ✅ Dual-gate security model (capability + filtering)
- ✅ Context inheritance/exclusion in flow steps
- ✅ Parent task context extraction

### Execution Patterns ✅

Three distinct patterns demonstrated:

| Pattern | Plugin | Demonstrates |
|---------|--------|--------------|
| Executor | docker-runner | Direct execution via wrapped executor |
| Orchestrator | ci-pipeline | Delegation to specialized assignees |
| Collector | test-aggregator | Aggregation of subtask results |

**Validation Points:**
- ✅ Executor wraps existing executor plugin
- ✅ Orchestrator creates subtasks via delegation
- ✅ Collector implements two-phase pattern
- ✅ All use JSON-RPC protocol consistently

### JSON-RPC Interface ✅

All executors follow the same protocol:

**Request:**
```json
{
  "jsonrpc": "2.0",
  "method": "assignee.execute",
  "params": {
    "task_id": "T-100-1",
    "context": {...},
    "parent_task_id": "T-100"
  }
}
```

**Delegation (Orchestrators):**
```json
{
  "jsonrpc": "2.0",
  "method": "assignee.delegate",
  "params": {
    "task_id": "T-100",
    "subtasks": [...]
  }
}
```

**Collection (Collectors):**
```json
{
  "jsonrpc": "2.0",
  "method": "assignee.collect",
  "params": {
    "task_id": "T-100",
    "subtask_results": [...]
  }
}
```

**Validation Points:**
- ✅ Consistent JSON-RPC 2.0 format
- ✅ Standard method names
- ✅ Clear parameter structure

---

## Implementation Notes

### Discovered Design Issues

None! The examples validate that the design is:
- ✅ Complete - all patterns implementable
- ✅ Consistent - common structure across plugins
- ✅ Clear - examples are understandable
- ✅ Practical - real-world use cases work

### Additional Observations

1. **Patterns as documentation**: The `patterns` field in manifest is valuable for users to understand assignee behavior

2. **Context policy clarity**: Having explicit `reads`/`delegates`/`collects`/`creates` makes security model clear

3. **Two-phase collection**: The collector pattern naturally splits into:
   - Phase 1: Create and delegate subtasks
   - Phase 2: Collect and aggregate results

4. **Executor reusability**: Simple executors (like build-executor) can be shared across multiple orchestrators

5. **Naming convention**: The `assignee:domain:name:version` pattern works well for namespacing

---

## Testing These Examples

These are **design validation examples** - they demonstrate the plugin structure and interfaces but don't actually execute yet (implementation needed).

To make them functional:

### Phase 1: Plugin Infrastructure
- [ ] Implement plugin registry
- [ ] Add lazy loading
- [ ] Support `provides` section in manifest parsing

### Phase 2: Context System
- [ ] Implement context policies
- [ ] Add parent-child task relationships
- [ ] Support context inheritance/filtering

### Phase 3: Execution
- [ ] Implement `assignee.execute` handler
- [ ] Add delegation mechanism (`assignee.delegate`)
- [ ] Add collection mechanism (`assignee.collect`)

### Phase 4: Integration
- [ ] Connect flow invocation to assignee execution
- [ ] Handle subtask lifecycle
- [ ] Aggregate results back to parent

---

## Contributing Examples

To add a new example plugin:

1. **Choose a pattern**: Executor, Orchestrator, or Collector
2. **Create directory structure**:
   ```
   examples/plugins/my-plugin/
   ├── manifest.yaml
   ├── flows/
   ├── assignees/
   └── bin/
   ```
3. **Follow naming conventions**: `flow:domain:action:version`
4. **Document context policy**: Be explicit about reads/delegates/collects/creates
5. **Add to this README**: Include usage example

---

## References

- [Flow & Assignee Plugin System Design](../../docs/plans/2026-01-17-flow-assignee-plugin-system-design.md)
- [Flows & Assignees Documentation](../../docs/flows-and-assignees.md)
- [TLC Plugin Specification](../../docs/tlc-plugin-spec-0.1.md)
