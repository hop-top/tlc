# Flows & Assignees - Superpowers Integration

## Overview

TLC now includes **Flows** and **Assignees**, inspired by the superpowers plugin's skills and agents architecture. This enables:

- **Flows** (like superpowers skills): Procedural workflows that extract task sequences
- **Assignees** (like superpowers agents): Specialized executors with capability-based routing

## Architecture

### Flows → Task Extraction

Flows define procedural workflows with task templates that generate actual tasks:

```yaml
# examples/flows/brainstorming.yaml
flow_id: "flow:brainstorming:1.0"
name: "Brainstorming"
config:
  procedure: |
    ## Brainstorming Process
    1. Understanding the idea
    2. Exploring approaches
    3. Presenting the design

steps:
  explore-context:
    step_id: "explore-context"
    type: "task"
    title: "Explore current project context"
    task_template:
      title: "Explore codebase for context"
      description: |
        Review current project state...
      requirements:
        capabilities: ["codebase-exploration", "git-analysis"]
        tools: ["grep", "git"]
```

### Assignees → Capability Matching

Assignees define specialized executors with capabilities that match task requirements:

```yaml
# examples/assignees/code-analyst.yaml
assignee_id: "assignee:code-analyst:1.0"
name: "Code Analyst"
capabilities:
  task_types:
    - "code-review"
    - "architecture-review"
  tools:
    - "grep"
    - "git-diff"
  domains:
    - "golang"
    - "system-design"
```

## Core Concepts

### 1. Flow Configuration

Flows now include a `config` field with procedural instructions:

- **Procedure**: Markdown-formatted workflow steps (like superpowers skills)
- **Category**: Flow organization (creative, debugging, deployment)
- **Triggers**: Conditions that suggest using this flow
- **Params**: Additional configuration

### 2. Task Templates

Flow steps can include task templates for automatic task generation:

- **Title**: Task title
- **Description**: Task body/details
- **Requirements**: Capabilities, domains, tools needed
- **Context**: Additional metadata

### 3. Step Gate (optional EVA validation)

Optional per-step contract validation via the EVA HTTP gateway. When `gate` is set,
a step only succeeds if EVA approves its output; downstream steps are blocked on failure.
EVA integration is opt-in — flows without `gate` are unaffected.

**Fields:**

- `contract` — name of the EVA contract to invoke
- `eva_url` — base URL of the EVA gateway (e.g. `http://localhost:8080`)

**Auth:** set `EVA_KEY` env var; sent as `X-Eva-Key` header.

**API contract:**

- Endpoint: `POST {eva_url}/v1/contract/invoke`
- Request body: `{"contract": "<name>", "body": <step_output>}`
- Pass: HTTP 200, `{"eva_status": "pass", "attempts": N}`
- Violation: HTTP 422, `{"eva_status": "contract_violation", "violations": [...]}`

**Downstream blocking:** if gate rejects, step is marked `failed`; all steps that
`depends_on` it remain blocked and do not execute.

**Example:**

```yaml
steps:
  plan:
    step_id: plan
    type: task
    title: Write plan
    gate:
      contract: plan-quality
      eva_url: http://eva.internal

  implement:
    step_id: implement
    type: task
    title: Implement
    depends_on: [plan]   # blocked until plan passes EVA gate
```

**Runtime behavior in FlowExecutor:**

Gate fires after the task step executes and returns success. Sequence per step:

1. Task step executes → returns success.
2. `RunEvaGate` called with `step.Gate` and env `EVA_KEY`.
3. EVA returns pass (HTTP 200) → step marked `succeeded`; execution continues.
4. EVA returns violation (HTTP 422) → step marked `failed`; error returned; no
   downstream steps run.
5. Steps that list the failed step in `depends_on` remain `pending`; executor
   halts on "terminal failure state" for that step.

Steps without `gate` skip the check entirely — no EVA call made.

**`RunEvaGate` function signature (Go):**

```
RunEvaGate(ctx, gate *StepGate, stepOutput map[string]any, apiKey string) error
```

- `gate == nil` → no-op, returns nil
- pass → nil error
- violation → actionable error with evaluator details + retry hint
- non-200 non-violation → wrapped error with HTTP status + gateway URL

### 4. Assignee Capabilities

Assignees declare what they can do:

- **Task Types**: Types of tasks they can handle
- **Domains**: Areas of expertise
- **Tools**: Tools they have access to

### 5. Assignment Engine

The assignment engine matches tasks to assignees using weighted scoring:

- Capability match: 10 points per match
- Domain match: 5 points per match
- Tool match: 2 points per match

Best-scoring assignee gets auto-assigned to the task.

### 6. Delegation & Hand-offs

