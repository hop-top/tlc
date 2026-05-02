# TypeID library decision

Context: `2026-05-02-typeid-ids-design.md` calls for a maintained Go TypeID library. T-0810.

## Decision

Use **`go.jetify.com/typeid` v1.3.0**.

## Rationale

- **Reference implementation.** Authored and maintained by Jetify, the same group that authors the TypeID spec at <https://github.com/jetify-com/typeid>. No spec/library drift risk.
- **Active.** Tagged v1.3.0 in 2025; commits within the last 6 months.
- **Clean Go API.** `typeid.WithPrefix("task")` returns `TypeID`, `typeid.Parse(s)` round-trips, `.Prefix()` and `.Suffix()` accessors, `.String()` for the canonical form. Generic-typed variants exist for compile-time prefix safety but aren't required.
- **Minimal deps.** Pulls only `github.com/gofrs/uuid/v5` (uuidv7 backing).
- **License.** Apache 2.0 — compatible with tlc's MIT.

## Usage shape (for downstream tasks)

```go
import "go.jetify.com/typeid"

// Generate
id, err := typeid.WithPrefix("task")    // task_01h455vb4pex5vsknk084sn02q

// Parse
parsed, err := typeid.Parse("task_01h455vb4pex5vsknk084sn02q")
parsed.Prefix()  // "task"
parsed.Suffix()  // "01h455vb4pex5vsknk084sn02q"
parsed.String()  // full canonical form
```

## Pinned version

`go.jetify.com/typeid v1.3.0` (transitive: `github.com/gofrs/uuid/v5 v5.2.0`).

## Alternatives considered

- Hand-roll over `github.com/google/uuid` (already a tlc dep) — rejected: spec drift risk, base32 Crockford encoding correctness is non-trivial.
- Other community ports — rejected: jetify is the spec author, no upside to forks.
