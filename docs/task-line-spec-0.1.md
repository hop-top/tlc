# Task Line Specification (TLS) v0.1

## Version

- Version: 0.1
- Generated at: 2026-01-15T15:50:16Z

## Summary

Task Line Spec (TLS) defines a single-line, human-friendly, machine-parseable representation of tasks.

TLS is intended to be the canonical syntax used in `./TODO`.

This spec OWNS:
- task line grammar
- parsing and validation rules
- normalization rules

This spec DOES NOT own:
- task persistence format beyond line grammar (see task-crud-spec-0.1.md)
- execution protocol (see task-exec-spec-0.1.md)
- collaboration claiming rules (see task-collab-spec-1.0.md)

## Design Principles

- Every task line MUST be parseable without context.
- Every task line MUST contain a stable task identifier.
- Task title MUST remain readable as free text.
- Optional metadata MUST be explicit and safely ignorable.
- Unknown metadata keys MUST be preserved (forward compatibility).

## Canonical Task Line Grammar (Normative)

A task line is:

- `<status_token> <id> <title> <meta_tokens...>`

Where:
- `status_token` is required
- `id` is required
- `title` is required (may include spaces)
- metadata tokens are optional and order-independent

### Status Token

Allowed status tokens:

- `[ ]`  -> TODO
- `[~]`  -> IN_PROGRESS
- `[x]`  -> DONE
- `[-]`  -> SKIPPED

### ID

- ID MUST be a single token (no spaces).
- ID MUST be stable once created.
- Recommended format:
  - `T-0001`
  - `T-0002`
  - ...

### Title

- Title MUST be non-empty.
- Title continues until the first recognized metadata token.

### Metadata Tokens

Metadata tokens begin after the title and follow one of these forms:

- `key=value`          (no spaces around '=')
- `ref:<pointer>`      (reference pointer)
- `prio:P0|P1|P2|P3`   (priority)
- `due:YYYY-MM-DD`     (due date)
- `area:<name>`        (component/domain)
- `@assignee`          (shorthand for assigned_to)
- `#tag`               (shorthand for tags)

#### Allowed keys (v0.1)

- `ref`
- `prio`
- `due`
- `area`

Unknown keys are allowed and must be preserved under an \"extra\" metadata map.

## Parsing Rules (Normative)

### Tokenization

1) Split the line into whitespace-delimited tokens.
2) Token[0] MUST be a valid status token.
3) Token[1] MUST be the ID token.
4) Remaining tokens are processed left to right:
   - title consumes tokens until metadata begins
   - once metadata begins, ALL remaining tokens MUST be metadata tokens

### Metadata Detection Rule

A token is metadata if:
- it begins with `@`
- it begins with `#`
- it contains exactly one `=` and left side is non-empty
- it begins with `ref:`, `prio:`, `due:`, or `area:`

If a non-metadata token appears after metadata has begun, parsing MUST fail.

### Normalization Rules

- Multiple `#tag` tokens accumulate.
- Duplicate tags MUST be deduplicated preserving first occurrence.
- If multiple `@assignee` tokens exist, the last one wins.
- If multiple `ref:` tokens exist, parsing MUST fail (reference must be singular).
- Unknown `key=value` pairs MUST be preserved.

## Validation Rules

A TLS parser MUST reject lines where:
- status token is missing/invalid
- id is missing
- title is empty
- invalid `due:` format (must be YYYY-MM-DD)
- invalid `prio:` value (must be P0/P1/P2/P3)

## Canonical Examples

### Minimal TODO

- `[ ] T-0001 Add login endpoint`

### In progress with assignee + tags + reference

- `[~] T-0002 Fix token refresh race @codex #auth #bug ref:docs/rfc/012.md`

### Done with metadata

- `[x] T-0003 Add CLI help output prio:P2 area:cli`

### Skipped with explanation in title

- `[-] T-0004 Replace DB engine (postponed) area:db prio:P3`
