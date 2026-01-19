# TLC Flows & Assignees Design

**Date**: 2026-01-17
**Status**: Design Draft

## Overview

Clone the superpowers plugin's skills and agents functionality into TLC using flows and assignees.

## Core Mapping

| Superpowers | TLC | Purpose |
|-------------|-----|---------|
| **Skills** | **Flows** | Procedural workflows that extract task sequences |
| **Agents** | **Assignees** | Specialized executors that perform task work |

## Design Principles

1. **Flows coordinate, Assignees execute**
2. **Flows can hand-off to other flows** (via subflow steps)
3. **Assignees can hand-off via re-assignment** (task delegation)
4. **Tasks bridge flows and assignees** (flows create tasks, assignees execute them)

---

## Part 1: Flow Definition Schema

### Flow File Structure

Flows are YAML files with embedded markdown:

```yaml
# flows/brainstorming.yaml
flow_id: "flow:brainstorming:1.0"
name: "Brainstorming"
version: "1.0"
description: "Transform ideas into designs through collaborative dialogue"
entry_step: "understand-context"

# Flow metadata
meta:
  category: "creative"
  triggers: ["new feature", "adding functionality", "building component"]

# Flow-specific config
config:
  # Procedural instructions (like superpowers skills)
  procedure: |
    ## The Process

    1. Understanding the idea:
       - Check current project state
       - Ask questions one at a time
       - Focus on purpose, constraints, success criteria

    2. Exploring approaches:
       - Propose 2-3 different approaches with trade-offs
       - Lead with recommended option

    3. Presenting the design:
       - Break into 200-300 word sections
       - Validate each section before proceeding

# Flow steps (orchestration)
steps:
  understand-context:
    step_id: "understand-context"
    type: "task"
    title: "Understand current project context"
    task_ref: "task:explore-codebase"

  gather-requirements:
    step_id: "gather-requirements"
    type: "task"
    title: "Ask clarifying questions and gather requirements"
    task_ref: "task:ask-questions"
    depends_on: ["understand-context"]

  explore-approaches:
    step_id: "explore-approaches"
    type: "task"
    title: "Propose 2-3 approaches with trade-offs"
    task_ref: "task:analyze-approaches"
    depends_on: ["gather-requirements"]

  write-design:
    step_id: "write-design"
    type: "task"
    title: "Write validated design document"
    task_ref: "task:document-design"
    depends_on: ["explore-approaches"]

  handoff-to-planning:
    step_id: "handoff-to-planning"
    type: "subflow"
    title: "Hand-off to implementation planning"
    flow_ref: "flow:writing-plans:1.0"
    depends_on: ["write-design"]
```

### Flow Execution Model

When a flow is invoked:

1. **Flow engine creates tasks** from flow steps
2. **Tasks are assigned to capable assignees** based on task requirements
3. **Assignees execute tasks** following their capabilities
4. **Flow monitors progress** and coordinates handoffs
5. **Subflow steps delegate** to other flows

---

## Part 2: Assignee Definition Schema

### Assignee File Structure

Assignees are YAML files with capability declarations:

```yaml
# assignees/code-analyst.yaml
assignee_id: "assignee:code-analyst:1.0"
name: "Code Analyst"
version: "1.0"
description: "Specialized in code review, architecture analysis, and quality assessment"

# Assignee capabilities
capabilities:
  # Task types this assignee can handle
  task_types:
    - "code-review"
    - "architecture-analysis"
    - "quality-assessment"
    - "pattern-matching"

  # Skills/tools available to this assignee
  tools:
    - "grep"
    - "ast-parser"
    - "linter"
    - "static-analyzer"

  # Domain expertise
  domains:
    - "golang"
    - "python"
    - "javascript"
    - "system-design"

# Assignee instructions (system prompt)
instructions: |
  You are a Senior Code Reviewer with expertise in software architecture,
  design patterns, and best practices.

  When executing code review tasks:

  1. Plan Alignment Analysis:
     - Compare implementation against original plan
     - Identify deviations and assess if justified

  2. Code Quality Assessment:
     - Review for patterns and conventions
     - Check error handling, type safety
     - Evaluate test coverage

  3. Architecture Review:
     - Ensure SOLID principles
     - Verify separation of concerns

  4. Issue Identification:
     - Categorize: Critical / Important / Suggestions
     - Provide actionable recommendations

# Delegation rules
delegation:
  # When to delegate to other assignees
  handoff_conditions:
    - when: "security_issue_found"
      delegate_to: "assignee:security-specialist:1.0"

    - when: "performance_bottleneck_identified"
      delegate_to: "assignee:performance-engineer:1.0"

  # When to unblock other tasks
  unblocks:
    - "code-implementation" # After review passes, implementation can proceed
```

