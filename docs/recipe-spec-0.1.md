# Recipe Specification (TLC-RECIPE) v0.1

## Version

- Version: 0.1
- Status: normative

## Summary

A recipe is a generation-only template: it is parsed, validated, expanded,
rendered and materialized into a track of tasks, then forgotten. It never
executes anything itself. The task executor behind `tlc track execute`
and `tlc task execute` is the single engine that runs the tasks a recipe
created, driven by the fields the recipe wrote onto them.

This spec OWNS:

- the recipe document grammar and its validation rules
- template namespaces, reserved names and rendering order
- expansion of `include` and `repeat` blocks
- step selection (`--task`, `--with-deps`)
- due arithmetic
- the run ledger schema
- the materialize and reconcile contracts
- the executor dispatch contract, the exec result shape, and the `when`
  condition grammar

This spec DOES NOT own:

- the task schema and statuses (see task-crud-spec-0.1.md)
- claiming and ownership between actors (see task-collab-spec-0.1.md)
- the audit log schema (see task-log-spec-0.1.md)
- assignee profiles and the assignment engine (see recipes.md)

Authoritative code: `internal/core/recipe*.go`, `condition.go`,
`executor*.go`, `exec_runner.go`, `exec_dispatcher.go`, `task_spec.go`,
`internal/storage/migrations.go`.

---

## 1. Document

### 1.1 Encoding

- A recipe is one YAML or JSON document. JSON is selected by a `.json`
  file name; anything else is decoded as YAML.
- The root MUST be a mapping. Unknown keys at any level are errors.
- Directory scans (`recipe list`, name resolution) pick up `.yaml` and
  `.yml` files only; a `.json` recipe is reachable by path.
- The raw bytes are fingerprinted with SHA-256 (hex); the hash travels
  with every run and task the recipe produces.
- Validation always runs on parse. There is no extension point that skips
  it.

### 1.2 Top-level keys

| Key | Type | Required | Meaning |
|-----|------|----------|---------|
| `recipe` | string | yes | Name. MUST match `^[a-z0-9][a-z0-9-]*$` (file-name and reference safe). |
| `version` | string | yes | Any non-empty string; compared semver-style (§1.4). |
| `description` | string | no | Free text shown by `recipe list` and `recipe show`. |
| `requires.subject` | `task` \| `track` \| `none` | no | What the recipe must be applied to. Empty means `none`. |
| `vars` | map of name → VarDef | no | Declared variables (§2). |
| `agent` | string | no | Default agent for `agent`-kind steps that name none. |
| `track.title` | string | no | Title of the track `track create --recipe` builds; templatable. |
| `track.type` | string | no | Track type when `--type` is not given. |
| `track.plan` | string | no | Body written to the track's `plan.md`; templatable. |
| `steps` | list of Step | yes | At least one step (§3). |

### 1.3 References

A recipe reference is one of:

- a **path**: anything containing `/` or `\`, ending in `.yaml`, `.yml`
  or `.json`, or naming an existing file. Opened directly.
- a **name**: resolved through the search path. The first layer that has
  the name wins, at its highest version.
- a pinned **`name@version`**: the exact version, searched across every
  layer in order.

Search path, in precedence order: `recipe.dir` (config, relative to the
project root; no default), the project's `<config dir>/recipes`
(`.tlc/recipes`), the user config dir's `recipes`. Missing directories
are skipped. A reference no layer resolves is an error naming the
reference and the directories searched.

`recipe.dir` and `recipe.assignees_dir` MUST be relative paths.

### 1.4 Version ordering

Versions are compared without a semver dependency: an optional leading
`v` is dropped; the numeric core is compared part by part with missing
parts as zero (`0.1` == `0.1.0`); a release sorts above any of its
pre-releases; pre-release parts are compared numerically when both are
numbers, numeric before alphanumeric, else as strings.

---

## 2. Variables

```yaml
vars:
  pr:
    description: Pull request number
    required: true
  reviewer:
    default: alice
