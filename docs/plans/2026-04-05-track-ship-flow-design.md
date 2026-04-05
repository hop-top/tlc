# Track Ship Flow

**Author:** $USER
**Date:** 2026-04-05
**Status:** Draft

## Overview

`track-ship` is an end-to-end delivery flow. Given a track ID,
it creates a worktree, implements all tasks, pushes, opens a PR,
waits for review, applies feedback, merges, and cleans up.

Single input: `track`. Everything else derived.

## Flow Spec Extensions

Two new step-level fields:

### `delay: <duration>`

Wait before first execution. Applied once. Duration format
matches Go (`5m`, `30s`, `1h30m`).

### `retry`

```yaml
retry:
  interval: 30s       # wait between attempts
  max_attempts: 600   # ceiling
  on: "exit_code != 0"  # or "no_review"
```

Executor polls at `interval` up to `max_attempts`. Step exits 0
to break the loop.

## Phases

### 1. Setup — `create-worktree`

- `tlc track show {{track}}` — validate
- `/usr/bin/git hop add feat/{{track}}` — worktree + branch
- Read plan, list tasks

### 2. Implement — `implement-track`

Per-task loop ordered by dependency:
1. Claim task
2. Read description, implement, test, commit
3. Complete task

Failure per task: retry 3x → skip + mark SKIPPED.
Dependents of skipped tasks also skipped.

### 3. Merge Gate — `merge-gate`

Read `/tmp/track-ship-skipped.json`.
- Empty → proceed to push
- Non-empty → halt (exit 1), worktree stays

### 4. Push + PR — `push-and-pr`

```bash
git push -u origin feat/{{track}}
gh pr create --title "..." --body "..."
```

Captures PR number to `/tmp/track-ship-pr-number.txt`.

### 5. Review Loop

**`wait-for-review`:**
- `delay: 5m` before first poll
- `retry: { interval: 30s, max_attempts: 600 }`
- Polls `gh pr view --json reviews` for non-COMMENTED reviews
- Exit 0 = review received; exit 1 = retry

**`apply-review`:**
- APPROVED → skip to merge
- CHANGES_REQUESTED → run `/superpowers:receiving-code-review`

**`push-review-fixes`:**
- `git push`
- Loops back to `wait-for-review`

### 6. Merge + Cleanup

```bash
gh pr merge $PR --squash --delete-branch
echo "y" | /usr/bin/git hop remove feat/{{track}}
git checkout main && git pull
tlc track update {{track}} --status completed
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Track shipped |
| 1 | Halted: skipped tasks |
| 2 | Review timeout (5h) |
| 3 | Setup failure |

## State Artifacts

All in `/tmp/track-ship-*`:
- `tasks.json` — initial task list
- `skipped.json` — skipped task IDs + reasons
- `pr-number.txt` — PR number
- `pr-url.txt` — PR URL
- `review.json` — latest review

## Spec Extensions Required

The `delay` and `retry` fields are new to the flow spec.
Implementation needed in `internal/core/flow_executor.go`:
- Parse `delay` as `time.Duration`
- Parse `retry` struct: interval, max_attempts, on (condition)
- Executor sleep loop with condition check

No other flows use these fields yet; they are backward-compatible
additions (zero-value = no delay, no retry).
