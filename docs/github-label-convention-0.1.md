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

**Examples**:
- `type:feat`
- `type:fix`
- `priority:high`
- `status:in-progress`
- `domain:auth`

### Rules

1. Category prefix MUST be present — tlc classifies a label by its colon, and an unprefixed label is discarded on sync (see [Type Labels](#type-labels))
2. Separator MUST be colon (`:`)
3. Name component MUST use hyphens for multi-word (no spaces)
4. All characters MUST be lowercase
5. SHOULD be concise (category + name ≤ 25 chars)

GitHub's own stock labels (`bug`, `enhancement`, `documentation`) carry no colon and so are invisible to tlc. They are harmless if left in place, but they do not reach a task — use the `type:*` axis instead.

---

## Label Categories (Normative)

### Type Labels

Categorize the nature of the issue. One label per Conventional Commits type, plus `type:breaking`:

| Label | Color | Description |
|-------|-------|-------------|
| `type:feat` | `#0052CC` | New feature |
| `type:fix` | `#D73A4A` | Bug fix |
| `type:refactor` | `#5319E7` | Behaviour-preserving restructure |
| `type:docs` | `#0075CA` | Documentation only |
| `type:test` | `#0E8A16` | Tests only |
| `type:chore` | `#CFD3D7` | Maintenance, no src or test change |
| `type:perf` | `#FF8C00` | Performance improvement |
| `type:build` | `#8D6E63` | Build system or dependencies |
| `type:ci` | `#1D76DB` | CI configuration |
| `type:style` | `#D4C5F9` | Formatting, no behaviour change |
| `type:breaking` | `#B60205` | Breaking change (`!` or `BREAKING CHANGE:`) |

`type:breaking` has no Conventional Commits *type* of its own. It represents the `!` marker and the `BREAKING CHANGE:` trailer, which modify any type, and is on the axis because it is the one commit fact the vocabulary could not otherwise express.

#### The `type:` prefix is mandatory

A bare `feat` is not a weaker label than `type:feat` — it is a label that does not survive a round trip. Two independent readers classify a label by its colon:

- **The sync mapper.** `mapLabelsToTask` in the github-sync plugin ignores any label with no colon in it. A bare `feat` on an issue is silently discarded on pull; a prefixed one it does not otherwise recognise becomes a task tag.
- **The TLS parser.** `isMetaToken` treats a token as metadata only when it carries a known `dimension:` prefix (or `@`, `#`, `=`). An unprefixed word is title text, so a bare `feat` on a todo.txt line parses into the task TITLE.

So an unprefixed type label is dropped in one direction and corrupts the title in the other. Keep the prefix on every type label you create.

**Usage**:
- MUST apply at least one type label per issue
- MUST use the `type:` prefix — see above
- Type SHOULD match branch type and commit type
- Use `domain:*` for the area of the codebase; the type axis says WHAT KIND of change, the domain axis says WHERE

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

Track issue lifecycle. This axis is deliberately small, because an issue's open/closed state already encodes most of it:

| Label | Color | Description |
|-------|-------|-------------|
| `status:in-progress` | `#1D76DB` | Actively being worked on |
| `status:blocked` | `#5C1A00` | Blocked — task carries a blocked reason |

A status label is generated only for a workflow status that is neither terminal nor initial. A terminal status is a closed issue and the initial status is an open issue nobody has touched, so labelling either would restate what the issue state already says — and the two could then disagree after an edit on the forge. With the built-in vocabulary that leaves `IN_PROGRESS` alone; a vocabulary that adds `IN_REVIEW` yields `status:in-review` as well.

`status:blocked` is not a workflow status. It mirrors the task's blocked reason, an orthogonal field a task carries WHILE it sits in some status, which is why it is fixed rather than generated.

**Usage**:
- MAY apply one or more status labels
- Update as issue progresses through workflow
- Remove previous status when updating

### Needs Labels

What the issue is waiting on from a PERSON, as opposed to from another task:

| Label | Color | Description |
|-------|-------|-------------|
| `needs:triage` | `#B08800` | Unreviewed — needs a first pass |
| `needs:repro` | `#8E6A3F` | Cannot reproduce — needs steps or a case |
| `needs:decision` | `#5319E7` | Blocked on a human decision, not on a task |

`status:blocked` means blocked-by-task. Nothing on that axis can say "this is open because a human has not answered yet", which is the gap these three fill.

### Domain Labels (Project-Specific)

Identify codebase subsystem or architectural layer. Labels auto-adapt based on project type.

There is no `domain:tests` on any set: a test-only change is `type:test`, and a test for a subsystem takes that subsystem's domain.

#### `go-binary` — Go Binary/CLI Application

| Label | Color | Description |
|-------|-------|-------------|
| `domain:cli` | `#BFD4F2` | CLI |
| `domain:core` | `#0052CC` | Core logic |
| `domain:config` | `#0E8A16` | Config |
| `domain:io` | `#1D76DB` | I/O |
| `domain:storage` | `#006B75` | Persistence and schema |

`domain:io` is the boundary the process reads and writes ACROSS; `domain:storage` is the durable state it OWNS, where a change means a migration and a compatibility question.

#### `node-backend` — Node Service

| Label | Color | Description |
|-------|-------|-------------|
| `domain:api` | `#1D76DB` | HTTP and API surface |
| `domain:db` | `#0E8A16` | Database and persistence |
| `domain:auth` | `#5319E7` | Authn and authz |
| `domain:jobs` | `#FBCA04` | Background jobs and queues |

#### `react-frontend` — React Frontend

| Label | Color | Description |
|-------|-------|-------------|
| `domain:frontend` | `#E99695` | Frontend UI |
| `domain:components` | `#1D76DB` | Components |
| `domain:state` | `#0052CC` | Client state management |
| `domain:api` | `#0075CA` | API integration |

#### `python-mvc` — Python MVC Application (Django/Flask)

| Label | Color | Description |
|-------|-------|-------------|
| `domain:models` | `#0052CC` | Models and ORM |
| `domain:views` | `#E99695` | Views and templates |
| `domain:api` | `#1D76DB` | API layer |
| `domain:migrations` | `#D93F0B` | Schema migrations |

There is no `domain:controllers`: Django calls that tier views, so the pair would give two labels for one file.

#### `library` — Consumed Package

| Label | Color | Description |
|-------|-------|-------------|
| `domain:api` | `#0052CC` | Public API surface |
| `domain:internal` | `#6A737D` | Internal implementation |
| `domain:docs` | `#0075CA` | Docs and examples |

`domain:api` here means the exported surface consumers compile against, and `domain:internal` is its complement — "is this visible to consumers?" is the first question asked of any change in this shape.

#### `monorepo` — Multi-Package Repo

| Label | Color | Description |
|-------|-------|-------------|
| `domain:tooling` | `#FBCA04` | Build graph and workspace tooling |
| `domain:release` | `#5319E7` | Versioning and publishing |
| `domain:deps` | `#8D6E63` | Shared dependencies |

Layer-shaped domains would be wrong here — the packages have their own layers, and they differ per package. These three name the work that exists BECAUSE the packages share a repo.

#### `infra` — Declarative Infrastructure

| Label | Color | Description |
|-------|-------|-------------|
| `domain:terraform` | `#5319E7` | Terraform and IaC |
| `domain:k8s` | `#1D76DB` | Kubernetes manifests and charts |
| `domain:network` | `#0E8A16` | Networking, DNS, ingress |
| `domain:secrets` | `#B60205` | Secrets and credentials |

Split by what BREAKS when the change is wrong.

#### `generic` — Everything Else

For projects that don't fit a specific pattern or span multiple types:

| Label | Color | Description |
|-------|-------|-------------|
| `domain:core` | `#0052CC` | Core logic |
| `domain:api` | `#1D76DB` | API layer |
| `domain:docs` | `#0075CA` | Documentation |
| `domain:ci` | `#BFD4F2` | CI and automation |

`domain:ci` overlaps `type:ci`, and that is fine: `type:ci` is the kind of change, `domain:ci` is the part of the repo it lands in. A fix to a flaky workflow is `type:fix` + `domain:ci`, and neither label alone places it.

**Usage**:
- MAY apply multiple domain labels if issue spans subsystems
- Labels SHOULD be adapted to match project architecture
- Helps route issues to appropriate team members
- Create domain labels as needed for your project structure

### Effort Labels

Estimate complexity (t-shirt sizing):

| Label | Color | Description |
|-------|-------|-------------|
| `effort:xs` | `#C2E0C6` | Extra small |
| `effort:s` | `#9EDAB0` | Small |
| `effort:m` | `#7BC99B` | Medium |
| `effort:l` | `#4FA97F` | Large |
| `effort:xl` | `#2E8B62` | Extra large |

**Usage**:
- SHOULD apply one effort label for planning
- Estimate before starting work
- Helps with sprint planning and capacity

### Special Labels

GitHub-native conventions that predate the axes. They carry no colon, so tlc does not read them — they are for humans and for GitHub's own features (`good-first-issue` and `help-wanted` drive repository discovery). Do not rely on them reaching a task:

| Label | Color | Description |
|-------|-------|-------------|
| `good-first-issue` | `#7057FF` | Good for newcomers |
| `help-wanted` | `#008672` | Extra attention needed |
| `security` | `#D73A4A` | Security vulnerability |
| `dependencies` | `#0366D6` | Dependency updates |
| `question` | `#D876E3` | Question or discussion |

Use `type:breaking` for a breaking change, not a `breaking-change` label. The two would be a second name for one fact, free to disagree, and only the prefixed one survives a sync.

---

## Label Combinations

Each axis answers a different question, so a well-labelled issue carries one of each: `type:*` says what kind of change, `domain:*` says where, `priority:*` and `effort:*` size it.

### Feature Request

```
type:feat
priority:medium
domain:api
effort:m
```

### Critical Bug (Go Application)

```
type:fix
priority:critical
domain:core
effort:s
status:in-progress
```

### Breaking Feature (Python MVC)

```
type:feat
type:breaking
priority:high
domain:models
domain:api
effort:l
```

### Documentation Task

```
type:docs
priority:low
domain:docs
effort:xs
good-first-issue
```

---

## Project Type Auto-Detection

### Detection Strategy

`tlc label init` detects the project type from marker files in the working
directory and suggests the matching domain set. Order matters: the first
branch that matches wins.

| Order | Type | Markers |
|-------|------|---------|
| 1 | `monorepo` | `pnpm-workspace.yaml`, `go.work`, or `turbo.json` |
| 2 | `infra` | a `terraform/`, `helm/`, `k8s/` or `kubernetes/` directory, or any root `*.tf` |
| 3 | `go-binary` | `go.mod` **with** a root `main.go` or a `cmd/` directory |
| 3 | `library` | `go.mod` **without** either |
| 4 | `react-frontend` | `package.json` with `src/App.tsx` or `src/App.jsx` |
| 4 | `node-backend` | `package.json` otherwise |
| 5 | `python-mvc` | `manage.py` or `requirements.txt` |
| 6 | `generic` | nothing above matched |

Monorepo is checked first because a workspace root carries the same markers
its members do — a `go.work` root almost always has a root `go.mod` — so any
language branch above it would claim the root. Infra is checked before the
language branches because an infra repo is frequently ALSO a Go or Node repo,
and there the infrastructure is the product.

### CLI Command

`label init` PRINTS the suggested set. It does not contact the forge and
creates nothing — see [Label Management](#label-management) to apply it.

```bash
# Auto-detect and print the suggested set
tlc label init

# Output:
# Detected project type: go-binary
# Suggested labels for go-binary:
#   - type:feat (0052CC): New feature
#   - type:fix (D73A4A): Bug fix
#   ...
#   - domain:cli (BFD4F2): CLI
#   - domain:core (0052CC): Core logic
#   - domain:config (0E8A16): Config
#   - domain:io (1D76DB): I/O
#   - domain:storage (006B75): Persistence and schema
#
# ✓ Labels initialized locally

# Manual override
tlc label init --type python-mvc

# List available templates
tlc label templates
```

---

## CLI Usage

Labels without spaces enable easy GitHub CLI usage:

```bash
# Create issue with labels (Python MVC project)
gh issue create \
  --title "Add GitHub OAuth support" \
  --label type:feat \
  --label priority:high \
  --label domain:models \
  --label domain:api \
  --label effort:m

# Create issue (Go binary project)
gh issue create \
  --title "Add config file support" \
  --label type:feat \
  --label priority:medium \
  --label domain:config \
  --label domain:io \
  --label effort:s

# Filter issues by type
gh issue list --label type:feat

# Filter by domain
gh issue list --label domain:frontend

# Multiple filters — type and domain narrow independently
gh issue list --label type:fix --label priority:high --label domain:api

# Add a label
gh issue edit 42 --add-label domain:auth

# Remove labels
gh issue edit 42 --remove-label status:in-progress
```

---

## Type and Scope Are Separate Axes

The type of a change and the area it lands in are two independent facts, and each has its own axis. Do NOT fold the area into the type label as `feat:<scope>`.

### Format

```
type:<conventional-commits-type>      # what kind of change
domain:<area>                         # where it lands
```

`type:*` is a closed set — the eleven values in [Type Labels](#type-labels) and nothing else. `domain:*` is open, and its suggested values vary by project type (see [Domain Labels](#domain-labels-project-specific)).

### Examples

**Authentication**:
```
type:feat + domain:auth        # OAuth integration, 2FA
type:fix  + domain:auth        # session timeout, JWT validation bug
```

**API**:
```
type:feat + domain:api         # API v2, REST endpoints, GraphQL
type:fix  + domain:api         # rate limiting bug
type:perf + domain:api         # response time
```

**UI**:
```
type:feat  + domain:frontend   # dark mode, dashboard
type:fix   + domain:frontend   # mobile layout
type:style + domain:components # CSS formatting
```

**Infrastructure**:
```
type:ci    + domain:ci         # workflow, coverage reporting
type:build + domain:tooling    # build config
type:chore + domain:deps       # dependencies
```

### Why not `feat:<scope>`

Three reasons, in ascending order of severity:

1. A scoped type label is one label carrying two facts, so neither can be filtered independently. `type:feat` + `domain:auth` lets you list all features, or all auth work, or the intersection. `feat:auth` gives you only the intersection.
2. The scope half is unbounded, so `feat:auth`, `feat:oauth` and `feat:2fa` accumulate as three labels where one `domain:auth` would do.
3. `feat:auth` is not on the type axis at all. tlc reads it as an unrecognised `dimension:value` pair and files it as a plain task tag — it never becomes the task's type.

A scoped label at least survives the round trip, which a bare `feat` does not. But it arrives as a tag, not as a type.

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
            const TYPES = [
              'feat', 'fix', 'refactor', 'docs', 'test',
              'chore', 'perf', 'build', 'ci', 'style'
            ];

            const branch = context.payload.pull_request.head.ref;
            // Extract: feat/0042-github-oauth -> type:feat
            const match = branch.match(/^([^/]+)\//);

            // Only label a branch prefix that IS a Conventional Commits
            // type. An unknown prefix gets no label rather than a
            // `type:` label naming something off the axis.
            if (match && TYPES.includes(match[1])) {
              await github.rest.issues.addLabels({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: context.payload.pull_request.number,
                labels: [`type:${match[1]}`]
              });
            }
```

The branch's scope half is deliberately not turned into a label. Apply `domain:*` from the files the PR touches, or by hand — the scope in a branch name is prose, not a controlled vocabulary.

### Auto-Labeling from PR Title

```yaml
# Match PR title pattern: "feat(github-oauth)!: add OAuth support"
const TYPES = [
  'feat', 'fix', 'refactor', 'docs', 'test',
  'chore', 'perf', 'build', 'ci', 'style'
];

const title = context.payload.pull_request.title;
const titleMatch = title.match(/^([a-z]+)(?:\([^)]+\))?(!)?:/);

if (titleMatch && TYPES.includes(titleMatch[1])) {
  const labels = [`type:${titleMatch[1]}`];

  // `!` or a BREAKING CHANGE: trailer adds the breaking label
  // alongside the type — it does not replace it.
  if (titleMatch[2] || /^BREAKING CHANGE:/m.test(context.payload.pull_request.body || '')) {
    labels.push('type:breaking');
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

`tlc label init` prints the suggested set for the detected project type but does not create anything on the forge. Creating labels is a manual `gh label create` step.

```bash
# Type labels
gh label create type:feat --color "0052CC" --description "New feature"
gh label create type:fix --color "D73A4A" --description "Bug fix"
gh label create type:chore --color "CFD3D7" --description "Maintenance, no src or test change"

# Priority labels
gh label create priority:critical --color "B60205" --description "Critical priority"
gh label create priority:high --color "D93F0B" --description "High priority"
gh label create priority:medium --color "FBCA04" --description "Medium priority"
gh label create priority:low --color "0E8A16" --description "Low priority"

# Domain labels (example: Go binary project)
gh label create domain:cli --color "BFD4F2" --description "CLI"
gh label create domain:core --color "0052CC" --description "Core logic"
gh label create domain:config --color "0E8A16" --description "Config"
gh label create domain:storage --color "006B75" --description "Persistence and schema"
```

### Bulk Label Creation Script

```bash
#!/bin/bash
# create-labels.sh
set -euo pipefail

# Type axis — the full Conventional Commits set plus breaking.
while read -r name color desc; do
  gh label create "type:$name" --color "$color" --description "$desc" --force
done <<'EOF'
feat     0052CC New feature
fix      D73A4A Bug fix
refactor 5319E7 Behaviour-preserving restructure
docs     0075CA Documentation only
test     0E8A16 Tests only
chore    CFD3D7 Maintenance, no src or test change
perf     FF8C00 Performance improvement
build    8D6E63 Build system or dependencies
ci       1D76DB CI configuration
style    D4C5F9 Formatting, no behaviour change
breaking B60205 Breaking change (! or BREAKING CHANGE:)
EOF

# Priority labels
gh label create priority:critical --color "B60205" --description "Critical priority" --force
gh label create priority:high --color "D93F0B" --description "High priority" --force
gh label create priority:medium --color "FBCA04" --description "Medium priority" --force
gh label create priority:low --color "0E8A16" --description "Low priority" --force

# Effort labels
gh label create effort:xs --color "C2E0C6" --description "Extra small" --force
gh label create effort:s --color "9EDAB0" --description "Small" --force
gh label create effort:m --color "7BC99B" --description "Medium" --force
gh label create effort:l --color "4FA97F" --description "Large" --force
gh label create effort:xl --color "2E8B62" --description "Extra large" --force

echo "Labels created successfully"
```

`--force` overwrites an existing label's colour and description but does NOT rename one. If the repo still carries bare `feat`/`fix` labels, run the renames in [Migration to Prefixed Labels](#migration-to-prefixed-labels) first — otherwise this script leaves the bare labels in place alongside the new prefixed ones.

---

## Best Practices

### DO ✅

- Use colon (`:`) for category separator — on EVERY label
- Use hyphens for multi-word names
- Apply one `type:*` and one or more `domain:*`, rather than folding the area into the type
- Match type labels to branch/commit types
- Add `type:breaking` alongside the type it modifies, never instead of it
- Create `domain:*` labels as needed for your project

### DON'T ❌

- Create a bare type label (`feat`, `fix`) — it is dropped on sync and parsed into task titles
- Fold the area into the type (`feat:auth`) — it arrives as a tag, not a type
- Invent a `type:*` value outside the eleven above
- Use spaces in label names
- Use underscores instead of hyphens
- Forget to update labels during workflow
- Over-label (5-8 labels maximum per issue)

---

## Migration to Prefixed Labels

Two starting points need migrating, and both end at the same place: every label carries a `dimension:value` prefix.

`tlc label init` does NOT do this for you. It detects the project type and PRINTS the suggested set to stdout; it never calls the forge, so no label is created, renamed or recoloured by running it. Use it to see the target set, then apply the renames yourself with `gh label edit`.

```bash
# See the target set (prints only — does not touch the forge)
tlc label init
```

### From hyphenated labels

If the project has `type-feature` style labels:

```bash
gh label edit type-feature      --name type:feat
gh label edit type-bug          --name type:fix
gh label edit priority-high     --name priority:high
gh label edit status-in-progress --name status:in-progress
gh label edit area-api          --name domain:api
```

### From bare type labels

If the project has bare `feat` / `fix` labels — earlier drafts of this document recommended them — rename them onto the type axis. Until you do, these labels are dropped on sync and parsed into task titles by the TLS reader, as described under [Type Labels](#type-labels):

```bash
gh label edit feat     --name type:feat
gh label edit fix      --name type:fix
gh label edit refactor --name type:refactor
gh label edit docs     --name type:docs
gh label edit test     --name type:test
gh label edit chore    --name type:chore
gh label edit perf     --name type:perf
gh label edit build    --name type:build
gh label edit ci       --name type:ci
gh label edit style    --name type:style
```

`gh label edit --name` renames in place, so every issue already carrying the label keeps it — no per-issue re-labelling is needed. Renaming also preserves the existing colour; pass `--color` as well if you want to move onto the swatches in the table above.

Scoped labels of the old `feat:<scope>` form are a different case: `feat:auth` already has a colon, so it round-trips as a tag rather than being dropped. It is not on the type axis, though, so rename it to the domain it actually names (`domain:auth`) and let `type:feat` carry the type.

### Re-labelling issues explicitly

Only needed if you created a NEW label rather than renaming an old one:

```bash
gh issue list --label feat --json number --jq '.[].number' | \
  xargs -I {} gh issue edit {} \
    --remove-label feat \
    --add-label type:feat
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
**Last Updated**: 2026-09-10
**Status**: Normative
