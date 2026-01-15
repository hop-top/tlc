# TLC Development Workflow Conventions

## Overview

This document defines the Git worktree usage and branch naming conventions for software development tasks in TLC (Task Line CLI). These conventions ensure clean isolation, consistent commit messages, and proper issue tracking.

## Git Worktree Workflow

### Why Worktrees?

Git worktrees provide isolated working directories for each development task, enabling:
- **Parallel development**: Work on multiple features/fixes simultaneously
- **Clean separation**: Each task has its own filesystem state
- **Fast context switching**: No need to stash/commit incomplete work
- **Safe experimentation**: Changes in one worktree don't affect others

### Worktree Directory Structure

```
$PROJECT_ROOT/
├── .worktrees/                          # Git-ignored worktree directory
│   ├── feat-0013-add-authentication/   # Feature branch worktree
│   ├── fix-0042-login-timeout/         # Bug fix branch worktree
│   ├── chore-0089-refactor-auth/       # Refactor parent task worktree
│   ├── chore-0089-refactor-login/      # Child task worktree
│   └── chore-0089-refactor-session/    # Child task worktree
├── .gitignore                           # Must include .worktrees/
└── ...
```

### Worktree Rules

#### MUST Requirements

1. **Worktree location**: All worktrees MUST be in `$PWD/.worktrees/`
2. **Gitignore**: `.worktrees/` MUST be in `.gitignore`
3. **Parent tasks**: Parent tasks MUST always use worktrees
4. **Directory creation**: Create `.worktrees/` if it doesn't exist

#### SHOULD Requirements

1. **Naming**: Worktree directory name SHOULD match branch name
2. **Scope-based**: Child tasks SHOULD use worktrees if scope is non-trivial
3. **Cleanup**: Remove worktrees after task completion

### Worktree Lifecycle

#### 1. Setup Phase

```bash
# Ensure worktrees directory exists
mkdir -p .worktrees

# Create worktree for new branch
git worktree add .worktrees/feat-0042-api-rate-limiting feat/0042-api-rate-limiting

# Navigate to worktree
cd .worktrees/feat-0042-api-rate-limiting
```

Or create branch from worktree:

```bash
# Create worktree and branch simultaneously
git worktree add -b feat/0042-api-rate-limiting .worktrees/feat-0042-api-rate-limiting

cd .worktrees/feat-0042-api-rate-limiting
```

#### 2. Development Phase

```bash
# Install dependencies (if needed)
npm install

# Make code changes
# ... edit files ...

# Run tests
npm test

# Commit changes
git add .
git commit -m "feat: implement API rate limiting"
```

#### 3. Cleanup Phase

```bash
# Return to main working directory
cd $PROJECT_ROOT

# Push changes (if needed)
cd .worktrees/feat-0042-api-rate-limiting
git push origin feat/0042-api-rate-limiting
cd ../..

# Remove worktree
git worktree remove .worktrees/feat-0042-api-rate-limiting

# Optional: Delete remote branch after merge
git branch -d feat/0042-api-rate-limiting
```

### Worktree Management Commands

```bash
# List all worktrees
git worktree list

# Prune stale worktree references
git worktree prune

# Remove worktree (with force if dirty)
git worktree remove .worktrees/feat-0042-api-rate-limiting --force

# Move worktree to different location
git worktree move .worktrees/old-name .worktrees/new-name
```

---

## Branch Naming Convention

### Format

```
<type>/<issue-id>-<purpose>
```

**Components:**
- `<type>`: Conventional commit type (required)
- `<issue-id>`: Zero-padded 4-digit issue number (optional)
- `<purpose>`: Hyphen-separated description (required)

### Valid Types

| Type | Description | Example |
|------|-------------|---------|
| `feat` | New feature | `feat/0013-add-authentication` |
| `fix` | Bug fix | `fix/0042-resolve-timeout` |
| `chore` | Maintenance/refactoring | `chore/0089-refactor-auth` |
| `docs` | Documentation | `docs/0023-update-api-docs` |
| `test` | Tests | `test/0018-add-integration-tests` |
| `perf` | Performance | `perf/0056-optimize-queries` |
| `refactor` | Code refactoring | `refactor/0034-extract-helpers` |
| `style` | Code style | `style/0012-fix-linting` |
| `ci` | CI/CD changes | `ci/0007-add-github-actions` |
| `build` | Build system | `build/0003-update-webpack` |

### Issue Number Rules

#### With Issue Number

- **Format**: Zero-padded to 4 digits minimum
- **Examples**:
  - Issue #13 → `feat/0013-add-authentication`
  - Issue #456 → `fix/0456-memory-leak`
  - Issue #7 → `docs/0007-api-guide`

#### Without Issue Number

- **Format**: Omit issue number entirely
- **Examples**:
  - `feat/add-authentication`
  - `fix/memory-leak`
  - `chore/refactor-auth-module`

#### Invalid Examples

❌ **Never use placeholders:**
- `feat/0000-add-authentication` (invalid placeholder)
- `fix/9999-bug-fix` (invalid placeholder)
- `feat/TODO-add-authentication` (invalid placeholder)

