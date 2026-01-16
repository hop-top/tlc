# GitHub Milestone Convention v0.1

## Overview

This document defines the milestone naming convention for GitHub project planning in TLC development. Milestones use version-based or date-based naming without spaces for easy CLI usage and clear release tracking.

## Design Principles

1. **No spaces**: Use hyphens for multi-word milestones
2. **Sortable**: Names sort chronologically or semantically
3. **Versioned**: Semantic versioning for releases
4. **Dated**: ISO 8601 format for time-based planning
5. **Predictable**: Clear patterns for automation

---

## Milestone Types (Normative)

### Type 1: Version Milestones

For tracking releases using semantic versioning:

#### Format

```
v<major>.<minor>.<patch>
```

#### Examples

```
v0.1.0
v0.2.0
v1.0.0
v1.1.0
v2.0.0-beta.1
v2.0.0-rc.1
```

#### Rules

- MUST start with lowercase `v`
- MUST follow semantic versioning (MAJOR.MINOR.PATCH)
- MAY include pre-release suffix (`-alpha.N`, `-beta.N`, `-rc.N`)
- MUST use dots (`.`) not hyphens in version numbers

#### Use Cases

- Official releases
- Version planning
- Breaking change tracking
- API version alignment

---

### Type 2: Sprint Milestones

For time-boxed development cycles (weekly or bi-weekly):

#### Format (ISO Week)

```
sprint-<YYYY>-wWW
```

#### Examples

```
sprint-2025-w03
sprint-2025-w04
sprint-2025-w15
```

#### Rules

- MUST start with `sprint-`
- MUST use ISO 8601 week format: `YYYY-wWW`
- Year MUST be 4 digits
- Week MUST be zero-padded 2 digits
- Prefix `w` MUST be lowercase

#### Use Cases

- Agile sprint planning
- Weekly goals
- Short-term iteration tracking

---

### Type 3: Quarter Milestones

For quarterly planning and OKRs:

#### Format

```
<YYYY>-qN
```

#### Examples

```
2025-q1
2025-q2
2025-q3
2025-q4
2026-q1
```

#### Rules

- MUST use format `YYYY-qN`
- Year MUST be 4 digits
- Quarter MUST be `q1`, `q2`, `q3`, or `q4`
- Prefix `q` MUST be lowercase

#### Use Cases

- Quarterly objectives
- Long-term roadmap
- Business planning alignment

---

### Type 4: Month Milestones

For monthly planning cycles:

#### Format

```
<YYYY>-<MM>
```

#### Examples

```
2025-01
2025-02
2025-12
2026-01
```

#### Rules

- MUST use ISO 8601 month format: `YYYY-MM`
- Year MUST be 4 digits
- Month MUST be zero-padded 2 digits (01-12)

#### Use Cases

- Monthly goals
- Monthly releases
- Regular cadence tracking

---

### Type 5: Named Milestones

For thematic or feature-based milestones:

#### Format

```
<name>[-<version>]
```

#### Examples

```
mvp
beta-launch
public-release
auth-redesign
api-v2
performance-improvements
```

#### Rules

- MUST use lowercase
- MUST use hyphens for spaces
- MAY include version suffix if multiple phases
- SHOULD be descriptive and concise

#### Use Cases

- Feature epics
- Major initiatives
- Thematic goals
- Project phases

---

## Milestone Selection Guide

| Scenario | Recommended Type | Example |
|----------|-----------------|---------|
| Software release | Version | `v1.2.0` |
| Agile sprint | Sprint | `sprint-2025-w15` |
| Quarterly OKRs | Quarter | `2025-q2` |
| Monthly iteration | Month | `2025-06` |
| Feature epic | Named | `auth-redesign` |
| MVP milestone | Named | `mvp` |
| Beta release | Version | `v1.0.0-beta.1` |

---

## Milestone Lifecycle

### Creating Milestones

```bash
# Version milestone
gh milestone create "v1.0.0" \
  --title "Version 1.0.0 Release" \
  --description "First major release" \
  --due-date "2025-06-30"

# Sprint milestone
gh milestone create "sprint-2025-w15" \
  --title "Sprint 2025 Week 15" \
  --description "Apr 7-13, 2025" \
  --due-date "2025-04-13"

# Quarter milestone
gh milestone create "2025-q2" \
  --title "Q2 2025" \
  --description "April-June 2025 objectives" \
  --due-date "2025-06-30"

# Named milestone
gh milestone create "auth-redesign" \
  --title "Authentication System Redesign" \
  --description "Complete auth module refactoring"
```

### Assigning Issues

```bash
# Assign issue to milestone
gh issue edit 42 --milestone "v1.0.0"

# Assign multiple issues
gh issue list --label type-feature --json number --jq '.[].number' | \
  xargs -I {} gh issue edit {} --milestone "sprint-2025-w15"
```

### Tracking Progress

```bash
# View milestone status
gh milestone view "v1.0.0"

# List open issues in milestone
gh issue list --milestone "v1.0.0" --state open

# List all milestones
gh milestone list
```

---

## Milestone Description Format

### Version Milestones

