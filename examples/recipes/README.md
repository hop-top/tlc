# Example recipes

Recipes are versioned templates that create tracks and tasks. Point any
`tlc` command at one by path, or drop the file into a directory on the
search path and use its name.

```bash
tlc recipe list                                   # everything on the search path
tlc recipe show sequential-chain                  # header, vars and expanded steps
tlc recipe validate examples/recipes/conditional.yaml
tlc track create --recipe conditional --var target=staging
```

To use these files by name rather than by path, put the directory on the
search path — `recipe.dir` in `.tlc/config.yaml`, `.tlc/recipes/`, or
`~/.config/tlc/recipes/`:

```bash
tlc -c recipe.dir=examples/recipes recipe list
```

## Recipes

| Recipe | Needs | What it does |
|---|---|---|
| `brainstorming` | — | Turn an idea into a design through collaborative dialogue |
| `code-review` | — | Review code against the plan and the quality standards |
| `composition` | `repo` | Build pipeline that includes `composition-child` as one step |
| `composition-child` | `working_dir` | Reusable lint-then-test pipeline, includable or standalone |
| `conditional` | `target` | One deploy recipe for staging and production; the other target's steps skip |
| `executing-plans` | — | Execute a written implementation plan with review checkpoints |
| `finishing-development-branch` | — | Finish the work and choose the integration path |
| `gate-checkpoint` | — | Producer, EVA gate, consumer |
| `multi-agent` | — | Three steps, each dispatched to a different agent |
| `oss-release-prep` | — | Take a private repo to its first public release |
| `oss-release-prep-validated` | — | The same, with an EVA contract closing each phase |
| `parallel-fanout` | — | Three independent checks, then one report |
| `pr-review-loop` | `repo`, `pr` | Bounded review and fix cycles that stop once approved |
| `retry-timeout` | `service_url` | Retry with backoff, an exec timeout, and teardown |
| `sequential-chain` | — | A to B to C, each step building on the last |
| `systematic-debugging` | — | Find the root cause before fixing it |
| `task-generation` | `sprint_name` | Generate sprint tasks with a dependency chain |
| `test-driven-development` | — | Tests first, then the implementation |
| `track-aware` | `track` | Work a track's tasks in dependency order, then verify |
| `track-plan-review` | `track_id` | Review a track's planning artifacts for gaps and conflicts |
| `track-ship` | a track subject | Worktree, pull request, human review, merge, cleanup |
| `verification-before-completion` | — | Evidence before assertions |
| `writing-plans` | — | Write the implementation plan before coding |

A var with no default is required: pass it with `--var key=value`, or the
command stops and names what is missing. `track-ship` also needs a
subject, so it is applied to an existing track with `--for`.

## Fixtures

`tlc recipe test` runs a recipe hermetically against recorded tool
output. Each recipe's fixtures live beside it:

```
fixtures/<recipe>/<run>/
  test.yaml      # run name, description, expected_exit, vars
  contracts/     # <step-id>.yaml, one per gated step
  record/        # <step-id>/ cassettes, one directory per step
```

A run directory named `happy-path` is the convention for the case where
everything succeeds. Cassettes are keyed by step id, so renaming a step
orphans its recordings.
