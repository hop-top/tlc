# TLC Identifiers Specification v0.1

**Status**: Ratified — typeid-ids track 2026-05  
**Author**: Jad Bitar  
**Date**: 2026-05-02

---

## Overview

TLC uses a two-tier identifier system: durable TypeIDs backed by UUIDv7 for global uniqueness and stable references, paired with human-facing display aliases for CLI ergonomics. This spec defines both forms, lookup rules, and integration across TLC surfaces.

---

## Two-Tier Identity

Every task and track carries two identifiers, scoped to its entity type:

| Entity | Durable ID (Primary) | Display Alias | Scope |
|--------|---|---|---|
| **Task** | `task_01h455vb4pex5vsknk084sn02q` | `T-0042` | Per-project sequence |
| **Track** | `track_01h455vbqkfsn02nk084ksn02q` | `bug-fixes` | Per-project slug |

**Invariant**: The durable ID is immutable and globally unique. The display alias is unique within its project but may collide across projects.

---

## TypeID Format

TLC implements [TypeID](https://github.com/jetify-com/typeid) (Jetify reference implementation, v1.3.0):

```
<prefix>_<26-char-base32-uuidv7>
```

- **Prefix**: `task` or `track` (namespace)
- **Suffix**: 26-character Crockford base32 encoding of a UUIDv7 (lexicographically ordered by timestamp)
- **Canonical form**: lowercase; all output uses this form

**Examples**:
```
task_01h455vb4pex5vsknk084sn02q
track_01h455vbqkfsn02nk084ksn02q
```

**Properties**:
- Globally unique across all TLC instances (UUID uniqueness)
- Lexicographically sortable by creation time (UUIDv7)
- Human-readable for copy/paste and debugging
- Collision-proof (2^128 space)

---

## Display Alias

### Tasks: T-NNNN+

**Format**: `T-` followed by per-project sequence number.

**Rules**:
- Minimum width: 4 digits (preserves legacy `T-0001` through `T-9999`)
- Above 9999: grows naturally without padding (e.g., `T-10000`, `T-1234567`)
- Always zero-padded to 4 digits for numbers < 10000
- Unique within a project; same alias can exist in different projects

**Examples**:
```
T-0001       (seq = 1, padded)
T-0042       (seq = 42, padded)
T-9999       (seq = 9999, padded)
T-10000      (seq = 10000, unpadded)
T-1234567    (seq = 1234567, unpadded)
```

**Rendering**:

```go
// FormatTaskAlias renders a Task as T-NNNN+ (minimum 4 digits)
func FormatTaskAlias(t *Task) string {
    if t == nil || t.Seq == 0 {
        return ""
    }
    return fmt.Sprintf("T-%04d", t.Seq)
}
```

### Tracks: user-supplied slug

**Format**: Alphanumeric + hyphens, 3–64 characters.

**Regex**: `^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`

- Must start and end with alphanumeric
- Internal hyphens permitted; contiguous hyphens forbidden
- Case-insensitive input, stored lowercase
- Unique within a project

**Auto-derivation**: If slug not supplied, `SlugFromTitle(title)` normalizes the track title:
- Lowercase, collapse whitespace, replace non-alphanumeric with hyphens, trim edges
- Example: "Bug Fixes & Refactor" → `bug-fixes-and-refactor`

**Examples**:
```
bug-fixes
feature-xyz
docs-2026-may
infrastructure
```

---

## CLI Input Lookup

Every TLC command accepting a task/track reference resolves user input to the durable TypeID. Accepted forms:

### Tasks

```bash
# Durable TypeID (returned as-is after validation)
tlc task show task_01h455vb4pex5vsknk084sn02q

# Display alias (resolved via project + seq lookup)
tlc task show T-0042

# Bare number (treated as seq, resolved same as alias)
tlc task show 42
```

**Resolution**:
1. If input matches `task_<26chars>`: validate syntax, check existence, return TypeID
2. If input matches `T-\d+`: parse seq, lookup via `(project_id, seq)`, return TypeID
3. If input is bare digits: treat as seq, lookup via `(project_id, seq)`, return TypeID
4. Otherwise: error (must be TypeID, T-NNNN, or number)

### Tracks

```bash
# Durable TypeID
tlc track show track_01h455vbqkfsn02nk084ksn02q

# User slug (resolved via project + slug lookup)
tlc track show bug-fixes
```

**Resolution**:
1. If input matches `track_<26chars>`: validate syntax, check existence, return TypeID
2. Otherwise: validate as slug, lookup via `(project_id, slug)`, return TypeID
3. If not found: error

---

## CLI Output Rendering

Default output uses the display alias for readability:

```bash
$ tlc task show T-0042
Title:    Fix login flow
Status:   IN_PROGRESS
ID:       T-0042  ← Display alias (from Seq)

$ tlc task list
ID      Title                Status         Assigned
T-0001  Set up CI            DONE           alice
T-0042  Fix login flow       IN_PROGRESS    bob
T-0099  Refactor auth        TODO           —
```

**Verbose output** (`-V`, `--verbose`) or structured formats (`--format json/yaml`) include the full TypeID:

```bash
$ tlc task show T-0042 -V
ID:      task_01h455vb4pex5vsknk084sn02q
Display: T-0042
Title:   Fix login flow

$ tlc task show T-0042 --format json
{
  "id": "task_01h455vb4pex5vsknk084sn02q",
  "seq": 42,
  "title": "Fix login flow",
  ...
}
```

---

## URIs

TLC URI scheme (`tlc://`, `flow://`) uses TypeIDs for stable persistence:

```
tlc://project-id/task_01h455vb4pex5vsknk084sn02q
tlc://hop-top/tlc/task_01h455vb4pex5vsknk084sn02q
```

**Shorthand aliases accepted for legibility** but internally resolved to TypeID:

```
tlc://project/T-0042  ← Resolved to task_01h455vb4pex5vsknk084sn02q before persistence
```

For URI parsing and resolution details, see the `poly-cite` reference.

---

## Logs and Audit

**LogEntry.TaskID** stores the durable TypeID:

```go
type LogEntry struct {
    TaskID    string    // task_01h455vb4pex5vsknk084sn02q (immutable)
    Title     string    // For audit trail context
    Seq       int64     // For display alias reconstruction
    Action    string    // CREATED, CLAIMED, DONE, ...
    Actor     string    // agent ID
    Timestamp time.Time
}
```

**Display layer** renders the alias using the Seq column. Cross-reference [task-log-spec-0.1.md](task-log-spec-0.1.md).

---

## RFC 5545 UID (vtodo Export)

For calendar/todo sync, the UID (unique identifier in iCalendar) combines TypeID and domain:

```
UID: task_01h455vb4pex5vsknk084sn02q@tlc.example.com
URL: tlc://project/task_01h455vb4pex5vsknk084sn02q
```

This ensures round-trip stability across calendar systems. See vtodo-export-sync spec for details.

---

## Plugin Integration

When syncing with external systems (GitHub, Jira, Linear):

**Outbound**: Plugin embeds TypeID as an HTML comment in the remote issue body for round-trip:
```html
<!-- tlc-uid: task_01h455vb4pex5vsknk084sn02q -->
```

**Inbound**: Foreign UIDs (GitHub issue number, Jira key) are stored in `Task.Reference` and `Meta["external_uid"]`. On first sync, TLC mints a fresh TypeID and records the foreign UID for future lookups.

Example flow:
1. Import GitHub issue #123 → Create task with TypeID `task_01h...`, store `Meta["external_uid"] = "gh-123"`
2. On export, embed `<!-- tlc-uid: task_01h... -->` in GitHub issue
3. On re-import, recognize UID, reuse same task (no duplicate)

See [tlc-plugin-spec-0.1.md](tlc-plugin-spec-0.1.md) and [sync-architecture-0.1.md](sync-architecture-0.1.md).

---

## Migration Note

This system represents a **fresh start** for the typeid-ids track. Existing task IDs from prior versions are **not migrated**; all tasks created going forward use TypeID + seq. 

**Future inbound external IDs** (e.g., GitHub issue numbers, Jira keys) are stored in `Task.Reference` field or `Meta["external_uid"]`, not as primary task IDs.

---

## See Also

- [task-crud-spec-0.1.md](task-crud-spec-0.1.md) — Task entity model and mutations
- [task-log-spec-0.1.md](task-log-spec-0.1.md) — Audit log and LogEntry schema
- [sync-architecture-0.1.md](sync-architecture-0.1.md) — External system sync and Plugin integration
- [TypeID spec](https://github.com/jetify-com/typeid) — Reference implementation