### Assignee Assignment Model

When a task is created:

1. **Task specifies required capabilities** (either explicitly or inferred from task type)
2. **Assignment engine matches** task requirements to assignee capabilities
3. **Best-fit assignee is assigned** based on capability match score
4. **Assignee executes task** following their instructions
5. **Assignee can delegate** if handoff conditions met

---

## Part 3: Flow-to-Tasks Extraction

### Task Generation from Flow Steps

Each flow step generates one or more tasks:

```yaml
# Flow step definition
steps:
  code-review:
    step_id: "code-review"
    type: "task"
    title: "Review implementation against plan"
    task_ref: "task:review-auth-implementation"

    # Task template for generation
    task_template:
      title: "Review authentication implementation"
      description: |
        Review the completed authentication implementation against
        the original plan in docs/plans/2026-01-15-auth-design.md.

        Verify:
        - Plan alignment
        - Code quality
        - Architecture adherence
        - Test coverage

      # Task requirements (for assignee matching)
      requirements:
        capabilities: ["code-review", "architecture-analysis"]
        domains: ["golang", "security"]

      # Task context
      context:
        plan_file: "docs/plans/2026-01-15-auth-design.md"
        implementation_files:
          - "internal/auth/*.go"
          - "internal/auth/*_test.go"
```

### Task Creation Process

```go
// Pseudo-code for task extraction
func (f *FlowExecutor) ExtractTasks(flow *Flow) ([]*Task, error) {
    tasks := []*Task{}

    for _, step := range flow.Steps {
        switch step.Type {
        case StepTypeTask:
            task := &Task{
                ID:          generateTaskID(),
                Title:       step.TaskTemplate.Title,
                Description: step.TaskTemplate.Description,
                Status:      StatusTodo,
                Reference:   step.TaskTemplate.Context.PlanFile,
                Meta: map[string]interface{}{
                    "flow_id":    flow.ID,
                    "flow_run_id": f.runID,
                    "step_id":    step.ID,
                    "requirements": step.TaskTemplate.Requirements,
                },
            }
            tasks = append(tasks, task)

        case StepTypeSubflow:
            // Hand-off to another flow
            subflowTasks := f.ExecuteSubflow(step.FlowRef)
            tasks = append(tasks, subflowTasks...)
        }
    }

    return tasks, nil
}
```

---

## Part 4: Task-to-Assignee Routing

### Capability Matching Algorithm

```go
type AssignmentEngine struct {
    assignees []*Assignee
}

func (e *AssignmentEngine) FindBestAssignee(task *Task) (*Assignee, error) {
    requirements := task.Meta["requirements"].(TaskRequirements)

    scores := make(map[string]float64)

    for _, assignee := range e.assignees {
        score := 0.0

        // Match capabilities
        for _, reqCap := range requirements.Capabilities {
            if contains(assignee.Capabilities.TaskTypes, reqCap) {
                score += 10.0
            }
        }

        // Match domains
        for _, reqDomain := range requirements.Domains {
            if contains(assignee.Capabilities.Domains, reqDomain) {
                score += 5.0
            }
        }

        // Match tools
        for _, reqTool := range requirements.Tools {
            if contains(assignee.Capabilities.Tools, reqTool) {
                score += 2.0
            }
        }

        scores[assignee.ID] = score
    }

    // Return highest scoring assignee
    return findMax(scores)
}
```

---

