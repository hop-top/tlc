# Git Branch Naming Convention v0.1

## Overview

This document defines the branch naming convention for TLC development. Branch names provide structured metadata that enables automated commit message generation, issue tracking, and clear communication of intent.

## Format (Normative)

Branch names MUST follow this format:

```
<type>/<issue-id>-<purpose>
```

Or without issue number:

```
<type>/<purpose>
```

### Components

- **type** (required): Conventional commit type
- **issue-id** (optional): Zero-padded 4-digit issue number
- **purpose** (required): Hyphen-separated description

### Rules

1. **Type prefix**: MUST be one of the valid types (see below)
2. **Separator**: MUST use `/` between type and rest
3. **Issue number**: MUST be zero-padded to 4 digits if present
4. **Issue separator**: MUST use `-` after issue number
5. **Purpose**: MUST use hyphens (not spaces or underscores)
6. **Lowercase**: SHOULD use lowercase throughout
7. **No placeholders**: NEVER use `0000`, `9999`, `TODO`, or similar

---

## Valid Types (Normative)

| Type | Description | Use Case |
|------|-------------|----------|
| `feat` | New feature | Adding new functionality |
| `fix` | Bug fix | Fixing defects or issues |
| `chore` | Maintenance | Refactoring, dependencies, tooling |
| `docs` | Documentation | README, guides, comments |
| `test` | Testing | Adding or updating tests |
| `perf` | Performance | Performance improvements |
| `refactor` | Refactoring | Code restructuring without behavior change |
| `style` | Code style | Formatting, linting fixes |
| `ci` | CI/CD | Pipeline, workflow, automation |
| `build` | Build system | Webpack, bundler, build config |

---

## Issue Number Rules (Normative)

### With Issue Number

When branch is associated with a GitHub issue:

- **Format**: Zero-pad to minimum 4 digits
- **Examples**:
  - Issue #7 → `feat/0007-add-authentication`
  - Issue #13 → `fix/0013-login-timeout`
  - Issue #456 → `chore/0456-refactor-auth`
  - Issue #1234 → `docs/1234-api-guide`

### Without Issue Number

When no issue exists or issue tracking not needed:

- **Format**: Omit issue number entirely
- **Examples**:
  - `feat/add-authentication`
  - `fix/login-timeout`
  - `chore/update-dependencies`

### Invalid Examples

❌ **Never use these patterns:**

```bash
feat/0000-placeholder        # Invalid: placeholder number
fix/9999-temp-fix           # Invalid: placeholder number
chore/TODO-refactor         # Invalid: non-numeric placeholder
feat/13-add-auth            # Invalid: not zero-padded
fix/0013_login_bug          # Invalid: underscore instead of hyphen
```

---

## Branch Name Examples

### Features

```bash
feat/0013-add-user-authentication
feat/0042-api-rate-limiting
feat/0156-oauth-integration
feat/add-dark-mode
feat/websocket-support
```

### Bug Fixes

```bash
fix/0108-resolve-login-timeout
fix/0234-memory-leak-in-cache
fix/0005-cors-headers
fix/session-expiry
fix/null-pointer-exception
```

### Chores and Refactoring

```bash
chore/0089-refactor-auth-module
chore/0005-update-dependencies
chore/0123-migrate-to-typescript
chore/cleanup-legacy-code
chore/improve-error-handling
```

### Documentation

```bash
docs/0023-update-api-documentation
docs/0067-add-contributing-guide
docs/readme-improvements
docs/api-examples
```

### Tests

```bash
test/0018-add-integration-tests
test/0089-auth-unit-tests
test/e2e-checkout-flow
test/increase-coverage
```

### Performance

```bash
perf/0056-optimize-db-queries
perf/0078-cache-expensive-calls
perf/reduce-bundle-size
```

### CI/CD and Build

```bash
ci/0007-add-github-actions
ci/0034-deploy-preview-env
build/0003-update-webpack-config
build/optimize-production-build
```

---

