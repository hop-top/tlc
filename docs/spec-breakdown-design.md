# Spec/Plan Task Breakdown - Design Document

## Overview

This document describes the design for implementing spec/plan task breakdown functionality in TLC, allowing users to create hierarchical task structures (spec → phases → steps → sub-tasks) without introducing new database entities.

## Design Decisions

### 1. Spec Reusability
**Decision**: When a reusable spec is used in a project:
- ✅ Copy all phases/steps to project-specific tasks
- ✅ Link to the reusable spec via Reference field
- ✅ Allow both project-scoped and cross-project specs

**Implementation**:
- When `tlc task use-spec <spec-id>` is called:
  - Creates new tasks for the spec and all its descendants
  - Sets `Meta["spec_id"]` to track origin spec
  - Sets `Reference` to the original spec's ID
  - Copies hierarchy with new IDs for project-specific tasks

### 2. Breakdown Prompt Configuration
**Decision**: Yes, configurable templates/prompts in local project config for different workflows.

**Config Location**: `.tlc/config.yaml`

**Config Structure**:
```yaml
task:
  # ... existing config ...

  # Breakdown prompt templates
  breakdown:
    # Default template
    default: |
      Break down this task into:
      - Major phases (high-level components)
      - Steps within each phase
      - Specific sub-tasks for complex steps

      For each item, provide:
      - Title (clear and actionable)
      - Estimated complexity (1-5)
      - Dependencies (if any)

    # Workflow-specific templates
    templates:
      engineering: |
        Break down this technical task into phases:
        1. Research/design phase
        2. Implementation phases
        3. Testing phases
        4. Documentation phases

        For each phase, identify key steps.
        Each step may have granular sub-tasks.

        Be realistic and consider complexity.

      brainstorming: |
        Generate a comprehensive list of all potential tasks needed.
        Group related tasks into logical phases.
        Don't worry about perfect ordering yet.

      research: |
        Break down into research phases:
        1. Literature review
        2. Proof of concept
        3. Validation
        4. Documentation

        Include specific deliverables for each phase.

      product_management: |
        Break down into:
        1. Discovery/research phase
        2. Design/definition phases
        3. Implementation phases
        4. Launch/marketing phases

        Include stakeholder deliverables.

  # Auto-complete spec behavior
  auto_complete_spec: true  # Mark spec DONE when progress = 100%
```

### 3. Progress Calculation
**Decision**: Regardless of task type - simple count of completed vs total.

**Algorithm**:
```go
// Calculate progress for a spec/phase
children := getAllChildren(taskID)
completedCount := countChildrenWithStatus(taskID, "DONE", "SKIPPED")
totalCount := len(children)

progress := (completedCount / totalCount) * 100
```

**Display**: `"Phase 1 (3/5 tasks, 60%)"`

### 4. Task Reordering
**Decision**: Implement `tlc task reorder <child1> <child2> <child3>` command.

**Implementation**:
- Updates `Meta["phase_order"]` or `Meta["step_order"]` fields
- Relative reordering (move child to new position)
- Validates all children share same parent

**Usage**:
```bash
tlc task reorder T-0002 T-0003 T-0001  # Reorders phases under spec
tlc task reorder --parent T-0002 T-0010 T-0011  # Reorders steps under phase
```

### 5. AI Tool Contract
**Decision**: User-configured format (YAML/JSON) along with the prompt.

**Config Structure**:
```yaml
ai:
  enabled: true  # Enable AI-assisted breakdown

  # AI provider configuration (existing)
  provider: "claude"  # or "gemini", "openai", etc.
  model: "claude-3-5-sonnet"
  api_key: "..."

  # Breakdown configuration
  breakdown:
    # Format for task breakdown output
    format: "yaml"  # "yaml" or "json"

    # Use project-specific or global template
    template: "engineering"  # or "default", "brainstorming", etc.

    # Custom prompt override (optional)
    prompt_override: null
```

**Expected Output Format**:

**YAML Format (default)**:
```yaml
spec: T-0001
title: Build new API
children:
  - title: "Phase 1: Design"
    type: "phase"
    children:
      - title: "Step 1.1: API specification"
        type: "step"
        children:
          - title: "Define endpoints"
            type: "sub-task"
            description: "Document all REST endpoints"
            tags: ["#backend", "#docs"]
          - title: "Define data models"
            type: "sub-task"
            description: "Specify request/response schemas"
            tags: ["#backend", "#data"]

      - title: "Step 1.2: Architecture design"
        type: "step"
        children:
          - title: "Create architecture diagram"
            type: "sub-task"
            description: "Design system architecture"
            tags: ["#architecture"]

  - title: "Phase 2: Implementation"
    type: "phase"
    phase_order: 2
    children:
      - title: "Step 2.1: Core endpoints"
        type: "step"
        step_order: 1
        children:
          - title: "Implement authentication"
            type: "sub-task"
            tags: ["#auth", "#security"]
          - title: "Implement CRUD operations"
            type: "sub-task"
            tags: ["#backend"]

      - title: "Step 2.2: Testing"
        type: "step"
        step_order: 2
        children:
          - title: "Write unit tests"
            type: "sub-task"
          - title: "Write integration tests"
            type: "sub-task"
```

