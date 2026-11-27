# Recipes & Assignees

A **recipe** is a versioned YAML template that creates a track and its tasks
the same way every time: an ordered list of steps with dependencies, a kind
per step (`agent`, `exec` or `human`), retries, gates and due dates, plus
the variables a run binds. Recipes only *create* tasks; `tlc track execute`
is the single engine that runs them.

An **assignee** is a specialized executor profile (capabilities, tools,
domains) that the assignment engine matches against a task's requirements.

- [Write a recipe](#write-a-recipe)
- [Find and inspect recipes](#find-and-inspect-recipes)
- [Create a track or tasks from a recipe](#create-a-track-or-tasks-from-a-recipe)
- [Run the tasks](#run-the-tasks)
- [Keep a track in sync with its recipe](#keep-a-track-in-sync-with-its-recipe)
- [Turn existing work into a recipe](#turn-existing-work-into-a-recipe)
- [Assignees](#assignees)
- [Reference](#reference)

The normative grammar lives in [recipe-spec-0.1.md](recipe-spec-0.1.md).

## Write a recipe

A minimal recipe is a name, a version and one step:

```yaml
recipe: smoke
version: 0.1.0
steps:
  - id: build
    kind: exec
    exec:
      argv: [make, build]
```

A complete one declares its vars, the track it builds, and steps of every
kind:

```yaml
recipe: code-review
version: 1.0.0
description: Review a pull request end to end
requires:
  subject: none            # task | track | none (default)
vars:
  pr:
    description: Pull request number
    required: true
  reviewer:
    default: alice
  sprint_end:
    default: "2026-10-01"
agent: claude              # default agent for agent-kind steps
track:
  title: "Review PR {{pr}}"
  type: chore
  plan: |
    # Review PR {{pr}}
    Checked out, linted, reviewed and signed off.
  meta:                    # user meta on the created track
    pr: "{{pr}}"

steps:
  - id: fetch
    kind: exec
    title: "Check out PR {{pr}}"
    exec:
      argv: [gh, pr, checkout, "{{pr}}"]
      timeout: 2m

  - id: lint
    kind: exec
    depends_on: [fetch]
    exec:
      argv: [make, lint]
    retry:
      max_attempts: 2
      backoff: 30s

  - id: review
    title: "Review PR {{pr}}"
    depends_on: [lint]
    when: "results.lint.exit_code == 0"
    due: "{{sprint_end}}"
    gate:
      contract: review-quality
      eva_url: http://eva.internal

  - id: sign-off
    kind: human
    title: "Sign off on PR {{pr}}"
    depends_on: [review]
    assignee: "{{reviewer}}"
    due: {after: review, offset: 2d}
    human:
      timeout: 48h
      on_timeout: reject
```

Validate it before anything else:

```bash
tlc recipe validate ./code-review.yaml
```

### Steps and kinds

Every step needs an `id` (`[a-z0-9][a-z0-9-]*`) and is exactly one of a
task step, an `include` or a `repeat` block. A task step's `kind` decides
who does the work:

| Kind | Who runs it | Step keys |
|------|-------------|-----------|
| `agent` (default) | the `agent` named on the step, else the recipe's `agent`, else `track execute --agent` | `agent` |
| `exec` | the executor itself, as a literal command | `exec.argv` (required), `exec.cwd`, `exec.env`, `exec.timeout`, `exec.stdout_max` |
| `human` | a person, through `tlc task approve` / `tlc task reject` | `human.assignee` (used when the step has no `assignee`), `human.timeout`, `human.on_timeout` (`approve` or `reject`) |

Any task step may carry `title`, `description`, `depends_on`, `when`,
`assignee`, `due`, `retry` (`max_attempts`, `backoff`) and `gate`
(`contract`, `eva_url`). `title` defaults to the step id.

A step also sets what its task carries beyond the work itself: `tags`
(a list), `meta` (user meta, a map) and `priority`. They behave as on a
hand-made task: tags go through the tag policy, `priority` must be in the
configured vocabulary and counts as set by hand, `meta` may not use a key
tlc reserves (`blocked_by`, `priority_source`, `origin_*`, ...). Values are
templated like `title`. The recipe's `track.meta` does the same for the
track `track create --recipe` builds. Every check runs before anything is
created, and `recipe validate` runs the static ones.

```yaml
track:
  title: "Release {{version}}"
  meta: {owner: release-team}
steps:
  - id: changelog
    tags: [docs, "release:{{version}}"]
    priority: P1
    meta: {ticket: "REL-{{version}}"}
```

Reconcile and `recipe diff` leave these fields alone, as they do `title`
and `description`: a task keeps the definition it was created from, and
changing them in the recipe shows up only as content drift.

Keys starting with `x-` are yours to use for anything: they are kept and
written back, and ignored. `x-tlc` is reserved.

### Variables and templates

`vars` declares what a run binds. A var is `required: true` or has a
`default` (never both). Values come from `--var key=value` (comma-separate
several pairs, or repeat the flag), from a run's recorded vars when
reconciling, or from a terminal prompt when a required var is missing.

Placeholders are `{{name}}` (or `{{vars.name}}`) and are rendered when the
tasks are created. Other namespaces:

| Placeholder | Bound from |
|-------------|------------|
| `{{subject.id}}`, `.title`, `.description`, `.track`, `.tags`, `.project` | the task or track the recipe is applied to (`--for`) |
| `{{run.id}}`, `{{run.recipe}}`, `{{run.version}}` | the materialization itself |
| `{{run.iteration}}` | the 1-based iteration, inside a `repeat` block only |
| `{{steps.<id>.<field>}}` | another step's rendered `id`, `title`, `description`, `due`, `assignee`, `when`, `agent` or `kind` |
| `{{results.<step>.<path>}}` | left as written at creation; only `when` reads results, at dispatch |

A placeholder that names an undeclared var or an unknown field fails
validation, so a typo never ships as literal braces in a task. The names
`vars`, `subject`, `run`, `steps` and `results` are reserved.

### Due dates

`due` takes any form `tlc task create --due` accepts (`tomorrow`, `in 3d`,
`friday`, `2026-10-01`, RFC 3339), optionally with an offset:

```yaml
due: "{{sprint_end}}"                   # a var, a date, a relative form
due: "{{steps.review.due}} + 2d"        # another step's due plus an offset
due: {after: review, offset: 2d}        # the same, structured
```

Offsets use recipe durations (`30s`, `48h`, `5d`, `2w`) and are applied to
absolute bases; a relative base is handed to the due parser as written.

### Conditions (`when`)

`when` gates a task at dispatch time. It is a single comparison
`lhs OP rhs` with `==`, `!=`, `<`, `<=`, `>`, `>=`; the left side reads an
upstream result (`results.<step>.<path>`) or a var (`vars.<name>`). The
step named must be a dependency, directly or transitively. When the
condition is false the task is skipped; when it cannot be evaluated the
task is blocked with the error as its reason.

A condition the recipe vars alone decide is evaluated when the recipe is
materialized: a step whose `when` is false then is not created at all, and
a step that depends on it simply has no edge to it (a skipped task is
terminal, so its dependents run either way). A true condition still
travels with the task and is checked again at dispatch. A condition that
reads `results.*`, is not a single comparison, or fails to evaluate (for
example an undefined var) is left to the executor; the last case is
reported as a warning. `--dry-run` and `recipe diff` follow the same rule:
the dry run lists only the steps that would be created, and `recipe diff`
reports a step left out this way as `dropped`, not `missing`.

An exec task's result is `exit_code`, `stdout`, `stderr`, `duration_ms`
and `truncated`; an agent task's result is the `outputs` object its
results file reports.

### Compose recipes (`include`)

A step may splice another recipe in, binding its vars through `with`:

```yaml
steps:
  - id: sec
    include: security-scan@1.2.0
    with:
      target: "{{pr}}"
  - id: merge
    depends_on: [sec]      # after the whole block
```

Included steps get ids `sec/<child>`; edges on the block mean "after every
leaf of the block". Every required var of the child must be bound under
`with` (or have a default); the nesting depth is capped at 8 and cycles
are refused. `tlc recipe show` prints the expanded list.

### Loop (`repeat` / `until`)

A block can be unrolled a fixed number of times, stopping early once a
result satisfies `until`:

```yaml
steps:
  - id: fix
    repeat: 3
    until: "results.test.exit_code == 0"
    steps:
      - id: patch
        title: "Fix attempt {{run.iteration}}"
      - id: test
        kind: exec
        depends_on: [patch]
        exec: {argv: [make, test]}
```

Iterations become `fix/1/patch`, `fix/1/test`, `fix/2/patch`, ... each
chained after the previous iteration's leaves. From the second iteration
on, every step carries a derived `when` that reads the previous
iteration's result and negates `until`, so the loop stops as soon as the
tests pass. Steps inside a block with `until` cannot declare their own
`when`.

## Find and inspect recipes

Recipes are searched in order:

1. `recipe.dir` from config, relative to the project root (no default);
2. the project's `.tlc/recipes`;
3. the user config dir, `~/.config/tlc/recipes`;
4. the built-in set compiled into tlc (`--source builtin`).

```bash
tlc recipe list                          # every recipe, by layer then name and version
tlc recipe list --source .tlc/recipes    # one layer only
tlc recipe show code-review              # header, vars, expanded steps
tlc recipe show code-review -f json      # the expanded document
tlc recipe validate code-review@1.0.0    # exit 0 when valid, 1 with the first problem
```

A recipe is referenced by name, by `name@version`, or by file path. An
unpinned name resolves to the first layer that has it, highest version in
that layer; a pinned `name@version` is searched across every layer. A
recipe of the same name in any directory therefore shadows a built-in one,
whatever its version, while `name@<built-in version>` still reaches the
built-in.

```yaml
# .tlc/config.yaml
recipe:
  dir: recipes                # extra project layer, searched first
  assignees_dir: examples/assignees
```

### Built-in recipes

These ship inside the tlc binary, so every project has them with nothing
to copy. Their sources and test fixtures live in `internal/recipes/`.

| Recipe | Needs | What it does |
|---|---|---|
| `track-consolidate` | `scope` | Audit a set of tracks against the code and end with fewer tracks, losing no work |
| `decision-record` | `question` | Weigh the options, let the owner decide, write a numbered record and tag the tasks it affects |

`track-consolidate` takes `scope` (a topic matched against track titles
and plans, or track ids separated by spaces), `keep` (pinned track ids,
never removed, may gain tasks), `exclude`, `code_root` (default `.`) and
`workdir` (default `track-consolidate`, where it writes its files). List
values are space-separated because `--var` splits on commas.

Its steps: inventory the tracks; verify every open task against the code
(DONE needs a file:line or commit SHA; SUPERSEDED and DUPLICATE cite the
replacement; anything uncertain stays open); propose the regrouping;
generate `apply.sh`, a script of exact tlc commands; a human approval
task; run the script, logging each failure and carrying on; verify the
result. The first four steps only read tlc. Nothing in tlc changes until
the approval task is approved.

```bash
tlc track create --recipe track-consolidate --var "scope=L-0003 L-0007" --var keep=L-0003
tlc track execute <track>      # runs up to the approval task
# read track-consolidate/proposal.md and apply.sh, then:
tlc task approve <approval-task>
tlc track execute <track>      # apply and verify
```

`decision-record` takes `question` (the decision, phrased as a question),
`owner` (assignee of the decision task), `affected` (task ids separated by
spaces, or a topic matched against task titles), `source_of_truth` (where
the facts live, default `.`), `record_dir` (default `docs/decisions`) and
`workdir` (default `decision-record`).

Its steps: frame the question with cited facts and resolve the affected
tasks; lay out at least two options, one of them the status quo, with
their trade-offs; recommend one and draft the record (status Proposed);
generate `apply.sh`; a human decision task assigned to the owner; run the
script, which numbers the record after the highest one in `record_dir`,
marks it Accepted, marks any record it supersedes, and tags each affected
task `decision:<NNNN>-<slug>`; verify. To take a different option than the
one recommended, edit the draft before approving. Links run one way: the
record names no task ids, the tasks carry the tag.

```bash
tlc track create --recipe decision-record --var "question=Keep SQLite as the only backend?" \
  --var owner=alice --var "affected=T-0012 T-0019"
tlc track execute <track>      # runs up to the decision task
# read decision-record/options.md, recommendation.md and record.md, then:
tlc task approve <decision-task>
tlc track execute <track>      # record and tag
tlc task list --tag decision:0001-keep-sqlite-as-the-only-backend
```

To replace a built-in, put a recipe with the same name in any directory
on the search path; `tlc recipe show <name>` names the source in use.

## Create a track or tasks from a recipe

The 90% path: a new track whose title, type and `plan.md` come from the
recipe's `track` block, with one task per step:

```bash
tlc track create --recipe code-review --var pr=42
tlc track create "Review PR 42 (hotfix)" --recipe code-review --var pr=42 --type fix
```

Add tasks to an existing track, or create them trackless:

```bash
tlc task create --recipe code-review --var pr=42 --track review-42
tlc task create --recipe code-review --var pr=42
```

Both commands honour `--dry-run`, which prints what would be created
without writing. Every materialization is recorded as a **run** (see
[Runs ledger](#runs-ledger)).

### Create a track from a recipe over HTTP

`tlc serve` exposes the same operation as `POST /tracks`:

```json
{"recipe": "code-review", "vars": {"pr": 42}, "title": "Review 42", "slug": "review-42", "type": "feature"}
```

`recipe` is a name or `name@version` resolved through the search path;
file paths are refused. `vars` binds the recipe's vars by name (numbers and
booleans are taken in their plain spelling, lists and maps as JSON text).
`title`, `slug` and `type` are optional: they default to the recipe's
`track` block and the configured defaults, as on the CLI. `meta` cannot be
combined with `recipe`, and a recipe that requires a task subject cannot be
created this way. Nothing prompts: a missing required var is an error.

The `201` body is the created track, as for a plain create, plus
`recipe_run` with the run id, the created and skipped steps and the
warnings. A bad reference, var, title, slug or recipe is a `422` in the
usual error shape (`code` is `recipe_not_found`, `invalid_recipe`,
`invalid_vars` or `validation_error`).

### Apply a recipe to a task or track (subjects)

A recipe that decomposes something names it with `--for`; the reference
is bound as `subject.*` for templates and recorded on the run:

```bash
tlc task create --recipe fix-bug --for T-0042    # into T-0042's track
tlc task create --recipe fix-bug --for auth      # into the auth track
```

With a task subject, the task is blocked on the run's leaves (the steps
nothing else depends on) and completes automatically once they are done.
A recipe can insist on a subject with `requires.subject: task` or `track`;
`tlc track create --recipe` makes the new track the subject of a
track-requiring recipe when no `--for` is given.

### Pick a subset of steps

`--task` keeps only some steps, by ordinal, range or id, in recipe order:

```bash
tlc track create "Review 42" --recipe code-review --var pr=42 --task 1-3
tlc task create --recipe code-review --var pr=42 --task lint,review --with-deps
```

Without `--with-deps`, a dependency on an unselected step is dropped and
recorded on the run; with it, the dependencies are pulled in with their
closure. Step ids are the stable form: ordinals shift when a recipe gains
a step.

### Assign steps automatically

`--assign` runs the assignment engine for steps that name no assignee
(see [Assignees](#assignees)):

```bash
tlc track create --recipe code-review --var pr=42 --assign
```

## Run the tasks

```bash
tlc track execute review-42 --agent claude
tlc track execute review-42 --dry-run                 # batch plan and ready set only
tlc track execute review-42 --agent claude --concurrency 4
```

The executor loops until nothing is ready. Each round it:

1. recomputes readiness from `blocked_by` (every blocker terminal);
2. evaluates `when` on the ready tasks — false skips, an error blocks;
3. claims each ready task (`TODO` → `IN_PROGRESS`, stamping the actor and
   `claimed_at`) and dispatches it by kind: agent tasks to their agent,
   exec tasks as their argv, in step order;
4. applies the outcome: done (after the eva gate passes, when one is
   set), retried up to `retry.max_attempts` with `retry.backoff` between
   attempts, or blocked with the failure as reason once attempts are
   exhausted.

Only tasks created by a recipe run are dispatched; `--permissive` also
dispatches hand-made tasks. A task carrying a `blocked_reason` is never
dispatched.

### Human tasks

Human tasks are never dispatched. When only human tasks remain ready, the
command lists them and exits 0, or keeps polling with `--wait --poll 30s`.
Resolve them by hand:

```bash
tlc task approve T-0050 --by lead --note "release notes look good"
tlc task reject T-0050 --reason "scope creep; split the change"
```

`approve` completes the task (forced past the state machine: a decision
never passes through `IN_PROGRESS`) and completes the run's subject when
it was the last open leaf. `reject` leaves the status alone and marks the
task blocked with the reason, so everything behind it waits until someone
unblocks or skips it. A `human.timeout` with `on_timeout` applies the
policy lazily, at the next execute after the timeout has elapsed since
the task was created.

### Agents, pods and exec tasks

- `--agent <name>` supplies the agent for agent-kind tasks that name none;
  names resolve through `agents.yaml` (see
  [agents-yaml-spec-0.1.md](agents-yaml-spec-0.1.md)); `--trust-project`
  accepts a project-local registry without prompting; `--ctxt` hands the
  agent ctxt query handles.
- `--with-pod` (the default) runs each agent in its own container, up to
  `--concurrency` at once; `--with-pod false` runs agents on the host, one
  at a time whatever the bound. Passing `--with-pod` explicitly (true or
  an image) also runs exec tasks in a fresh pod: the working tree is
  copied to `/workspace`, `exec.env` is set at creation and `exec.cwd`
  resolves under `/workspace`.
- `--reclaim 30m` takes over claims older than 30 minutes — the leftovers
  of an actor that died — and re-dispatches them; human tasks are never
  reclaimed.
- `--timeout` bounds the whole run.

### Run a recipe for one task

`tlc task execute <task> --recipe <r>` materializes the recipe with the
task as subject, into the task's track, and runs the created tasks
straight away; the task completes once every leaf is done:

```bash
tlc task execute T-0042 --recipe fix-bug --var branch=main
tlc task execute T-0042 --recipe fix-bug --dry-run
```

## Keep a track in sync with its recipe

The run ledger, not the task table, says which steps a track already has.
`tlc track execute --recipe` reconciles before it runs: steps with no
ledger row are created in a new run (its `parent_run` is the latest run
of that recipe on the track), steps whose task exists are skipped, and
vars come from the latest run merged under `--var`:

```bash
tlc track execute review-42 --recipe code-review
tlc track execute review-42 --recipe code-review --dry-run     # reconcile plan + batch plan
tlc track execute review-42 --recipe code-review --recreate    # also recreate deleted steps
```

A step whose task was deleted after materialization is reported and left
alone unless `--recreate` is set; dependencies on it are dropped and
recorded. When the recipe's version or content hash differs from what the
latest run recorded, the command warns: existing tasks keep the
definition they were created from.

### See what differs

```bash
tlc recipe diff review-42                          # against the latest run's recipe
tlc recipe diff review-42 --recipe ./code-review.yaml
```

Per step the report says `materialized` (its task exists), `missing`
(never materialized), `deleted` (materialized, task since removed) or
`extra` (a task of the run chain the recipe no longer declares). The
header notes version and content-hash drift. Nothing is changed.

### Runs ledger

Every materialization is a run: which recipe, at which version and hash,
with which vars, for which subject, into which track, by whom, and which
task each step became.

```bash
tlc recipe runs                                   # this project, newest first
tlc recipe runs code-review                       # one recipe
tlc recipe runs code-review@1.0.0 --track review-42
tlc recipe runs --all-projects -f json            # full ledger rows
```

The TUI's `v` key cycles Dashboard → Kanban → Runs to browse the same
ledger. Task rows carry their provenance too: `run_id`, `step_id` and
`step_ordinal`, plus `recipe`, `recipe_version`, `recipe_hash` and
`subject` in `meta`.

## Turn existing work into a recipe

Capture a track as a recipe — one step per task in dependency order,
`depends_on` from the blocked-by edges inside the track:

```bash
tlc recipe import release-42                          # writes ./release-42.yaml
tlc recipe import release-42 --var pr=42              # "Review PR 42" becomes "Review PR {{pr}}"
tlc recipe import release-42 --install                # into the project's recipes directory
tlc recipe import release-42 -o - --name release --version 1.0.0
tlc recipe import release-42 --external-deps          # pull in blockers outside the track
```

Step ids come from recipe provenance where the tasks have it, otherwise
from the titles. `--var name=value` lifts every whole-word occurrence of
the value back into `{{name}}` and declares the var; the vars of the
track's latest run are lifted the same way. Edges to tasks outside the
track are dropped unless `--external-deps` captures those tasks as steps.

A markdown procedure reachable by URL (a GitHub blob or raw link) can be
converted too. The document and the relative markdown files it links are
fetched, rewritten in the recipe format by the model behind `LLM_API_KEY`
(or `OPENROUTER_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`;
`LLM_API_URL` and `LLM_MODEL` tune the endpoint), validated, and kept
verbatim as `track.plan`:

```bash
tlc recipe import https://github.com/org/repo/blob/main/docs/release.md --name release
```

An existing file is never overwritten without `--force`; `--dry-run` shows
what would be written.

### Test a recipe deterministically

Cassette-backed sandbox runs of a recipe are a developer tool; see
[AGENTS.md](../AGENTS.md).

## Assignees

Assignees are specialized executors that handle tasks based on their
capabilities, defined as YAML files in `recipe.assignees_dir` (default
`examples/assignees`):

```yaml
# examples/assignees/code-analyst.yaml
assignee_id: "assignee:code-analyst:1.0"
name: "Code Analyst"
version: "1.0"
description: "Senior code reviewer for plan alignment and quality assessment"
capabilities:
  task_types: [code-review, plan-alignment-check, architecture-review]
  tools: [grep, git-diff, ast-parser, linter]
  domains: [golang, python, javascript, system-design, testing]
instructions: |
  You are a Senior Code Reviewer ...
delegation:
  handoff_conditions:
    - when: "security_vulnerability_found"
      delegate_to: "assignee:security-specialist:1.0"
  unblocks: [implementation, deployment, integration]
```

```bash
tlc assignee list                                 # every assignee with its capabilities
tlc assignee show assignee:code-analyst:1.0       # full definition, delegation rules, instructions
```

### Capabilities

Assignees declare what they can do:

- **Task types** — the kinds of work they handle
- **Domains** — areas of expertise
- **Tools** — the tools they have access to

### Assignment engine

The engine scores every assignee against a task's requirements: 10 points
per capability match, 5 per domain match, 2 per tool match. The best
score wins; a score of zero leaves the task unassigned. It runs on
`--assign` for recipe steps that name no assignee.

### Delegation and hand-offs

`delegation.handoff_conditions` names the assignee to delegate to when a
condition arises; `delegation.unblocks` lists the task types the
assignee's completed work unblocks.

## Reference

| Command | Purpose |
|---------|---------|
| `tlc recipe list [--source <dir\|builtin>]` | recipes in the search path |
| `tlc recipe show <recipe>` | header, vars, expanded steps (`-f json` for the document) |
| `tlc recipe validate <recipe>` | parse, validate, expand; no storage opened |
| `tlc recipe import <track\|url> [--var --name --version -o --install --force --external-deps]` | capture or convert into a recipe file |
| `tlc recipe runs [<recipe>[@<version>]] [--all-projects --track <track>]` | the run ledger |
| `tlc recipe diff <track> [--recipe <recipe>]` | steps vs. ledger vs. tasks |
| `tlc track create [title] --recipe <r> [--var --task --with-deps --for --assign --type --dry-run]` | new track + tasks |
| `tlc task create --recipe <r> [--var --task --with-deps --track --for --assign --dry-run]` | tasks into a track, a subject's track, or trackless |
| `tlc task execute <task> --recipe <r> [--var --agent --with-pod --dry-run]` | decompose a task and run the steps |
| `tlc track execute <track> [--recipe <r> --recreate --var] [--agent --with-pod --concurrency --permissive --reclaim --wait --poll --timeout --trust-project --ctxt --dry-run]` | reconcile, then run |
| `tlc task approve <task> [--by --note]` / `tlc task reject <task> --reason <why> [--by]` | decide a human task |
| `tlc assignee list` / `tlc assignee show <assignee-id>` | assignee profiles |

Config keys: `recipe.dir` (extra project layer, relative to the project
root) and `recipe.assignees_dir` (default `examples/assignees`).

Recipe URIs: `tlc://recipe/<name>` and `tlc://<project>/recipe:<name>`
(see [identifiers-spec-0.1.md](identifiers-spec-0.1.md)).