```markdown
# Version 1.0.0 Release

**Release Date**: June 30, 2025

## Goals
- Complete authentication system
- Implement API rate limiting
- Achieve 90% test coverage

## Features
- closes #42 API rate limiting
- closes #89 Auth module refactor

## Breaking Changes
- Authentication endpoints require API version header
```

### Sprint Milestones

```markdown
# Sprint 2025-W15 (Apr 7-13)

**Sprint Goal**: Complete authentication redesign

## Committed Work
- #89 Refactor auth module
- #90 Add OAuth support
- #91 Update auth docs

## Stretch Goals
- #92 Improve error messages
```

### Quarter Milestones

```markdown
# Q2 2025 (April-June)

**Theme**: Performance & Scalability

## Objectives
- Reduce API response time by 50%
- Support 10k concurrent users
- Achieve 99.9% uptime

## Key Results
- Implement caching layer
- Add load balancing
- Optimize database queries
```

---

## CLI Usage Examples

### Filtering Issues

```bash
# Version release issues
gh issue list --milestone "v1.0.0"

# Current sprint
gh issue list --milestone "sprint-2025-w15" --state open

# Quarter planning
gh issue list --milestone "2025-q2" --label priority-high
```

### Milestone Management

```bash
# Close completed milestone
gh milestone close "sprint-2025-w14"

# Reopen milestone
gh milestone reopen "v1.0.0"

# Edit milestone
gh milestone edit "v1.0.0" --due-date "2025-07-15"

# Delete milestone
gh milestone delete "old-milestone"
```

### Bulk Operations

```bash
# Move unfinished issues to next sprint
gh issue list --milestone "sprint-2025-w14" --state open --json number \
  --jq '.[].number' | \
  xargs -I {} gh issue edit {} --milestone "sprint-2025-w15"
```

---

## Integration with Versioning

### Release Flow

1. **Plan**: Create milestone `v1.0.0`
2. **Assign**: Add features and fixes to milestone
3. **Develop**: Work on issues in milestone
4. **Release**: Tag release when milestone complete
5. **Document**: Generate changelog from milestone issues

### Changelog Generation

```bash
#!/bin/bash
# Generate changelog from milestone

MILESTONE="v1.0.0"

echo "# Changelog - $MILESTONE"
echo ""

echo "## Features"
gh issue list --milestone "$MILESTONE" --label type-feature --state closed \
  --json number,title --jq '.[] | "- \(.title) (#\(.number))"'

echo ""
echo "## Bug Fixes"
gh issue list --milestone "$MILESTONE" --label type-bug --state closed \
  --json number,title --jq '.[] | "- \(.title) (#\(.number))"'
```

---

## Best Practices

### DO ✅

- Use consistent naming patterns
- Set realistic due dates
- Update milestone descriptions with goals
- Close milestones when complete
- Use semantic versioning for releases
- Keep milestone count manageable (3-5 active)

### DON'T ❌

- Create milestones with spaces
- Use inconsistent date formats
- Leave milestones perpetually open
- Create too many concurrent milestones
- Forget to update due dates
- Mix different milestone types inconsistently

---

## Automation

### Auto-Milestone PRs

```yaml
# .github/workflows/auto-milestone.yml
name: Auto-assign Milestone

on:
  pull_request:
    types: [opened]

jobs:
  milestone:
    runs-on: ubuntu-latest
    steps:
      - name: Add to current sprint
        uses: actions/github-script@v6
        with:
          script: |
            const milestone = await github.rest.issues.listMilestones({
              owner: context.repo.owner,
              repo: context.repo.repo,
              state: 'open',
              sort: 'due_on',
              direction: 'asc'
            });

            if (milestone.data.length > 0) {
              await github.rest.issues.update({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: context.payload.pull_request.number,
                milestone: milestone.data[0].number
              });
            }
```

### Milestone Metrics

```bash
# Progress tracking
gh api repos/:owner/:repo/milestones/:milestone_number | \
  jq '{
    title: .title,
    open: .open_issues,
    closed: .closed_issues,
    progress: (.closed_issues / (.open_issues + .closed_issues) * 100)
  }'
```

---

## Migration Strategy

### From Spaced Names

```bash
# Rename milestones to convention
gh api --method PATCH repos/:owner/:repo/milestones/:number \
  -f title="v1.0.0"  # was "Version 1.0.0"

gh api --method PATCH repos/:owner/:repo/milestones/:number \
  -f title="sprint-2025-w15"  # was "Sprint 2025 Week 15"
```

---

## Calendar Integration

### Due Date Conventions

```
Version releases: End of month (last day)
  v1.0.0 due 2025-06-30

Sprints: End of sprint week (Sunday)
  sprint-2025-w15 due 2025-04-13

Quarters: End of quarter (last day)
  2025-q2 due 2025-06-30

Months: End of month (last day)
  2025-06 due 2025-06-30
```

---

## References

- git-branch-convention-0.1.md — Branch naming
- git-commit-convention-0.1.md — Commit messages
- github-label-convention-0.1.md — Issue labels
- [Semantic Versioning](https://semver.org/)
- [ISO 8601 Week](https://en.wikipedia.org/wiki/ISO_8601#Week_dates)
- [GitHub Milestones](https://docs.github.com/en/issues/using-labels-and-milestones-to-track-work/about-milestones)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