**JSON Format**:
```json
{
  "spec": "T-0001",
  "title": "Build new API",
  "children": [
    {
      "title": "Phase 1: Design",
      "type": "phase",
      "phase_order": 1,
      "children": [
        {
          "title": "Step 1.1: API specification",
          "type": "step",
          "step_order": 1,
          "description": "Document all REST endpoints",
          "tags": ["#backend", "#docs"],
          "children": [
            {
              "title": "Define endpoints",
              "type": "sub-task",
              "description": "Document all REST endpoints",
              "tags": ["#backend", "#docs"]
            },
            {
              "title": "Define data models",
              "type": "sub-task",
              "description": "Specify request/response schemas",
              "tags": ["#backend", "#data"]
            }
          ]
        }
      ]
    }
  ]
}
```

## Data Structure

### Meta Fields
```go
Meta: {
    // Task type classification
    "task_type": "spec",           // Values: "spec", "phase", "step", "sub-task", "task"

    // Hierarchical relationships
    "parent_id": "T-0001",        // Direct parent task ID
    "spec_id": "my-api-spec",       // ID for grouping reusable specs

    // Display ordering (optional, flexible)
    "phase_order": 1,               // For phase ordering within spec
    "step_order": 2,                // For step ordering within phase

    // Progress tracking (optional, for caching)
    "completed_count": 3,            // Cache for performance
    "total_count": 5,
}
```

### Reference Encoding
```go
Spec:     "spec://my-api-spec"           // Reusable spec ID
Phase:     "phase://1:spec://my-api-spec"   // Phase 1 of spec
Step:      "step://auth:phase://1:spec://my-api-spec"  // Auth step of phase 1
Sub-task:  "task://login:step://auth:phase://1:spec://my-api-spec"  // Login task
```

### Tags
```go
["#spec", "#backend", "auth"]     // Type + domain tags
["#phase", "#backend"]               // Type + domain tags
["#step", "#auth"]                   // Type + domain tags
["#sub-task", "#ui"]                // Type + domain tags
```

## Commands to Implement

### Spec Management
```bash
# Create a spec
tlc task create --type spec "Build new API"

# Mark existing task as spec
tlc task mark-as-spec <task-id> [--spec-id <name>]

# Breakdown spec (AI-assisted)
tlc task breakdown <spec-id> [--template <template-name>]

# Import from AI (bulk)
tlc task import --from-ai <file.yaml>

# Use reusable spec in current project
tlc task use-spec <spec-id>
```

### Hierarchy Management
```bash
# Create child tasks
tlc task create --parent <spec-id> --type phase "Phase 1: Design"
tlc task create --parent <phase-id> --type step "Step 1.1: API spec"
tlc task create --parent <step-id> --type sub-task "Define endpoints"

# Link existing tasks to parent
tlc task link --parent <spec-id> <child1> <child2> <child3>

# Reorder children
tlc task reorder <child1> <child2> <child3>
tlc task reorder --parent <phase-id> <child1> <child2>

# Unlink from parent
tlc task unlink <task-id>
```

### Viewing & Progress
```bash
# Show hierarchy tree
tlc task expand <spec-id> [--depth <levels>]

# Show progress
tlc task progress <spec-id>

# List by type
tlc task list --type spec
tlc task list --type phase --parent <spec-id>
tlc task list --type step --parent <phase-id>
tlc task list --type sub-task --parent <step-id>

# Filter by spec_id
tlc task list --spec-id my-api-spec

# Combined filters
tlc task list --type step --parent <phase-id> --status TODO
```

### Configuration
```bash
# Set breakdown template
tlc config set task.breakdown.template engineering

# Set custom prompt
tlc config set task.breakdown.prompt_override "Break down as an agile engineer..."

# Disable auto-completion
tlc config set task.auto_complete_spec false
```

## Query Extensions

### New Filters
```go
// In core.Query
Filters: []FieldFilter{
    {Field: "task_type", Operator: ":", Value: "spec"},
    {Field: "parent_id", Operator: ":", Value: "T-0001"},
    {Field: "spec_id", Operator: ":", Value: "my-api-spec"},
}
```

### SQL Implementation
```go
// In sqlite.go ListTasks
if field == "task_type" {
    groupClauses = append(groupClauses,
        fmt.Sprintf("json_extract(meta, '$.%s') %s ?", field, op))
    args = append(args, f.Value)
}
if field == "parent_id" {
    groupClauses = append(groupClauses,
        fmt.Sprintf("json_extract(meta, '$.%s') %s ?", field, op))
    args = append(args, f.Value)
}
if field == "spec_id" {
    groupClauses = append(groupClauses,
        fmt.Sprintf("json_extract(meta, '$.%s') %s ?", field, op))
    args = append(args, f.Value)
}
```

