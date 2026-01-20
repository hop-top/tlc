# Getting Started with TLC

This guide walks you through TLC concepts progressively, from basic task management to advanced multi-agent workflows.

## Table of Contents

- [🎯 Learning Path](#-learning-path)
  - [Stage 1: Solo Task Management](#stage-1-solo-task-management-5-minutes)
  - [Stage 2: Task Line Syntax](#stage-2-task-line-syntax-5-minutes)
  - [Stage 3: AI Agent Integration](#stage-3-ai-agent-integration-10-minutes)
  - [Stage 4: Multi-Agent Collaboration](#stage-4-multi-agent-collaboration-15-minutes)
  - [Stage 5: GitHub Sync](#stage-5-github-sync-15-minutes)
  - [Stage 6: Flows & Assignees](#stage-6-flows--assignees-20-minutes)
  - [Stage 7: Advanced Flows](#stage-7-advanced-flows-30-minutes)
  - [Stage 8: Interactive TUI](#stage-8-interactive-tui-10-minutes)
  - [Stage 9: Git Conventions](#stage-9-git-conventions-optional-15-minutes)
  - [Stage 10: Custom Plugins](#stage-10-custom-plugins-advanced)
- [🎓 Learning Checkpoints](#-learning-checkpoints)
- [🚫 Common Mistakes to Avoid](#-common-mistakes-to-avoid)
- [🎯 Quick Reference by Role](#-quick-reference-by-role)
- [📚 Next Steps](#-next-steps)
- [❓ Still Confused?](#-still-confused)

## 🎯 Learning Path

Follow these stages in order. Each builds on the previous:

### Stage 1: Solo Task Management (5 minutes)
**Goal**: Use TLC as a personal todo list

**Concepts**: Tasks, statuses, basic CLI
**Docs**: None needed yet
**Try**:
```bash
tlc init
tlc task create "Fix login bug" --tag bug
tlc task list
tlc task update T-0001 --status IN_PROGRESS
tlc task update T-0001 --status DONE
```

**When to move on**: You can create, update, and list tasks comfortably.

---

### Stage 2: Task Line Syntax (5 minutes)
**Goal**: Understand the human-readable format

**Concepts**: TLS format, direct file editing
**Docs**: [Task Line Syntax](task-line-spec-0.1.md#canonical-examples)
**Try**:
```bash
cat ~/.local/share/tlc/todo.txt
# Edit the file directly with your editor
# Watch it sync back to SQLite automatically
tlc task list  # See your manual edits reflected
```

**When to move on**: You understand the `[status] ID title @assignee #tag` format.

---

### Stage 3: AI Agent Integration (10 minutes)
**Goal**: Let AI agents manage their own tasks

**Concepts**: Internal vs external tasks, tool definition
**Docs**: [AGENTS.md](../AGENTS.md), [README AI Integration](../README.md#ai-agent-integration)
**Try**:
1. Add TLC config to `~/.claude/CLAUDE.md` (see README)
2. Ask Claude: "Create a task to refactor the auth module"
3. Observe Claude using `tlc task create` instead of TodoWrite
4. Check: `tlc task list --mine`

**When to move on**: Your AI assistant is creating and managing TLC tasks.

---

### Stage 4: Multi-Agent Collaboration (15 minutes)
**Goal**: Coordinate work between multiple agents/humans

**Concepts**: Claiming, delegation, ownership transfer
**Docs**: [Task Collaboration Spec](task-collab-spec-0.1.md#example-workflows)
**Try**:
```bash
# Create a task someone else will do
tlc task create "Write API docs" --assigned-to @teammate

# Claim an unassigned task
tlc task claim T-0005

# Release if you can't finish
tlc task unclaim T-0005

# View audit trail
tlc log T-0005
```

**When to move on**: You understand claiming, releasing, and delegation patterns.

---

### Stage 5: GitHub Sync (15 minutes)
**Goal**: Sync TLC with external issue trackers

**Concepts**: Internal vs external tasks, bidirectional sync
**Docs**: [Sync Architecture](sync-architecture-0.1.md#use-cases)
**Try**:
```bash
# Setup GitHub sync (auto-configures on first run)
tlc sync pull github

# List synced tasks (have origin_system)
tlc task list | grep github

# Make a local change
tlc task update T-0010 --status IN_PROGRESS

# Push back to GitHub
tlc sync push github
```

**When to move on**: You can sync tasks with GitHub Issues.

---

### Stage 6: Flows & Assignees (20 minutes)
**Goal**: Automate workflows with procedural task generation

**Concepts**: Flow definitions, capability-based assignment
**Docs**: [Flows & Assignees](flows-and-assignees.md)
**Try**:
```bash
# List available flows
tlc flow list

# Invoke a brainstorming flow
tlc flow invoke examples/flows/brainstorming.yaml

# See generated tasks with auto-assignment
tlc task list --tag flow

# Check assignee capabilities
tlc assignee list
tlc assignee show assignee:code-analyst:1.0
```

**When to move on**: You've invoked a flow and understand how tasks get auto-assigned.

---

### Stage 7: Advanced Flows (30 minutes)
**Goal**: Create custom workflows for your team

**Concepts**: Sequential/parallel execution, branching, retries
**Docs**: [Task Flow Spec](task-flow-spec-0.1.md#canonical-examples)
**Examples to study**:
- `examples/flows/test-driven-development.yaml` - Sequential with dependencies
- `examples/flows/systematic-debugging.yaml` - Conditional branching
- `examples/flows/code-review.yaml` - Parallel checks with join

**Try**: Create your own flow YAML for your team's workflow

**When to move on**: You understand flow structure and can write basic flows.

---

### Stage 8: Interactive TUI (10 minutes)
**Goal**: Use the visual terminal interface

**Concepts**: Dashboard, Kanban board, flow monitoring
**Docs**: [TUI Spec](tlc-tui-spec-0.1.md)
**Try**:
```bash
tlc tui

# Keybindings:
# v - cycle views (Dashboard → Kanban → Flows)
# n - create new task (interactive form)
# c - claim task
# s - cycle status
# / - filter/search
# t - theme picker
```

**When to move on**: You're comfortable navigating the TUI.

---

### Stage 9: Git Conventions (Optional, 15 minutes)
**Goal**: Integrate TLC with Git workflow

**Concepts**: Branch naming, commit messages, worktrees
**Docs**:
- [Git Branch Convention](git-branch-convention-0.1.md#examples-by-scenario)
- [Git Commit Convention](git-commit-convention-0.1.md#examples-by-type)
- [Git Worktree Convention](git-worktree-convention-0.1.md)

**Try**: Follow the conventions when working on TLC tasks

---

### Stage 10: Custom Plugins (Advanced)
**Goal**: Extend TLC with custom integrations

**Concepts**: Plugin types, gRPC interface
**Docs**: [Plugin Spec](tlc-plugin-spec-0.1.md)

---

## 🎓 Learning Checkpoints

After completing stages 1-3, you should be able to:
- ✅ Manage personal tasks via CLI
- ✅ Edit tasks in human-readable format
- ✅ Have AI agents create tasks instead of using TodoWrite

After completing stages 4-6, you should be able to:
- ✅ Coordinate work across multiple agents/humans
- ✅ Sync tasks with GitHub/Jira/Linear
- ✅ Automate workflows with flows

After completing stages 7-8, you should be able to:
- ✅ Design custom workflows for your team
- ✅ Use the visual TUI for day-to-day work

---

## 🚫 Common Mistakes to Avoid

### Mistake 1: Mixing internal and external tasks
**Problem**: Trying to sync agent-created tasks to GitHub
**Solution**: Internal tasks (no `origin_system`) stay local. Only import external tasks via `tlc sync pull`.
**Docs**: [Sync Architecture Use Cases](sync-architecture-0.1.md#use-cases)

### Mistake 2: Modifying completed tasks
**Problem**: Trying to claim or update tasks in DONE/SKIPPED status
**Solution**: These states are terminal. Create a new task or revert status first.
**Docs**: [Task Collaboration Invariants](task-collab-spec-0.1.md#ownership-invariants-normative)

### Mistake 3: Skipping `tlc init`
**Problem**: Commands fail because TLC isn't initialized
**Solution**: Always run `tlc init` in new projects before using TLC commands.
**Docs**: [README Quick Start](../README.md#quick-start)

### Mistake 4: Editing SQLite directly
**Problem**: Manual SQLite changes might break invariants
**Solution**: Edit `todo.txt` or use CLI commands. Let TLC manage the database.
**Docs**: [Task CRUD Spec](task-crud-spec-0.1.md)

### Mistake 5: Using flows before understanding basic tasks
**Problem**: Flows feel overwhelming and confusing
**Solution**: Master stages 1-4 first. Flows are just automated task creation.
**Docs**: This guide (follow the stages)

---

## 🎯 Quick Reference by Role

### For Solo Developers
**Learn**: Stages 1-2, 8 (CLI + TUI)
**Skip**: Stages 4, 6-7 (collaboration, flows)
**Maybe**: Stage 5 (GitHub sync if you use issues)

### For AI Agent Developers
**Learn**: Stages 1-3, 4 (task basics + AI integration + collaboration)
**Skip**: Stages 8-9 (TUI, Git conventions)
**Focus**: [AGENTS.md](../AGENTS.md), [Sync Architecture Agent Use Case](sync-architecture-0.1.md#agent-tool-function)

### For Team Leads
**Learn**: All stages
**Focus**: Stages 6-7 (flows for team workflows)
**Customize**: Create team-specific flows in `examples/flows/`

### For DevOps Engineers
**Learn**: Stages 1-5, 9 (basics through GitHub sync + Git integration)
**Skip**: Stages 6-7 (flows, unless automating deployments)
**Consider**: Stage 10 (custom plugins for CI/CD)

---

## 📚 Next Steps

After completing this guide:

1. **Read the specs** that interest you (see [docs/README.md](README.md))
2. **Customize your setup** (`.tlc/config.yaml`)
3. **Create team flows** (`examples/flows/your-workflow.yaml`)
4. **Contribute** (see [CONTRIBUTING.md](../CONTRIBUTING.md))

## ❓ Still Confused?

If you're stuck:
1. Check which **stage** you're on
2. Re-read the **docs** for that stage
3. Look at **examples** in the linked specs
4. Ask in [GitHub Discussions](https://github.com/IdeaCraftersLabs/oss-tlc-cli/discussions)

**Remember**: Each stage builds on the previous. Don't skip ahead!