```

| Key | Type | Meaning |
|-----|------|---------|
| `description` | string | Shown by `recipe show` and used as the prompt label. |
| `required` | bool | The run MUST bind a value. |
| `default` | any | Used when the run binds nothing. Keeps its declared type. |

Rules:

- A var name MUST match `^[A-Za-z_][A-Za-z0-9_]*$`.
- `required: true` and a `default` together are an error: a default makes
  the var optional.
- `subject`, `results`, `run`, `steps` and `vars` are reserved and MUST
  NOT be declared.

Binding (`ResolveRecipeVars`):

1. Every provided key MUST be declared; an unknown key is rejected before
   missing ones are checked, so a typo reads as "unknown", not "missing".
2. For each declared var, in name order: a provided value (a string, even
   when empty) wins, else the default, else the var is missing when
   required.
3. All missing required vars are reported together with the `--var` fix.

`--var` accepts `key=value[,key=value]` and is repeatable; the first `=`
splits key from value, so values may contain `=` but not `,`.

On reconcile (§10) the latest run's recorded vars are carried in under
the explicit `--var` values, keeping only names the recipe still
declares. On a terminal, missing required vars are prompted for.

---

## 3. Steps

`steps` is an ordered list. Each entry is exactly one of a **task step**
(default), an **include** (`include:` present) or a **repeat block**
(`repeat:` or nested `steps:` present). After expansion (§5) every step is
a task step with a 1-based `ordinal`.

### 3.1 Keys common to every step

| Key | Meaning |
|-----|---------|
| `id` | Required. MUST match `^[a-z0-9][a-z0-9-]*$`; `/` is reserved for expansion. Unique within its list. |
| `depends_on` | Step ids of the same list that must be terminal first. |
| `when` | Dispatch-time condition (§13). Not allowed on steps inside a block that declares `until`. |
| `title` | Task title; defaults to the step id. Templatable. |
| `description` | Task description (trimmed). Templatable. |
| `assignee` | Task assignee. Templatable. |
| `due` | Due date, string or structured form (§7). Templatable. |

### 3.2 Task step keys, per kind

`kind` is `agent` (default when absent), `exec` or `human`.

| Kind | Keys |
|------|------|
| `agent` | `agent` — agent name; falls back to the recipe's `agent`, then to `track execute --agent`. Templatable. |
| `exec` | `exec.argv` (required, non-empty; templatable), `exec.cwd` (templatable; relative values resolve against the executor's default cwd), `exec.env` (map; values templatable; an empty value unsets the variable), `exec.timeout` (duration, default 60s), `exec.stdout_max` (bytes per stream, default 1 MiB). |
| `human` | `human.assignee` (templatable), `human.timeout` (duration), `human.on_timeout` (`approve` or `reject`). |

Keys every task step may carry:

| Key | Meaning |
|-----|---------|
| `retry.max_attempts` | Attempts including the first; `0`/absent means 1. MUST NOT be negative. |
| `retry.backoff` | Duration slept after a failed attempt before the next round. |
| `gate.contract` | Required when `gate` is set: the eva contract the result must satisfy. Templatable. |
| `gate.eva_url` | Eva gateway base URL. |

Shape rules:

- `exec` block requires `kind: exec`; `kind: exec` requires `exec.argv`.
- `human` block requires `kind: human`; `human.on_timeout` MUST be
  `approve`, `reject` or absent.
- `kind`, `exec`, `human`, `retry` and `gate` belong on task steps only.
- `due.after` MUST NOT name the step itself; `due.offset` (optionally
  signed) MUST be a duration.
- Every duration field (`exec.timeout`, `human.timeout`, `retry.backoff`,
  `due.offset`) MUST parse (§16).

### 3.3 Include

```yaml
- id: sec
  include: security-scan@1.2.0
  with:
    target: "{{pr}}"
  depends_on: [fetch]
```

| Key | Meaning |
|-----|---------|
| `include` | Recipe reference (§1.3) resolved through the search path. |
| `with` | Values for the child's vars. Only valid on an include step; every key MUST be a var the child declares. String values are templatable in the parent's scope. |

An include MUST NOT combine with `steps`, `repeat` or `until`.

### 3.4 Repeat

```yaml
- id: fix
  repeat: 3
  until: "results.test.exit_code == 0"
  steps:
    - id: patch
    - id: test
      kind: exec
      depends_on: [patch]
      exec: {argv: [make, test]}
