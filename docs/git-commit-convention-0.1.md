# Git Commit Message Convention v0.1

## Overview

This document defines the commit message convention for TLC development. It follows the [Conventional Commits](https://www.conventionalcommits.org/) specification with TLC-specific extensions for issue tracking and co-authoring.

## Format (Normative)

Commit messages MUST follow this format:

```
<type>: <description>[ (<scope>)][ (closes #<issue>)]

[optional body]

[optional footer(s)]
```

### Required Components

- **type**: One of the valid types (see below)
- **description**: Short summary of the change (lowercase, no period)

### Optional Components

- **scope**: Area of codebase affected (in parentheses)
- **issue closure**: GitHub issue reference
- **body**: Detailed explanation of changes
- **footer**: Breaking changes, co-authors, references

---

## Valid Types (Normative)

MUST use one of these types:

| Type | Description | Example |
|------|-------------|---------|
| `feat` | New feature | `feat: add user authentication` |
| `fix` | Bug fix | `fix: resolve login timeout` |
| `chore` | Maintenance | `chore: update dependencies` |
| `docs` | Documentation | `docs: update API guide` |
| `test` | Testing | `test: add integration tests` |
| `perf` | Performance | `perf: optimize database queries` |
| `refactor` | Refactoring | `refactor: extract auth helpers` |
| `style` | Code style | `style: fix linting errors` |
| `ci` | CI/CD | `ci: add GitHub Actions workflow` |
| `build` | Build system | `build: update webpack config` |

---

## Title Format (Normative)

### Basic Format

```
<type>: <description>
```

**Rules**:
- Description MUST be lowercase
- Description MUST NOT end with period
- Description SHOULD be imperative mood ("add" not "added")
- Description SHOULD be concise (50 characters or less)

**Examples**:

```
feat: add OAuth2 authentication
fix: resolve memory leak in cache
docs: update installation guide
```

### With Scope

```
<type>(<scope>): <description>
```

**Rules**:
- Scope MUST be lowercase
- Scope SHOULD be single word or hyphenated
- Scope indicates subsystem or module

**Examples**:

```
feat(auth): add two-factor authentication
fix(api): handle null responses correctly
perf(db): optimize user lookup query
```

### With Issue Closure

```
<type>: <description> (closes #<issue>)
```

**Rules**:
- Issue reference MUST use "closes #N" format
- Issue number MUST match GitHub issue
- Automatically closes issue when merged to default branch

**Examples**:

```
feat: add rate limiting (closes #42)
fix: resolve login timeout (closes #108)
chore: refactor auth module (closes #89)
```

### Combined Format

```
<type>(<scope>): <description> (closes #<issue>)
```

**Examples**:

```
feat(api): add rate limiting middleware (closes #42)
fix(auth): resolve session expiry bug (closes #108)
perf(db): add query caching (closes #56)
```

---

## Commit Body (Optional)

### When to Include

Use body when:
- Change needs explanation beyond title
- Multiple related changes included
- Breaking changes need documentation
- Implementation requires context

### Format

```
<title>

<blank line>

<body paragraph 1>

<body paragraph 2>
```

**Rules**:
- MUST have blank line after title
- SHOULD wrap at 72 characters
- SHOULD use bullet points for lists
- SHOULD explain "why" not "what"

### Example

```
feat: add API rate limiting (closes #42)

Implements token bucket algorithm for rate limiting API requests.
This prevents abuse and ensures fair resource allocation.

Key changes:
- Add rate limiter middleware
- Configure limits per endpoint
- Add Redis backend for distributed tracking
- Return 429 status when limit exceeded

The default limit is 100 requests per minute per API key.
This can be configured via environment variables.
```

---

## Commit Footer (Optional)

### Co-Authoring

For commits with Claude Code assistance:

```
feat: add authentication (closes #13)

<body>

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

### Breaking Changes

For incompatible API changes:

```
feat: redesign authentication API (closes #156)

<body>

BREAKING CHANGE: Authentication endpoints now require API version header.
Previous /auth/login endpoint is deprecated. Use /v2/auth/login instead.
```

### Multiple Footers

```
feat: add OAuth2 support (closes #234)

<body>

Reviewed-By: John Doe <john@example.com>
Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
BREAKING CHANGE: Removed legacy auth endpoints
```

---

## Automated Generation from Branch Names

When squashing commits, generate commit message from branch name:

### Generation Rules

1. Extract `type` from branch prefix
2. Extract `issue` number if present
3. Convert `purpose` to sentence case description
4. Combine into commit format

### Examples

| Branch Name | Generated Commit |
|-------------|------------------|
| `feat/0042-api-rate-limiting` | `feat: add API rate limiting (closes #42)` |
| `fix/0108-login-timeout` | `fix: resolve login timeout (closes #108)` |
| `feat/add-oauth` | `feat: add OAuth support` |
| `chore/0089-refactor-auth` | `chore: refactor authentication module (closes #89)` |
| `docs/0023-api-guide` | `docs: update API guide (closes #23)` |

### Implementation Example

```javascript
function generateCommitMessage(branchName) {
  // Parse: type/[NNNN-]purpose
  const match = branchName.match(/^([^/]+)\/(?:(\d+)-)?(.+)$/);
  if (!match) throw new Error('Invalid branch name');

  const [, type, issueNum, purpose] = match;

  // Convert hyphenated purpose to sentence
  const description = purpose.replace(/-/g, ' ');

  // Build commit message
  let message = `${type}: ${description}`;

  if (issueNum) {
    message += ` (closes #${parseInt(issueNum)})`;
  }

  return message;
}
```

---

## Examples by Type

### Features

```
feat: add user authentication

feat: add OAuth2 provider support (closes #234)

feat(api): implement rate limiting middleware (closes #42)
```

### Bug Fixes

```
fix: resolve memory leak in cache

fix: handle null API responses (closes #108)

fix(auth): correct session expiry calculation (closes #67)
```

### Documentation

```
docs: update installation guide

docs: add API examples (closes #23)

docs(readme): improve getting started section
```

### Refactoring

```
refactor: extract authentication helpers

chore: migrate to TypeScript (closes #89)

refactor(api): simplify error handling
```

### Performance

```
perf: optimize database queries

perf: add Redis caching layer (closes #56)

perf(db): index frequently queried columns
```

---

## Multi-Commit Squashing

When squashing multiple commits into one:

### Approach 1: Summarize All Work

```
feat: implement complete user dashboard (closes #234)

- Add dashboard layout and navigation
- Implement widget system
- Connect to analytics API
- Add real-time updates via WebSocket

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

### Approach 2: List Key Changes

```
chore: refactor authentication module (closes #89)

Refactored authentication module for better maintainability:
- Extract login logic to separate service
- Add comprehensive error handling
- Improve session management
- Update tests for new structure

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

---

## Best Practices

### DO ✅

- Use imperative mood ("add" not "added" or "adds")
- Keep title under 50 characters
- Explain "why" in body, not "what"
- Reference issues when applicable
- Use conventional types consistently
- Include co-author footer for Claude assistance

### DON'T ❌

- End title with period
- Use past tense ("added", "fixed")
- Include implementation details in title
- Write vague descriptions ("fix bug", "update code")
- Mix multiple unrelated changes
- Forget to reference closing issues

---

## Validation

Implementations SHOULD validate commit messages:

```javascript
function validateCommitMessage(message) {
  const titlePattern = /^(feat|fix|chore|docs|test|perf|refactor|style|ci|build)(\([a-z-]+\))?: [a-z].+$/;

  const lines = message.split('\n');
  const title = lines[0];

  // Check title format
  if (!titlePattern.test(title)) {
    return { valid: false, error: 'Invalid title format' };
  }

  // Check title length
  if (title.length > 72) {
    return { valid: false, error: 'Title too long (max 72 chars)' };
  }

  // Check blank line after title if body exists
  if (lines.length > 1 && lines[1] !== '') {
    return { valid: false, error: 'Missing blank line after title' };
  }

  return { valid: true };
}
```

---

## GitHub Integration

### Automatic Issue Closure

Commits with `(closes #N)` automatically close issues when merged:

```bash
# On feature branch
git commit -m "feat: add dark mode (closes #42)"

# When merged to main via PR
# → Issue #42 automatically closed
```

### Other Keywords

GitHub recognizes these keywords:
- `closes #N`
- `fixes #N`
- `resolves #N`

**Convention**: Use `closes` for consistency.

---

## References

- git-branch-convention-0.1.md — Branch naming format
- git-worktree-convention-0.1.md — Worktree workflow
- github-label-convention-0.1.md — Issue labels
- [Conventional Commits](https://www.conventionalcommits.org/)
- [GitHub Issue Closure](https://docs.github.com/en/issues/tracking-your-work-with-issues/linking-a-pull-request-to-an-issue)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
