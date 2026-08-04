# TLC Query Specification v0.1

## Overview

This document defines the query and filter language for TLC task operations. The query syntax enables filtering, searching, and selecting tasks through CLI commands and programmatic interfaces.

## Design Principles

1. **Simple for common cases**: `status:TODO` is easier than complex syntax
2. **Composable**: Combine filters with AND/OR/NOT logic
3. **Git-inspired**: Familiar to developers (`@me`, `#tag` patterns)
4. **Type-safe**: Clear semantics for comparisons
5. **Shell-friendly**: No special escaping required for basic queries

---

## Query Syntax

### Basic Filter Pattern

```
<field><operator><value>
```

**Examples**:
```
status:TODO
assigned_to=codex
meta.priority=high
```

### Shorthand Patterns

| Pattern | Meaning | Example |
|---------|---------|---------|
| `@<user>` | Assigned to user | `@codex` |
| `@me` | Assigned to current user | `@me` |
| `#<tag>` | Has tag | `#auth`, `#bug` |
| `<field>:<value>` | Field equals value | `status:TODO` |
| `<field>=<value>` | Field equals value | `priority=high` |

---

## Field References

### Top-Level Fields

| Field | Type | Examples |
|-------|------|----------|
| `id` | string | `id:T-0042`, `id=T-0042` |
| `title` | string | `title~"API"` (contains) |
| `description` | string | `description~"rate limit"` |
| `status` | enum | `status:TODO`, `status:IN_PROGRESS` |
| `assigned_to` | string | `assigned_to:codex`, `@codex` |
| `tags` | array | `tags:auth`, `#auth` |
| `reference` | string | `reference~"github"` |
| `created_at` | timestamp | `created_at>2025-01-01` |
| `updated_at` | timestamp | `updated_at<2025-01-15` |

### Metadata Fields

Access nested metadata with dot notation:

```
meta.<key><operator><value>
```

**Examples**:
```
meta.priority=high
meta.domain:api
meta.branch_name~"feat/"
meta.effort=md
```

### External Sync Fields

```
meta.origin_system:github
meta.origin_id:456
meta.last_sync_at>2025-01-16T10:00:00Z
```

---

## Operators

### Equality Operators

| Operator | Meaning | Types | Example |
|----------|---------|-------|---------|
| `:` | Equals | string, enum | `status:TODO` |
| `=` | Equals | all | `assigned_to=codex` |
| `!=` | Not equals | all | `status!=DONE` |

**Note**: `:` and `=` are interchangeable for equality.

### Comparison Operators

| Operator | Meaning | Types | Example |
|----------|---------|-------|---------|
| `>` | Greater than | number, date | `created_at>2025-01-01` |
| `>=` | Greater or equal | number, date | `created_at>=2025-01-15` |
| `<` | Less than | number, date | `updated_at<2025-01-16` |
| `<=` | Less or equal | number, date | `updated_at<=2025-01-15` |

### Pattern Matching

| Operator | Meaning | Types | Example |
|----------|---------|-------|---------|
| `~` | Contains (substring) | string | `title~"API"` |
| `!~` | Does not contain | string | `title!~"deprecated"` |
| `^` | Starts with | string | `reference^"github:"` |
| `$` | Ends with | string | `title$"[draft]"` |

### Array Operators

| Operator | Meaning | Example |
|----------|---------|---------|
| `:` | Has element | `tags:auth` |
| `!:` | Does not have | `tags!:deprecated` |

---

## Logical Operators

### AND (Space)

Multiple filters separated by space are ANDed:

```bash
status:TODO @codex
# Means: status=TODO AND assigned_to=codex

#auth meta.priority=high
# Means: has tag "auth" AND priority is high
```

### OR (Pipe `|`)

```bash
status:TODO|status:IN_PROGRESS
# Means: status=TODO OR status=IN_PROGRESS

@codex|@agent
# Means: assigned to codex OR agent
```

### NOT (Exclamation `!`)

```bash
!status:DONE
# Means: status != DONE

!@codex
# Means: NOT assigned to codex

status:TODO !#deprecated
# Means: status=TODO AND NOT tagged with "deprecated"
```

### Grouping with Parentheses

```bash
(status:TODO|status:IN_PROGRESS) @codex
# Means: (status=TODO OR status=IN_PROGRESS) AND assigned_to=codex

#auth (meta.priority=high|meta.priority=critical)
# Means: has tag "auth" AND (priority is high OR critical)
```

