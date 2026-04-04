# Flow Examples

Declarative flow definitions for common engineering workflows.
Each flow conforms to `task-flow-spec-0.1.md`.

## Flows

### Base flows

| File | Purpose | Entry |
|------|---------|-------|
| [oss-release-prep.yaml](oss-release-prep.yaml) | Prepare a codebase for initial OSS release | `audit-repo` |
| [cheatsheet-update.yaml](cheatsheet-update.yaml) | Create/update human + agent cheatsheets | `audit-surface` |
| [create-agent-skill.yaml](create-agent-skill.yaml) | Create a folder-based agent skill for any tool | `decide-shape` |
| [code-review.yaml](code-review.yaml) | Systematic code review against plan and standards | `load-context` |
| [systematic-debugging.yaml](systematic-debugging.yaml) | Structured bug investigation | — |
| [test-driven-development.yaml](test-driven-development.yaml) | TDD workflow | — |
| [verification-before-completion.yaml](verification-before-completion.yaml) | Evidence-before-assertions gate | `identify-claims` |
| [writing-plans.yaml](writing-plans.yaml) | Structured plan authoring | — |
| [executing-plans.yaml](executing-plans.yaml) | Plan execution with tracking | — |
| [brainstorming.yaml](brainstorming.yaml) | Divergent idea generation | — |
| [finishing-development-branch.yaml](finishing-development-branch.yaml) | Branch completion + integration | `verify-completion` |
| [pr-review-loop.yaml](pr-review-loop.yaml) | Automated reviewer→fixer loop until PR approved | `load-pr-context` |

### Validated variants (`-validated.yaml`)

Extend the base flows with two additional layers:

- **eva** (`~/.w/ideacrafterslabs/eva`) — deterministic contract gating between
  phases; Tier 1 evaluators only (zero LLM calls, binary exit codes)
- **routellm** (`~/.w/ideacrafterslabs/routellm`) — cost-routing for LLM-heavy
  generative steps; routes cheap queries to weak model, expensive ones to strong

| File | Base flow | eva gates added | routellm-routed steps |
|------|-----------|-----------------|-----------------------|
| [oss-release-prep-validated.yaml](oss-release-prep-validated.yaml) | oss-release-prep | 6 gates (audit, legal, docs, community, cicd, release) | 3 (readme→strong, contributing→medium, security→weak) |
| [cheatsheet-update-validated.yaml](cheatsheet-update-validated.yaml) | cheatsheet-update | 3 gates (structure, flags, render) | 3 (human→medium, agent→strong, cross-check→medium) |
| [create-agent-skill-validated.yaml](create-agent-skill-validated.yaml) | create-agent-skill | 4 gates (structure, frontmatter, scripts, discovery) | 4 (skill-md→strong, references→medium, scripts→medium, description→strong) |

---

## Tool-Assisted Steps

Several flows contain steps where a known tool does the deterministic
work — the LLM only interprets results or makes decisions requiring
judgement. This section documents every such step, the tool used, and
the token budget saved vs. pure-LLM execution.

Token estimates use a rough model:
- **LLM-only** — agent reads context, reasons through the operation,
  generates output: ~2 000–8 000 tokens per step depending on content size
- **Tool-assisted** — agent runs a command, reads a short report,
  acts on failures only: ~200–600 tokens per step

---

### `oss-release-prep` — tool-assisted steps

| Step | Tool | What the tool does | Tokens saved |
|------|------|--------------------|-------------|
| `audit-repo` | **gitleaks** + git one-liner | Full-history secret scan; large-blob list | ~4 000 |
| `add-license` | **gh CLI** (`gh repo edit --license`) | Writes canonical LICENSE text; no generation | ~2 500 |
| `add-copyright-headers` | **addlicense** | Walks tree, applies SPDX headers, skips vendor | ~3 500 |
| `add-code-of-conduct` | **npx covgen** | Emits Contributor Covenant v2.1 verbatim | ~2 000 |
| `add-dependency-bot` | **npx create-renovate-config** / heredoc | Writes dependabot.yml or renovate.json | ~1 500 |
| `add-release-workflow` | **goreleaser init** / **semantic-release** | Scaffolds full release pipeline YAML | ~4 000 |
| `run-full-test-suite` | **just / make** | Runs tests; exits non-zero on failure | ~1 000 |
| `cut-initial-tag` | **gh release create** | Tags, pushes, creates GitHub Release with notes | ~1 500 |

