# GitHub Label Convention v0.1

## Overview

This document defines the label taxonomy for GitHub issues and pull requests in TLC projects. Labels use colon-separated categories with hyphenated names for easy CLI usage and clear categorization.

## Design Principles

1. **No spaces**: Use hyphens for multi-word names (enables CLI usage)
2. **Lowercase**: Consistent casing across all labels
3. **Colon separator**: Use `:` between category and name
4. **Hierarchical**: Category prefix groups related labels
5. **Descriptive**: Clear meaning without context

---

## Label Format (Normative)

### Pattern

```
<category>:<name>
```

Or for single-word labels without category:

```
<name>
```

**Examples**:
- `feat:auth`
- `fix:github-oauth`
- `priority:high`
- `status:in-progress`
- `bug` (standalone)

### Rules

1. Category prefix SHOULD be used for grouping
2. Separator MUST be colon (`:`)
3. Name component MUST use hyphens for multi-word (no spaces)
4. All characters MUST be lowercase
5. SHOULD be concise (category + name ≤ 25 chars)

---

## Label Categories (Normative)

### Type Labels

Categorize the nature of the issue (maps to commit types):

| Label | Color | Description |
|-------|-------|-------------|
| `feat` | `#0052CC` | New feature or enhancement |
| `feat:<scope>` | `#0052CC` | Feature in specific area |
| `fix` | `#D73A4A` | Bug fix or defect |
| `fix:<scope>` | `#D73A4A` | Bug fix in specific area |
| `chore` | `#FEF2C0` | Maintenance, refactoring, dependencies |
| `docs` | `#0075CA` | Documentation improvements |
| `test` | `#1D76DB` | Test additions or improvements |
| `perf` | `#F9D0C4` | Performance optimization |
| `refactor` | `#FBCA04` | Code refactoring |
| `ci` | `#7057FF` | CI/CD pipeline changes |
| `build` | `#BFD4F2` | Build system or tooling |

**Scoped Examples**:
```
feat:auth              # Authentication feature
feat:api               # API feature
feat:ui                # UI feature
fix:github-oauth       # OAuth bug fix
fix:session-timeout    # Session bug fix
chore:deps             # Dependency updates
```

**Usage**:
- MUST apply at least one type label per issue
- MAY use scoped variant for clarity (e.g., `feat:auth`)
- MAY use both generic and scoped (e.g., `feat` + `feat:auth`)
- Type SHOULD match branch type and commit type

### Priority Labels

Indicate urgency and importance:

| Label | Color | Description |
|-------|-------|-------------|
| `priority:critical` | `#B60205` | Urgent: blocks release or causes data loss |
| `priority:high` | `#D93F0B` | Important: should be addressed soon |
| `priority:medium` | `#FBCA04` | Normal: standard priority |
| `priority:low` | `#0E8A16` | Minor: nice to have |

**Usage**:
- SHOULD apply exactly one priority label
- Default: `priority:medium` if unspecified
- `priority:critical`: Security issues, data loss, complete breakage
- `priority:high`: Major bugs, important features
- `priority:medium`: Standard work items
- `priority:low`: Minor improvements, optimizations

### Status Labels

Track issue lifecycle:

| Label | Color | Description |
|-------|-------|-------------|
| `status:triage` | `#FFFFFF` | Needs review and categorization |
| `status:blocked` | `#D73A4A` | Blocked by external dependency |
| `status:in-progress` | `#0052CC` | Actively being worked on |
| `status:review` | `#FBCA04` | Ready for review |
| `status:ready` | `#0E8A16` | Ready to be worked on |
| `status:wont-fix` | `#808080` | Closed without resolution |
| `status:duplicate` | `#CFD3D7` | Duplicate of another issue |

**Usage**:
- MAY apply one or more status labels
- Update as issue progresses through workflow
- Remove previous status when updating

### Domain Labels (Project-Specific)

Identify codebase subsystem or architectural layer. Labels auto-adapt based on project type.

#### Web Application (React/Vue/Angular)

| Label | Color | Description |
|-------|-------|-------------|
| `domain:frontend` | `#E99695` | Frontend/UI components |
| `domain:backend` | `#1D76DB` | Backend API/services |
| `domain:api` | `#0052CC` | API endpoints and contracts |
| `domain:auth` | `#5319E7` | Authentication/authorization |
| `domain:db` | `#0E8A16` | Database and persistence |
| `domain:tests` | `#7057FF` | Testing infrastructure |
| `domain:infra` | `#FBCA04` | Infrastructure/deployment |

#### Go Binary/CLI Application