## Part 5: Hand-off Mechanisms

### Flow-to-Flow Hand-off (Subflow Steps)

Already supported by existing TLC flow spec via `subflow` step type:

```yaml
steps:
  design-complete:
    step_id: "design-complete"
    type: "task"
    title: "Write design document"
    task_ref: "task:write-design"

  handoff-to-planning:
    step_id: "handoff-to-planning"
    type: "subflow"
    title: "Create implementation plan"
    flow_ref: "flow:writing-plans:1.0"
    depends_on: ["design-complete"]
    inputs:
      design_doc: "${design-complete.output.doc_path}"
```

### Assignee-to-Assignee Hand-off

Two mechanisms:

**1. Re-assignment (Delegation)**

```go
func (a *Assignee) DelegateTask(task *Task, reason string) error {
    // Check delegation rules
    for _, rule := range a.Delegation.HandoffConditions {
        if rule.When == reason {
            newAssignee := rule.DelegateTo

            // Create delegation log entry
            logEntry := &LogEntry{
                TaskID:    task.ID,
                By:        a.ID,
                Action:    "DELEGATED",
                Note:      fmt.Sprintf("Delegating to %s: %s", newAssignee, reason),
            }

            // Re-assign task
            task.AssignedTo = &newAssignee
            task.Status = StatusTodo // Reset to TODO for new assignee

            return nil
        }
    }

    return errors.New("no delegation rule matches")
}
```

**2. Unblocking (Dependency Resolution)**

```go
func (a *Assignee) CompleteTask(task *Task) error {
    // Mark task as done
    task.Status = StatusDone

    // Check what this unblocks
    for _, unblockedTaskType := range a.Delegation.Unblocks {
        // Find tasks blocked by this task
        blockedTasks := findBlockedTasks(task.ID, unblockedTaskType)

        for _, blockedTask := range blockedTasks {
            // Unblock task by removing blocker
            removeBlocker(blockedTask, task.ID)

            // Log unblock action
            logEntry := &LogEntry{
                TaskID: blockedTask.ID,
                By:     a.ID,
                Action: "UNBLOCKED",
                Note:   fmt.Sprintf("Unblocked by completion of %s", task.ID),
            }
        }
    }

    return nil
}
```

---

## Part 6: Example Flow Conversions

### Brainstorming Skill → Brainstorming Flow

```yaml
flow_id: "flow:brainstorming:1.0"
name: "Brainstorming"
version: "1.0"
description: "Transform ideas into designs through collaborative dialogue"
entry_step: "explore-context"

config:
  procedure: |
    # Brainstorming Process

    1. Understanding the idea
    2. Exploring approaches
    3. Presenting the design
    4. Documentation
    5. Implementation setup

steps:
  explore-context:
    step_id: "explore-context"
    type: "task"
    title: "Explore current project context"
    task_ref: "task:codebase-exploration"

  gather-requirements:
    step_id: "gather-requirements"
    type: "task"
    title: "Ask clarifying questions"
    task_ref: "task:requirements-gathering"
    depends_on: ["explore-context"]

  analyze-approaches:
    step_id: "analyze-approaches"
    type: "task"
    title: "Propose 2-3 approaches with trade-offs"
    task_ref: "task:approach-analysis"
    depends_on: ["gather-requirements"]

  validate-design:
    step_id: "validate-design"
    type: "task"
    title: "Present design in sections for validation"
    task_ref: "task:design-validation"
    depends_on: ["analyze-approaches"]

  write-documentation:
    step_id: "write-documentation"
    type: "task"
    title: "Write design document"
    task_ref: "task:design-documentation"
    depends_on: ["validate-design"]

  setup-implementation:
    step_id: "setup-implementation"
    type: "parallel"
    title: "Setup for implementation"
    children: ["create-worktree", "create-plan"]
    depends_on: ["write-documentation"]

  create-worktree:
    step_id: "create-worktree"
    type: "subflow"
    title: "Create isolated git worktree"
    flow_ref: "flow:git-worktrees:1.0"

  create-plan:
    step_id: "create-plan"
    type: "subflow"
    title: "Create implementation plan"
    flow_ref: "flow:writing-plans:1.0"
```