### Hierarchy Queries
```go
// Get all descendants (recursive)
func getAllChildren(parentID string) []Task {
    // Get direct children
    directChildren := ListTasks(Query{Filters: []FieldFilter{
        {Field: "parent_id", Operator: "=", Value: parentID}
    }})

    // Recursively get grandchildren
    for _, child := range directChildren {
        descendants := getAllChildren(child.ID)
        directChildren = append(directChildren, descendants...)
    }

    return directChildren
}

// Calculate progress
func calculateProgress(taskID string) (int, int, int) {
    children := getAllChildren(taskID)
    completed := 0
    for _, child := range children {
        if child.Status == "DONE" || child.Status == "SKIPPED" {
            completed++
        }
    }

    total := len(children)
    progress := 0
    if total > 0 {
        progress = (completed / total) * 100
    }

    return progress, completed, total
}
```

## Implementation Phases

### Phase 1: Core Data & Queries
- [ ] Extend SQLite queries to filter by meta fields (task_type, parent_id, spec_id)
- [ ] Implement `getAllChildren()` recursive function
- [ ] Implement `calculateProgress()` function
- [ ] Add tests for meta field filtering
- [ ] Add tests for hierarchy queries

### Phase 2: Spec Commands
- [ ] `tlc task create --type <spec|phase|step|sub-task>`
- [ ] `tlc task mark-as-spec <id> [--spec-id <name>]`
- [ ] Set appropriate tags/meta for each type
- [ ] `tlc task list --type <spec|phase|step|sub-task>`
- [ ] Add tests for spec creation
- [ ] Add tests for type filtering

### Phase 3: Hierarchy Commands
- [ ] `tlc task create --parent <id> --type <type>`
- [ ] `tlc task link --parent <id> <child...>`
- [ ] `tlc task unlink <id>`
- [ ] `tlc task expand <id> [--depth <n>]` - Tree display
- [ ] `tlc task reorder <child...>`
- [ ] Add tests for linking
- [ ] Add tests for reordering
- [ ] Add tests for tree display

### Phase 4: Breakdown & Progress
- [ ] `tlc task breakdown <id> [--template <name>]`
- [ ] `tlc task progress <id>` - Progress display
- [ ] AI integration (call external tool, parse YAML/JSON)
- [ ] Create hierarchy from breakdown output
- [ ] Auto-complete-spec logic in task updates
- [ ] Config option for auto-completion
- [ ] Add tests for breakdown parsing
- [ ] Add tests for progress calculation
- [ ] Add tests for auto-completion

### Phase 5: Spec Reusability
- [ ] `tlc task use-spec <spec-id>`
- [ ] Copy spec hierarchy to new tasks
- [ ] Set Reference to original spec
- [ ] Set Meta["spec_id"] to track origin
- [ ] Handle cross-project vs project-scoped specs
- [ ] Add tests for spec reuse
- [ ] Add tests for cross-project spec copying

### Phase 6: AI Import & Configuration
- [ ] `tlc task import --from-ai <file>` - Bulk import
- [ ] Support YAML and JSON formats
- [ ] Handle nested hierarchies
- [ ] Parse and validate breakdown output
- [ ] Add breakdown config section to schema
- [ ] Template system for prompts
- [ ] Prompt override support
- [ ] Add tests for YAML import
- [ ] Add tests for JSON import
- [ ] Add tests for template system

### Phase 7: UX & Display
- [ ] Tree view in `tlc task expand` (ASCII/unicode tree)
- [ ] Progress bar visualization
- [ ] Status colors based on progress
- [ ] Indicators for task types (📋 spec, 📁 phase, 📌 step, ✓ sub-task)
- [ ] Filter combos in `tlc task list`
- [ ] Add tests for tree display
- [ ] Add tests for progress display
- [ ] Visual tests for unicode/ascii trees

## Edge Cases & Considerations

1. **Cyclic Dependencies**: Detect and prevent a task from being its own ancestor
2. **Orphaned Tasks**: Handle tasks whose parent doesn't exist (warn, don't filter them out)
3. **Reordering Across Levels**: Prevent reordering tasks from different parents
4. **Cross-Project Linking**: Allow linking to tasks in other projects (via project_id)
5. **Spec Deletion**: Cascade delete (ask confirmation) or preserve children
6. **Progress Consistency**: Recalculate progress when children change status
7. **Concurrent Updates**: Handle multiple users updating hierarchy
8. **AI Tool Errors**: Graceful handling when AI tool fails or returns invalid format
9. **Large Hierarchies**: Pagination for `expand` command on large specs
10. **Empty Specs**: Handle specs with no children (0% progress)

## Testing Strategy

### Unit Tests
- Meta field filtering
- Hierarchy queries
- Progress calculation
- Task type assignment
- Parent-child linking

### Integration Tests
- Spec breakdown flow
- Hierarchy management
- Spec reusability
- AI import workflow

### E2E Tests
- Complete spec lifecycle
- Multi-user scenarios
- Large spec handling
- Cross-project operations
- Reordering edge cases

## Backward Compatibility

- Existing tasks without task_type work as regular tasks
- Existing queries without type filters return all tasks
- Reference field format is additive (can coexist with task://)
- No database schema changes required