| Label | Color | Description |
|-------|-------|-------------|
| `domain:cli` | `#BFD4F2` | Command-line interface |
| `domain:core` | `#0052CC` | Core business logic |
| `domain:io` | `#1D76DB` | File I/O and data handling |
| `domain:config` | `#0E8A16` | Configuration management |
| `domain:networking` | `#5319E7` | Network/HTTP handling |
| `domain:tests` | `#7057FF` | Testing infrastructure |
| `domain:build` | `#FBCA04` | Build system |

#### Python MVC Application (Django/Flask)

| Label | Color | Description |
|-------|-------|-------------|
| `domain:models` | `#0E8A16` | Data models/ORM |
| `domain:views` | `#E99695` | Views/templates |
| `domain:controllers` | `#1D76DB` | Controllers/routes |
| `domain:api` | `#0052CC` | REST/GraphQL API |
| `domain:auth` | `#5319E7` | Authentication/authorization |
| `domain:middleware` | `#FBCA04` | Middleware/hooks |
| `domain:tests` | `#7057FF` | Testing infrastructure |

#### Microservices Architecture

| Label | Color | Description |
|-------|-------|-------------|
| `domain:auth-service` | `#5319E7` | Auth microservice |
| `domain:user-service` | `#0052CC` | User microservice |
| `domain:api-gateway` | `#1D76DB` | API gateway |
| `domain:message-queue` | `#FBCA04` | Message queue/events |
| `domain:storage` | `#0E8A16` | Storage/database |
| `domain:observability` | `#F9D0C4` | Logging/monitoring |
| `domain:orchestration` | `#7057FF` | K8s/Docker orchestration |

#### Go Socket Server

| Label | Color | Description |
|-------|-------|-------------|
| `domain:websocket` | `#1D76DB` | WebSocket handling |
| `domain:protocol` | `#0052CC` | Protocol implementation |
| `domain:connection` | `#5319E7` | Connection management |
| `domain:auth` | `#FBCA04` | Authentication/security |
| `domain:broadcast` | `#E99695` | Message broadcasting |
| `domain:persistence` | `#0E8A16` | State persistence |
| `domain:tests` | `#7057FF` | Testing infrastructure |

#### Generic/Multi-Project Labels

For projects that don't fit specific patterns or span multiple types:

| Label | Color | Description |
|-------|-------|-------------|
| `domain:core` | `#0052CC` | Core business logic |
| `domain:api` | `#1D76DB` | API/interface layer |
| `domain:data` | `#0E8A16` | Data layer |
| `domain:ui` | `#E99695` | User interface |
| `domain:integration` | `#5319E7` | External integrations |
| `domain:infra` | `#FBCA04` | Infrastructure |
| `domain:tests` | `#7057FF` | Testing |
| `domain:docs` | `#0075CA` | Documentation |

**Usage**:
- MAY apply multiple domain labels if issue spans subsystems
- Labels SHOULD be adapted to match project architecture
- Helps route issues to appropriate team members
- Create domain labels as needed for your project structure

### Effort Labels

Estimate complexity (t-shirt sizing):

| Label | Color | Description |
|-------|-------|-------------|
| `effort:xs` | `#C5DEF5` | Extra small: < 1 hour |
| `effort:sm` | `#BFD4F2` | Small: 1-4 hours |
| `effort:md` | `#5EBEFF` | Medium: 1-2 days |
| `effort:lg` | `#1D76DB` | Large: 3-5 days |
| `effort:xl` | `#0052CC` | Extra large: 1-2 weeks |

**Usage**:
- SHOULD apply one effort label for planning
- Estimate before starting work
- Helps with sprint planning and capacity

### Special Labels

| Label | Color | Description |
|-------|-------|-------------|
| `good-first-issue` | `#7057FF` | Good for newcomers |
| `help-wanted` | `#008672` | Extra attention needed |
| `breaking-change` | `#B60205` | Introduces breaking API change |
| `security` | `#D73A4A` | Security vulnerability |
| `dependencies` | `#0366D6` | Dependency updates |
| `question` | `#D876E3` | Question or discussion |

---

## Label Combinations

### Feature Request

```
feat:api
priority:medium
domain:api
effort:md
status:ready
```

### Critical Bug (Go Application)

```
fix:auth
priority:critical
domain:auth
domain:core
effort:sm
status:in-progress
```

### Scoped Feature (Python MVC)

```
feat:github-oauth
priority:high
domain:auth
domain:controllers
domain:models
effort:lg
status:review
```

### Documentation Task

```
docs
priority:low
domain:docs
effort:xs
good-first-issue
```

---

## Project Type Auto-Detection

### Detection Strategy

Automatically detect project type and suggest appropriate domain labels:

```javascript
function detectProjectType(repoPath) {
  const indicators = {
    'go-binary': ['go.mod', 'main.go', 'cmd/'],
    'go-socket': ['go.mod', 'websocket', 'net/http'],
    'python-mvc': ['requirements.txt', 'manage.py', 'models.py'],
    'react-frontend': ['package.json', 'src/App.jsx', 'public/'],
    'microservices': ['docker-compose.yml', 'k8s/', 'services/'],
    'monorepo': ['nx.json', 'lerna.json', 'pnpm-workspace.yaml']
  };

  // Scan for indicators
  // Return detected type
}

function suggestDomainLabels(projectType) {
  const labelSets = {
    'go-binary': [
      'domain:cli', 'domain:core', 'domain:io',
      'domain:config', 'domain:networking', 'domain:tests'
    ],
    'python-mvc': [
      'domain:models', 'domain:views', 'domain:controllers',
      'domain:api', 'domain:auth', 'domain:middleware', 'domain:tests'
    ],
    'react-frontend': [
      'domain:frontend', 'domain:components', 'domain:hooks',
      'domain:api', 'domain:state', 'domain:tests'
    ]
  };

  return labelSets[projectType] || labelSets['generic'];
}
```

### CLI Command

```bash
# Auto-detect and create domain labels
tlc labels init

# Output:
# Detected project type: go-binary
# Creating labels:
#   ✓ domain:cli
#   ✓ domain:core
#   ✓ domain:io
#   ✓ domain:config
#   ✓ domain:networking
#   ✓ domain:tests
#   ✓ domain:build

# Manual override
tlc labels init --type python-mvc

# List available templates
tlc labels templates
```

---

## CLI Usage

Labels without spaces enable easy GitHub CLI usage:

```bash
# Create issue with labels (Python MVC project)
gh issue create \
  --title "Add GitHub OAuth support" \
  --label feat:github-oauth \
  --label priority:high \
  --label domain:auth \
  --label domain:controllers \
  --label effort:md

# Create issue (Go binary project)
gh issue create \
  --title "Add config file support" \
  --label feat:config \
  --label priority:medium \
  --label domain:config \
  --label domain:io \
  --label effort:sm

# Filter issues by type and scope
gh issue list --label feat:auth

# Filter by domain
gh issue list --label domain:frontend

# Multiple filters
gh issue list --label fix --label priority:high --label domain:api

# Add scoped label
gh issue edit 42 --add-label fix:session-timeout

# Remove labels
gh issue edit 42 --remove-label status:ready
```

---

## Scoped Type Label Pattern

### Purpose

Scoped labels provide fine-grained categorization while maintaining conventional commit alignment.

### Format

```
<type>:<scope>
```

### Examples by Domain

**Authentication**:
```
feat:auth              # Generic auth feature
feat:oauth             # OAuth integration
feat:2fa               # Two-factor authentication
fix:session-timeout    # Session bug
fix:jwt-validation     # JWT bug
```

**API**:
```
feat:api-v2            # API version 2
feat:rest-endpoints    # REST API
feat:graphql           # GraphQL support
fix:rate-limiting      # Rate limit bug
perf:response-time     # Performance improvement
```

**UI**:
```
feat:dark-mode         # Dark mode feature
feat:dashboard         # Dashboard feature
fix:mobile-layout      # Mobile bug
style:component-css    # CSS styling
```

**Infrastructure**:
```
ci:github-actions      # CI workflow
ci:test-coverage       # Coverage reporting
build:webpack          # Build config
chore:deps             # Dependencies
```

### When to Use Scoped Labels

**Use scoped labels when**:
- Issue is clearly tied to specific subsystem
- Scope aids in routing to right team/person
- Want to track work in specific area
- Scope adds meaningful context

**Use generic labels when**:
- Issue spans multiple areas
- Scope is obvious from title
- Want simpler label set
- Cross-cutting concern

**Can use both**:
```
feat
feat:github-oauth
domain:auth
```

This allows filtering by either generic `feat` or specific `feat:github-oauth`.

---

## Automation

### Auto-Labeling from Branch

```yaml
# .github/workflows/auto-label.yml
name: Auto-label PR

on:
  pull_request:
    types: [opened]

jobs:
  label:
    runs-on: ubuntu-latest
    steps:
      - name: Label from branch
        uses: actions/github-script@v6
        with:
          script: |
            const branch = context.payload.pull_request.head.ref;
            // Extract: feat/0042-github-oauth -> feat:github-oauth
            const match = branch.match(/^([^/]+)\/(?:\d+-)?(.+)$/);

            if (match) {
              const [, type, scope] = match;
              const scopeNormalized = scope.replace(/-/g, '-');

              // Add generic type label
              await github.rest.issues.addLabels({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: context.payload.pull_request.number,
                labels: [type]
              });

              // Add scoped label if meaningful
              if (scopeNormalized.length > 0) {
                await github.rest.issues.addLabels({
                  owner: context.repo.owner,
                  repo: context.repo.repo,
                  issue_number: context.payload.pull_request.number,
                  labels: [`${type}:${scopeNormalized}`]
                });
              }
            }
```