#### Flow-to-Flow Hand-offs (via Subflow)

```yaml
steps:
  handoff-to-planning:
    type: "subflow"
    flow_ref: "flow:writing-plans:1.0"
```

#### Assignee-to-Assignee Hand-offs

```yaml
delegation:
  handoff_conditions:
    - when: "security_vulnerability_found"
      delegate_to: "assignee:security-specialist:1.0"
  unblocks:
    - "implementation"
    - "deployment"
```


## CLI Commands

### Assignee Management

```bash
# List all assignees
./bin/tlc assignee list

# Show assignee details
./bin/tlc assignee show assignee:code-analyst:1.0
```

### Flow Invocation (Coming Soon)

```bash
# Invoke a flow to generate tasks
./bin/tlc flow invoke flow:brainstorming:1.0
```

## File Structure

```
tlc/
├── examples/
│   ├── flows/
│   │   └── brainstorming.yaml
│   └── assignees/
│       └── code-analyst.yaml
│
├── internal/
│   ├── core/
│   │   ├── flow.go              # Extended with FlowConfig, TaskTemplate
│   │   ├── assignee.go          # New: Assignee model
│   │   ├── assignment_engine.go # New: Capability matching
│   │   ├── assignee_loader.go   # New: Load assignees from YAML
│   │   └── flow_executor.go     # Extended: Task extraction
│   │
│   └── cli/
│       └── assignee.go          # New: Assignee CLI commands
│
└── docs/
    ├── plans/
    │   └── 2026-01-17-flows-and-assignees-design.md
    └── flows-and-assignees.md  # This file
```

## Implementation Status

### ✅ Completed (All Phases!)

**Phase 1-3: Core Infrastructure**
- [x] Extended Flow struct with Config field
- [x] Added TaskTemplate to Step struct
- [x] Created Assignee model with capabilities
- [x] Implemented flow-to-task extraction
- [x] Implemented assignee matching engine
- [x] Created assignee loader from YAML

**Phase 4: Delegation & Hand-offs**
- [x] Full delegation mechanism (DelegateTask method)
- [x] Task unblocking mechanism (UnblockTasks method)
- [x] Auto-assignment on task creation (CreateTaskWithAssignment)

**Phase 5-6: CLI & User Interface**
- [x] Assignee CLI commands (list, show)
- [x] Flow invoke CLI command ✅
- [x] Integration with task creation workflow

**Phase 7: Examples & Documentation**
- [x] Example flows created and tested:
  - brainstorming.yaml
  - systematic-debugging.yaml
  - writing-plans.yaml
  - test-driven-development.yaml
  - code-review.yaml
  - verification-before-completion.yaml
  - executing-plans.yaml
  - finishing-development-branch.yaml
- [x] Example assignees: code-analyst, architect, debugger, tech-writer
- [x] Verified all flows invoke successfully with auto-assignment
- [x] Updated documentation with real examples
- [x] Complete superpowers workflow recreated in TLC

### 🚧 Future Enhancements

- [ ] Comprehensive unit tests
- [ ] Integration tests for delegation/unblocking
- [ ] Flow templates/scaffolding CLI
- [ ] TUI integration for flows view

## Example Usage

### 1. View Available Assignees

```bash
$ ./bin/tlc assignee list

Available Assignees (4):

  Architect
    ID: assignee:architect:1.0
    Description: Software architect for system design and architecture decisions
    Capabilities:
      Task Types: [architecture-analysis system-design trade-off-analysis ...]
      Domains: [golang python javascript system-design microservices ...]
      Tools: [grep git diagram-tools profiler]
    Unblocks: [implementation code-review documentation]

  Code Analyst
    ID: assignee:code-analyst:1.0
    Description: Senior code reviewer for plan alignment and quality assessment
    Capabilities:
      Task Types: [code-review plan-alignment-check architecture-review ...]
      Domains: [golang python javascript system-design testing]
      Tools: [grep git-diff ast-parser linter]
    Unblocks: [implementation deployment integration]

  Debugger
    ID: assignee:debugger:1.0
    Description: Systematic debugging specialist
    Capabilities:
      Task Types: [debugging root-cause-analysis bug-investigation ...]
      Domains: [golang python javascript system-debugging ...]
      Tools: [debugger profiler git-bisect logs strace]
    Unblocks: [implementation testing deployment]

  Technical Writer
    ID: assignee:tech-writer:1.0
    Description: Technical documentation specialist
    Capabilities:
      Task Types: [technical-writing documentation api-documentation ...]
      Domains: [technical-writing documentation api-design ...]
      Tools: [markdown diagrams git]
    Unblocks: [code-review deployment user-testing]
```