---

## Special Values

### Current User (`@me`)

```bash
@me
# Expands to: assigned_to=<current-user>
```

### Current Date/Time

```bash
@today
# Expands to: YYYY-MM-DD (current date)

@now
# Expands to: current timestamp

# Examples
created_at>@today        # Created today or later
updated_at<@now          # Updated before now
```

### Null/Empty

```bash
assigned_to:null         # Not assigned
assigned_to!=null        # Is assigned

tags:[]                  # No tags
tags!=[]                 # Has tags

description:""           # Empty description
description!=""          # Has description
```

---

## Text Search

### Full-Text Search

Search across title and description:

```bash
/search-term
```

**Examples**:
```bash
/API                     # Contains "API" in title or description
/"rate limiting"         # Phrase search (with quotes)
/auth /oauth            # Multiple search terms (AND)
```

### Field-Specific Search

```bash
title~"API"              # Contains "API" in title
description~"oauth"      # Contains "oauth" in description
```

---

## Date/Time Filters

### Absolute Dates

```bash
created_at:2025-01-16                    # Exact date (00:00:00)
created_at>2025-01-15                    # After date
created_at>=2025-01-01                   # On or after
updated_at<2025-01-16T10:00:00Z          # Before timestamp
```

### Relative Dates

```bash
created_at>-7d           # Created in last 7 days
updated_at>-1h           # Updated in last hour
created_at<-30d          # Created more than 30 days ago
```

**Time Units**:
- `s` — seconds
- `m` — minutes
- `h` — hours
- `d` — days
- `w` — weeks

### Date Ranges

```bash
created_at:2025-01-01..2025-01-31        # January 2025
updated_at:-7d..@now                     # Last 7 days
```

---

## Sorting

### Sort Syntax

```bash
--sort-by <field> [--order asc|desc]
```

**Default order**:
- Timestamps: `desc` (newest first)
- Strings: `asc` (alphabetical)
- Numbers: `asc` (smallest first)

### Examples

```bash
# Sort by created date (newest first)
tlc task list --sort-by created_at

# Sort by status (alphabetical)
tlc task list --sort-by status --order asc

# Sort by priority (custom order)
tlc task list --sort-by meta.priority --order desc

# Multiple sort fields
tlc task list --sort-by status,created_at
```

---

## Predefined Queries (Shortcuts)

Common queries available as flags:

| Flag | Equivalent Query | Description |
|------|------------------|-------------|
| `--mine` | `@me` | My tasks |
| `--blocked` | `status:BLOCKED` | Blocked tasks |
| `--urgent` | `meta.priority=critical\|meta.priority=high` | High-priority tasks |
| `--in-progress` | `status:IN_PROGRESS` | In-progress tasks |
| `--todo` | `status:TODO` | Todo tasks |
| `--done` | `status:DONE` | Completed tasks |
| `--recent` | `created_at>-7d` | Created in last 7 days |
| `--updated` | `updated_at>-24h` | Updated in last 24 hours |

### Examples

```bash
# Shortcut
tlc task list --mine --urgent

# Equivalent to
tlc task list "@me (meta.priority=critical|meta.priority=high)"
```

---

## Examples

### Basic Filters

```bash
# All TODO tasks
tlc task list status:TODO

# My in-progress tasks
tlc task list status:IN_PROGRESS @me

# High-priority auth tasks
tlc task list meta.priority=high #auth

# Tasks without assignee
tlc task list assigned_to:null
```

### Complex Filters

```bash
# TODO or IN_PROGRESS, assigned to me
tlc task list "(status:TODO|status:IN_PROGRESS) @me"

# High/critical priority, not done
tlc task list "(meta.priority=high|meta.priority=critical) !status:DONE"

# Auth tasks updated in last 3 days
tlc task list "#auth updated_at>-3d"

# External tasks from GitHub
tlc task list "meta.origin_system:github"
```

### Text Search

```bash
# Tasks mentioning "API"
tlc task list /API

# Tasks with "rate limiting" in title
tlc task list "title~'rate limiting'"

# Tasks with OAuth in description
tlc task list "description~oauth"
```

### Date Filters

