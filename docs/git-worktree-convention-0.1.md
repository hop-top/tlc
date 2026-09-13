# Git Worktree Convention v0.1

## Overview

This document defines the Git worktree workflow convention for TLC development. Worktrees provide isolated working directories for parallel development, enabling clean task separation and fast context switching.

## Why Worktrees?

Git worktrees solve common development challenges:

- **Parallel development**: Work on multiple features/fixes simultaneously
- **Clean isolation**: Each task has independent filesystem state
- **Fast context switching**: No stashing or committing incomplete work
- **Safe experimentation**: Changes isolated to specific worktree
- **CI/build separation**: Run builds in separate worktrees without conflicts

---

## Worktree Directory Structure (Normative)

### Standard Location

```
$PROJECT_ROOT/
├── .worktrees/                          # Worktree container (gitignored)
│   ├── feat-0042-api-rate-limiting/   # Feature worktree
│   ├── fix-0108-login-timeout/         # Bugfix worktree
│   ├── chore-0089-refactor-auth/       # Parent task worktree
│   ├── chore-0089-refactor-login/      # Child task worktree
│   └── chore-0089-refactor-session/    # Child task worktree
├── .gitignore                           # Must contain .worktrees/
└── ... (main working directory)
```

### Rules (Normative)

1. **Location**: Worktrees MUST be in `$PWD/.worktrees/`
2. **Gitignore**: `.worktrees/` MUST be in `.gitignore`
3. **Creation**: Create `.worktrees/` if it doesn't exist
4. **Naming**: Worktree directory SHOULD match branch name
5. **Cleanup**: Remove worktrees after task completion

---

## When to Use Worktrees (Normative)

### MUST Use Worktrees

- **Parent tasks**: All parent tasks require worktrees
- **Large refactors**: Multi-file changes across modules
- **Breaking changes**: Changes requiring isolation from main work

### SHOULD Use Worktrees

- **Non-trivial child tasks**: Significant scope changes
- **Long-running work**: Tasks spanning multiple days
- **Experimental features**: High-risk or exploratory work

### MAY Skip Worktrees

- **Trivial fixes**: Single-line or documentation-only changes
- **Quick hotfixes**: Emergency fixes requiring immediate deployment
- **Small child tasks**: Minor changes in single file

---

## Worktree Lifecycle

### 1. Setup Phase

#### Create Worktree for Existing Branch

```bash
# Ensure worktrees directory exists
mkdir -p .worktrees

# Create worktree for existing branch
git worktree add .worktrees/feat-0042-api-rate-limiting feat/0042-api-rate-limiting

# Navigate to worktree
cd .worktrees/feat-0042-api-rate-limiting
```

#### Create Worktree with New Branch

```bash
# Create worktree and branch simultaneously
git worktree add -b feat/0042-api-rate-limiting .worktrees/feat-0042-api-rate-limiting

# Or specify base branch
git worktree add -b feat/0042-api-rate-limiting .worktrees/feat-0042-api-rate-limiting main

cd .worktrees/feat-0042-api-rate-limiting
```

#### Setup Script

```bash
#!/bin/bash
# setup-worktree.sh

BRANCH=$1
WORKTREE_NAME=$(echo "$BRANCH" | sed 's/\//-/g')  # feat/0042-auth -> feat-0042-auth
WORKTREE_PATH=".worktrees/$WORKTREE_NAME"

mkdir -p .worktrees

if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
  # Branch exists
  git worktree add "$WORKTREE_PATH" "$BRANCH"
else
  # Create new branch
  git worktree add -b "$BRANCH" "$WORKTREE_PATH"
fi

cd "$WORKTREE_PATH"
echo "Worktree ready: $WORKTREE_PATH"
```

### 2. Development Phase

Work proceeds normally within the worktree:

```bash
# Inside worktree
cd .worktrees/feat-0042-api-rate-limiting

# Install dependencies (if needed)
npm install

# Make changes
# ... edit files ...

# Run tests
npm test

# Commit work
git add .
git commit -m "feat: implement rate limiting middleware"

# Push to remote
git push origin feat/0042-api-rate-limiting
```

### 3. Cleanup Phase

#### Manual Cleanup

```bash
# Return to main directory
cd $PROJECT_ROOT

# Remove worktree
git worktree remove .worktrees/feat-0042-api-rate-limiting

# If worktree has uncommitted changes
git worktree remove .worktrees/feat-0042-api-rate-limiting --force

# Delete branch after merge
git branch -d feat/0042-api-rate-limiting
```