**Subtotal saved: ~20 000 tokens** across 8 of 22 steps.

Steps that remain LLM-only (judgement required):
`write-readme`, `write-contributing`, `write-security-policy`,
`add-issue-templates`, `add-pr-template`, `add-codeowners`,
`final-review`, `cut-initial-tag` (version decision).

---

### `cheatsheet-update` — tool-assisted steps

| Step | Tool | What the tool does | Tokens saved |
|------|------|--------------------|-------------|
| `audit-surface` | **validate-doc-commands.sh** + `--help` sweep | Enumerates all flags; diffs against docs; exits non-zero on stale | ~3 000 |
| `validate-human-examples` | **validate-doc-commands.sh** + **markdownlint** | Probes every flag in human cheatsheet; structure check | ~2 500 |
| `validate-agent-examples` | **validate-doc-commands.sh** + bypass-flag probes | Same + targeted grep checks on bypass flags | ~2 500 |
| `write-cheatsheets` (gate) | **validate-doc-commands.sh** | Final exit-0 gate before commit | ~500 |
| `verify-render` | **markdownlint** + **prettier** | Table structure, fencing, heading order, line length | ~1 500 |

**Subtotal saved: ~10 000 tokens** across 5 of 14 steps.

Steps that remain LLM-only:
`read-cli-spec`, `read-agents-md`, `read-existing-cheatsheets`,
`draft-human-cheatsheet`, `draft-agent-cheatsheet`, `cross-check`.

---

### `create-agent-skill` — tool-assisted steps

| Step | Tool | What the tool does | Tokens saved |
|------|------|--------------------|-------------|
| `audit-tool` | **`--help` sweep** + **just --list** / **OpenAPI dump** | Dumps full CLI surface and ops list to temp files | ~3 500 |
| `scaffold-folder` | **skill-creator init_skill.py** | Creates canonical folder layout + frontmatter placeholder | ~1 000 |
| `validate-structure` | **skill-creator quick_validate.py** | Checks name format, description length, word count, line length | ~2 000 |
| `validate-examples` | **shellcheck** + flag probes | Lints all scripts; probes every documented flag | ~2 500 |
| `register-skill` | **ln + ls** | Installs symlinks; verifies they resolve | ~500 |

**Subtotal saved: ~9 500 tokens** across 5 of 16 steps.

Steps that remain LLM-only:
`decide-shape`, `research-errors`, `research-ops`,
`write-skill-md`, `write-references`, `write-scripts`, `write-description`.

---

## Total Estimated Savings

| Flow | Steps with tooling | ~Tokens saved |
|------|--------------------|---------------|
| `oss-release-prep` | 8 / 22 | ~20 000 |
| `cheatsheet-update` | 5 / 14 | ~10 000 |
| `create-agent-skill` | 5 / 16 | ~9 500 |
| **Total** | **18 / 52** | **~39 500** |

At Sonnet pricing (~$3 / 1M input tokens) that is roughly **$0.12 saved
per full flow run** — but the more important gain is determinism:
tools either pass or fail with an exact error; the LLM never has to
re-read or re-derive a result that a binary already computed.

---

---

## eva — Deterministic Contract Gating

eva (`~/.w/ideacrafterslabs/eva`) enforces behavioral contracts between flow
phases. A gate step runs `eva run --dataset <path> --no-tui` and the flow
cannot proceed if eva exits non-zero.

### Why eva, not the LLM

| | LLM check | eva gate |
|-|-----------|----------|
| Deterministic | No | Yes — same inputs always same result |
| Token cost | 500–2 000 / check | 0 — Tier 1 evaluators use no LLM |
| Speed | 2–10 s | < 200 ms |
| Auditable | Hard | Every result stored in SQLite |

### Gate anatomy

Each gate step carries an `eva_contract_body` field showing the contract
inline for reference. At runtime the contract lives in `eva_contracts_dir`:

```
evals/
  oss-release/
    oss-audit.yaml        oss-audit-dataset.yaml
    oss-legal.yaml        oss-legal-dataset.yaml
    oss-docs.yaml         oss-docs-dataset.yaml
    oss-community.yaml    oss-community-dataset.yaml
    oss-cicd.yaml         oss-cicd-dataset.yaml
    oss-release.yaml      oss-release-dataset.yaml
  cheatsheet/
    cheatsheet-structure.yaml   structure-dataset.yaml
    cheatsheet-flags.yaml       flags-dataset.yaml
    cheatsheet-render.yaml      render-dataset.yaml
  agent-skill/
    skill-structure.yaml        structure-dataset.yaml
    skill-frontmatter.yaml      frontmatter-dataset.yaml
    skill-scripts.yaml          scripts-dataset.yaml
```

