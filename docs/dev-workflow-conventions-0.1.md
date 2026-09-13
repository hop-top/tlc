# Development Workflow Conventions v0.1

## Overview

This document provides an overview of all development workflow conventions for TLC projects. Each convention is defined in its own document for focused reference and maintenance.

## Convention Documents

### Git Conventions

#### [git-branch-convention-0.1.md](git-branch-convention-0.1.md)
**Branch naming format for development work**

- Format: `<type>/<issue-id>-<purpose>`
- Valid types: feat, fix, chore, docs, test, perf, refactor, style, ci, build
- Zero-padded 4-digit issue numbers
- Hyphenated purpose descriptions

**Key Examples**:
```
feat/0042-api-rate-limiting
fix/0108-login-timeout
chore/0089-refactor-auth-module
```

---

#### [git-commit-convention-0.1.md](git-commit-convention-0.1.md)
**Commit message format following Conventional Commits**

- Format: `<type>: <description> (closes #<issue>)`
- Imperative mood, lowercase description
- Optional body and footers
- Automated generation from branch names

**Key Examples**:
```
feat: add API rate limiting (closes #42)
fix: resolve login timeout (closes #108)
chore: refactor auth module (closes #89)
```

---

#### [git-worktree-convention-0.1.md](git-worktree-convention-0.1.md)
**Worktree workflow for isolated development**

- Location: `$PWD/.worktrees/`
- Parent tasks MUST use worktrees
- Child tasks SHOULD use worktrees for non-trivial work
- Complete lifecycle: setup → develop → cleanup

**Key Usage**:
```bash
git worktree add .worktrees/feat-0042-api-rate-limiting feat/0042-api-rate-limiting
cd .worktrees/feat-0042-api-rate-limiting
# ... develop ...
cd ../..
git worktree remove .worktrees/feat-0042-api-rate-limiting
```

---

### GitHub Conventions

#### [github-label-convention-0.1.md](github-label-convention-0.1.md)
**Issue and PR label taxonomy**

- Hyphenated lowercase naming (no spaces)
- Categories: type, priority, status, area, effort
- Color-coded by category
- CLI-friendly format

**Key Labels**:
```
feat, fix, chore, docs, test
feat:auth, feat:api, fix:github-oauth
priority:critical, priority:high, priority:medium, priority:low
status:in-progress, status:review, status:ready
domain:api, domain:auth, domain:db, domain:ui
effort:xs, effort:sm, effort:md, effort:lg, effort:xl
```

---

#### [github-milestone-convention-0.1.md](github-milestone-convention-0.1.md)
**Milestone naming for project planning**

- Version milestones: `v1.0.0`
- Sprint milestones: `sprint-2025-w15`
- Quarter milestones: `2025-q2`
- Month milestones: `2025-06`
- Named milestones: `auth-redesign`, `mvp`

**Key Patterns**:
```
v1.0.0                    # Semantic version
sprint-2025-w15           # ISO week number
2025-q2                   # Quarterly planning
auth-redesign             # Feature epic
```

---

## Workflow Integration

### Complete Feature Development Flow

```
1. Create GitHub Issue
   → Label: type-feature, priority-high, area-api
   → Assign to milestone: v1.0.0

2. Create Branch
   → Name: feat/0042-api-rate-limiting

3. Setup Worktree
   → git worktree add .worktrees/feat-0042-api-rate-limiting feat/0042-api-rate-limiting

4. Develop
   → Work in worktree
   → Commit regularly
   → Push to remote

5. Create Pull Request
   → Auto-labeled from branch type
   → Links to issue

6. Merge
   → Squash commit: "feat: add API rate limiting (closes #42)"
   → Issue auto-closed
   → Milestone updated

7. Cleanup
   → git worktree remove .worktrees/feat-0042-api-rate-limiting
   → Delete branch
```

---

## Quick Reference

### Branch Naming

| Scenario | Branch Name |
|----------|-------------|
| Feature with issue | `feat/0042-add-authentication` |
| Feature without issue | `feat/add-authentication` |
| Bug fix | `fix/0108-login-timeout` |
| Refactoring | `chore/0089-refactor-auth` |
| Documentation | `docs/0023-api-guide` |

### Commit Messages

| Branch | Commit |
|--------|--------|
| `feat/0042-api-limiting` | `feat: add API rate limiting (closes #42)` |
| `fix/0108-timeout` | `fix: resolve login timeout (closes #108)` |
| `docs/update-readme` | `docs: update installation guide` |

### Labels

| Issue Type | Labels |
|------------|--------|
| New feature | `feat:api priority:medium domain:api effort:md` |
| Critical bug | `fix:auth priority:critical domain:auth effort:sm` |
| Documentation | `docs priority:low domain:docs effort:xs` |
| Refactoring | `refactor priority:medium domain:db effort:lg` |

### Milestones

| Planning Type | Milestone |
|---------------|-----------|
| Release | `v1.0.0` |
| Sprint | `sprint-2025-w15` |
| Quarter | `2025-q2` |
| Feature Epic | `auth-redesign` |

---

## CLI Examples

### Complete Task Workflow

```bash
# 1. Create issue
gh issue create \
  --title "Add API rate limiting" \
  --label feat:api \
  --label priority:high \
  --label domain:api \
  --milestone v1.0.0

# 2. Create worktree
git worktree add -b feat/0042-api-rate-limiting .worktrees/feat-0042-api-rate-limiting
cd .worktrees/feat-0042-api-rate-limiting

# 3. Develop
npm install
# ... make changes ...
npm test

# 4. Commit
git add .
git commit -m "feat: implement rate limiting middleware"

# 5. Push and create PR
git push origin feat/0042-api-rate-limiting
gh pr create --title "feat: add API rate limiting (closes #42)"

# 6. After merge, cleanup
cd ../..
git worktree remove .worktrees/feat-0042-api-rate-limiting
git branch -d feat/0042-api-rate-limiting
```

---

## References

Individual convention documents:
- [git-branch-convention-0.1.md](git-branch-convention-0.1.md)
- [git-commit-convention-0.1.md](git-commit-convention-0.1.md)
- [git-worktree-convention-0.1.md](git-worktree-convention-0.1.md)
- [github-label-convention-0.1.md](github-label-convention-0.1.md)
- [github-milestone-convention-0.1.md](github-milestone-convention-0.1.md)

Related specifications:
- [recipe-spec-0.1.md](recipe-spec-0.1.md) — Recipe grammar and step semantics
- [task-crud-spec-0.1.md](task-crud-spec-0.1.md) — Task metadata

External references:
- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)
- [Git Worktree Documentation](https://git-scm.com/docs/git-worktree)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Overview (references normative documents)