### Code Reviewer Agent → Code Analyst Assignee

```yaml
assignee_id: "assignee:code-analyst:1.0"
name: "Code Analyst"
version: "1.0"
description: "Senior code reviewer for plan alignment and quality assessment"

capabilities:
  task_types:
    - "code-review"
    - "plan-alignment-check"
    - "architecture-review"
    - "quality-assessment"

  tools:
    - "grep"
    - "git-diff"
    - "ast-parser"
    - "linter"

  domains:
    - "golang"
    - "python"
    - "javascript"
    - "system-design"
    - "testing"

instructions: |
  You are a Senior Code Reviewer with expertise in software architecture,
  design patterns, and best practices. Your role is to review completed
  project steps against original plans.

  Review Process:

  1. Plan Alignment Analysis
  2. Code Quality Assessment
  3. Architecture and Design Review
  4. Documentation and Standards
  5. Issue Identification and Recommendations
  6. Communication Protocol

delegation:
  handoff_conditions:
    - when: "security_vulnerability_found"
      delegate_to: "assignee:security-specialist:1.0"

    - when: "performance_issue_identified"
      delegate_to: "assignee:performance-engineer:1.0"

    - when: "critical_bugs_found"
      delegate_to: "assignee:debugger:1.0"

  unblocks:
    - "implementation"
    - "deployment"
    - "integration"
```

---

## Part 7: Implementation Plan

### Phase 1: Core Schema & Models

1. Add `Flow` extended schema with `config.procedure` field
2. Add `Assignee` model with capabilities
3. Add `TaskRequirements` to task metadata
4. Update task-flow-spec to include task templates

### Phase 2: Flow-to-Task Extraction

1. Implement task template expansion
2. Add task generation from flow steps
3. Add task requirement extraction

### Phase 3: Assignee Matching

1. Implement capability matching algorithm
2. Add assignee registry
3. Add auto-assignment on task creation

### Phase 4: Hand-off Mechanisms

1. Implement delegation logic
2. Add unblocking mechanism
3. Add subflow hand-off tracking

### Phase 5: CLI & TUI

1. Add `tlc flow invoke <flow-id>` command
2. Add `tlc assignee list` command
3. Update TUI to show flow progress
4. Add assignee view to TUI

### Phase 6: Example Flows & Assignees

1. Convert brainstorming skill → flow
2. Convert systematic-debugging skill → flow
3. Convert code-reviewer agent → assignee
4. Add test coverage

---

## File Structure

```
oss-tlc-cli/
├── flows/                    # Flow definitions
│   ├── brainstorming.yaml
│   ├── systematic-debugging.yaml
│   ├── writing-plans.yaml
│   └── git-worktrees.yaml
│
├── assignees/                # Assignee definitions
│   ├── code-analyst.yaml
│   ├── debugger.yaml
│   ├── architect.yaml
│   └── tester.yaml
│
├── internal/
│   ├── core/
│   │   ├── flow.go           # Existing
│   │   ├── flow_executor.go  # Existing
│   │   ├── assignee.go       # NEW
│   │   └── assignment_engine.go  # NEW
│   │
│   └── cli/
│       ├── flow.go           # Existing
│       └── assignee.go       # NEW
│
└── docs/
    ├── flow-spec-0.2.md      # Extended spec
    └── assignee-spec-0.1.md  # New spec
```

---

## Success Criteria

✓ Flows can extract task sequences like superpowers skills
✓ Assignees can execute specialized work like superpowers agents
✓ Flow-to-flow hand-offs work via subflow steps
✓ Assignee-to-assignee hand-offs work via delegation
✓ Task requirements match assignee capabilities
✓ CLI can invoke flows and manage assignees
✓ TUI displays flow progress and assignee status

---

## Next Steps

1. Review and validate this design
2. Create detailed specs for flow config and assignee schemas
3. Implement Phase 1 (Core Schema & Models)
4. Build Phase 2 (Flow-to-Task Extraction)
5. Continue through remaining phases