### Dataset convention

Each dataset sends the artifact being validated as the "response" field
of a synthetic test case. The gate contract's evaluators then assert
structural properties against that response.

```yaml
# evals/oss-release/oss-legal-dataset.yaml
name: oss_legal
target: file://LICENSE   # or a shell script that cats the relevant files
tests:
  - id: license_file
    input: "validate LICENSE file"
  - id: copyright_headers
    input: "validate source file headers"
```

### Tier 1 evaluators used in gates

| Evaluator | What it checks | Token cost |
|-----------|---------------|------------|
| `contains` | Substring present in response | 0 |
| `regex` | Pattern match/no-match in response | 0 |
| `json_schema_valid` | Response is valid JSON/YAML against schema | 0 |

`mode: binary` — fail if evaluator returns 0.
`mode: warn` — record but do not fail the run.
`mode: threshold` — fail if score < min_score.

---

## routellm — LLM Cost Routing

routellm (`~/.w/ideacrafterslabs/routellm`) routes LLM calls between a strong
(expensive, high-quality) model and a weak (cheap, fast) model based on
estimated query complexity.

### Why routellm for these flows

The validated flows contain LLM-generative steps that differ sharply in
quality requirements:

- Writing a skill's **discovery description** or a project's **README** is
  high-stakes — poor quality means the artifact is never used. → strong model.
- Writing **reference docs** or a **security policy boilerplate** follows a
  predictable template. → weak model handles it reliably.

Routing saves ~60–80% of LLM cost on generative steps without degrading output
quality on the steps that matter.

### Threshold convention

Each routellm-routed step carries a `routellm_model` key with the model string
used in the API call:

```
model="router-mf-{threshold}"
```

| Threshold | Routing behaviour | Use for |
|-----------|------------------|---------|
| 0.05–0.15 | Almost always strong | Discovery-critical or error-prone content |
| 0.30–0.40 | Mixed; mf decides per query | Medium-complexity structured docs |
| 0.50–0.60 | Balanced | Consistency checks, reference docs |
| 0.70–0.80 | Almost always weak | Boilerplate, templated content |

### Quick setup

```python
from routellm.controller import Controller
import yaml

with open("config/routellm.yaml") as f:
    config = yaml.safe_load(f)

client = Controller(
    routers=["mf"],
    strong_model="gpt-4o",
    weak_model="gpt-4o-mini",
    config=config,
)

# In a flow step with routellm_model: "router-mf-0.15"
response = client.chat.completions.create(
    model="router-mf-0.15",
    messages=[{"role": "user", "content": prompt}],
)
```

### Token savings from routing

Assuming strong model = 5× cost of weak model and 50% of routed calls
go to weak model at a 0.30 threshold:

| Flow | Routed steps | Estimated saving vs all-strong |
|------|-------------|-------------------------------|
| oss-release-prep-validated | 3 | ~40% on doc-generation steps |
| cheatsheet-update-validated | 3 | ~35% on draft steps |
| create-agent-skill-validated | 4 | ~45% on write steps |

---

## Tool Prerequisites

Install these once; all three flows draw on them.

```bash
# Secret scanning
brew install gitleaks

# License headers
go install github.com/google/addlicense@latest

# Code-of-conduct generation
npm install -g covgen

# Markdown linting + formatting
npm install -g markdownlint-cli prettier

# Go release pipeline
brew install goreleaser

# Skill validation (if using create-agent-skill)
# skill-creator scripts are in ~/.agents/skills/skill-creator/scripts/
```

---

## Convention: `meta.tool` field

Steps that delegate to a tool carry a `meta.tool` key naming the
binary, and a `meta.commands` block with the exact invocations.
`meta.guidance` covers only the residual LLM work (interpretation,
decisions, content generation).

```yaml
meta:
  tool: "gitleaks"
  commands: |
    gitleaks detect --source . --report-format json \
      --report-path gitleaks-report.json
  guidance: |
    - Review report; block release on HIGH findings (LLM decision)
```

Steps without `meta.tool` are pure LLM tasks.
