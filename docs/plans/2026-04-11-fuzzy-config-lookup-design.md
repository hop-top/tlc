# Fuzzy Config Lookup

**Date:** 2026-04-11
**Status:** Draft

## Problem

`tlc -c <path>` requires a full filesystem path. Users who manage several projects must type (or recall) paths like `/Users/jadb/.w/ideacrafterslabs/tlc/.tlc/config.yaml` each time they cross project boundaries. A short query like `idea/tlc` should resolve to the right config.

## Solution

Add a fuzzy-resolution step to the `-c, --config` flag. When the supplied value is not a valid path, resolve it against a corpus of known config files drawn from tlc's project registry. Package the resolution algorithm as a reusable Go module (`hop-top/hay`) so other projects can adopt it.

## Behavior

### Resolution order

1. **Stat first.** If the value is an existing file, use it. If it is a directory, resolve to `<dir>/<localConfigDir>/config.yaml`. No change from current behavior.
2. **Fuzzy fallback.** Build a corpus of `<project-root>/.tlc/*.yaml` paths from `core.RegisteredProject` entries. Score each path against the query using subsequence matching. `config.yaml` entries receive a scoring bonus so bare queries land on the main config.
3. **Apply policy.** If the top score is unique, use it. Otherwise, consult the `lookup.ambiguous` config to decide what to do.

### Ambiguity policy

Two orthogonal settings under `lookup.ambiguous`:

```yaml
lookup:
  ambiguous:
    action: list   # list | pick
    fail: true     # true | false
```

| `action` | `fail` | Behavior |
|---|---|---|
| `list` | `true` | Print candidates, exit non-zero (default) |
| `list` | `false` | Print candidates to stderr, fall back to literal path, continue |
| `pick` | `true` | Print what was picked, exit non-zero |
| `pick` | `false` | Pick top score silently |

### Stale entries

Registered projects may point at deleted directories.

- **During corpus build,** stat the project root. Drop failures from scoring. Track skipped entries at `log.Debug`.
- **Surface staleness only when it matters:**
  - Resolution failed and stale entries exist → hint: `tlc project list --stale`, `tlc project prune`.
  - Ambiguity error fired and a stale entry would have matched → same hint.
  - Post-match stat fails (file vanished between build and use) → louder message.
- **Successful resolution stays silent** even if other entries were stale.

### Sample outputs

Unique match (silent):

```
$ tlc -c idea/tlc task list
T-0042  IN_PROGRESS  Wire fuzzy config resolver
```

Unique match (verbose):

```
$ tlc -V -c idea/tlc task list
DEBUG config: query "idea/tlc" → /Users/jadb/.w/ideacrafterslabs/tlc/.tlc/config.yaml (score 47, next 12)
DEBUG config: skipped 2 stale projects during corpus build
T-0042  IN_PROGRESS  Wire fuzzy config resolver
```

Ambiguous (`list` + `fail: true`):

```
$ tlc -c tlc task list
Error: config: "tlc" matches multiple files:
  /Users/jadb/.w/ideacrafterslabs/tlc/.tlc/config.yaml      (score 38)
  /Users/jadb/code/old-tlc/.tlc/config.yaml                 (score 36)
  /Users/jadb/.w/ideacrafterslabs/tlc-experiments/.tlc/config.yaml  (score 34)
hint: add more of the path to disambiguate (e.g. "idea/tlc")
```

Ambiguous with stale entries:

```
$ tlc -c tlc task list
Error: config: "tlc" matches multiple files:
  /Users/jadb/.w/ideacrafterslabs/tlc/.tlc/config.yaml      (score 38)
  /Users/jadb/code/old-tlc/.tlc/config.yaml                 (score 36)
note: 1 stale project also matched "tlc" but was skipped (directory missing)
next: `tlc project list --stale` to inspect, `tlc project prune` to remove
hint: add more of the path to disambiguate
```

No match with stale hint:

```
$ tlc -c oldproj task list
Error: config: no match for "oldproj"
note: 3 registered projects were skipped because their directories no longer exist
next: `tlc project list --stale` to inspect, `tlc project prune` to remove
```

No match (clean):

```
$ tlc -c nonsense task list
Error: config: no match for "nonsense"
hint: try `tlc project list` to see registered projects
```

File vanished mid-lookup:

```
$ tlc -c idea/tlc task list
Error: config: matched file disappeared during lookup: /Users/jadb/.w/ideacrafterslabs/tlc/.tlc/config.yaml
next: `tlc project prune` if this project is gone, otherwise retry
```

Ambiguous (`pick` + `fail: false`): silent, top score wins.

Ambiguous (`pick` + `fail: true`):

```
$ tlc -c tlc task list
Warning: config: "tlc" was ambiguous; picked /Users/jadb/.w/ideacrafterslabs/tlc/.tlc/config.yaml (score 38, next 36)
```

Ambiguous (`list` + `fail: false`):

```
$ tlc -c tlc task list
Warning: config: "tlc" matches multiple files:
  /Users/jadb/.w/ideacrafterslabs/tlc/.tlc/config.yaml      (score 38)
  /Users/jadb/code/old-tlc/.tlc/config.yaml                 (score 36)
falling back to literal path lookup
Error: open tlc: no such file or directory
```