```

| Key | Meaning |
|-----|---------|
| `repeat` | Iteration count, MUST be positive when nested `steps` are present. |
| `steps` | The block's steps; required when `repeat` is set. |
| `until` | Condition (§13) ending the loop early; only valid with `repeat`. Its `results.<step>` reference MUST name a step of the block. |

---

## 4. Validation rules

`ValidateRecipe` runs at parse time on the authored document, with
include and repeat blocks as opaque nodes; `Expand` runs the graph rules
again on the flattened list. All rules are errors.

Header:

1. `recipe` present and matching the name grammar.
2. `version` present.
3. `steps` non-empty.
4. `requires.subject` ∈ {absent, `none`, `task`, `track`}.

Vars: §2 rules.

Per step (recursively into blocks, passing the enclosing `until`):

5. `id` present, matching the id grammar.
6. `when` and an enclosing block's `until` are mutually exclusive.
7. Exactly one of task / include / repeat (§3.2–§3.4 shape rules).
8. Block `until` parses as a condition.

Per list (the top level and every block):

9. No duplicate ids.
10. Every `depends_on` entry names a step of the same list.
11. No `depends_on` cycle.
12. `when` parses; a `results.<target>` reference names a step that is
    upstream of this step (in `depends_on`, directly or transitively) and
    not the step itself — or is deferred (rule 16).
13. `due.after` names another step of the list, or is deferred.
14. Every template reference in a templatable field resolves:
    - bare name or `vars.<name>` → a declared var;
    - `subject.<field>` → `id`, `title`, `description`, `track`, `tags`
      or `project`, and `requires.subject` is `task` or `track`;
    - `run.<field>` → `id`, `recipe`, `version`, or `iteration` inside a
      repeat block only;
    - `results.<step>.<path>` → a non-empty path and an upstream target
      as in rule 12;
    - `steps.<id>.<field>` → not the step itself; a field among `id`,
      `title`, `description`, `due`, `assignee`, `when`, `agent`, `kind`;
      a target that exists or is deferred;
    - any other namespace is an error.
15. Inside a repeat block the enclosing lists' ids are visible; a
    reference to one is accepted and checked for upstream-ness after
    expansion. An `until` whose `results.<target>` is not a step of the
    block is an error.
16. Deferred references: a target that is an enclosing list's id, or a
    `<head>/<rest>` path whose head is a block of this list or an
    enclosing list. They are checked once the graph is flat.

After expansion: rules 9–14 on the flattened list, plus a render-order
check — the implicit edges of `steps.*` references and `due.after` MUST
NOT form a cycle with `depends_on`.

---

## 5. Templates

### 5.1 Placeholder grammar

```
placeholder := "{{" WS? ref WS? "}}"
ref         := IDENT ( "." SEG )*
IDENT       := [A-Za-z_][A-Za-z0-9_]*
SEG         := [A-Za-z0-9_/-]+
```

Later segments admit `/` and `-` so expanded step ids can be referenced
(`{{results.sec/scan.stdout}}`, `{{steps.fix/1/test.due}}`).

### 5.2 Namespaces

| Namespace | Bound at | Fields |
|-----------|----------|--------|
| (bare) / `vars.` | create | the resolved vars |
| `subject.` | create | `id`, `title`, `description`, `track`, `tags`, `project` (§11) |
| `run.` | create | `id` (the run id), `recipe`, `version`, `iteration` (repeat only) |
| `steps.` | create | `id`, `title`, `description`, `due`, `assignee`, `when`, `agent`, `kind` of an already rendered step |
| `results.` | dispatch | left verbatim by rendering; read by `when` (§13) |

Reserved var names are exactly the five namespaces.

### 5.3 Templatable fields

Rendering, validation and expansion share one list of fields a
placeholder may appear in: `title`, `description`, `assignee`, `due`
(string form), `when`, `until`, `agent`, `exec.argv[*]`, `exec.cwd`,
`exec.env` values, `human.assignee`, `gate.contract`, and string values
under `with`. The `track.title` and `track.plan` headers are rendered
too.

### 5.4 Rendering

- Every placeholder is substituted; an unresolvable one is an error, so a
  typo never ships as literal braces.
- A string renders as itself; a list of strings (or of any values) joins
  with `,`; anything else prints with `%v`.
- Steps render in a topological order over `depends_on` plus the implicit
  edges of `steps.<id>` references and `due.after`; ties keep recipe
  order. As each step is rendered its field values become visible as
  `steps.<id>.*`, and its due is resolved (§7). Output keeps the input
  order.
- The input step list is never mutated; rendering returns copies.

---

## 6. Expansion

`Expand` flattens a recipe into the ordered task steps materialization
creates, then validates that list (§4). Ordinals are assigned after
expansion, 1-based.

### 6.1 Block dependencies

Within a list, a `depends_on` entry naming a block is replaced by the
block's **leaves** — its expanded steps that no other step of the block
depends on. An edge on a block therefore means "after the whole block".

### 6.2 `prefixBlock`

Every block, once its children are flat, is namespaced:

- child ids become `<prefix>/<child-id>`;
- `depends_on` entries naming siblings are renamed; a child with no
  dependencies inherits the block's own `depends_on` (the "inherit" edges);
- `results.<id>.` and `steps.<id>.` references in every templatable field
  — inside placeholders or bare in conditions — and a structured
  `due.after` are renamed the same way;
- the block's leaves are returned for the edges that pointed at it.

### 6.3 Include

1. Resolve the child through the locator. A child whose `name@version`
   is already on the expansion stack is an **include cycle** error. A
   stack already 8 deep refuses a further include.
2. Bind the child's vars: for each declared var, the parent's `with`
   value (rendered later in the parent's scope, i.e. spliced in as text),
   else the child's default, else — when required — an error naming the
   var. A `with` key the child does not declare is an error.
3. Expand the child's steps recursively (the child's own includes and
   repeats).
4. Substitute the bindings into the flat steps: `{{name}}` and
   `{{vars.name}}` become the bound text.
5. Prefix with the include step's id and its `depends_on` (§6.2).

### 6.4 Repeat

For `i` in `1..repeat`:

1. Expand the block's steps.
2. Substitute `{{run.iteration}}` with `i`.
3. Prefix with `<block-id>/<i>`; roots inherit `prev`, which is the
   block's `depends_on` for `i = 1` and the previous iteration's leaves
   afterwards — iterations are chained.
4. When `until` is set and `i > 1`, every step of the iteration gets
   `when` = the **negation** of `until` (`==`↔`!=`, `<`↔`>=`, `>`↔`<=`)
   with its `results.<child>` reference renamed to the previous
   iteration (`results.<block>/<i-1>/<child>`). The loop therefore stops
   as soon as an iteration satisfies `until`: later iterations are
   skipped at dispatch.

The block's leaves are the last iteration's leaves.

---

## 7. Due arithmetic

`due` is either a scalar or a mapping `{after, offset}` (any other key is
an error).

- **Scalar**: rendered, then matched against `^(.+?)\s+([+-])\s+(\S+)$`
  where the last group is a recipe duration. `"+2d"` and `"2026-10-01"`
  alone do not match and pass straight to the due parser.
- **Structured**: the base is the referenced step's rendered due; a
  target that is not rendered or has no due is an error. `offset` is a
  signed duration.

`applyDueOffset(base, offset)`: an empty offset returns the base. When
the base parses as one of `2006-01-02`, RFC 3339, `2006-01-02T15:04` or
`2006-01-02 15:04`, the offset is added and the result formatted in the
same layout. Otherwise the text `"<base> + <offset>"` (or `-`) is handed
to the due parser at materialization.

At materialization the resolved text is parsed relative to the run's
`now` with the same parser as `task create --due` (`tomorrow`, `in 3d`,
`+2d`, weekday names, `next week`, month names, ISO 8601 and variants).
An unparsable due is an error before anything is written.

---

## 8. Selection

`--task` values (repeatable, comma-separated pieces):

```
piece := ORDINAL | RANGE | STEP_ID
ORDINAL := [0-9]+            ; >= 1
RANGE   := [0-9]+ "-" [0-9]+  ; ascending, from >= 1
STEP_ID := id grammar (§3.1)
```

A piece made only of digits and dashes that is neither an ordinal nor an
ascending range is an error; an unknown ordinal or id is an error naming
the step count or the id.

`Select(steps, selector, withDeps)`:

- an empty selector keeps everything;
- kept steps stay in recipe order with their **ordinals preserved**, so a
  later `--task 3` means the same step;
- with `--with-deps`, the transitive `depends_on` closure of every kept
  step is kept too;
- otherwise a dependency on an unselected step is **dropped** from the
  kept step and reported under the dependent's id; the run records the
  drops as `"<step>:<dep>"` entries in `dropped_deps`.

---

## 9. Materialize contract

`Materializer.Materialize(MaterializeInput)` turns expanded, selected and
rendered steps into tasks and records the pass as a run.

Input: the recipe; the flat steps (ordinals set, `due` already a string
the due parser accepts); the target `TrackID` (empty = trackless),
`ProjectID`, actor `By`; `SubjectType`/`SubjectID`; the bound `Vars`; the
raw `Selection` and `DroppedDeps`; `Existing` (step id → task id already
materialized); `Recreate`, `DryRun`, `ParentRun`, an optional `RunID`
(so `{{run.id}}` rendered into the steps matches the ledger row) and a
`PreCreate` gate.

Contract:

1. Steps present in `Existing` are skipped and reported; every other
   step is created.
2. Before anything is allocated or written, every dependency and due is
   validated: a `depends_on` target MUST be in the batch or in
   `Existing`, else the pass fails naming the step and the fix. A bad
   step leaves no partial run behind.
3. A run id is minted (`run_<typeid>`) unless given.
4. **Dry run**: created steps are reported without task ids; nothing is
   written.
5. Write order: the run row (§10.1) first, then the tasks, then the
   step → task rows. If task creation fails part-way, the tasks that
   were created are still recorded on the run so a later reconcile
   resumes from them; a run that created nothing is deleted. A ledger
   failure is appended to the original error, never replaces it.
6. Each task is built from its step:

| Task field | Value |
|-----------|-------|
| `title` | `step.title`, else the step id |
| `description` | `step.description`, trimmed |
| `status` | `TODO` |
| `assigned_to` | `step.assignee` |
| `kind` | effective kind (`agent` when unset) |
| `spec` | `{agent, when, exec, human, retry, gate}` with `agent` = step agent, else recipe agent; omitted when every field is empty |
| `due_at` | parsed due, relative to the run's `now` |
| `run_id`, `step_id`, `step_ordinal` | the run, the expanded step id, its ordinal |
| `track_id`, `project_id` | the target |
| `meta.recipe`, `meta.recipe_version`, `meta.recipe_hash`, `meta.subject` | provenance; keys are removed when empty |
| `blocked_by` | `depends_on` resolved to batch task ids or `Existing` ids |
| `meta.blocked_by_recipe` | the edges this run authored; a later pass may replace exactly those and MUST leave edges added out-of-band untouched |

7. `PreCreate` runs on every task before it is persisted; an error aborts
   the batch. The CLI gate applies the same policy as a hand-made
   `task create` (tag validation, the validation config, the project
   stage gate, priority derivation, scheduling config) and, with
   `--assign`, the assignment engine for tasks with no assignee.
8. The materializer never updates or deletes a task it did not create in
   the same call.

CLI entry points:

- `track create [title] --recipe`: mints the track id first (so subject
  binding and the run can name it); title = the argument, else
  `track.title` rendered against vars, subject and `run.recipe`/
  `run.version`; type = `--type`, else `track.type`, else config; the
  track is created with `track.plan` rendered against the full scope,
  then the steps are materialized into it.
- `task create --recipe`: no title argument allowed; target = `--track`
  (offering to create a missing track), else the `--for` subject's track,
  else trackless.
- `task execute <task> --recipe`: subject = the task; target = its track;
  after materialization the created tasks run (§12).

---

## 10. Reconcile contract and the run ledger

### 10.1 Run ledger schema

Migration v21 adds the ledger and the executor columns:

```sql
ALTER TABLE tasks ADD COLUMN kind         TEXT NOT NULL DEFAULT 'agent';
ALTER TABLE tasks ADD COLUMN attempts     INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN claimed_at   TEXT;
ALTER TABLE tasks ADD COLUMN run_id       TEXT;
ALTER TABLE tasks ADD COLUMN step_id      TEXT;
ALTER TABLE tasks ADD COLUMN step_ordinal INTEGER;
ALTER TABLE tasks ADD COLUMN spec         TEXT;   -- JSON TaskSpec
ALTER TABLE tasks ADD COLUMN result       TEXT;   -- JSON object
CREATE INDEX idx_tasks_run_id ON tasks(run_id);

