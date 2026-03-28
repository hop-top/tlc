# TLC URI Specification v0.1

Author: $USER
Status: Draft
Date: 2026-03-28

---

## Overview

TLC uses a structured URI scheme to identify tasks and flows across projects.
Backed by `hop.top/uri` — a lightweight identifier parser — and extended in
`internal/uri/resolver.go`.

---

## URI Grammar

```
tlc-uri   = absolute-uri / shorthand
absolute-uri = scheme "://" project-id "/" task-id
shorthand    = project-id "/" task-id / task-id
scheme       = "task" / "tlc" / "flow"
project-id   = segment *("/" segment)   ; one or more slash-separated segments
task-id      = "T-" 1*DIGIT             ; canonical form: T-NNNN (4-digit padded)
segment      = 1*(ALPHA / DIGIT / "-" / "_" / ".")
```

### Examples

| Input | project-id | task-id |
|---|---|---|
| `T-0001` | *(local)* | `T-0001` |
| `tlc/T-0001` | `tlc` | `T-0001` |
| `hop-top/tlc/T-0001` | `hop-top/tlc` | `T-0001` |
| `task://hop-top/tlc/T-0001` | `hop-top/tlc` | `T-0001` |
| `tlc:///T-0001` | *(local)* | `T-0001` |

---

## Parsing Rules

### Step 1 — Scheme detection

If input contains `://`, treat as absolute URI; delegate to `url.Parse`.

- `Scheme` ← URL scheme (`task`, `tlc`, `flow`, …)
- `Space`  ← URL host (first path segment before the host separator)
- `ID`     ← URL path with leading `/` stripped

Otherwise (shorthand):

- Split on first `/` → `Space` = left, `ID` = right
- If no `/`, `Space` = empty, `ID` = whole string

### Step 2 — Task ID disambiguation

After library parsing, apply `splitProjectTask(Space, ID)`:

```
combined = Space + "/" + ID   (or ID alone when Space is empty)
taskID   = last segment of combined
projectID = combined minus last segment
```

This correctly handles multi-segment project IDs regardless of whether
the input used a scheme or shorthand form.

### Step 3 — Resolution

| projectID | Outcome |
|---|---|
| empty | Local storage lookup by `taskID` |
| non-empty | Registry lookup by `projectID` → open remote DB → lookup `taskID` |

---

## Disambiguation Logic

| Form | Has scheme | Has slashes | Interpretation |
|---|---|---|---|
| `T-0001` | no | no | Relative task ID in local project |
| `42` | no | no | Bare number → normalized to `T-0042` |
| `@T-0001` | no | no | `@`-prefixed → stripped → local |
| `tlc/T-0001` | no | yes | `project-id=tlc`, `task-id=T-0001` |
| `hop-top/tlc/T-0001` | no | yes | `project-id=hop-top/tlc`, `task-id=T-0001` |
| `task://hop-top/tlc/T-0001` | yes | yes | `project-id=hop-top/tlc`, `task-id=T-0001` |
| `tlc:///T-0001` | yes | no host | Local storage (empty project-id) |

Key rule: **the last slash-delimited segment is always the task ID**.
Everything before it (host + path prefix) is the project ID.

---

## Alignment with `hop.top/uri`

`hop.top/uri.Parse` maps input to `{Scheme, Space, ID}`:

- **Scheme** — URI scheme or empty
- **Space** — URL host (for absolute URIs) or first shorthand segment
- **ID** — remainder of the path

Because `Space` carries only the first segment, multi-segment project IDs
surface in `ID` as a slash-prefixed continuation.  The `splitProjectTask`
helper in `internal/uri/resolver.go` re-combines `Space` and `ID`, then
splits at the *last* slash to recover the full `project-id` and `task-id`.

---

## Flexible ID Normalization (pre-parse)

Applied by `NormalizeTaskID` before URI parsing:

| Input | Normalized |
|---|---|
| `T-0046` | `T-0046` |
| `@T-0046` | `T-0046` |
| `46` | `T-0046` |
| `0046` | `T-0046` |
| `tlc/T-0001` | *(pass-through)* |
| `task://…` | *(pass-through)* |

---

## Reference Implementation

- Parser: `hop.top/uri` (`uri.Parse`)
- Resolver: `internal/uri/resolver.go` (`ResolveTask`, `splitProjectTask`)
- Normalizer: `internal/uri/normalize.go` (`NormalizeTaskID`)
- Tests: `internal/uri/resolver_test.go`
