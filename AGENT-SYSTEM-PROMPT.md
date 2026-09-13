# TLC Task List System Prompt for AI Agents

## Overview

You are working on the TLC (Task Line CLI) project. The project uses a task list file in **Task Line Syntax (TLS)** format located at:

```
/sessions/vibrant-admiring-gates/mnt/tlc/TODO
```

Since the TLC CLI tool doesn't exist yet (you're building it!), you need to manually work with this task list file until the tool is functional.

## Task Line Syntax (TLS) Format

Each task is a single line with this structure:

```
[status] ID title @assignee #tag1 #tag2 prio:PN domain:area ref:path/to/doc.md
```

### Status Tokens
- `[ ]` - TODO (not started)
- `[~]` - IN_PROGRESS (actively being worked on)
- `[x]` - DONE (completed)
- `[-]` - SKIPPED (won't do)

### Components
- **ID**: Task identifier (e.g., `T-0001`, `T-0042`)
- **Title**: Brief description of the task
- **@assignee**: Who owns this task (e.g., `@engineer-1`, `@engineer-2`, `@engineer-3`)
- **#tags**: Categorization tags (e.g., `#storage`, `#cli`, `#auth`)
- **prio:PN**: Priority level (`prio:P0` = critical, `prio:P1` = high, `prio:P2` = medium, `prio:P3` = low)
- **domain:area**: Functional domain (e.g., `domain:storage`, `domain:cli`, `domain:auth`)
- **ref:path**: Reference to specification document (e.g., `ref:docs/task-crud-spec-0.1.md`)

### Example Tasks

```
[ ] T-0001 Initialize project structure and Go module @engineer-1 #setup #infra prio:P0 domain:infra
[~] T-0004 Implement Task repository interface @engineer-1 #storage #interface prio:P0 domain:storage
[x] T-0031 Setup Cobra CLI framework @engineer-2 #cli #setup prio:P0 domain:cli ref:docs/tlc-cli-spec-0.1.md
```

## Working with the Task List

### 1. Finding Your Tasks

Use `grep` to filter tasks by assignee:

```bash
# Find all tasks for engineer-1
grep "@engineer-1" TODO

# Find P0 tasks for engineer-1
grep "@engineer-1" TODO | grep "prio:P0"

# Find TODO tasks for engineer-2 in CLI domain
grep "@engineer-2" TODO | grep "\[ \]" | grep "domain:cli"

# Find all in-progress tasks
grep "\[~\]" TODO
```

### 2. Updating Task Status

Manually edit the TODO file to change status:

**Starting work on a task:**
```bash
# Change [ ] to [~] for the task you're starting
sed -i 's/\[ \] T-0001/\[~\] T-0001/' TODO
```

**Completing a task:**
```bash
# Change [~] to [x] when finished
sed -i 's/\[~\] T-0001/\[x\] T-0001/' TODO
```

**Or use a text editor** to manually change the status token.

### 3. Common Workflows

#### Starting Your Day (Engineer 1 Example)

```bash
# 1. See what you're currently working on
grep "@engineer-1" TODO | grep "\[~\]"

# 2. See what P0 tasks are pending
grep "@engineer-1" TODO | grep "\[ \]" | grep "prio:P0"

# 3. Pick a task and mark it in-progress
# Edit TODO file: change [ ] T-0002 to [~] T-0002
```

#### Checking Dependencies

Tasks are ordered with dependencies in mind. Generally:
- Work on P0 tasks before P1 tasks
- Tasks earlier in the file may be dependencies for later tasks
- Check `ref:` paths to read relevant specifications

#### Reading Specifications

When you see `ref:docs/some-spec.md`, read that document:

```bash
# Example: T-0003 references task-crud-spec
cat /sessions/vibrant-admiring-gates/mnt/tlc/docs/task-crud-spec-0.1.md
```

### 4. Reporting Progress

When updating status or completing work:

1. **Mark task status appropriately** (`[ ]` → `[~]` → `[x]`)
2. **Commit your code changes** with reference to task ID
3. **Update TODO file** in the same commit or separately

Example commit message:
```
T-0031: Setup Cobra CLI framework

- Initialize cobra-cli project structure
- Add root command with version flag
- Configure viper for config management

Updates TODO: T-0031 [ ] → [x]
```

## Task Breakdown by Engineer

### Engineer 1: Core Engine & Storage
- **Focus**: Storage layer, state machine, audit logs, config system, recipe engine, plugins
- **Domains**: `domain:storage`, `domain:core`, `domain:config`, `domain:recipe`, `domain:plugin`
- **Filter**: `grep "@engineer-1" TODO`

**Priority P0 Tasks** (critical path):
- T-0001 to T-0005: Project setup and database
- T-0007: Query filter design
- T-0014 to T-0016: State machine
- T-0018 to T-0019: Audit logging
- T-0023 to T-0025: Config system
- T-0029: Mock implementations

### Engineer 2: CLI & Interactive UI
- **Focus**: Cobra/Viper CLI, commands, output formatters, TUI with Bubble Tea
- **Domains**: `domain:cli`, `domain:tui`
- **Filter**: `grep "@engineer-2" TODO`

**Priority P0 Tasks** (critical path):
- T-0031 to T-0035: CLI framework and core commands
- T-0042: JSON formatter
- T-0044: Table formatter

### Engineer 3: Sync & External Integrations
- **Focus**: Authentication, GitHub/Jira/Linear sync, label management
- **Domains**: `domain:auth`, `domain:sync`, `domain:labels`
- **Filter**: `grep "@engineer-3" TODO`

**Priority P0 Tasks** (critical path):
- T-0063 to T-0065: Keychain and GitHub auth
- T-0074 to T-0075: GitHub plugin setup
- T-0080 to T-0082: Schema mapping and sync

## Bootstrap Process

You're in a unique situation: building a task management tool while managing tasks manually. Here's the approach:

1. **Phase 1A** (Weeks 1-2): Core engineers work on foundation
   - Engineer 1: Storage, state machine, basic CRUD
   - Engineer 2: CLI framework, basic commands
   - Engineer 3: Auth and GitHub plugin skeleton

2. **Phase 1B** (Week 2-3): Integration point
   - Get `tlc task list` working
   - Get `tlc task update` working
   - **Switch from manual TODO editing to using TLC!**

3. **Phase 2+**: Use TLC to manage TLC development
   - Once basic commands work, use `tlc task list @me status:TODO prio:P0`
   - Use `tlc task update T-0042 --status IN_PROGRESS`
   - Eat your own dog food!

## Quick Reference

### Find tasks to work on
```bash
grep "@engineer-1" TODO | grep "\[ \]" | grep "prio:P0"
```

### Mark task in progress
```bash
sed -i 's/\[ \] T-0001/\[~\] T-0001/' TODO
```

### Mark task complete
```bash
sed -i 's/\[~\] T-0001/\[x\] T-0001/' TODO
```

### Count progress
```bash
# Total tasks
wc -l TODO

# Completed
grep "\[x\]" TODO | wc -l

# In progress
grep "\[~\]" TODO | wc -l

# Pending
grep "\[ \]" TODO | wc -l
```

### View tasks by domain
```bash
grep "domain:storage" TODO
grep "domain:cli" TODO
grep "domain:auth" TODO
```

## Tips for AI Agents

1. **Always check task dependencies**: Read earlier tasks in the same domain
2. **Read referenced specs**: The `ref:` token points to critical documentation
3. **Update status promptly**: Keep TODO file current so other agents know what's being worked on
4. **One task at a time**: Move task to `[~]` when starting, `[x]` when complete
5. **Check for conflicts**: Before starting a task, grep for `[~]` to see what others are working on
6. **Use consistent formatting**: Don't add extra spaces or change structure
7. **Respect priorities**: P0 tasks should be completed before P1, P1 before P2

## Testing Guidelines

When testing CLI commands (using Cobra and Viper):

1. **Always create fresh command instances** using helper functions like `newTestCmd()` or `newTestInitCmd()` from `internal/cli/test_helpers.go`.
2. **Never share command instances across test runs** to avoid Cobra flag state leakage.
3. **Use `viper.Reset()` at the start of each test** to clear configuration state.
4. **Reset environment variables** to avoid cross-test pollution when testing configuration-related functionality.
5. **Create temporary directories** for test isolation and clean them up with `defer os.RemoveAll()`.

Example test structure:
```go
t.Run("test_name", func(t *testing.T) {
    viper.Reset()

    cmd := newTestCmd()
    initCmd := newTestInitCmd()
    cmd.AddCommand(initCmd)

    buf := new(strings.Builder)
    cmd.SetOut(buf)
    cmd.SetErr(buf)
    cmd.SetArgs([]string{"init", "--track"})

    err = cmd.Execute()
    // ... assertions ...
})
```

## Documentation References

All specification documents are in `/sessions/vibrant-admiring-gates/mnt/tlc/docs/`:

- `task-crud-spec-0.1.md` - Task data model and CRUD operations
- `task-line-spec-0.1.md` - Task Line Syntax grammar
- `tlc-cli-spec-0.1.md` - CLI command reference
- `tlc-config-spec-0.1.md` - Configuration system
- `tlc-query-spec-0.1.md` - Query and filter language
- `tlc-plugin-spec-0.1.md` - Plugin system architecture
- `tlc-auth-spec-0.1.md` - Authentication for external systems
- `tlc-tui-spec-0.1.md` - Terminal UI design with Bubble Tea
- `recipe-spec-0.1.md` - Recipe grammar and step semantics
- `task-log-spec-0.1.md` - Audit logging system

Read these documents before starting work on related tasks.

## Example: A Day in the Life of Engineer 1

```bash
# Morning: Check status
grep "@engineer-1" TODO | grep "\[~\]"
# Output: [~] T-0004 Implement Task repository interface...

# Continue working on T-0004...
# ... implement code ...
# ... write tests ...

# Complete T-0004
sed -i 's/\[~\] T-0004/\[x\] T-0004/' TODO
git add internal/storage/repository.go internal/storage/repository_test.go
git commit -m "T-0004: Implement Task repository interface

- Add TaskRepository interface with CRUD methods
- Implement SQLite-backed repository
- Add unit tests with 95% coverage

Updates TODO: T-0004 [~] → [x]"

# Pick next task
grep "@engineer-1" TODO | grep "\[ \]" | grep "prio:P0" | head -5
# Output shows: T-0005 Implement Task CRUD operations

# Read the spec
cat docs/task-crud-spec-0.1.md

# Start T-0005
sed -i 's/\[ \] T-0005/\[~\] T-0005/' TODO
git add TODO
git commit -m "TODO: Start work on T-0005 (Task CRUD operations)"

# ... continue working ...
```

---

**Remember**: This manual process is temporary. Once `tlc task` commands are functional (around end of Phase 1A), you'll switch to using TLC itself to manage these tasks. Until then, treat the TODO file as the source of truth and update it diligently.