#### Cleanup Script

```bash
#!/bin/bash
# cleanup-worktree.sh

BRANCH=$1
WORKTREE_NAME=$(echo "$BRANCH" | sed 's/\//-/g')
WORKTREE_PATH=".worktrees/$WORKTREE_NAME"

# Return to project root
cd "$PROJECT_ROOT"

# Check if worktree exists
if [ -d "$WORKTREE_PATH" ]; then
  # Remove worktree (force if dirty)
  git worktree remove "$WORKTREE_PATH" --force

  echo "Removed worktree: $WORKTREE_PATH"

  # Optionally delete branch
  read -p "Delete branch $BRANCH? (y/N) " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    git branch -d "$BRANCH"
    echo "Deleted branch: $BRANCH"
  fi
else
  echo "Worktree not found: $WORKTREE_PATH"
fi
```

---

## Worktree Management

### List Worktrees

```bash
# Show all worktrees
git worktree list

# Example output:
# /path/to/project              a1b2c3d [main]
# /path/to/project/.worktrees/feat-0042-api-rate-limiting  e4f5g6h [feat/0042-api-rate-limiting]
```

### Prune Stale References

```bash
# Remove references to deleted worktrees
git worktree prune

# Dry run to see what would be pruned
git worktree prune --dry-run
```

### Move Worktree

```bash
# Rename/move worktree directory
git worktree move .worktrees/old-name .worktrees/new-name
```

### Lock Worktree

```bash
# Lock worktree to prevent automatic cleanup
git worktree lock .worktrees/feat-0042-api-rate-limiting --reason "Long-running work"

# Unlock
git worktree unlock .worktrees/feat-0042-api-rate-limiting
```

---

## Parent and Child Task Patterns

### Pattern 1: Parent with Sequential Children

```bash
# Parent task (issue #89)
git worktree add -b chore/0089-refactor-auth .worktrees/chore-0089-refactor-auth

# Child 1 (work sequentially)
git worktree add -b chore/0089-refactor-login .worktrees/chore-0089-refactor-login

# After child 1 complete, merge and cleanup
cd .worktrees/chore-0089-refactor-auth
git merge chore/0089-refactor-login
cd ../..
git worktree remove .worktrees/chore-0089-refactor-login

# Child 2
git worktree add -b chore/0089-refactor-session .worktrees/chore-0089-refactor-session
```

### Pattern 2: Parent with Parallel Children

```bash
# Parent task
git worktree add -b chore/0089-refactor-auth .worktrees/chore-0089-refactor-auth

# Multiple children in parallel
git worktree add -b chore/0089-refactor-login .worktrees/chore-0089-refactor-login
git worktree add -b chore/0089-refactor-session .worktrees/chore-0089-refactor-session
git worktree add -b chore/0089-refactor-permissions .worktrees/chore-0089-refactor-permissions

# Work on all in parallel (different terminals/agents)

# After all complete, merge into parent
cd .worktrees/chore-0089-refactor-auth
git merge chore/0089-refactor-login
git merge chore/0089-refactor-session
git merge chore/0089-refactor-permissions

# Cleanup all child worktrees
cd ../..
git worktree remove .worktrees/chore-0089-refactor-login
git worktree remove .worktrees/chore-0089-refactor-session
git worktree remove .worktrees/chore-0089-refactor-permissions
```

---

## Task Metadata

Development tasks using worktrees SHOULD include metadata:

```json
{
  "id": "T-0042",
  "title": "Implement API rate limiting",
  "status": "IN_PROGRESS",
  "assigned_to": "codex",
  "meta": {
    "issue_number": 42,
    "branch_name": "feat/0042-api-rate-limiting",
    "worktree_path": ".worktrees/feat-0042-api-rate-limiting",
    "commit_type": "feat",
    "worktree_created_at": "2025-01-16T10:00:00Z"
  }
}
```

Parent task with children:

```json
{
  "id": "T-0089",
  "title": "Refactor authentication module",
  "status": "IN_PROGRESS",
  "meta": {
    "issue_number": 89,
    "branch_name": "chore/0089-refactor-auth-module",
    "worktree_path": ".worktrees/chore-0089-refactor-auth-module",
    "child_tasks": ["T-0090", "T-0091", "T-0092"],
    "child_worktrees": [
      ".worktrees/chore-0089-refactor-login",
      ".worktrees/chore-0089-refactor-session",
      ".worktrees/chore-0089-refactor-permissions"
    ]
  }
}
```

---

## Best Practices

### DO ✅