CREATE TABLE recipe_runs (
  id           TEXT PRIMARY KEY,           -- run_<typeid>
  project_id   TEXT NOT NULL DEFAULT '',
  recipe_id    TEXT NOT NULL,              -- recipe name
  version      TEXT NOT NULL DEFAULT '',
  hash         TEXT NOT NULL DEFAULT '',   -- sha256 of the document
  vars         TEXT,                       -- JSON, the bound vars
  subject_type TEXT,                       -- task | track
  subject_id   TEXT,                       -- durable id
  track_id     TEXT,
  selection    TEXT,                       -- raw --task selector
  dropped_deps TEXT,                       -- JSON list of "<step>:<dep>"
  parent_run   TEXT,                       -- set by reconcile
  created_by   TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL
);
CREATE INDEX idx_recipe_runs_recipe_id  ON recipe_runs(recipe_id);
CREATE INDEX idx_recipe_runs_track_id   ON recipe_runs(track_id);
CREATE INDEX idx_recipe_runs_subject    ON recipe_runs(subject_type, subject_id);
CREATE INDEX idx_recipe_runs_project_id ON recipe_runs(project_id);

CREATE TABLE recipe_run_tasks (
  run_id  TEXT NOT NULL,
  step_id TEXT NOT NULL,
  task_id TEXT NOT NULL,
  PRIMARY KEY (run_id, step_id),
  FOREIGN KEY (run_id) REFERENCES recipe_runs(id) ON DELETE CASCADE
);
CREATE INDEX idx_recipe_run_tasks_task_id ON recipe_run_tasks(task_id);
```

Migration v22 drops the `flow_runs` table.

Store contract (`RecipeRunStore`): create a run; get one by id (nil when
absent); list newest first, scoped to the detected project unless a
project is named or all projects are requested, filterable by recipe,
track, subject; delete a run and its rows; add a run's step → task rows;
list a run's rows in insertion order; list every row on a track, runs
oldest first.

Task JSON exposes the executor fields as `kind`, `spec`, `result`,
`attempts`, `claimed_at`, `run_id`, `step_id`, `step_ordinal` (omitted
when empty).

### 10.2 Reconcile

`Materializer.Reconcile` brings a track up to date with a recipe. It
REQUIRES a `TrackID`.

1. Read the ledger scoped to **this recipe on this track** (another
   recipe's steps on the same track are neither drift nor existing steps,
   even when ids collide). The latest run becomes `parent_run`; per step,
   the most recent run's task id wins.
2. Drift: when the recipe's `version` or `hash` differs from the latest
   run's, a warning is reported. Existing tasks keep the definition they
   were created from.
3. Partition the steps:
   - in `Existing` → skipped;
   - no ledger row → created;
   - ledger row and the task exists → skipped, and available as a
     dependency target;
   - ledger row but the task is gone → created again when `Recreate` is
     set, else reported and skipped, and treated as **deleted**.
4. A dependency on a deleted step is dropped from the steps being created
   and recorded on the run.
5. Run the materialize pass (§9) with the resulting plan.
6. Existing tasks are never updated or deleted.

`tlc track execute <track> --recipe` performs the reconcile before its
readiness loop, then runs. The subject is rebound from the latest run
(a task subject that no longer exists is an error naming the fix); a
track-requiring recipe with no run yet takes the track itself as
subject. `tlc recipe diff` reports the same partition without writing:
`materialized`, `missing`, `deleted`, `extra` (a task or ledger row of
the run chain whose step the recipe no longer declares), plus the drift
lines.

---

## 11. Subjects

`requires.subject` declares what a recipe must be applied to; `--for`
supplies it. Enforcement: a required subject that is missing, or of the
other type, is an error naming the fix.

Resolution of `--for`: a task-shaped reference (`task_<typeid>`,
`T-NNNN`, bare digits, optional `@` prefix) is a task; anything else is
tried as a track (typeid, `L-NNNN`, slug), then as a literal task id.

Bound fields:

| Field | Task subject | Track subject |
|-------|-------------|---------------|
| `id` | `T-NNNN` alias | track slug |
| `title` | task title | track title |
| `description` | task description | `""` |
| `track` | the task's track slug (raw id when unresolvable; `""` when trackless) | track slug |
| `tags` | tags joined with `,` | `""` |
| `project` | project id | project id |

The run records `subject_type` and the durable `subject_id`; every task
records the subject in `meta.subject`.

**Task subject blocking**: after a pass that created tasks for a task
subject, the run's leaves — tasks nothing else in the pass (created or
skipped) depends on — are appended to the subject's `blocked_by`.

**Subject completion**: whenever a run task completes (executor
completion, `human.on_timeout: approve`, or `tlc task approve`), and the
task carries a `run_id` and a `meta.subject`, the run's leaves are
checked. A leaf is a task of the run with no dependents among the run's
tasks. When every leaf is terminal and the subject exists, is not
terminal and is not assigned to another actor, the subject is moved to
the completed status (forced past the state machine) with a `DONE` log
entry noting `completed via recipe <name>[@<version>]`. An unresolvable
subject reference completes nothing and is not an error.

---

## 12. Executor dispatch contract

The executor (`core.Executor`) is the single engine behind
`tlc track execute` and `tlc task execute … --recipe`. It owns every
state transition; dispatchers only execute.

### 12.1 Options

| Option | Flag | Semantics |
|--------|------|-----------|
| Actor | (current user) | claimant and log author |
| Concurrency | `--concurrency` | in-flight dispatches; `< 1` means 1; `1` runs strictly in order |
| Permissive | `--permissive` | also dispatch tasks without `run_id` |
| Reclaim | `--reclaim` | when positive, re-dispatch active tasks whose claim is older |
| Wait / Poll | `--wait` / `--poll` (default 5s) | keep polling while only human tasks are ready |
| EvaKey | `EVA_KEY` env | sent to eva gates |
| Timeout | `--timeout` | cancels the context; the report so far is returned |

Statuses are resolved by workflow role: initial (`TODO`), active
(`IN_PROGRESS`), completed (`DONE`), and skipped (`SKIPPED`) when the
vocabulary declares one.

### 12.2 Scope

`RunTrack` lists every task of the track (archived included);
`RunTasks` takes an explicit id list (the `task execute --recipe`
scope). The listing is recomputed every round from the store.

### 12.3 Round

Each round classifies every task of the scope:

1. **Eligible**: no non-empty `blocked_reason`, and either `run_id` set
   or permissive mode. Ineligible tasks are ignored.
2. **In the initial status**, else ignored.
3. **Ready**: every `blocked_by` entry is terminal. Blockers outside the
   scope are looked up; ids that resolve to nothing are ignored.
4. **Human kind**: the timeout policy (§12.7) is applied if due; when it
   fires the task counts as progressed, otherwise the task is *waiting*.
   Human tasks are never dispatched.
5. **`when`** (§13): absent → run; false → skipped (§12.6), progressed;
   evaluation error → blocked with `when: <error>` as reason, progressed.
6. Otherwise the task is dispatchable.

Stale claims (§12.8) are collected. Dispatchable tasks are sorted by
`step_ordinal`, then id, and dispatched together with the stale ones
under the concurrency bound. A round that dispatched or progressed
anything is followed by another; a round with nothing to do ends the run
unless human tasks are waiting and `--wait` is set, in which case the
executor sleeps `Poll` and loops. Cancellation returns the report so far.

### 12.4 Claim

A ready task is taken through a compare-and-set: exactly one claimant
moves the row from the initial to the active status, stamping the actor
as assignee and `claimed_at`; a task not in the initial status, assigned
to another actor, in another project, or missing is refused (no error,
the task is simply not ours). A won claim logs `CLAIMED` with
`(TODO → IN_PROGRESS, assigned to @<actor>)`.

### 12.5 Dispatch by kind

A kind with no registered dispatcher blocks the task with
`no dispatcher registered for kind <k>`.

**exec** (`ExecDispatcher`): runs `spec.exec.argv` literally — on the
host by default, in a fresh pod under `--with-pod`. `spec.exec.timeout`
(default 60s) bounds the run; on the host the whole process group is
killed at the deadline. `stdout` and `stderr` are each capped at
`spec.exec.stdout_max` (default 1 MiB); output past the cap is dropped
and flagged, not an error. The result is:

```json
{"exit_code": 0, "stdout": "…", "stderr": "…", "duration_ms": 412, "truncated": false}
```

| Outcome | Status | Summary |
|---------|--------|---------|
| exit 0 | `succeeded` | `exit 0 in <ms>ms` |
| nonzero exit | `failed` | `exit <code>: <first line of stderr>` |
| deadline hit | `timeout` | `argv run timed out after <timeout>`; the partial result is kept |
| spawn failure (binary or cwd missing) | dispatch error | the error text |

In a pod the working tree is copied to `/workspace`, `exec.env` is set at
pod creation (an empty value cannot unset an image variable),
`exec.cwd` resolves under `/workspace`, the timeout only ends the wait
(the command dies with the pod) and the output cap is applied after
transfer.

**agent** (`agentDispatcher`): the agent is `spec.agent`, else
`--agent`; none is a dispatch error naming the fix. The name is resolved
through the registry (a project-local registry needs `--trust-project`),
`--with-pod=<image>` overrides the image, the task context is built
(with `--ctxt` handles), and the agent runs through the same path as
`tlc task execute` without a state transition. The result is the
`outputs` object of the agent's results file; the status is the results
file's `status` and the summary its `summary`. Local agents
(`--with-pod false`) share the working tree and run one at a time.

### 12.6 Outcome

After dispatch the task is reloaded (an agent may have edited it) and
`result` is stored on it.

- A dispatch error, a nil result, or a status other than `succeeded` is
  a **failure** whose summary is the error, the result summary, or the
  status.
- Otherwise, when `spec.gate` is set, the result is POSTed to
  `{eva_url}/v1/contract/invoke` as `{"contract": <name>, "body":
  <result>}` with `X-Eva-Key` when `EVA_KEY` is set. HTTP 200 passes;
  HTTP 422 (`contract_violation`) and any other non-200 status is a
  failure with summary `gate <contract>: <error>`.
- **Complete**: `claimed_at` cleared, transition to the completed status
  with the summary as note, `DONE` logged, then the subject rule (§11).
- **Fail**: `attempts` incremented, `claimed_at` cleared; the task returns
  to the initial status with note `<kind> failed ×<attempts>: <summary>`.
  With `attempts >= max_attempts` (default 1) the note becomes the
  `blocked_reason` and `BLOCKED` is logged; otherwise `RETRY` is logged
  and the executor sleeps `retry.backoff` before the next round.
- **Skip** (false `when`): transition to the skipped status with note
  `when false: <expr>`, `SKIPPED` logged. A vocabulary without a skipped
  role blocks the task instead, naming the missing role.
- **Block**: `blocked_reason` set, status unchanged, `BLOCKED` logged.

Transitions the vocabulary refuses are forced, and the note records
`(forced: <reason>)`.

### 12.7 Human tasks

Never dispatched; reported as waiting. `spec.human.timeout` with
`on_timeout` is applied lazily — at the next execute once the timeout
has elapsed since the task's `created_at`:

- `approve`: forced transition to the completed status, note
  `auto-approved: no decision after <timeout>`, `APPROVED` logged,
  subject rule applied;
- `reject`: `blocked_reason` = `rejected: no decision after <timeout>
  timeout`, `REJECTED` logged.

`tlc task approve <task> [--by] [--note]` completes a human task with a
forced transition (a decision never passes through `IN_PROGRESS`), logs
`APPROVED` with note `approved[: <note>]`, clears `claimed_at` and
applies the subject rule. `tlc task reject <task> --reason <why> [--by]`
sets `blocked_reason` = `rejected: <why>` and logs `REJECTED`; the
status is unchanged. Both refuse non-human tasks. `--by` defaults to the
current user.

### 12.8 Reclaim

With `--reclaim <d>`, an eligible task in the active status whose
`claimed_at` is at least `d` old — whoever holds it — is taken over in
place: assignee and `claimed_at` are overwritten with the actor and now,
`RECLAIMED` is logged with `reclaimed from <owner> after <age>`, and the
task is dispatched with the ready set. Human tasks are never reclaimed.

### 12.9 Report

`Done`, `Skipped`, `Failed` (attempts that will be retried), `Blocked`,
`Reclaimed`, `WaitingHuman` — task id lists printed by the CLI.

### 12.10 Log actions

The executor and the human decision commands write: `CLAIMED`, `DONE`,
`SKIPPED`, `RETRY`, `BLOCKED`, `APPROVED`, `REJECTED`, `RECLAIMED`.

---

## 13. Condition grammar (`when`, `until`)

```
expr := lhs OP rhs
lhs  := "results." STEP "." PATH      ; PATH is a dotted path into the step's result
      | "vars." IDENT
      | IDENT                          ; a bare name resolves as a var