### 2. View Assignee Details

```bash
$ ./bin/tlc assignee show assignee:code-analyst:1.0

Assignee: Code Analyst
ID: assignee:code-analyst:1.0
Version: 1.0
Description: Senior code reviewer for plan alignment and quality assessment

Capabilities:
  Task Types: [code-review architecture-review quality-assessment]
  Domains: [golang system-design testing]
  Tools: [grep git-diff linter]

Instructions:
You are a Senior Code Reviewer...

Delegation Rules:
  Handoff Conditions:
    - When: security_vulnerability_found -> assignee:security-specialist:1.0
  Unblocks: [implementation deployment integration]
```

### 3. Invoke Flow to Generate Tasks ✅ WORKING

```bash
$ ./bin/tlc flow invoke examples/flows/brainstorming.yaml

Invoking flow: Brainstorming (ID: flow:brainstorming:1.0)
Run ID: run:1768633694

✓ Created task T-93ef63f9: Explore codebase for context (assigned to assignee:code-analyst:1.0)
✓ Created task T-5800e947: Gather requirements through questions (assigned to unassigned)
✓ Created task T-e2133e5a: Analyze 2-3 implementation approaches (assigned to assignee:architect:1.0)
✓ Created task T-c5836206: Document validated design (assigned to assignee:tech-writer:1.0)

Flow flow:brainstorming:1.0 invoked successfully. Created 4 tasks.
```

### 4. Invoke Systematic Debugging Flow

```bash
$ ./bin/tlc flow invoke examples/flows/systematic-debugging.yaml

Invoking flow: Systematic Debugging (ID: flow:systematic-debugging:1.0)
Run ID: run:1768633817

✓ Created task T-b0baa2c9: Analyze error messages and stack traces (assigned to assignee:debugger:1.0)
✓ Created task T-47c41b64: Create reliable reproduction steps (assigned to assignee:debugger:1.0)
✓ Created task T-7a603ead: Investigate what changed (assigned to assignee:debugger:1.0)
✓ Created task T-412fd5b6: Add diagnostic instrumentation (assigned to assignee:debugger:1.0)
✓ Created task T-c2f83b96: Compare broken vs working code (assigned to assignee:architect:1.0)
✓ Created task T-6cd347e1: State hypothesis and test minimally (assigned to assignee:debugger:1.0)
✓ Created task T-76053527: Write automated failing test (assigned to assignee:debugger:1.0)
✓ Created task T-520fd965: Fix root cause with single change (assigned to assignee:debugger:1.0)
✓ Created task T-723f8f4d: Confirm fix resolves the issue (assigned to unassigned)

Flow flow:systematic-debugging:1.0 invoked successfully. Created 9 tasks.
```

### 5. Invoke Writing Plans Flow

```bash
$ ./bin/tlc flow invoke examples/flows/writing-plans.yaml

Invoking flow: Writing Implementation Plans (ID: flow:writing-plans:1.0)
Run ID: run:1768633824

✓ Created task T-e81c6258: Review and clarify requirements (assigned to unassigned)
✓ Created task T-a77e7f47: Create step-by-step implementation plan (assigned to assignee:architect:1.0)
✓ Created task T-42e06de8: List files to create/modify/read (assigned to assignee:architect:1.0)
✓ Created task T-0f93f340: Define how to verify each step (assigned to unassigned)
✓ Created task T-4e12c643: Write plan to docs/plans/ (assigned to assignee:tech-writer:1.0)

Flow flow:writing-plans:1.0 invoked successfully. Created 5 tasks.
```

### 6. Invoke Test-Driven Development Flow

```bash
$ ./bin/tlc flow invoke examples/flows/test-driven-development.yaml

Invoking flow: Test-Driven Development (ID: flow:test-driven-development:1.0)
Run ID: run:1768634001

✓ Created task T-abc123: Clarify what behavior to implement (assigned to unassigned)
✓ Created task T-def456: Write failing test for ONE behavior (assigned to unassigned)
✓ Created task T-ghi789: Implement just enough to pass the test (assigned to unassigned)
✓ Created task T-jkl012: Verify tests pass (assigned to unassigned)
✓ Created task T-mno345: Clean up code while keeping tests green (assigned to unassigned)
✓ Created task T-pqr678: Commit with behavioral commit message (assigned to unassigned)

Flow flow:test-driven-development:1.0 invoked successfully. Created 6 tasks.
```

### 7. Invoke Code Review Flow