## Parent and Child Task Patterns

### Pattern 1: Shared Issue Number

Parent and child tasks share the same issue number with descriptive suffixes:

```bash
# Parent task (issue #89)
chore/0089-refactor-auth-module

# Child tasks (same issue, different aspects)
chore/0089-refactor-login
chore/0089-refactor-session
chore/0089-refactor-permissions
chore/0089-refactor-jwt
```

### Pattern 2: Separate Issues

Each task has its own issue number:

```bash
# Parent task
feat/0234-user-dashboard

# Child tasks (separate issues)
feat/0235-dashboard-layout
feat/0236-dashboard-widgets
feat/0237-dashboard-api
```

### Pattern 3: No Issue Numbers

Internal work without GitHub issues:

```bash
# Parent task
refactor/improve-api-layer

# Child tasks
refactor/improve-api-validation
refactor/improve-api-errors
refactor/improve-api-docs
```

---

## Integration with Commit Messages

Branch names automatically inform commit message generation during squash merges:

| Branch Name | Generated Commit Message |
|-------------|-------------------------|
| `feat/0013-add-auth` | `feat: add authentication (closes #13)` |
| `fix/0042-login-timeout` | `fix: resolve login timeout (closes #42)` |
| `feat/add-auth` | `feat: add authentication` |
| `chore/0089-refactor-auth` | `chore: refactor auth module (closes #89)` |

**Extraction Rules**:
1. Type → commit type prefix
2. Issue number → appends "(closes #N)" if present
3. Purpose → converted to sentence case for commit title

---

## CLI Usage

Branch names without spaces enable easy command-line usage:

```bash
# Easy to reference
git checkout feat/0042-api-rate-limiting
git branch -d fix/0013-login-timeout

# Works in scripts
BRANCH="feat/0042-api-rate-limiting"
git push origin $BRANCH

# No quoting needed
gh pr create --head feat/0042-api-rate-limiting --base main
```

---

## Validation Rules

Implementations SHOULD validate branch names:

```javascript
function isValidBranchName(name) {
  // Pattern: type/[NNNN-]purpose
  const pattern = /^(feat|fix|chore|docs|test|perf|refactor|style|ci|build)\/(\d{4,}-)?[a-z0-9-]+$/;

  if (!pattern.test(name)) return false;

  // If has issue number, check it's not a placeholder
  const issueMatch = name.match(/\/(\d{4,})-/);
  if (issueMatch) {
    const issueNum = parseInt(issueMatch[1]);
    if (issueNum === 0 || issueNum === 9999) return false;
  }

  return true;
}
```

---

## Best Practices

### DO ✅

- Use descriptive purpose that summarizes the change
- Zero-pad issue numbers to 4+ digits
- Keep purpose concise but meaningful
- Use hyphens for word separation
- Match branch name to actual work being done

### DON'T ❌

- Use spaces (breaks CLI usage)
- Use underscores (convention uses hyphens)
- Include ticket prefixes like "JIRA-123" (just use number)
- Create generic names like "fix/bug" or "feat/feature"
- Reuse branch names for different work

---

## Examples by Scenario

### Starting new feature work

```bash
# With issue #42
git checkout -b feat/0042-api-rate-limiting

# Without issue
git checkout -b feat/add-pagination
```

### Fixing a bug

```bash
# Critical bug with issue #108
git checkout -b fix/0108-security-vulnerability

# Minor fix without issue
git checkout -b fix/typo-in-error-message
```

### Large refactoring (parent task)

```bash
# Parent branch for issue #89
git checkout -b chore/0089-refactor-auth-module

# Work in worktree
git worktree add .worktrees/chore-0089-refactor-auth-module chore/0089-refactor-auth-module
```

---

## References

- git-commit-convention-0.1.md — Commit message format
- git-worktree-convention-0.1.md — Worktree workflow
- github-label-convention-0.1.md — Issue label taxonomy
- [Conventional Commits](https://www.conventionalcommits.org/)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