### Auto-Labeling from PR Title

```yaml
# Match PR title pattern: "feat(github-oauth): add OAuth support"
const titleMatch = context.payload.pull_request.title.match(/^([^(:]+)(?:\(([^)]+)\))?:/);

if (titleMatch) {
  const [, type, scope] = titleMatch;
  const labels = [type];

  if (scope) {
    labels.push(`${type}:${scope}`);
  }

  await github.rest.issues.addLabels({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: context.payload.pull_request.number,
    labels
  });
}
```

---

## Label Management

### Creating Labels

Using GitHub CLI:

```bash
# Generic type labels
gh label create feat --color "0052CC" --description "New feature"
gh label create fix --color "D73A4A" --description "Bug fix"
gh label create chore --color "FEF2C0" --description "Maintenance"

# Scoped type labels
gh label create feat:auth --color "0052CC" --description "Authentication feature"
gh label create feat:api --color "0052CC" --description "API feature"
gh label create fix:github-oauth --color "D73A4A" --description "OAuth bug fix"

# Priority labels
gh label create priority:critical --color "B60205" --description "Critical priority"
gh label create priority:high --color "D93F0B" --description "High priority"
gh label create priority:medium --color "FBCA04" --description "Medium priority"
gh label create priority:low --color "0E8A16" --description "Low priority"

# Domain labels (example: Go binary project)
gh label create domain:cli --color "BFD4F2" --description "Command-line interface"
gh label create domain:core --color "0052CC" --description "Core business logic"
gh label create domain:config --color "0E8A16" --description "Configuration management"
gh label create domain:networking --color "5319E7" --description "Network/HTTP handling"
```

### Bulk Label Creation Script

```bash
#!/bin/bash
# create-labels.sh

# Generic types
for type in feat fix chore docs test perf refactor ci build; do
  case $type in
    feat) color="0052CC" ;;
    fix) color="D73A4A" ;;
    chore) color="FEF2C0" ;;
    docs) color="0075CA" ;;
    test) color="1D76DB" ;;
    perf) color="F9D0C4" ;;
    refactor) color="FBCA04" ;;
    ci) color="7057FF" ;;
    build) color="BFD4F2" ;;
  esac

  gh label create "$type" --color "$color" --description "$(echo $type | tr a-z A-Z) type" --force
done

# Priority labels
gh label create priority:critical --color "B60205" --description "Critical priority" --force
gh label create priority:high --color "D93F0B" --description "High priority" --force
gh label create priority:medium --color "FBCA04" --description "Medium priority" --force
gh label create priority:low --color "0E8A16" --description "Low priority" --force

# Common scoped labels
gh label create feat:auth --color "0052CC" --description "Authentication feature" --force
gh label create feat:api --color "0052CC" --description "API feature" --force
gh label create fix:github-oauth --color "D73A4A" --description "OAuth bug fix" --force

echo "Labels created successfully"
```

---

## Best Practices

### DO ✅

- Use colon (`:`) for category separator
- Use hyphens for multi-word names
- Apply both generic and scoped labels when helpful
- Match type labels to branch/commit types
- Keep scope names concise and meaningful
- Create scoped labels as needed for your project

### DON'T ❌

- Use spaces in label names
- Use underscores instead of hyphens
- Create too many scoped variants (creates clutter)
- Use inconsistent scoping patterns
- Forget to update labels during workflow
- Over-label (5-8 labels maximum per issue)

---

## Migration from Hyphenated Labels

If project has existing `type-feature` style labels:

```bash
# Rename to colon format
gh label edit type-feature --name feat
gh label edit type-bug --name fix
gh label edit priority-high --name priority:high
gh label edit status-in-progress --name status:in-progress
gh label edit area-api --name domain:api

# Update all issues (example for one label)
gh issue list --label type-feature --json number --jq '.[].number' | \
  xargs -I {} gh issue edit {} \
    --remove-label type-feature \
    --add-label feat
```

---

## References

- git-branch-convention-0.1.md — Branch naming
- git-commit-convention-0.1.md — Commit messages
- github-milestone-convention-0.1.md — Milestone naming
- [GitHub Labels Documentation](https://docs.github.com/en/issues/using-labels-and-milestones-to-track-work/managing-labels)
- [Conventional Commits](https://www.conventionalcommits.org/)

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