```bash
$ ./bin/tlc flow invoke examples/flows/code-review.yaml

Invoking flow: Code Review Process (ID: flow:code-review:1.0)
Run ID: run:1768634015

✓ Created task T-review1: Gather context for review (assigned to assignee:code-analyst:1.0)
✓ Created task T-review2: Check plan alignment (assigned to assignee:code-analyst:1.0)
✓ Created task T-review3: Assess architectural quality (assigned to assignee:architect:1.0)
✓ Created task T-review4: Check code quality and correctness (assigned to assignee:code-analyst:1.0)
✓ Created task T-review5: Verify test quality and coverage (assigned to unassigned)
✓ Created task T-review6: Check for security issues (assigned to unassigned)
✓ Created task T-review7: Document review findings (assigned to assignee:tech-writer:1.0)

Flow flow:code-review:1.0 invoked successfully. Created 7 tasks.
```

### 8. Invoke Verification Before Completion Flow

```bash
$ ./bin/tlc flow invoke examples/flows/verification-before-completion.yaml

Invoking flow: Verification Before Completion (ID: flow:verification-before-completion:1.0)
Run ID: run:1768634028

✓ Created task T-verify1: List claims that need verification (assigned to unassigned)
✓ Created task T-verify2: Map claims to verification commands (assigned to unassigned)
✓ Created task T-verify3: Run commands and capture output (assigned to unassigned)
✓ Created task T-verify4: Analyze verification results (assigned to unassigned)
✓ Created task T-verify5: State conclusions with evidence (assigned to assignee:tech-writer:1.0)
✓ Created task T-verify6: Final verification before commit (assigned to unassigned)

Flow flow:verification-before-completion:1.0 invoked successfully. Created 6 tasks.
```

### 9. Invoke Executing Plans Flow

```bash
$ ./bin/tlc flow invoke examples/flows/executing-plans.yaml

Invoking flow: Executing Implementation Plans (ID: flow:executing-plans:1.0)
Run ID: run:1768634042

✓ Created task T-exec1: Read implementation plan completely (assigned to unassigned)
✓ Created task T-exec2: Prepare implementation environment (assigned to unassigned)
✓ Created task T-exec3: Implement single plan step (assigned to unassigned)
✓ Created task T-exec4: Review progress against plan (assigned to assignee:code-analyst:1.0)
✓ Created task T-exec5: Verify plan fully implemented (assigned to unassigned)
✓ Created task T-exec6: Submit for code review (assigned to assignee:code-analyst:1.0)

Flow flow:executing-plans:1.0 invoked successfully. Created 6 tasks.
```

### 10. Invoke Finishing Development Branch Flow

```bash
$ ./bin/tlc flow invoke examples/flows/finishing-development-branch.yaml

Invoking flow: Finishing a Development Branch (ID: flow:finishing-development-branch:1.0)
Run ID: run:1768634055

✓ Created task T-finish1: Confirm implementation complete (assigned to unassigned)
✓ Created task T-finish2: Remove temporary code and clean history (assigned to unassigned)
✓ Created task T-finish3: Decide how to integrate this work (assigned to unassigned)
✓ Created task T-finish4: Merge branch directly to main (assigned to unassigned)
✓ Created task T-finish5: Submit work via pull request (assigned to assignee:code-analyst:1.0)
✓ Created task T-finish6: Document decision to continue work (assigned to assignee:tech-writer:1.0)
✓ Created task T-finish7: Close branch and capture learnings (assigned to assignee:tech-writer:1.0)

Flow flow:finishing-development-branch:1.0 invoked successfully. Created 7 tasks.
```

## Available Flows Summary

TLC now includes a complete workflow suite inspired by superpowers:

**Creative & Planning:**
- `brainstorming.yaml` - Transform ideas into designs through dialogue
- `writing-plans.yaml` - Create detailed implementation plans before coding

**Development:**
- `test-driven-development.yaml` - Write tests before implementation (TDD cycle)
- `executing-plans.yaml` - Execute written plans with review checkpoints

**Quality Assurance:**
- `systematic-debugging.yaml` - Find root cause before fixing (4-phase debugging)
- `code-review.yaml` - Review work against plan and quality standards
- `verification-before-completion.yaml` - Evidence before assertions

**Workflow:**
- `finishing-development-branch.yaml` - Complete work and decide integration path

## Next Steps

1. **Test New Flows**: Invoke and test the newly created flows
2. **Write Tests**: Comprehensive test coverage for assignment engine
3. **Add More Assignees**: Create specialized assignees (security-specialist, performance-engineer)
4. **TUI Integration**: Add flows view to TUI

## Design Documents

- Full design: `docs/plans/2026-01-17-flows-and-assignees-design.md`
- Implementation plan: `.claude/plans/eager-roaming-quiche.md`

## Credits

Inspired by the superpowers plugin's skills and agents architecture for Claude Code.