- Create `.worktrees/` directory before adding worktrees
- Add `.worktrees/` to `.gitignore`
- Match worktree directory name to branch name
- Use worktrees for all parent tasks
- Clean up worktrees after task completion
- Lock long-running worktrees with reason
- Use absolute paths when referencing worktrees

### DON'T ❌

- Commit `.worktrees/` directory to repository
- Manually delete worktree directories (use `git worktree remove`)
- Create worktrees outside `.worktrees/` directory
- Leave stale worktrees indefinitely
- Share worktree directories across machines
- Use worktrees for trivial single-file changes

---

## Troubleshooting

### Issue: "fatal: '.worktrees/xyz' already exists"

**Cause**: Directory exists from previous worktree

**Solution**:
```bash
# Check if it's a valid worktree
git worktree list

# If not listed, remove directory and prune
rm -rf .worktrees/xyz
git worktree prune
```

### Issue: "fatal: 'branch' is already checked out"

**Cause**: Branch is checked out in another worktree

**Solution**:
```bash
# Find which worktree has the branch
git worktree list

# Remove the conflicting worktree
git worktree remove .worktrees/conflicting-worktree
```

### Issue: Cannot delete worktree with uncommitted changes

**Cause**: Worktree has modified or untracked files

**Solution**:
```bash
# Option 1: Force remove (loses changes)
git worktree remove .worktrees/xyz --force

# Option 2: Commit changes first
cd .worktrees/xyz
git add .
git commit -m "WIP: save progress"
cd ../..
git worktree remove .worktrees/xyz

# Option 3: Stash changes
cd .worktrees/xyz
git stash
cd ../..
git worktree remove .worktrees/xyz
```

### Issue: Worktree list shows deleted directory

**Cause**: Directory deleted manually without `git worktree remove`

**Solution**:
```bash
# Prune stale worktree references
git worktree prune
```

### Issue: Cannot create worktree - "Invalid reference"

**Cause**: Branch doesn't exist locally

**Solution**:
```bash
# Fetch branch from remote first
git fetch origin feat/0042-api-rate-limiting

# Then create worktree
git worktree add .worktrees/feat-0042-api-rate-limiting feat/0042-api-rate-limiting

# Or create with remote tracking
git worktree add .worktrees/feat-0042-api-rate-limiting -b feat/0042-api-rate-limiting origin/feat/0042-api-rate-limiting
```

---

## Integration with TLC Workflows

### Recipe

```yaml
recipe: feature-dev
version: 1.0.0
description: Feature development in a dedicated worktree
vars:
  branch:
    required: true
track:
  title: "Feature: {{branch}}"
  type: feature
steps:
  - id: create-worktree
    kind: exec
    title: "Create worktree for {{branch}}"
    exec:
      argv: [git, worktree, add, ".worktrees/{{branch}}", "{{branch}}"]

  - id: implement
    title: "Implement {{branch}}"
    depends_on: [create-worktree]

  - id: cleanup
    kind: exec
    title: "Remove worktree for {{branch}}"
    depends_on: [implement]
    exec:
      argv: [git, worktree, remove, ".worktrees/{{branch}}"]
```

```bash
tlc track create --recipe feature-dev --var branch=feat/0042-api-rate-limiting
tlc track execute feature-feat-0042-api-rate-limiting --agent claude
```

---

## Advanced Patterns

### Worktree for CI/CD

```bash
# Create worktree for build without affecting main work
git worktree add .worktrees/ci-build HEAD

cd .worktrees/ci-build
npm install
npm run build
npm test

# Build artifacts in isolated directory
cd ../..
git worktree remove .worktrees/ci-build
```

### Worktree for Code Review

```bash
# Check out PR for review
gh pr checkout 123 --worktree .worktrees/pr-123

cd .worktrees/pr-123
# Review code, run tests

cd ../..
git worktree remove .worktrees/pr-123
```

### Worktree for Hotfix

```bash
# Create hotfix from production
git worktree add -b hotfix/critical-bug .worktrees/hotfix-critical-bug production

cd .worktrees/hotfix-critical-bug
# Fix bug
git commit -am "fix: critical production bug"
git push origin hotfix/critical-bug

cd ../..
git worktree remove .worktrees/hotfix-critical-bug
```

---

## References

- git-branch-convention-0.1.md — Branch naming format
- git-commit-convention-0.1.md — Commit message format
- recipe-spec-0.1.md — Recipe grammar and step semantics
- [Git Worktree Documentation](https://git-scm.com/docs/git-worktree)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