### Branch Name Examples

#### Features

```bash
feat/0013-add-user-authentication
feat/0042-api-rate-limiting
feat/add-oauth-support
```

#### Bug Fixes

```bash
fix/0108-resolve-login-timeout
fix/0234-memory-leak-in-cache
fix/session-expiry-bug
```

#### Chores/Refactoring

```bash
chore/0089-refactor-auth-module
chore/0005-update-dependencies
chore/cleanup-legacy-code
```

#### Parent/Child Task Examples

```bash
# Parent task
chore/0089-refactor-auth-module

# Child tasks (inherit parent issue number)
chore/0089-refactor-login
chore/0089-refactor-session
chore/0089-refactor-permissions
```

---

## Commit Message Generation

### From Branch Name to Commit Message

Branch names automatically inform commit messages during squash:

| Branch Name | Generated Commit Message |
|-------------|-------------------------|
| `feat/0013-add-auth` | `feat: add authentication (closes #13)` |
| `fix/0042-login-bug` | `fix: resolve login timeout (closes #42)` |
| `feat/add-auth` | `feat: add authentication` |
| `chore/0089-refactor-auth` | `chore: refactor authentication module (closes #89)` |

### Commit Message Rules

1. **Type extraction**: Take type from branch name prefix
2. **Title generation**: Convert purpose to sentence case
3. **Issue closure**: Append " (closes #N)" if issue number present
4. **Conventional commits**: Follow [Conventional Commits](https://www.conventionalcommits.org/) format

---

## Task Metadata for Development

### Development Task Schema

```json
{
  "id": "T-0042",
  "title": "Implement API rate limiting",
  "status": "IN_PROGRESS",
  "assigned_to": "codex",
  "tags": ["feature", "api", "security"],
  "meta": {
    "issue_number": 42,
    "branch_name": "feat/0042-api-rate-limiting",
    "worktree_path": ".worktrees/feat-0042-api-rate-limiting",
    "commit_type": "feat",
    "expected_commit": "feat: implement API rate limiting (closes #42)"
  }
}
```

### Parent Task with Children

```json
{
  "id": "T-0089",
  "title": "Refactor authentication module",
  "status": "IN_PROGRESS",
  "assigned_to": "codex",
  "tags": ["refactor", "auth"],
  "meta": {
    "issue_number": 89,
    "branch_name": "chore/0089-refactor-auth-module",
    "worktree_path": ".worktrees/chore-0089-refactor-auth-module",
    "commit_type": "chore",
    "child_tasks": ["T-0090", "T-0091", "T-0092"],
    "child_branches": [
      "chore/0089-refactor-login",
      "chore/0089-refactor-session",
      "chore/0089-refactor-permissions"
    ]
  }
}
```

---

## Best Practices

### DO

✅ Use worktrees for all parent tasks
✅ Use worktrees for non-trivial child tasks
✅ Create `.worktrees/` directory if missing
✅ Add `.worktrees/` to `.gitignore`
✅ Match worktree directory name to branch name
✅ Zero-pad issue numbers to 4 digits
✅ Clean up worktrees after task completion
✅ Use conventional commit types in branch names

### DON'T

❌ Work directly in main repository for parent tasks
❌ Use placeholder issue numbers (0000, 9999, TODO)
❌ Commit worktrees to repository
❌ Leave stale worktrees after task completion
❌ Mix different tasks in same worktree
❌ Use non-standard commit types in branch names

---

## Worktree Troubleshooting

### Issue: "fatal: '.worktrees/xyz' already exists"

**Solution**: Remove stale worktree reference
```bash
git worktree remove .worktrees/xyz --force
# or
rm -rf .worktrees/xyz
git worktree prune
```

### Issue: "fatal: 'xyz' is already checked out at..."

**Solution**: Branch is checked out in another worktree
```bash
# List all worktrees to find conflict
git worktree list

# Remove the conflicting worktree
git worktree remove path/to/conflicting/worktree
```

### Issue: Cannot delete worktree with uncommitted changes

**Solution**: Force removal or commit changes
```bash
# Option 1: Force remove
git worktree remove .worktrees/xyz --force

# Option 2: Commit changes first
cd .worktrees/xyz
git add .
git commit -m "WIP: save progress"
cd ../..
git worktree remove .worktrees/xyz
```

---

## Integration with TLC Flow Spec

See `task-flow-spec-0.1-dev.md` for:
- Complete flow examples using worktrees
- Multi-task development with worktree isolation
- Parallel child task execution in separate worktrees
- Automated worktree cleanup in flow definitions

---

## References

- task-flow-spec-0.1-dev.md — Development-specific flow patterns
- task-crud-spec-0.1.md — Task metadata schema
- task-flow-spec-0.1.md — Domain-agnostic flow patterns
- [Git Worktree Documentation](https://git-scm.com/docs/git-worktree)
- [Conventional Commits](https://www.conventionalcommits.org/)

---

**Version**: 0.1
**Last Updated**: 2025-01-15
**Applies To**: Software development tasks only