Explicit path (unchanged):

```
$ tlc -c ./local-overrides.yaml task list
T-0042  ...
```

## Generalization

The `lookup.ambiguous` policy governs all fuzzy matchers, not just `-c`. The shared package exposes two entry points:

- **`Resolve`** — pure fuzzy (config lookup, track/task/flow lookup).
- **`ResolveStaged`** — staged lookup: exact → alias → fuzzy (flag-value normalization in `fieldnorm.go`). Each stage short-circuits on ≥1 result. The ambiguity policy fires only when a stage itself returns multiple hits.

### Migration map

| Consumer | Type | Stages | When |
|---|---|---|---|
| `-c, --config` | `Resolve` | fuzzy only | this PR |
| `track_resolve.go` | `ResolveStaged` | exact ID → fuzzy name | follow-up |
| `task_resolve.go` | `ResolveStaged` | exact ID → glob → fuzzy | follow-up |
| flow lookup | `ResolveStaged` | exact name → fuzzy | follow-up |
| `fieldnorm.go` (status, priority) | `ResolveStaged` | exact → alias → fuzzy | follow-up |
| tags (hypothetical autocomplete) | `Resolve` | fuzzy only | if/when needed |

Each caller owns corpus construction and scoring. The shared package owns tie-break, stale-aware error types, and the ambiguity decision matrix.

## Package: `hop-top/hay`

The resolution algorithm lives in a standalone, reusable Go module at `hop-top/hay`. Both the library and its debug tool are managed in the same monorepo.

### Module structure

```
hop-top/hay/
├── hay.go             // Resolve, ResolveStaged, Policy, Result, error types
├── score.go           // default scorers: Subsequence, Substring, Levenshtein
├── score_fzf.go       // optional thin wrapper over sahilm/fuzzy
├── stage.go           // Stage type, ResolveStaged plumbing
├── hay_test.go
└── stack/             // debug CLI: `hay stack`
    └── main.go
```

Vanity import: `hop-top/hay`. Debug tool: `hop-top/hay/stack`.

### API surface

```go
package hay

// Pure fuzzy resolution.
func Resolve[T any](query string, corpus []T, opts Options[T]) (Result[T], error)

// Staged resolution: exact → alias → fuzzy. Short-circuits on first hit.
func ResolveStaged[T any](query string, stages []Stage[T], pol Policy) (Result[T], error)

type Policy struct {
    Action   AmbiguousAction // ActionList | ActionPick
    Fail     bool
}

type Options[T any] struct {
    Score         ScoreFn[T]
    Stale         StaleFn[T]       // returns true if entry should be skipped
    Policy        Policy
    TieMargin     int              // minimum score gap to declare a unique winner
    MaxCandidates int              // max candidates in error output
    Bonus         BonusFn[T]       // optional scoring bonus (e.g. config.yaml preference)
}

type Result[T any] struct {
    Winner     T
    Candidates []Scored[T]
    Stale      []T
    Ambiguous  bool
}

type Stage[T any] struct {
    Name   string
    Lookup func(query string) []T
}
```

### Design constraints

- **Zero tlc dependencies.** The package knows nothing about viper, `core.RegisteredProject`, or tlc's logger. Callers inject everything.
- **Typed errors.** `ErrAmbiguous`, `ErrNoMatch`, `ErrVanished` carry structured data (`Query`, `Candidates`, `Stale`). Callers render them.
- **No logging.** The package returns data; the caller decides what to print and at what level.
- **No locale or Unicode normalization.** ASCII subsequence + Levenshtein covers filenames, IDs, and CLI flag values.

### tlc integration

tlc wraps `hay` in a thin adapter layer (`internal/lookup`):

| Concern | Where |
|---|---|
| Corpus construction (glob `.tlc/*.yaml` from registry) | `internal/lookup` |
| Stale check (`os.Stat` on project root) | `internal/lookup` |
| Policy loader (reads `lookup.ambiguous.*` from viper) | `internal/lookup` |
| Error rendering (progressive-disclosure hints) | `internal/lookup` |
| `config.yaml` scoring bonus | `internal/lookup` |
| `tlc debug lookup` subcommand | `internal/cli` |

### Debug tool: `hay stack`

The `stack` binary lives inside the `hay` monorepo and is the standalone debug/inspection interface. It reads a corpus from stdin (one entry per line) and prints scored results:

```
$ tlc project list --paths | hay stack 'idea/tlc' --explain
score  path
47     /Users/jadb/.w/ideacrafterslabs/tlc/.tlc/config.yaml
12     /Users/jadb/code/old-tlc/.tlc/config.yaml
```

This exists for package development and power-user debugging. It is not a user-facing tool and not a replacement for fzf.

Inside tlc, the same inspection is available as `tlc debug lookup`.

## Out of scope

- Per-matcher config overrides (e.g. `lookup.config.ambiguous`). Add when a real divergence appears.
- Tag fuzzy-completion. Tags are free-form strings; no corpus to resolve against today.
- Interactive/TUI fuzzy selection (fzf-style). Not the domain of this package.
- `tlc project prune` and `tlc project list --stale`. Referenced in hints but tracked separately.