OP   := "==" | "!=" | "<" | "<=" | ">" | ">="
rhs  := "'" STRING "'" | '"' STRING '"' | NUMBER | true | false | IDENT
```

- Whitespace around tokens is tolerated. The earliest operator in the
  text is taken; at equal positions the longer one wins, so `<=` is never
  read as `<` followed by `=`.
- An empty expression is `true` (no gate). Empty operands are errors. A
  `results.` lhs MUST have both a step and a path.
- `==` and `!=` compare the value's `%v` rendering with the literal
  (quotes stripped): numbers and booleans match their quoted or unquoted
  spelling.
- `<`, `<=`, `>`, `>=` require both sides to be numbers (numeric strings
  widen); anything else is an error.
- Environment at dispatch: `results` holds the `result` of every task in
  the same run keyed by `step_id` (an empty object for a task without a
  result); `vars` holds the run's recorded vars. A task without a
  `run_id` sees empty maps.
- An unknown step, a path through a non-object, an unknown field, or an
  unknown var is an error — callers fail loudly rather than silently
  skip.
- Negation (used for `until`, §6.4): `==`↔`!=`, `<`↔`>=`, `>`↔`<=`.

Validation (§4) accepts exactly this grammar and additionally requires
the `results.` target to be upstream of the step.

---

## 14. Task spec on the wire

```json
{
  "agent": "claude",
  "when": "results.lint.exit_code == 0",
  "exec": {"argv": ["make", "lint"], "cwd": "", "env": {}, "timeout": "2m", "stdout_max": 0},
  "human": {"assignee": "", "timeout": "48h", "on_timeout": "reject"},
  "retry": {"max_attempts": 2, "backoff": "30s"},
  "gate": {"contract": "review-quality", "eva_url": "http://eva.internal"}
}
```

`spec` is one JSON blob on the task; `kind`, `attempts`, `claimed_at`,
`run_id`, `step_id` and `step_ordinal` are columns because the readiness,
ordering and reclaim queries filter on them.

---

## 15. Identifiers

- Run ids are TypeIDs with prefix `run` (`run_<suffix>`).
- Expanded step ids are `<block>/<child>` for includes and
  `<block>/<i>/<child>` for repeat iterations; `/` cannot appear in an
  authored id.
- Recipe URIs: `tlc://recipe/<name>` and `tlc://<project>/recipe:<name>`
  (see identifiers-spec-0.1.md).

---

## 16. Durations

Recipe durations are Go durations (`30s`, `5m`, `48h`) extended with
days and weeks (`5d`, `2w`, fractional allowed). They are used by
`exec.timeout`, `human.timeout`, `retry.backoff`, `due.offset` and the
scalar due arithmetic.