```bash
# Created today
tlc task list created_at:@today

# Updated in last hour
tlc task list updated_at>-1h

# Created in January 2025
tlc task list created_at:2025-01-01..2025-01-31

# Stale tasks (not updated in 30 days)
tlc task list updated_at<-30d
```

### Metadata Filters

```bash
# API domain tasks
tlc task list meta.domain:api

# Medium effort tasks
tlc task list meta.effort=md

# Tasks in specific worktree
tlc task list meta.worktree_path~".worktrees/"

# Tasks with branch name
tlc task list meta.branch_name!=null
```

### Combined Examples

```bash
# My urgent auth tasks updated today
tlc task list "@me meta.priority=critical #auth updated_at>@today"

# TODO/IN_PROGRESS tasks in API domain, not blocked
tlc task list "(status:TODO|status:IN_PROGRESS) meta.domain:api !status:BLOCKED"

# Recent high-priority features
tlc task list "#feat meta.priority=high created_at>-7d"

# External GitHub issues without local assignee
tlc task list "meta.origin_system:github assigned_to:null"
```

---

## Programmatic Usage

### JSON Query Format

For programmatic use, queries can be JSON:

```json
{
  "filters": [
    {"field": "status", "op": "=", "value": "TODO"},
    {"field": "assigned_to", "op": "=", "value": "codex"}
  ],
  "logic": "AND",
  "sort": [
    {"field": "created_at", "order": "desc"}
  ],
  "limit": 10
}
```

### CLI with JSON

```bash
tlc task list --query-json '{"filters":[{"field":"status","op":"=","value":"TODO"}]}'
```

---

## Query Builder (Interactive)

For complex queries, use interactive builder:

```bash
tlc task list --build-query

# Interactive prompt:
# 1. Select field: status
# 2. Select operator: equals
# 3. Enter value: TODO
# 4. Add another filter? (y/n): y
# 5. Select field: assigned_to
# 6. Select operator: equals
# 7. Enter value: codex
# 8. Add another filter? (y/n): n
#
# Generated query: status:TODO @codex
# Execute? (y/n): y
```

---

## Query Validation

TLC validates queries before execution:

```bash
tlc task list "status:INVALID"
# Error: Invalid status value "INVALID"
#        Valid values: TODO, IN_PROGRESS, DONE, SKIPPED

tlc task list "nonexistent_field:value"
# Error: Unknown field "nonexistent_field"
#        Valid fields: id, title, status, assigned_to, tags, ...

tlc task list "created_at>not-a-date"
# Error: Invalid date format "not-a-date"
#        Expected: YYYY-MM-DD or ISO8601 timestamp
```

---

## Performance Considerations

### Indexed Fields

These fields are indexed for fast filtering:
- `id`
- `status`
- `assigned_to`
- `created_at`
- `updated_at`
- `meta.origin_system`

### Full-Text Search

Text search (`/term`, `~"pattern"`) may be slower on large datasets. Use specific field filters when possible:

```bash
# Slower (full-text)
tlc task list /API

# Faster (indexed field)
tlc task list meta.domain:api
```

### Limit Results

Always use `--limit` for large result sets:

```bash
tlc task list --limit 100
tlc task list status:TODO --limit 50
```

---

## Grammar (EBNF)

Formal query grammar:

```ebnf
query = filter { " " filter }
      | filter { "|" filter }
      | "!" filter
      | "(" query ")"

filter = shorthand
       | field_filter
       | text_search

shorthand = "@" user
          | "#" tag

field_filter = field operator value

field = identifier { "." identifier }

operator = ":" | "=" | "!=" | ">" | ">=" | "<" | "<="
         | "~" | "!~" | "^" | "$"

value = string | number | date | special

special = "@me" | "@today" | "@now" | "null"

text_search = "/" pattern [ "/" ]

user = identifier

tag = identifier

identifier = [a-zA-Z_] [a-zA-Z0-9_-]*

string = quoted_string | unquoted_string

quoted_string = '"' [^"]* '"' | "'" [^']* "'"

unquoted_string = [a-zA-Z0-9_-]+
```

---

## References

- [task-crud-spec-0.1.md](task-crud-spec-0.1.md) — Task schema
- [task-line-spec-0.1.md](task-line-spec-0.1.md) — Task line syntax

---

**Version**: 0.1
**Last Updated**: 2025-01-16
**Status**: Normative
